// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package lsp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sebastienrousseau/askiso/internal/lsp"
	"github.com/sebastienrousseau/askiso/pkg/iso20022"
)

// The protocol frames messages with headers rather than newlines, so the tests
// speak it exactly as an editor would: anything looser would not catch a
// framing bug, and a framing bug looks like the server hanging.

// framed wraps a body in the protocol's headers.
func framed(body string) string {
	return fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body)
}

func frame(t *testing.T, msg map[string]any) string {
	t.Helper()
	body, err := json.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	return framed(string(body))
}

func request(t *testing.T, id int, method string, params map[string]any) string {
	return frame(t, map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
}

func notify(t *testing.T, method string, params map[string]any) string {
	return frame(t, map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

// decode reads framed replies off a stream.
func decode(t *testing.T, stream string) []map[string]any {
	t.Helper()

	var out []map[string]any
	r := strings.NewReader(stream)
	buf := make([]byte, 0)
	_ = buf

	data := []byte(stream)
	for i := 0; i < len(data); {
		sep := bytes.Index(data[i:], []byte("\r\n\r\n"))
		if sep < 0 {
			break
		}
		header := string(data[i : i+sep])

		length := -1
		for _, line := range strings.Split(header, "\r\n") {
			name, value, found := strings.Cut(line, ":")
			if found && strings.EqualFold(strings.TrimSpace(name), "content-length") {
				n, err := strconv.Atoi(strings.TrimSpace(value))
				if err != nil {
					t.Fatalf("Content-Length is not a number: %q", value)
				}
				length = n
			}
		}
		if length < 0 {
			t.Fatalf("a reply has no Content-Length: %q", header)
		}

		start := i + sep + 4
		if start+length > len(data) {
			t.Fatalf("a reply is shorter than its Content-Length says")
		}

		var msg map[string]any
		if err := json.Unmarshal(data[start:start+length], &msg); err != nil {
			t.Fatalf("a reply is not valid JSON: %v\n%s", err, data[start:start+length])
		}
		out = append(out, msg)
		i = start + length
	}
	_ = r
	return out
}

// session runs a server over a scripted exchange.
func session(t *testing.T, requests ...string) []map[string]any {
	t.Helper()
	return sessionWith(t, nil, requests...)
}

func sessionWith(t *testing.T, open lsp.CatalogueFunc, requests ...string) []map[string]any {
	t.Helper()

	var out, errOut bytes.Buffer
	s := lsp.New(strings.NewReader(strings.Join(requests, "")), &out, &errOut)
	s.SetVersion("test")
	s.SetCatalogue(open)
	s.SetCatalogue(nil) // a nil replacement must not disarm the server

	if err := s.Serve(context.Background()); err != nil {
		t.Fatalf("Serve: %v\nstderr: %s", err, errOut.String())
	}
	return decode(t, out.String())
}

// installedCatalogue opens the developer's real catalogue, skipping without one.
func installedCatalogue(t *testing.T) lsp.CatalogueFunc {
	t.Helper()
	cat, err := iso20022.OpenCatalogue("")
	if err != nil {
		t.Skip("no ISO 20022 catalogue installed")
	}
	if _, err := cat.SchemaPath("pacs.008.001.10"); err != nil {
		t.Skip("pacs.008.001.10 is not installed in this catalogue")
	}
	return func() (*iso20022.Catalogue, error) { return cat, nil }
}

func noCatalogue() (*iso20022.Catalogue, error) { return nil, errNoCatalogue }

var errNoCatalogue = errors.New("no catalogue installed")

var initialize = framed(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"capabilities":{}}}`)

func openDoc(t *testing.T, uri, text string) string {
	return notify(t, "textDocument/didOpen", map[string]any{
		"textDocument": map[string]any{
			"uri": uri, "languageId": "xml", "version": 1, "text": text,
		},
	})
}

// diagnosticsFor pulls the diagnostics out of a publish notification.
func diagnosticsFor(t *testing.T, replies []map[string]any, uri string) []map[string]any {
	t.Helper()
	for i := len(replies) - 1; i >= 0; i-- {
		if replies[i]["method"] != "textDocument/publishDiagnostics" {
			continue
		}
		params := replies[i]["params"].(map[string]any)
		if params["uri"] != uri {
			continue
		}
		raw := params["diagnostics"].([]any)
		out := make([]map[string]any, 0, len(raw))
		for _, d := range raw {
			out = append(out, d.(map[string]any))
		}
		return out
	}
	t.Fatalf("no diagnostics were published for %s", uri)
	return nil
}

func TestInitializeAdvertisesCapabilities(t *testing.T) {
	replies := session(t, request(t, 1, "initialize", map[string]any{}))
	if len(replies) != 1 {
		t.Fatalf("got %d replies, want 1", len(replies))
	}

	result := replies[0]["result"].(map[string]any)
	caps := result["capabilities"].(map[string]any)
	for _, want := range []string{"hoverProvider", "completionProvider", "documentSymbolProvider", "diagnosticProvider", "textDocumentSync"} {
		if _, ok := caps[want]; !ok {
			t.Errorf("%s is not advertised", want)
		}
	}

	// Full synchronisation, because rebuilding the index is cheaper than
	// applying incremental edits correctly.
	sync := caps["textDocumentSync"].(map[string]any)
	if sync["change"].(float64) != 1 {
		t.Errorf("textDocumentSync.change = %v, want 1 (full)", sync["change"])
	}

	info := result["serverInfo"].(map[string]any)
	if info["name"] != "askiso-lsp" || info["version"] != "test" {
		t.Errorf("serverInfo = %v", info)
	}
}

func TestPullDiagnosticsMatchesPublishedDiagnostics(t *testing.T) {
	replies := sessionWith(t, noCatalogue, initialize,
		openDoc(t, "file:///a.xml", addressInstance),
		request(t, 2, "textDocument/diagnostic", map[string]any{
			"textDocument": map[string]any{"uri": "file:///a.xml"},
		}))
	result := replies[len(replies)-1]["result"].(map[string]any)
	if result["kind"] != "full" {
		t.Fatalf("diagnostic report kind = %v", result["kind"])
	}
	items := result["items"].([]any)
	if len(items) == 0 {
		t.Fatal("pull diagnostics returned no address findings")
	}
}

func TestRequestsBeforeInitialize(t *testing.T) {
	replies := session(t, request(t, 1, "textDocument/hover", map[string]any{}))
	e := replies[0]["error"].(map[string]any)
	if !strings.Contains(e["message"].(string), "initialize") {
		t.Errorf("error = %v", e)
	}
}

func TestNotificationsBeforeInitializeAreSilent(t *testing.T) {
	replies := session(t, openDoc(t, "file:///a.xml", instance))
	if len(replies) != 0 {
		t.Errorf("a notification before initialize was answered: %v", replies)
	}
}

func TestShutdownAndExit(t *testing.T) {
	replies := session(t,
		initialize,
		request(t, 2, "shutdown", nil),
		request(t, 3, "textDocument/hover", map[string]any{}),
		notify(t, "exit", nil),
		request(t, 4, "shutdown", nil), // never reached: exit stops the loop
	)

	if len(replies) != 3 {
		t.Fatalf("got %d replies, want 3: %v", len(replies), replies)
	}
	if replies[1]["result"] != nil {
		t.Errorf("shutdown should reply with null, got %v", replies[1]["result"])
	}
	// After shutdown every request is refused until exit arrives.
	if _, ok := replies[2]["error"]; !ok {
		t.Errorf("a request after shutdown was served: %v", replies[2])
	}
}

func TestUnknownMethod(t *testing.T) {
	replies := session(t, initialize, request(t, 2, "textDocument/nonsense", map[string]any{}))
	e := replies[1]["error"].(map[string]any)
	if !strings.Contains(e["message"].(string), "nonsense") {
		t.Errorf("error = %v", e)
	}

	// An unknown notification is ignored rather than answered.
	quiet := session(t, initialize, notify(t, "textDocument/nonsense", map[string]any{}))
	if len(quiet) != 1 {
		t.Errorf("an unknown notification was answered: %v", quiet)
	}
}

func TestIgnoredMethods(t *testing.T) {
	replies := session(t, initialize,
		notify(t, "initialized", map[string]any{}),
		notify(t, "$/setTrace", map[string]any{"value": "verbose"}),
		notify(t, "textDocument/didSave", map[string]any{}),
		request(t, 2, "shutdown", nil))
	if len(replies) != 2 {
		t.Errorf("got %d replies, want 2: %v", len(replies), replies)
	}
}

// ---------------------------------------------------------------------------
// Diagnostics
// ---------------------------------------------------------------------------

func TestDiagnosticsOnOpen(t *testing.T) {
	// The fixture carries an unstructured address, which is what cbpr-2026
	// exists to catch.
	replies := sessionWith(t, noCatalogue, initialize,
		notify(t, "initialized", map[string]any{}),
		openDoc(t, "file:///a.xml", addressInstance))

	diags := diagnosticsFor(t, replies, "file:///a.xml")
	if len(diags) == 0 {
		t.Fatal("no diagnostics were reported for an unstructured address")
	}

	var sawAddressRule bool
	for _, d := range diags {
		if strings.HasPrefix(d["code"].(string), "CBPR-ADDR") {
			sawAddressRule = true
			// The diagnostic has to point at the address, not at the document.
			rng := d["range"].(map[string]any)["start"].(map[string]any)
			if rng["line"].(float64) == 0 {
				t.Errorf("the diagnostic points at line 0: %v", d)
			}
			// And it has to say how to fix it.
			if !strings.Contains(d["message"].(string), "TwnNm") {
				t.Errorf("the message does not say what to do: %q", d["message"])
			}
		}
	}
	if !sawAddressRule {
		t.Errorf("the address rules did not fire: %v", diags)
	}
}

func TestDiagnosticsLocateALintFinding(t *testing.T) {
	doc := strings.Replace(addressInstance, "GB29NWBK60161331926819", "GB29NWBK60161331926810", 1)

	replies := sessionWith(t, noCatalogue, initialize, openDoc(t, "file:///a.xml", doc))
	diags := diagnosticsFor(t, replies, "file:///a.xml")

	var found map[string]any
	for _, d := range diags {
		if strings.Contains(strings.ToLower(d["message"].(string)), "iban") {
			found = d
		}
	}
	if found == nil {
		t.Fatalf("a broken IBAN checksum was not reported: %v", diags)
	}

	// The underline must sit on the IBAN itself.
	rng := found["range"].(map[string]any)
	start := rng["start"].(map[string]any)
	line := strings.Split(doc, "\n")[int(start["line"].(float64))]
	if !strings.Contains(line, "IBAN") {
		t.Errorf("the diagnostic points at %q", line)
	}
}

func TestMalformedXMLIsOneDiagnostic(t *testing.T) {
	replies := sessionWith(t, noCatalogue, initialize,
		openDoc(t, "file:///a.xml", "<Document><GrpHdr>"))

	diags := diagnosticsFor(t, replies, "file:///a.xml")
	if len(diags) != 1 {
		t.Fatalf("got %d diagnostics for malformed XML, want 1: %v", len(diags), diags)
	}
	if !strings.Contains(diags[0]["message"].(string), "well-formed") {
		t.Errorf("message = %q", diags[0]["message"])
	}
}

func TestEmptyDocumentHasNoDiagnostics(t *testing.T) {
	replies := sessionWith(t, noCatalogue, initialize, openDoc(t, "file:///a.xml", "   "))
	if diags := diagnosticsFor(t, replies, "file:///a.xml"); len(diags) != 0 {
		t.Errorf("an empty document reported %v", diags)
	}
}

func TestDidChangeRepublishes(t *testing.T) {
	clean := `<?xml version="1.0"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">
  <FIToFICstmrCdtTrf><CdtTrfTxInf><Cdtr>
    <Nm>MUELLER GMBH</Nm>
    <PstlAdr><TwnNm>FRANKFURT</TwnNm><Ctry>DE</Ctry></PstlAdr>
  </Cdtr></CdtTrfTxInf></FIToFICstmrCdtTrf>
</Document>`

	replies := sessionWith(t, noCatalogue, initialize,
		openDoc(t, "file:///a.xml", addressInstance),
		notify(t, "textDocument/didChange", map[string]any{
			"textDocument":   map[string]any{"uri": "file:///a.xml", "version": 2},
			"contentChanges": []map[string]any{{"text": clean}},
		}))

	// The last publication reflects the edit: a structured address passes.
	if diags := diagnosticsFor(t, replies, "file:///a.xml"); len(diags) != 0 {
		t.Errorf("a structured address still reported %v", diags)
	}
}

func TestDidChangeWithNoChanges(t *testing.T) {
	replies := sessionWith(t, noCatalogue, initialize,
		openDoc(t, "file:///a.xml", addressInstance),
		notify(t, "textDocument/didChange", map[string]any{
			"textDocument":   map[string]any{"uri": "file:///a.xml", "version": 2},
			"contentChanges": []map[string]any{},
		}))

	// One publication, from the open; an empty change list republishes nothing.
	var publications int
	for _, r := range replies {
		if r["method"] == "textDocument/publishDiagnostics" {
			publications++
		}
	}
	if publications != 1 {
		t.Errorf("got %d publications, want 1", publications)
	}
}

func TestDidCloseClearsDiagnostics(t *testing.T) {
	replies := sessionWith(t, noCatalogue, initialize,
		openDoc(t, "file:///a.xml", addressInstance),
		notify(t, "textDocument/didClose", map[string]any{
			"textDocument": map[string]any{"uri": "file:///a.xml"},
		}))

	// An editor keeps showing the last set otherwise, on a file nobody has open.
	if diags := diagnosticsFor(t, replies, "file:///a.xml"); len(diags) != 0 {
		t.Errorf("closing left %v behind", diags)
	}
}

func TestConfigurationChangesTheProfile(t *testing.T) {
	replies := sessionWith(t, noCatalogue, initialize,
		openDoc(t, "file:///a.xml", addressInstance),
		notify(t, "workspace/didChangeConfiguration", map[string]any{
			"settings": map[string]any{"askiso": map[string]any{"profile": "base"}},
		}))

	// The base profile does not carry the address rules, so the same document
	// is clean under it.
	diags := diagnosticsFor(t, replies, "file:///a.xml")
	for _, d := range diags {
		if strings.HasPrefix(d["code"].(string), "CBPR-ADDR") {
			t.Errorf("an address rule fired under the base profile: %v", d)
		}
	}
}

func TestConfigurationCanDisableTheProfile(t *testing.T) {
	replies := sessionWith(t, noCatalogue, initialize,
		openDoc(t, "file:///a.xml", addressInstance),
		notify(t, "workspace/didChangeConfiguration", map[string]any{
			"settings": map[string]any{"askiso": map[string]any{"profile": ""}},
		}))
	diags := diagnosticsFor(t, replies, "file:///a.xml")
	for _, d := range diags {
		if strings.HasPrefix(d["code"].(string), "CBPR-ADDR") {
			t.Errorf("empty profile did not disable address rules: %v", d)
		}
	}
}

func TestServeCancellationInterruptsBlockedRead(t *testing.T) {
	reader, writer := io.Pipe()
	defer func() { _ = writer.Close() }()
	var out bytes.Buffer
	s := lsp.New(reader, &out, io.Discard)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Serve(ctx) }()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Serve returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Serve stayed blocked after cancellation")
	}
}

func TestUnknownProfileIsIgnored(t *testing.T) {
	replies := sessionWith(t, noCatalogue, initialize,
		openDoc(t, "file:///a.xml", addressInstance),
		notify(t, "workspace/didChangeConfiguration", map[string]any{
			"settings": map[string]any{"askiso": map[string]any{"profile": "no-such-profile"}},
		}))

	// The setting is refused rather than silently disabling every rule.
	diags := diagnosticsFor(t, replies, "file:///a.xml")
	var sawAddressRule bool
	for _, d := range diags {
		if strings.HasPrefix(d["code"].(string), "CBPR-ADDR") {
			sawAddressRule = true
		}
	}
	if !sawAddressRule {
		t.Error("an unknown profile disabled the rules that were working")
	}
}

func TestMalformedNotificationParams(t *testing.T) {
	// A client sending nonsense must not take the server down.
	replies := sessionWith(t, noCatalogue, initialize,
		notify(t, "textDocument/didOpen", map[string]any{"textDocument": "not an object"}),
		notify(t, "textDocument/didChange", map[string]any{"textDocument": 42}),
		notify(t, "textDocument/didClose", map[string]any{"textDocument": true}),
		notify(t, "workspace/didChangeConfiguration", map[string]any{"settings": 7}),
		request(t, 2, "shutdown", nil))

	if _, ok := replies[len(replies)-1]["result"]; !ok {
		t.Errorf("the server did not survive malformed parameters: %v", replies)
	}
}

// ---------------------------------------------------------------------------
// Hover, completion, symbols
// ---------------------------------------------------------------------------

func offsetOf(t *testing.T, text, needle string) lsp.Position {
	t.Helper()
	i := strings.Index(text, needle)
	if i < 0 {
		t.Fatalf("%q is not in the fixture", needle)
	}
	return lsp.Parse(text).PositionAt(i + len(needle)/2)
}

func TestHoverWithoutASchema(t *testing.T) {
	pos := offsetOf(t, addressInstance, "<Nm>")
	replies := sessionWith(t, noCatalogue, initialize,
		openDoc(t, "file:///a.xml", addressInstance),
		request(t, 2, "textDocument/hover", map[string]any{
			"textDocument": map[string]any{"uri": "file:///a.xml"},
			"position":     map[string]any{"line": pos.Line, "character": pos.Character},
		}))

	result := replies[len(replies)-1]["result"].(map[string]any)
	value := result["contents"].(map[string]any)["value"].(string)

	// Without the schema the hover says what the document says, and says how to
	// get the rest -- rather than inventing a type.
	if !strings.Contains(value, "/Document/") {
		t.Errorf("the hover does not show the element path:\n%s", value)
	}
	if !strings.Contains(value, "iso20022.org") {
		t.Errorf("the hover does not say how to install schemas:\n%s", value)
	}
}

func TestHoverOnNothing(t *testing.T) {
	replies := sessionWith(t, noCatalogue, initialize,
		openDoc(t, "file:///a.xml", addressInstance),
		request(t, 2, "textDocument/hover", map[string]any{
			"textDocument": map[string]any{"uri": "file:///a.xml"},
			"position":     map[string]any{"line": 0, "character": 0},
		}),
		request(t, 3, "textDocument/hover", map[string]any{
			"textDocument": map[string]any{"uri": "file:///missing.xml"},
			"position":     map[string]any{"line": 0, "character": 0},
		}))

	for _, r := range replies[len(replies)-2:] {
		if r["result"] != nil {
			t.Errorf("a hover over nothing returned %v", r["result"])
		}
	}
}

func TestHoverWithASchema(t *testing.T) {
	open := installedCatalogue(t)
	pos := offsetOf(t, schemaInstance, "<ChrgBr>")

	replies := sessionWith(t, open, initialize,
		openDoc(t, "file:///a.xml", schemaInstance),
		request(t, 2, "textDocument/hover", map[string]any{
			"textDocument": map[string]any{"uri": "file:///a.xml"},
			"position":     map[string]any{"line": pos.Line, "character": pos.Character},
		}))

	value := replies[len(replies)-1]["result"].(map[string]any)["contents"].(map[string]any)["value"].(string)
	for _, want := range []string{"ChargeBearerType1Code", "mandatory", "DEBT", "SHAR"} {
		if !strings.Contains(value, want) {
			t.Errorf("the hover is missing %q:\n%s", want, value)
		}
	}
}

func TestCompletionListsChildrenInSchemaOrder(t *testing.T) {
	open := installedCatalogue(t)

	// A cursor just inside <PmtId> is asking what belongs there.
	i := strings.Index(schemaInstance, "<PmtId>") + len("<PmtId>")
	pos := lsp.Parse(schemaInstance).PositionAt(i)

	replies := sessionWith(t, open, initialize,
		openDoc(t, "file:///a.xml", schemaInstance),
		request(t, 2, "textDocument/completion", map[string]any{
			"textDocument": map[string]any{"uri": "file:///a.xml"},
			"position":     map[string]any{"line": pos.Line, "character": pos.Character},
		}))

	result := replies[len(replies)-1]["result"].(map[string]any)
	items := result["items"].([]any)
	if len(items) == 0 {
		t.Fatal("no completions were offered inside PmtId")
	}

	var labels, sortKeys []string
	for _, raw := range items {
		item := raw.(map[string]any)
		labels = append(labels, item["label"].(string))
		sortKeys = append(sortKeys, item["sortText"].(string))
	}

	// ISO 20022 content models are ordered sequences, so the suggestions must
	// keep the schema's order rather than sorting alphabetically.
	for i := 1; i < len(sortKeys); i++ {
		if sortKeys[i-1] >= sortKeys[i] {
			t.Fatalf("the sort keys do not preserve schema order: %v", sortKeys)
		}
	}
	if labels[0] != "InstrId" {
		t.Errorf("the first suggestion is %q, want InstrId", labels[0])
	}
	if !contains(labels, "UETR") {
		t.Errorf("UETR is missing from %v", labels)
	}
}

func TestCompletionOffersCodesInsideAnEnumeratedElement(t *testing.T) {
	open := installedCatalogue(t)

	i := strings.Index(schemaInstance, "<ChrgBr>") + len("<ChrgBr>") + 1
	pos := lsp.Parse(schemaInstance).PositionAt(i)

	replies := sessionWith(t, open, initialize,
		openDoc(t, "file:///a.xml", schemaInstance),
		request(t, 2, "textDocument/completion", map[string]any{
			"textDocument": map[string]any{"uri": "file:///a.xml"},
			"position":     map[string]any{"line": pos.Line, "character": pos.Character},
		}))

	items := replies[len(replies)-1]["result"].(map[string]any)["items"].([]any)
	var labels []string
	for _, raw := range items {
		labels = append(labels, raw.(map[string]any)["label"].(string))
	}
	for _, want := range []string{"DEBT", "CRED", "SHAR"} {
		if !contains(labels, want) {
			t.Errorf("%s is missing from %v", want, labels)
		}
	}
}

func TestCompletionWithoutASchema(t *testing.T) {
	// Suggesting element names without the schema would be guessing, so the
	// list is empty rather than wrong.
	replies := sessionWith(t, noCatalogue, initialize,
		openDoc(t, "file:///a.xml", schemaInstance),
		request(t, 2, "textDocument/completion", map[string]any{
			"textDocument": map[string]any{"uri": "file:///a.xml"},
			"position":     map[string]any{"line": 2, "character": 0},
		}),
		request(t, 3, "textDocument/completion", map[string]any{
			"textDocument": map[string]any{"uri": "file:///missing.xml"},
			"position":     map[string]any{"line": 0, "character": 0},
		}))

	items := replies[len(replies)-2]["result"].(map[string]any)["items"].([]any)
	if len(items) != 0 {
		t.Errorf("completions were offered with no schema: %v", items)
	}
	if replies[len(replies)-1]["result"] != nil {
		t.Errorf("completion on an unknown document returned %v", replies[len(replies)-1]["result"])
	}
}

func TestDocumentSymbols(t *testing.T) {
	replies := sessionWith(t, noCatalogue, initialize,
		openDoc(t, "file:///a.xml", instance),
		request(t, 2, "textDocument/documentSymbol", map[string]any{
			"textDocument": map[string]any{"uri": "file:///a.xml"},
		}))

	roots := replies[len(replies)-1]["result"].([]any)
	if len(roots) != 1 {
		t.Fatalf("got %d roots, want 1", len(roots))
	}

	root := roots[0].(map[string]any)
	if root["name"] != "Document" {
		t.Errorf("root = %v", root["name"])
	}

	// The outline has to be a tree, not a flat list, or navigating a message is
	// no easier than reading it.
	transfer := root["children"].([]any)[0].(map[string]any)
	if transfer["name"] != "FIToFICstmrCdtTrf" {
		t.Errorf("the first child is %v", transfer["name"])
	}
	grpHdr := transfer["children"].([]any)[0].(map[string]any)
	if grpHdr["name"] != "GrpHdr" {
		t.Errorf("the first grandchild is %v", grpHdr["name"])
	}
	msgID := grpHdr["children"].([]any)[0].(map[string]any)
	if msgID["detail"] != "MSG-0001" {
		t.Errorf("a leaf does not carry its value: %v", msgID)
	}

	empty := sessionWith(t, noCatalogue, initialize,
		request(t, 2, "textDocument/documentSymbol", map[string]any{
			"textDocument": map[string]any{"uri": "file:///missing.xml"},
		}))
	if got := empty[len(empty)-1]["result"].([]any); len(got) != 0 {
		t.Errorf("symbols for an unknown document = %v", got)
	}
}

func TestRequestWithBadParams(t *testing.T) {
	replies := sessionWith(t, noCatalogue, initialize,
		frame(t, map[string]any{"jsonrpc": "2.0", "id": 2,
			"method": "textDocument/hover", "params": "not an object"}),
		frame(t, map[string]any{"jsonrpc": "2.0", "id": 3,
			"method": "textDocument/completion", "params": 7}),
		frame(t, map[string]any{"jsonrpc": "2.0", "id": 4,
			"method": "textDocument/documentSymbol", "params": []int{1}}))

	for _, r := range replies[1:] {
		if _, ok := r["error"]; !ok {
			t.Errorf("bad parameters were accepted: %v", r)
		}
	}
}

// ---------------------------------------------------------------------------
// Schema-backed diagnostics
// ---------------------------------------------------------------------------

func TestSchemaDiagnostics(t *testing.T) {
	open := installedCatalogue(t)

	// A document with its elements out of order: ISO 20022 content models are
	// ordered sequences, and this is the mistake the schema catches that the
	// linter cannot.
	broken := `<?xml version="1.0"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">
  <FIToFICstmrCdtTrf>
    <CdtTrfTxInf/>
    <GrpHdr/>
  </FIToFICstmrCdtTrf>
</Document>`

	replies := sessionWith(t, open, initialize, openDoc(t, "file:///a.xml", broken))
	diags := diagnosticsFor(t, replies, "file:///a.xml")

	var fromSchema int
	for _, d := range diags {
		if d["source"] == "askiso/schema" {
			fromSchema++
		}
	}
	if fromSchema == 0 {
		t.Errorf("the schema validator reported nothing for a malformed document: %v", diags)
	}
}

func TestNoSchemaDiagnosticsWithoutACatalogue(t *testing.T) {
	// Without the schemas the server must not pretend a document is valid, nor
	// invent errors. It simply reports nothing from that source.
	replies := sessionWith(t, noCatalogue, initialize,
		openDoc(t, "file:///a.xml", schemaInstance))

	for _, d := range diagnosticsFor(t, replies, "file:///a.xml") {
		if d["source"] == "askiso/schema" {
			t.Errorf("a schema diagnostic appeared with no catalogue: %v", d)
		}
	}
}

// ---------------------------------------------------------------------------
// Transport
// ---------------------------------------------------------------------------

func TestFramingErrors(t *testing.T) {
	cases := []struct {
		name, stream, want string
	}{
		{"no content length", "Header: 1\r\n\r\n{}", "Content-Length"},
		{"unparseable length", "Content-Length: abc\r\n\r\n{}", "not a number"},
		{"malformed header", "nonsense\r\n\r\n{}", "malformed header"},
		{"oversized", "Content-Length: 999999999\r\n\r\n{}", "limit"},
		{"truncated body", "Content-Length: 100\r\n\r\n{}", "unexpected EOF"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			s := lsp.New(strings.NewReader(tc.stream), &out, &errOut)
			err := s.Serve(context.Background())
			if err == nil {
				t.Fatal("Serve accepted a malformed stream")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestUnparseableBodyIsAnswered(t *testing.T) {
	// The framing was right, so the stream is still synchronised: the server
	// answers and carries on rather than dying.
	stream := framed("not json") + initialize

	var out, errOut bytes.Buffer
	s := lsp.New(strings.NewReader(stream), &out, &errOut)
	if err := s.Serve(context.Background()); err != nil {
		t.Fatalf("Serve: %v", err)
	}

	replies := decode(t, out.String())
	if len(replies) != 2 {
		t.Fatalf("got %d replies, want 2", len(replies))
	}
	if _, ok := replies[0]["error"]; !ok {
		t.Errorf("the unparseable body was not answered with an error: %v", replies[0])
	}
	if _, ok := replies[1]["result"]; !ok {
		t.Errorf("the server did not carry on after a bad body: %v", replies[1])
	}
}

func TestEmptyStream(t *testing.T) {
	var out bytes.Buffer
	s := lsp.New(strings.NewReader(""), &out, &bytes.Buffer{})
	if err := s.Serve(context.Background()); err != nil {
		t.Errorf("Serve on an empty stream = %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("an empty stream produced output: %q", out.String())
	}
}

func TestCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var out bytes.Buffer
	s := lsp.New(strings.NewReader(initialize), &out, &bytes.Buffer{})
	if err := s.Serve(ctx); err == nil {
		t.Error("Serve returned nil for a cancelled context")
	}
	if out.Len() != 0 {
		t.Errorf("a cancelled server replied: %q", out.String())
	}
}

func TestReadFailure(t *testing.T) {
	s := lsp.New(brokenReader{}, &bytes.Buffer{}, &bytes.Buffer{})
	if err := s.Serve(context.Background()); err == nil {
		t.Error("Serve returned nil for a broken stream")
	}
}

type brokenReader struct{}

func (brokenReader) Read([]byte) (int, error) { return 0, errBroken }

var errBroken = errors.New("the stream is broken")

func TestSetVersionIgnoresEmpty(t *testing.T) {
	s := lsp.New(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	s.SetVersion("")
	if s.Version != "dev" {
		t.Errorf("Version = %q", s.Version)
	}
}

func TestDiagnoseIsUsableDirectly(t *testing.T) {
	// The checks are exported so they can be exercised without a client on the
	// other end of a pipe.
	s := lsp.New(strings.NewReader(""), io.Discard, io.Discard)
	s.SetCatalogue(noCatalogue)

	if got := s.Diagnose(lsp.Parse(addressInstance)); len(got) == 0 {
		t.Error("Diagnose reported nothing for an unstructured address")
	}
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

const addressInstance = `<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">
  <FIToFICstmrCdtTrf>
    <CdtTrfTxInf>
      <Cdtr>
        <Nm>MUELLER GMBH</Nm>
        <PstlAdr>
          <AdrLine>HAUPTSTRASSE 12</AdrLine>
          <AdrLine>60311 FRANKFURT AM MAIN</AdrLine>
        </PstlAdr>
      </Cdtr>
      <CdtrAcct>
        <Id><IBAN>GB29NWBK60161331926819</IBAN></Id>
      </CdtrAcct>
    </CdtTrfTxInf>
  </FIToFICstmrCdtTrf>
</Document>`

const schemaInstance = `<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">
  <FIToFICstmrCdtTrf>
    <GrpHdr>
      <MsgId>MSG-0001</MsgId>
      <CreDtTm>2026-08-24T09:00:00Z</CreDtTm>
      <NbOfTxs>1</NbOfTxs>
      <SttlmInf><SttlmMtd>CLRG</SttlmMtd></SttlmInf>
    </GrpHdr>
    <CdtTrfTxInf>
      <PmtId>
        <EndToEndId>E2E-1</EndToEndId>
      </PmtId>
      <IntrBkSttlmAmt Ccy="EUR">25000.00</IntrBkSttlmAmt>
      <ChrgBr>SHAR</ChrgBr>
      <Dbtr><Nm>ACME</Nm></Dbtr>
      <DbtrAgt><FinInstnId><BICFI>BANKGB2LXXX</BICFI></FinInstnId></DbtrAgt>
      <CdtrAgt><FinInstnId><BICFI>BANKDEFFXXX</BICFI></FinInstnId></CdtrAgt>
      <Cdtr><Nm>MUELLER</Nm></Cdtr>
    </CdtTrfTxInf>
  </FIToFICstmrCdtTrf>
</Document>`
