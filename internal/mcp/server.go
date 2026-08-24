// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

// Package mcp serves AskIso over the Model Context Protocol.
//
// An assistant asked about an ISO 20022 message has two options: recall what it
// read about the standard, or check. This server is the second option. It
// exposes the same engine the CLI runs -- the linter, the validator, the rule
// profiles, the MT converter, the registry -- as tools an agent can call, so an
// answer about a message comes from the specification rather than from memory.
//
// The transport is newline-delimited JSON-RPC 2.0 over stdin and stdout, which
// is what the protocol's stdio transport specifies. Nothing is written to
// stdout except protocol messages; diagnostics go to stderr, because a stray
// line on stdout corrupts the stream.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
)

// ProtocolVersion is the revision of the Model Context Protocol this server
// implements. A client asking for a different revision is answered with this
// one, which the specification allows: the client then decides whether to
// continue.
const ProtocolVersion = "2025-06-18"

// Server speaks MCP over a pair of streams.
type Server struct {
	// Name and Version identify the server to the client.
	Name    string
	Version string

	in  io.Reader
	out io.Writer
	// errOut receives diagnostics. It must never be the same stream as out.
	errOut io.Writer

	tools  []Tool
	byName map[string]Tool
	// catalogue opens the user's installed schemas. SetCatalogue replaces it.
	catalogue CatalogueFunc
	writeMu   sync.Mutex

	// initialized records that the handshake completed. Tool calls before it
	// are refused, because a client that has not negotiated has not agreed to
	// anything.
	initialized bool
}

// New builds a server reading requests from in and writing replies to out.
// Diagnostics go to errOut, which must be a different stream from out.
func New(in io.Reader, out, errOut io.Writer) *Server {
	s := &Server{
		Name:    "askiso",
		Version: "dev",
		in:      in,
		out:     out,
		errOut:  errOut,
		byName:  map[string]Tool{},
	}
	s.catalogue = openInstalledCatalogue()
	s.register(defaultTools(s.catalogue)...)
	return s
}

// SetCatalogue replaces how the server reaches the user's installed schemas.
// Call it before serving; it rebuilds the tools that read them.
func (s *Server) SetCatalogue(open CatalogueFunc) {
	if open == nil {
		return
	}
	s.catalogue = open
	s.tools = nil
	s.byName = map[string]Tool{}
	s.register(defaultTools(open)...)
}

// SetVersion records the build version reported during the handshake.
func (s *Server) SetVersion(v string) {
	if v != "" {
		s.Version = v
	}
}

func (s *Server) register(tools ...Tool) {
	for _, t := range tools {
		if _, dup := s.byName[t.Name]; dup {
			continue
		}
		s.byName[t.Name] = t
		s.tools = append(s.tools, t)
	}
}

// Tools lists the registered tools in declaration order.
func (s *Server) Tools() []Tool { return s.tools }

// ---------------------------------------------------------------------------
// JSON-RPC framing
// ---------------------------------------------------------------------------

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// JSON-RPC 2.0 reserved error codes.
const (
	codeParse          = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternal       = -32603
)

// maxLine bounds a single request. The protocol has no framing header, so a
// client that never sends a newline must not be able to exhaust memory.
const maxLine = 8 << 20 // 8 MiB

// Serve reads requests until the input ends or the context is cancelled.
func (s *Server) Serve(ctx context.Context) error {
	scanner := bufio.NewScanner(s.in)
	scanner.Buffer(make([]byte, 0, 64<<10), maxLine)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Bytes()
		if len(trimSpace(line)) == 0 {
			continue
		}
		s.handleLine(ctx, line)
	}

	if err := scanner.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return fmt.Errorf("a request exceeded the %d-byte limit", maxLine)
		}
		return err
	}
	return nil
}

func trimSpace(b []byte) []byte {
	start := 0
	for start < len(b) && isSpace(b[start]) {
		start++
	}
	end := len(b)
	for end > start && isSpace(b[end-1]) {
		end--
	}
	return b[start:end]
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n'
}

func (s *Server) handleLine(ctx context.Context, line []byte) {
	var req request
	if err := json.Unmarshal(line, &req); err != nil {
		s.fail(nil, codeParse, "the request is not valid JSON", err.Error())
		return
	}
	if req.JSONRPC != "2.0" {
		s.fail(req.ID, codeInvalidRequest, `"jsonrpc" must be "2.0"`, nil)
		return
	}

	// A request without an id is a notification: it is acted on, but never
	// answered.
	notification := len(req.ID) == 0

	result, rpcErr := s.dispatch(ctx, req)
	if notification {
		return
	}
	if rpcErr != nil {
		s.write(response{JSONRPC: "2.0", ID: req.ID, Error: rpcErr})
		return
	}
	s.write(response{JSONRPC: "2.0", ID: req.ID, Result: result})
}

func (s *Server) dispatch(ctx context.Context, req request) (any, *rpcError) {
	switch req.Method {
	case "initialize":
		return s.initialize(req.Params)

	case "notifications/initialized", "initialized":
		s.initialized = true
		return map[string]any{}, nil

	case "ping":
		return map[string]any{}, nil

	case "tools/list":
		if !s.initialized {
			return nil, &rpcError{Code: codeInvalidRequest, Message: "initialize must complete first"}
		}
		return map[string]any{"tools": s.toolDescriptors()}, nil

	case "tools/call":
		if !s.initialized {
			return nil, &rpcError{Code: codeInvalidRequest, Message: "initialize must complete first"}
		}
		return s.callTool(ctx, req.Params)

	case "notifications/cancelled", "notifications/progress":
		// Nothing here is long-running enough to cancel or report on.
		return map[string]any{}, nil
	}

	return nil, &rpcError{Code: codeMethodNotFound, Message: "no such method: " + req.Method}
}

func (s *Server) initialize(params json.RawMessage) (any, *rpcError) {
	// The client's requested version is read but not required to match: the
	// specification says to answer with a version this server supports and let
	// the client decide.
	var p struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if len(params) > 0 {
		_ = json.Unmarshal(params, &p)
	}

	s.initialized = true
	return map[string]any{
		"protocolVersion": ProtocolVersion,
		"capabilities": map[string]any{
			"tools": map[string]any{"listChanged": false},
		},
		"serverInfo": map[string]any{
			"name":    s.Name,
			"version": s.Version,
		},
		"instructions": instructions,
	}, nil
}

const instructions = `AskIso answers questions about ISO 20022 from the specification rather than from memory.

Use askiso_lint and askiso_check_profile before telling anyone a message is correct: the
linter checks IBAN checksums, BIC structure, currency precision and UETR format, and the
cbpr-2026 profile checks the structured-address rules that take effect on 14 November 2026.

askiso_validate and askiso_diff read the user's own downloaded schemas and report
Installed=false rather than guessing when a schema is absent. Everything else works with
no files on disk.

AskIso redistributes no specification content. Schemas come from the user's own download
from iso20022.org.`

func (s *Server) write(resp response) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	data, err := json.Marshal(resp)
	if err != nil {
		_, _ = fmt.Fprintf(s.errOut, "askiso-mcp: could not encode a reply: %v\n", err)
		return
	}
	data = append(data, '\n')
	if _, err := s.out.Write(data); err != nil {
		_, _ = fmt.Fprintf(s.errOut, "askiso-mcp: could not write a reply: %v\n", err)
	}
}

func (s *Server) fail(id json.RawMessage, code int, message string, data any) {
	s.write(response{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: message, Data: data}})
}
