// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
)

// The Language Server Protocol frames messages with headers rather than
// newlines, so a message may contain any bytes at all. Getting this wrong
// desynchronises the stream in a way that looks like the server hanging, which
// is why it lives here on its own.

// maxContentLength bounds a single message. An editor sends whole documents on
// every keystroke, so this is generous, but it must not be unbounded.
const maxContentLength = 32 << 20 // 32 MiB

type message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *responseError  `json:"error,omitempty"`
}

type responseError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// Protocol error codes. The first five are JSON-RPC; the rest are the
// specification's own.
const (
	codeParse           = -32700
	codeInvalidRequest  = -32600
	codeMethodNotFound  = -32601
	codeInvalidParams   = -32602
	codeInternalError   = -32603
	codeServerNotInit   = -32002
	codeRequestFailed   = -32803
	codeInvalidResponse = -32001
)

// conn reads and writes framed messages.
type conn struct {
	r      *bufio.Reader
	closer io.Closer
	w      io.Writer
	mu     sync.Mutex
}

func newConn(r io.Reader, w io.Writer) *conn {
	c := &conn{r: bufio.NewReaderSize(r, 64<<10), w: w}
	if closer, ok := r.(io.Closer); ok {
		c.closer = closer
	}
	return c
}

func (c *conn) canInterruptRead() bool { return c.closer != nil }

// interruptRead releases a Read blocked in the transport. New treats a
// supplied io.ReadCloser as owned by Serve for precisely this reason.
func (c *conn) interruptRead() error {
	if c.closer == nil {
		return nil
	}
	return c.closer.Close()
}

// read returns the next message, or io.EOF when the stream ends.
func (c *conn) read() (*message, error) {
	length := -1

	for {
		line, err := c.r.ReadString('\n')
		if err != nil {
			if err == io.EOF && strings.TrimSpace(line) == "" {
				return nil, io.EOF
			}
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")

		// A blank line ends the headers.
		if line == "" {
			break
		}

		name, value, found := strings.Cut(line, ":")
		if !found {
			return nil, fmt.Errorf("malformed header: %q", line)
		}
		if !strings.EqualFold(strings.TrimSpace(name), "content-length") {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return nil, fmt.Errorf("Content-Length is not a number: %q", value)
		}
		length = n
	}

	switch {
	case length < 0:
		return nil, fmt.Errorf("a message arrived with no Content-Length header")
	case length > maxContentLength:
		return nil, fmt.Errorf("a message of %d bytes exceeds the %d-byte limit", length, maxContentLength)
	}

	body := make([]byte, length)
	if _, err := io.ReadFull(c.r, body); err != nil {
		return nil, err
	}

	var msg message
	if err := json.Unmarshal(body, &msg); err != nil {
		return nil, &parseError{err}
	}
	return &msg, nil
}

// parseError marks a body that framed correctly but did not decode, which is
// answerable rather than fatal.
type parseError struct{ err error }

func (e *parseError) Error() string { return e.err.Error() }

// write frames and sends a message.
func (c *conn) write(msg message) error {
	if !utf8.ValidString(msg.Method) {
		return fmt.Errorf("JSON-RPC method is not valid UTF-8")
	}
	msg.JSONRPC = "2.0"
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, err := fmt.Fprintf(c.w, "Content-Length: %d\r\n\r\n", len(body)); err != nil {
		return err
	}
	_, err = c.w.Write(body)
	return err
}

func (c *conn) reply(id json.RawMessage, result any) error {
	// A response must carry a result even when it is null, or a client waits
	// forever for one.
	if result == nil {
		result = json.RawMessage("null")
	}
	return c.write(message{ID: id, Result: result})
}

func (c *conn) replyError(id json.RawMessage, code int, text string) error {
	return c.write(message{ID: id, Error: &responseError{Code: code, Message: text}})
}

func (c *conn) notify(method string, params any) error {
	return c.write(message{Method: method, Result: nil, Params: mustRaw(params)})
}

func mustRaw(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("null")
	}
	return data
}
