// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package cmd

import (
	"context"
	"errors"
	"os"
	"sync"
	"syscall"
	"testing"

	"go.uber.org/goleak"
)

type controlledMockLifecycle struct {
	started     chan struct{}
	release     chan struct{}
	serveErr    error
	shutdownErr error
	once        sync.Once
}

func (s *controlledMockLifecycle) Start() error {
	close(s.started)
	<-s.release
	return s.serveErr
}

func (s *controlledMockLifecycle) Shutdown(context.Context) error {
	s.once.Do(func() { close(s.release) })
	return s.shutdownErr
}

func TestMockLifecycleWaitsForServerExitWithoutLeaks(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	for _, tc := range []struct {
		name        string
		serveErr    error
		shutdownErr error
		want        error
	}{
		{name: "clean", serveErr: nil, want: nil},
		{name: "serve error", serveErr: errors.New("serve"), want: errors.New("serve")},
		{name: "shutdown wins", serveErr: errors.New("serve"), shutdownErr: errors.New("shutdown"), want: errors.New("shutdown")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := &controlledMockLifecycle{
				started: make(chan struct{}), release: make(chan struct{}),
				serveErr: tc.serveErr, shutdownErr: tc.shutdownErr,
			}
			stop := make(chan os.Signal, 1)
			done := make(chan error, 1)
			go func() { done <- serveMockUntilSignal(srv, stop) }()
			<-srv.started
			stop <- syscall.SIGTERM
			if got := <-done; !sameError(got, tc.want) {
				t.Fatalf("serveMockUntilSignal()=%v want %v", got, tc.want)
			}
		})
	}
}

func sameError(got, want error) bool {
	if got == nil || want == nil {
		return got == nil && want == nil
	}
	return got.Error() == want.Error()
}
