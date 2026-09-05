// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"go.uber.org/goleak"
	"pgregory.net/rapid"
)

type observedReadCloser struct {
	io.ReadCloser
	once    sync.Once
	started chan struct{}
}

func (r *observedReadCloser) Read(p []byte) (int, error) {
	r.once.Do(func() { close(r.started) })
	return r.ReadCloser.Read(p)
}

func TestServeCancellationUnderContentionDoesNotLeak(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	const servers = 64
	var wg sync.WaitGroup
	errCh := make(chan error, servers)
	for range servers {
		pr, pw := io.Pipe()
		in := &observedReadCloser{ReadCloser: pr, started: make(chan struct{})}
		s := New(in, io.Discard, io.Discard)
		ctx, cancel := context.WithCancel(context.Background())
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { _ = pw.Close() }()
			done := make(chan error, 1)
			go func() { done <- s.Serve(ctx) }()
			select {
			case <-in.started:
				cancel()
			case <-time.After(time.Second):
				errCh <- errors.New("transport read did not start")
				cancel()
			}
			if err := <-done; !errors.Is(err, context.Canceled) {
				errCh <- fmt.Errorf("Serve returned %v, want context.Canceled", err)
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestRegistryReconfigurationAndReadsAreRaceFree(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	s := New(io.LimitReader(&zeroReader{}, 0), io.Discard, io.Discard)
	const workers = 32
	var wg sync.WaitGroup
	for worker := range workers {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := range 100 {
				if worker%4 == 0 {
					s.SetVersion("stress")
					s.SetCatalogue(openInstalledCatalogue())
				} else {
					tools := s.Tools()
					_ = s.ToolNames()
					_ = s.toolDescriptors()
					if len(tools) > 0 {
						tools[0].Name = "caller-mutation"
						tools[0].Schema["caller"] = true
					}
				}
				_ = i
			}
		}(worker)
	}
	wg.Wait()
	for _, tool := range s.Tools() {
		if tool.Name == "caller-mutation" {
			t.Fatal("Tools exposed the registry backing slice")
		}
		if _, exists := tool.Schema["caller"]; exists {
			t.Fatal("Tools exposed a mutable schema map")
		}
	}
}

func TestCloneJSONValueHasNoMutableAliases(t *testing.T) {
	if cloneJSONObject(nil) != nil {
		t.Fatal("nil object did not stay nil")
	}
	original := map[string]any{
		"objects": []any{map[string]any{"value": "original"}},
		"strings": []string{"original"},
		"scalar":  true,
	}
	clone := cloneJSONObject(original)
	clone["objects"].([]any)[0].(map[string]any)["value"] = "changed"
	clone["strings"].([]string)[0] = "changed"
	if original["objects"].([]any)[0].(map[string]any)["value"] != "original" {
		t.Fatal("nested object remained aliased")
	}
	if original["strings"].([]string)[0] != "original" {
		t.Fatal("string slice remained aliased")
	}
}

type zeroReader struct{}

func (*zeroReader) Read([]byte) (int, error) { return 0, io.EOF }

func TestClosableTransportProcessesBlankRecordsAndEOF(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	s := New(io.NopCloser(&scriptedReader{data: []byte("\n" + `{"jsonrpc":"2.0","id":1,"method":"ping"}` + "\n")}), io.Discard, io.Discard)
	if err := s.Serve(context.Background()); err != nil {
		t.Fatalf("Serve: %v", err)
	}
}

type scriptedReader struct{ data []byte }

func (r *scriptedReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, nil
}

type cancellingReader struct {
	cancel context.CancelFunc
	done   bool
}

func (r *cancellingReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, io.EOF
	}
	r.done = true
	r.cancel()
	return copy(p, "{}\n"), nil
}

func TestNonClosableTransportObservesCancellationBetweenRecords(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	s := New(&cancellingReader{cancel: cancel}, io.Discard, io.Discard)
	if err := s.Serve(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Serve=%v want context.Canceled", err)
	}
}

func TestProtocolStateMachineMatchesOracle(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		actions := rapid.SliceOfN(rapid.SampledFrom([]string{
			"initialize", "notifications/initialized", "tools/list", "tools/call", "ping", "unknown",
		}), 0, 200).Draw(t, "actions")

		s := New(&zeroReader{}, io.Discard, io.Discard)
		phase := 0 // 0=new, 1=initialize response sent, 2=operation
		for step, action := range actions {
			var params json.RawMessage
			if action == "tools/call" {
				params = json.RawMessage(`{"name":"askiso_lint","arguments":{}}`)
			}
			_, rpcErr := s.dispatch(context.Background(), request{
				JSONRPC: "2.0", ID: json.RawMessage("1"), Method: action, Params: params,
			})
			wantError := false
			switch action {
			case "initialize":
				phase = 1
			case "notifications/initialized":
				wantError = phase == 0
				if !wantError {
					phase = 2
				}
			case "tools/list", "tools/call":
				wantError = phase != 2
			case "unknown":
				wantError = true
			}
			if (rpcErr != nil) != wantError {
				t.Fatalf("step %d %q: error=%v wantError=%v phase=%d", step, action, rpcErr, wantError, phase)
			}
			s.mu.RLock()
			gotReceived, gotReady := s.initializeReceived, s.initialized
			s.mu.RUnlock()
			if gotReceived != (phase >= 1) || gotReady != (phase == 2) {
				t.Fatalf("step %d %q: state=(%v,%v), phase=%d", step, action, gotReceived, gotReady, phase)
			}
		}
	})
}
