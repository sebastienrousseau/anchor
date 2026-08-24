// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

// Package lsp serves Anchor as a language server for ISO 20022 XML.
//
// Someone editing a payment message wants the same answers the CLI gives, at
// the moment they type: is this IBAN's checksum right, does this element belong
// here, will this address still be accepted after 14 November 2026. A language
// server is how an editor asks.
//
// The transport is the Language Server Protocol's own: JSON-RPC 2.0 framed with
// Content-Length headers, over stdin and stdout. Diagnostics come from the same
// linter, schema validator and rule profiles the CLI runs, so an editor and a
// pipeline never disagree.
package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"

	"github.com/sebastienrousseau/anchor/internal/xsd"
	"github.com/sebastienrousseau/anchor/pkg/iso20022"
)

// Server is a language server for ISO 20022 XML documents.
type Server struct {
	Name    string
	Version string
	// Profile is the scheme rule profile applied alongside the linter. The
	// client may change it; cbpr-2026 is the default because the deadline it
	// checks is the one people are working towards.
	Profile string

	conn   *conn
	errOut io.Writer

	mu   sync.RWMutex
	docs map[string]*Document

	// catalogue opens the user's installed schemas. Absent one, the server
	// still lints and still applies rule profiles.
	catalogue CatalogueFunc

	initialized bool
	shutdown    bool
}

// CatalogueFunc opens the user's installed schemas.
type CatalogueFunc func() (*iso20022.Catalogue, error)

// New builds a server reading from in and writing to out. Diagnostics about the
// server itself go to errOut, which must not be the same stream as out.
func New(in io.Reader, out, errOut io.Writer) *Server {
	return &Server{
		Name:      "anchor-lsp",
		Version:   "dev",
		Profile:   "cbpr-2026",
		conn:      newConn(in, out),
		errOut:    errOut,
		docs:      map[string]*Document{},
		catalogue: openInstalledCatalogue(),
	}
}

// SetVersion records the build version reported to the client.
func (s *Server) SetVersion(v string) {
	if v != "" {
		s.Version = v
	}
}

// SetCatalogue replaces how the server reaches the user's installed schemas.
func (s *Server) SetCatalogue(open CatalogueFunc) {
	if open != nil {
		s.catalogue = open
	}
}

// openInstalledCatalogue reads the user's catalogue once and caches the result.
func openInstalledCatalogue() CatalogueFunc {
	var (
		once sync.Once
		cat  *iso20022.Catalogue
		err  error
	)
	return func() (*iso20022.Catalogue, error) {
		once.Do(func() { cat, err = iso20022.OpenCatalogue("") })
		return cat, err
	}
}

// Serve handles messages until the stream ends, the client says exit, or the
// context is cancelled.
func (s *Server) Serve(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		msg, err := s.conn.read()
		switch {
		case errors.Is(err, io.EOF):
			return nil
		case err != nil:
			var pe *parseError
			if errors.As(err, &pe) {
				// A body that did not decode is answerable; the stream is still
				// synchronised because the framing was correct.
				_ = s.conn.replyError(nil, codeParse, "the request body is not valid JSON: "+pe.Error())
				continue
			}
			// A framing error desynchronises the stream, so there is no
			// recovering from it.
			return err
		}

		if s.handle(msg) {
			return nil
		}
	}
}

// handle processes one message and reports whether the server should stop.
func (s *Server) handle(msg *message) (stop bool) {
	isRequest := len(msg.ID) > 0

	if msg.Method == "exit" {
		return true
	}

	if !s.initialized && msg.Method != "initialize" {
		if isRequest {
			_ = s.conn.replyError(msg.ID, codeServerNotInit, "initialize must complete first")
		}
		return false
	}
	if s.shutdown && msg.Method != "exit" {
		if isRequest {
			_ = s.conn.replyError(msg.ID, codeInvalidRequest, "the server is shutting down")
		}
		return false
	}

	switch msg.Method {
	case "initialize":
		s.initialized = true
		_ = s.conn.reply(msg.ID, s.capabilities())

	case "initialized":
		// A notification confirming the handshake; nothing to do.

	case "shutdown":
		s.shutdown = true
		_ = s.conn.reply(msg.ID, nil)

	case "textDocument/didOpen":
		s.didOpen(msg.Params)

	case "textDocument/didChange":
		s.didChange(msg.Params)

	case "textDocument/didClose":
		s.didClose(msg.Params)

	case "textDocument/didSave":
		// The document is already current; a save changes nothing.

	case "textDocument/hover":
		s.request(msg, s.hover)

	case "textDocument/completion":
		s.request(msg, s.completion)

	case "textDocument/documentSymbol":
		s.request(msg, s.documentSymbol)

	case "workspace/didChangeConfiguration":
		s.didChangeConfiguration(msg.Params)

	case "$/setTrace", "$/cancelRequest":
		// Accepted and ignored.

	default:
		if isRequest {
			_ = s.conn.replyError(msg.ID, codeMethodNotFound, "no such method: "+msg.Method)
		}
	}
	return false
}

