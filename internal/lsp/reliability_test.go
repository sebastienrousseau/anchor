// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package lsp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

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

func TestServeExitInterruptsSpeculativeRead(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	pr, pw := io.Pipe()
	s := New(pr, io.Discard, io.Discard)
	done := make(chan error, 1)
	go func() { done <- s.Serve(context.Background()) }()
	for _, body := range []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize"}`,
		`{"jsonrpc":"2.0","method":"exit"}`,
	} {
		if _, err := fmt.Fprintf(pw, "Content-Length: %d\r\n\r\n%s", len(body), body); err != nil {
			if !errors.Is(err, io.ErrClosedPipe) {
				t.Fatal(err)
			}
			break
		}
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("exit left the speculative transport read blocked")
	}
	_ = pw.Close()
}

func TestConnSerializesFramesUnderContention(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	var out bytes.Buffer
	c := newConn(bytes.NewReader(nil), &out)
	const writers = 32
	const writes = 32
	var wg sync.WaitGroup
	for worker := range writers {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for seq := range writes {
				if err := c.write(message{ID: json.RawMessage("1"), Result: []int{worker, seq}}); err != nil {
					t.Errorf("write: %v", err)
					return
				}
			}
		}(worker)
	}
	wg.Wait()

	reader := newConn(bytes.NewReader(out.Bytes()), io.Discard)
	for i := 0; i < writers*writes; i++ {
		if _, err := reader.read(); err != nil {
			t.Fatalf("frame %d/%d is corrupt: %v", i, writers*writes, err)
		}
	}
	if _, err := reader.read(); !errors.Is(err, io.EOF) {
		t.Fatalf("trailing transport data: %v", err)
	}
}

func TestNonClosableTransportInterruptionIsANoop(t *testing.T) {
	c := newConn(bytes.NewReader(nil), io.Discard)
	if err := c.interruptRead(); err != nil {
		t.Fatalf("interruptRead: %v", err)
	}
}

func TestBlockingTransportObservesPreCancelledContext(t *testing.T) {
	s := New(bytes.NewReader(nil), io.Discard, io.Discard)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.serveBlocking(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("serveBlocking=%v want context.Canceled", err)
	}
}

func TestClosableTransportEOFJoinsReader(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	s := New(io.NopCloser(bytes.NewReader(nil)), io.Discard, io.Discard)
	if err := s.Serve(context.Background()); err != nil {
		t.Fatalf("Serve: %v", err)
	}
}

func TestProtocolStateMachineMatchesOracle(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		actions := rapid.SliceOfN(rapid.SampledFrom([]string{
			"initialize", "initialized", "shutdown", "textDocument/hover", "unknown", "exit",
		}), 0, 200).Draw(t, "actions")

		s := New(bytes.NewReader(nil), io.Discard, io.Discard)
		var initialized, shutdown bool
		for step, action := range actions {
			stop := s.handle(&message{ID: json.RawMessage("1"), Method: action})
			wantStop := action == "exit"
			if stop != wantStop {
				t.Fatalf("step %d %q: stop=%v want %v", step, action, stop, wantStop)
			}
			if wantStop {
				break
			}
			if !initialized && action != "initialize" {
				// Refused before initialization.
			} else if shutdown {
				// Refused after shutdown.
			} else {
				switch action {
				case "initialize":
					initialized = true
				case "shutdown":
					shutdown = true
				}
			}
			if s.initialized != initialized || s.shutdown != shutdown {
				t.Fatalf("step %d %q: state=(%v,%v) want=(%v,%v)",
					step, action, s.initialized, s.shutdown, initialized, shutdown)
			}
		}
	})
}

func FuzzConnFrameRoundTrip(f *testing.F) {
	f.Add("initialize", "{}", int64(1))
	f.Add("textDocument/didChange", "A&B <xml> 東京 😀", int64(-1))
	f.Add("$/cancelRequest", "", int64(1<<62))

	f.Fuzz(func(t *testing.T, method, payload string, id int64) {
		if len(method) > 4096 || len(payload) > 1<<20 {
			return
		}
		var wire bytes.Buffer
		wantID := strconv.FormatInt(id, 10)
		encodedParams := mustRaw(map[string]string{"payload": payload})
		var wantParams map[string]string
		if err := json.Unmarshal(encodedParams, &wantParams); err != nil {
			t.Fatalf("seed params: %v", err)
		}
		writer := newConn(bytes.NewReader(nil), &wire)
		err := writer.write(message{
			ID: json.RawMessage(wantID), Method: method,
			Params: encodedParams,
		})
		if !utf8.ValidString(method) {
			if err == nil {
				t.Fatal("write accepted an invalid-UTF-8 method and would silently mutate it")
			}
			return
		}
		if err != nil {
			t.Fatalf("write: %v", err)
		}

		got, err := newConn(bytes.NewReader(wire.Bytes()), io.Discard).read()
		if err != nil {
			t.Fatalf("read rejected framed output: %v", err)
		}
		var params map[string]string
		if err := json.Unmarshal(got.Params, &params); err != nil {
			t.Fatalf("params: %v", err)
		}
		wantPayload := wantParams["payload"]
		if string(got.ID) != wantID || got.Method != method || params["payload"] != wantPayload {
			t.Fatalf("frame changed semantics: got=(%s,%q,%q) want=(%s,%q,%q)",
				got.ID, got.Method, params["payload"], wantID, method, wantPayload)
		}
	})
}
