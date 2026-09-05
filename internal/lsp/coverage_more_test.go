// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package lsp

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebastienrousseau/askiso/pkg/iso20022"
)

const coverageXSD = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"
 targetNamespace="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10"
 xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10" elementFormDefault="qualified">
 <xs:element name="Document" type="Doc"/><xs:complexType name="Doc"><xs:sequence/></xs:complexType>
</xs:schema>`

func coverageCatalogue(t *testing.T, body string) (*iso20022.Catalogue, string) {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "Payments", "Version 1.0", "Schemas")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "pacs.008.001.10.xsd")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cat, err := iso20022.OpenCatalogue(root)
	if err != nil {
		t.Fatal(err)
	}
	return cat, root
}

type stagedWriter struct{ calls int }

func (w *stagedWriter) Write(p []byte) (int, error) {
	w.calls++
	if w.calls > 1 {
		return 0, errors.New("body closed")
	}
	return len(p), nil
}

type closedWriter struct{}

func (closedWriter) Write([]byte) (int, error) { return 0, errors.New("closed") }

func TestInternalDiagnosticsDefensiveBranches(t *testing.T) {
	s := New(strings.NewReader(""), io.Discard, io.Discard)
	s.publish("file:///not-open.xml")

	doc := Parse("<")
	if len(doc.Elements) != 0 {
		t.Fatalf("malformed fixture unexpectedly has elements: %+v", doc.Elements)
	}
	_ = doc.errorRange()

	forced := &Document{Text: "<", Wellformed: true, byPath: map[string][]int{}, byName: map[string][]int{}}
	if got := s.lintDiagnostics(forced); got != nil {
		t.Fatalf("linter error should produce no secondary diagnostics: %+v", got)
	}
	s.Profile = "not-a-profile"
	valid := Parse(`<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10"/>`)
	if got := s.profileDiagnostics(valid); got != nil {
		t.Fatalf("unknown profile should produce no diagnostics: %+v", got)
	}
}

func TestSchemaAccessFailureModes(t *testing.T) {
	var logs bytes.Buffer
	s := New(strings.NewReader(""), io.Discard, &logs)
	if _, _, ok := s.schemaFor(Parse("<root/>")); ok {
		t.Fatal("non-ISO document should not resolve a schema")
	}

	validDoc := Parse(`<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10"/>`)
	s.SetCatalogue(func() (*iso20022.Catalogue, error) { return nil, errors.New("absent") })
	if _, id, ok := s.schemaFor(validDoc); ok || id != "pacs.008.001.10" {
		t.Fatalf("catalogue failure = %q %v", id, ok)
	}

	cat, _ := coverageCatalogue(t, coverageXSD)
	s.SetCatalogue(func() (*iso20022.Catalogue, error) { return cat, nil })
	missingDoc := Parse(`<Document xmlns="urn:iso:std:iso:20022:tech:xsd:camt.053.001.10"/>`)
	if _, id, ok := s.schemaFor(missingDoc); ok || id != "camt.053.001.10" {
		t.Fatalf("missing schema = %q %v", id, ok)
	}

	bad, _ := coverageCatalogue(t, "<not-closed>")
	s.SetCatalogue(func() (*iso20022.Catalogue, error) { return bad, nil })
	if _, _, ok := s.schemaFor(validDoc); ok || !strings.Contains(logs.String(), "parsing") {
		t.Fatalf("malformed installed schema was not logged: ok=%v logs=%q", ok, logs.String())
	}
}

func TestDocumentDiagnosticInputErrors(t *testing.T) {
	s := New(strings.NewReader(""), io.Discard, io.Discard)
	for _, raw := range []json.RawMessage{[]byte("{"), []byte(`{"textDocument":{}}`)} {
		if _, err := s.documentDiagnostic(raw); err == nil || !strings.Contains(err.Error(), "uri") {
			t.Fatalf("invalid diagnostic request should name uri: %v", err)
		}
	}
	if _, err := s.documentDiagnostic([]byte(`{"textDocument":{"uri":"file:///missing.xml"}}`)); err == nil || !strings.Contains(err.Error(), "not open") {
		t.Fatalf("unopened document should fail: %v", err)
	}
}

func TestRPCEncodingAndWriterFailures(t *testing.T) {
	c := newConn(strings.NewReader(""), io.Discard)
	if err := c.write(message{Result: make(chan int)}); err == nil {
		t.Fatal("unencodable response should fail")
	}
	c.w = closedWriter{}
	if err := c.write(message{Result: "ok"}); err == nil {
		t.Fatal("header write failure should propagate")
	}
	w := &stagedWriter{}
	c.w = w
	if err := c.write(message{Result: "ok"}); err == nil || w.calls != 2 {
		t.Fatalf("body write failure = %v after %d calls", err, w.calls)
	}
	if got := string(mustRaw(make(chan int))); got != "null" {
		t.Fatalf("mustRaw fallback = %q", got)
	}
}

func TestInstalledCatalogueOpenerCachesAndReturnsErrors(t *testing.T) {
	_, root := coverageCatalogue(t, coverageXSD)
	t.Setenv("ASKISO_CATALOG", root)
	open := openInstalledCatalogue()
	first, err := open()
	if err != nil {
		t.Fatal(err)
	}
	second, err := open()
	if err != nil || first != second {
		t.Fatalf("cached catalogue = %p/%p, %v", first, second, err)
	}

	t.Setenv("ASKISO_CATALOG", "")
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LocalAppData", t.TempDir())
	t.Chdir(t.TempDir())
	if _, err := openInstalledCatalogue()(); err == nil {
		t.Fatal("missing installed catalogue should return its resolution error")
	}
}