// request runs a handler that produces a result for a request, turning a
// failure into a protocol error rather than dropping the reply.
func (s *Server) request(msg *message, handler func(json.RawMessage) (any, error)) {
	result, err := handler(msg.Params)
	if err != nil {
		_ = s.conn.replyError(msg.ID, codeRequestFailed, err.Error())
		return
	}
	_ = s.conn.reply(msg.ID, result)
}

func (s *Server) capabilities() map[string]any {
	return map[string]any{
		"capabilities": map[string]any{
			// Full synchronisation: ISO 20022 messages are small, and rebuilding
			// the index is cheaper than applying incremental edits correctly.
			"textDocumentSync": map[string]any{
				"openClose": true,
				"change":    1,
			},
			"hoverProvider":          true,
			"documentSymbolProvider": true,
			"completionProvider": map[string]any{
				"triggerCharacters": []string{"<"},
			},
			"diagnosticProvider": map[string]any{
				"interFileDependencies": false,
				"workspaceDiagnostics":  false,
			},
		},
		"serverInfo": map[string]any{
			"name":    s.Name,
			"version": s.Version,
		},
	}
}

// ---------------------------------------------------------------------------
// Document lifecycle
// ---------------------------------------------------------------------------

type textDocumentItem struct {
	URI  string `json:"uri"`
	Text string `json:"text"`
}

func (s *Server) didOpen(params json.RawMessage) {
	var p struct {
		TextDocument textDocumentItem `json:"textDocument"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		s.logf("didOpen: %v", err)
		return
	}
	s.store(p.TextDocument.URI, p.TextDocument.Text)
	s.publish(p.TextDocument.URI)
}

func (s *Server) didChange(params json.RawMessage) {
	var p struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
		ContentChanges []struct {
			Text string `json:"text"`
		} `json:"contentChanges"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		s.logf("didChange: %v", err)
		return
	}
	if len(p.ContentChanges) == 0 {
		return
	}
	// Full synchronisation: the last change carries the whole document.
	s.store(p.TextDocument.URI, p.ContentChanges[len(p.ContentChanges)-1].Text)
	s.publish(p.TextDocument.URI)
}

func (s *Server) didClose(params json.RawMessage) {
	var p struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return
	}

	s.mu.Lock()
	delete(s.docs, p.TextDocument.URI)
	s.mu.Unlock()

	// Clearing the diagnostics matters: an editor keeps showing the last set
	// otherwise, on a file that is no longer open.
	_ = s.conn.notify("textDocument/publishDiagnostics", map[string]any{
		"uri":         p.TextDocument.URI,
		"diagnostics": []Diagnostic{},
	})
}

func (s *Server) didChangeConfiguration(params json.RawMessage) {
	var p struct {
		Settings struct {
			Anchor struct {
				Profile string `json:"profile"`
			} `json:"anchor"`
		} `json:"settings"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return
	}
	if p.Settings.Anchor.Profile == "" {
		return
	}
	if _, err := iso20022.CheckProfile([]byte("<Document/>"), p.Settings.Anchor.Profile, ""); err != nil {
		s.logf("ignoring unknown rule profile %q", p.Settings.Anchor.Profile)
		return
	}

	s.Profile = p.Settings.Anchor.Profile

	// The setting changes every verdict, so every open document is rechecked.
	s.mu.RLock()
	uris := make([]string, 0, len(s.docs))
	for uri := range s.docs {
		uris = append(uris, uri)
	}
	s.mu.RUnlock()

	sort.Strings(uris)
	for _, uri := range uris {
		s.publish(uri)
	}
}

func (s *Server) store(uri, text string) {
	doc := Parse(text)
	s.mu.Lock()
	s.docs[uri] = doc
	s.mu.Unlock()
}

func (s *Server) document(uri string) (*Document, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	doc, ok := s.docs[uri]
	return doc, ok
}

func (s *Server) logf(format string, args ...any) {
	// A failure to log is not worth propagating: the stream it would report on
	// is the one that just failed.
	_, _ = fmt.Fprintf(s.errOut, "anchor-lsp: "+format+"\n", args...)
}

// ---------------------------------------------------------------------------
// Schema access
// ---------------------------------------------------------------------------

// schemaFor loads the schema a document's namespace names, when the user has it
// installed. Everything that needs it degrades rather than failing.
func (s *Server) schemaFor(doc *Document) (*xsd.Schema, string, bool) {
	msgID, err := iso20022.MessageIDFromInstance([]byte(doc.Text))
	if err != nil {
		return nil, "", false
	}
	cat, err := s.catalogue()
	if err != nil {
		return nil, msgID, false
	}
	path, err := cat.SchemaPath(msgID)
	if err != nil {
		return nil, msgID, false
	}
	schema, err := xsd.ParseFile(path)
	if err != nil {
		s.logf("parsing %s: %v", path, err)
		return nil, msgID, false
	}
	return schema, msgID, true
}

// trimNamespace removes a namespace prefix from an element name.
func trimNamespace(name string) string {
	if i := strings.IndexByte(name, ':'); i >= 0 {
		return name[i+1:]
	}
	return name
}
