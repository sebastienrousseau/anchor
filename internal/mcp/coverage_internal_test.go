// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package mcp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/sebastienrousseau/askiso/pkg/iso20022"
)

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

func TestInternalProtocolFailureAndCancellationPaths(t *testing.T) {
	var out, errOut bytes.Buffer
	s := New(strings.NewReader(""), &out, &errOut)
	tool := Tool{Name: "duplicate"}
	s.register(tool, tool)
	count := 0
	for _, registered := range s.Tools() {
		if registered.Name == tool.Name {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("duplicate registration count = %d", count)
	}

	s.write(response{JSONRPC: "2.0", Result: make(chan int)})
	if !strings.Contains(errOut.String(), "could not encode") {
		t.Fatalf("marshal failure was not logged: %s", errOut.String())
	}
	errOut.Reset()
	s.out = failingWriter{err: errors.New("closed")}
	s.write(response{JSONRPC: "2.0", Result: map[string]any{"ok": true}})
	if !strings.Contains(errOut.String(), "could not write") {
		t.Fatalf("write failure was not logged: %s", errOut.String())
	}

	pr, pw := io.Pipe()
	s = New(pr, io.Discard, io.Discard)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Serve(ctx) }()
	time.Sleep(10 * time.Millisecond)
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Serve cancellation = %v", err)
	}
	_ = pw.Close()

	if got := string(trimSpace([]byte(" \t value \r\n"))); got != "value" {
		t.Fatalf("trimSpace = %q", got)
	}
}

func TestToolResultAndOperationalErrorBranches(t *testing.T) {
	res := toolResult("", make(chan int), false)
	if isError, _ := res["isError"].(bool); !isError {
		t.Fatalf("unencodable tool result should be an error: %#v", res)
	}

	missing := func() (*iso20022.Catalogue, error) { return nil, errors.New("not installed") }
	if _, err := generateTool(missing).Handler(context.Background(), map[string]any{
		"message_type": "seev.031.001.09",
	}); err == nil || !strings.Contains(err.Error(), "schemas") {
		t.Fatalf("schema generation without a catalogue: %v", err)
	}
	if _, err := generateTool(missing).Handler(context.Background(), map[string]any{
		"message_type": "pacs.008", "preset": "not-a-rail",
	}); err == nil {
		t.Fatal("invalid template preset should fail")
	}
	if _, err := translateTool().Handler(context.Background(), map[string]any{
		"mx_message": "<not-closed>",
	}); err == nil {
		t.Fatal("malformed MX should fail translation")
	}
}
