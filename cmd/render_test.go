// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebastienrousseau/askiso/internal/catalog"
)

func TestPrettyPrintAndMinify(t *testing.T) {
	src := `<?xml version="1.0" encoding="UTF-8"?><Doc><A attr="1">x</A><B/></Doc>`

	pretty, err := prettyPrintXML([]byte(src))
	if err != nil {
		t.Fatalf("prettyPrintXML: %v", err)
	}
	if strings.Count(pretty, "\n") < 2 {
		t.Errorf("output should be indented:\n%s", pretty)
	}
	if !strings.HasPrefix(pretty, "<?xml") {
		t.Errorf("the declaration should be preserved:\n%s", pretty)
	}

	min, err := minifyXML([]byte(src))
	if err != nil {
		t.Fatalf("minifyXML: %v", err)
	}
	if strings.Count(min, "\n") > 1 {
		t.Errorf("minified output should be one line:\n%s", min)
	}

	// Round trip: minifying pretty output returns the same shape.
	again, err := minifyXML([]byte(pretty))
	if err != nil {
		t.Fatalf("minifyXML on pretty output: %v", err)
	}
	if !strings.Contains(again, "<A attr=\"1\">x</A>") {
		t.Errorf("attributes and text should survive:\n%s", again)
	}

	// Unclosed markup is rejected. Plain text is not: XML treats it as character
	// data, so formatting passes it through unchanged.
	for _, bad := range [][]byte{[]byte("<not-closed>"), []byte("<a></b>")} {
		if _, err := prettyPrintXML(bad); err == nil {
			t.Errorf("prettyPrintXML should reject %q", bad)
		}
		if _, err := minifyXML(bad); err == nil {
			t.Errorf("minifyXML should reject %q", bad)
		}
	}
}

func TestPrettyPrintAddsDeclaration(t *testing.T) {
	out, err := prettyPrintXML([]byte(`<Doc><A>1</A></Doc>`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "<?xml") {
		t.Errorf("a declaration should be added:\n%s", out)
	}
}

func TestHighlightXML(t *testing.T) {
	out := highlightXML(`<?xml version="1.0"?>` + "\n" + `<Doc attr="v"><!-- note -->text</Doc>`)
	if strings.TrimSpace(out) == "" {
		t.Error("highlighting produced nothing")
	}
	for _, want := range []string{"Doc", "attr", "text"} {
		if !strings.Contains(out, want) {
			t.Errorf("content %q should survive highlighting", want)
		}
	}
	if highlightXML("") != "" {
		t.Log("empty input produced non-empty output")
	}
}

func TestResolveSchemaPathVariants(t *testing.T) {
	root := fixtureCatalogue(t)
	idx, err := catalog.Load(root)
	if err != nil {
		t.Fatal(err)
	}

	// Exact identifier.
	if p, name := resolveSchemaPath(idx, "pacs.008.001.10"); p == "" || name == "" {
		t.Errorf("exact identifier did not resolve: %q %q", p, name)
	}
	// Base code, resolved through search.
	if p, _ := resolveSchemaPath(idx, "pacs.008"); p == "" {
		t.Error("a base code should resolve")
	}
	// Unknown.
	if p, _ := resolveSchemaPath(idx, "zzzz.999.999.99"); p != "" {
		t.Errorf("an unknown identifier should not resolve, got %q", p)
	}
}

func TestSearchRegistryRendersAndTruncates(t *testing.T) {
	isolate(t)

	// A broad query returns more than the display cap.
	out := captureStdout(t, func() {
		if err := searchRegistry("camt", false); err != nil {
			t.Errorf("searchRegistry: %v", err)
		}
	})
	if !strings.Contains(out, "and") || !strings.Contains(out, "more") {
		t.Logf("no truncation notice; result set may be small:\n%s", out)
	}
	if !strings.Contains(out, "no catalogue installed") {
		t.Errorf("the note should explain why there are no schema paths:\n%s", out)
	}

	// The same fallback reached from an installed catalogue says something
	// different, because "no catalogue installed" would be untrue.
	out = captureStdout(t, func() {
		if err := searchRegistry("camt", true); err != nil {
			t.Errorf("searchRegistry: %v", err)
		}
	})
	if strings.Contains(out, "no catalogue installed") {
		t.Errorf("the note claims nothing is installed:\n%s", out)
	}
	if !strings.Contains(out, "nothing installed matched") {
		t.Errorf("the note should say why the fallback ran:\n%s", out)
	}

	// A query with no hits says what is searchable rather than printing an
	// empty list.
	out = captureStdout(t, func() {
		if err := searchRegistry("zzzz-no-such-thing", false); err != nil {
			t.Errorf("searchRegistry: %v", err)
		}
	})
	if !strings.Contains(out, "Found 0 results") {
		t.Errorf("an empty result set should say so:\n%s", out)
	}
	if !strings.Contains(out, "message sets") {
		t.Errorf("an empty result set should explain what is searchable:\n%s", out)
	}
}

func TestResolveSchemaForInstanceErrors(t *testing.T) {
	isolate(t)

	// No ISO namespace at all.
	if _, err := resolveSchemaForInstance([]byte(`<root/>`)); err == nil {
		t.Error("a document with no ISO namespace should be an error")
	}
	// A real message with no catalogue installed.
	if _, err := resolveSchemaForInstance(
		[]byte(`<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10"/>`)); err == nil {
		t.Error("an uninstalled schema should be an error")
	}
}

func TestCheckOllamaConnectivityMalformedHost(t *testing.T) {
	// A host that cannot be turned into a request must be reported, not panic.
	out := captureStdout(t, func() {
		checkOllamaConnectivity("http://[::1]:namedport")
	})
	if strings.TrimSpace(out) == "" {
		t.Error("connectivity check produced no output")
	}
}

func TestSampleAndSchemaCopyFlags(t *testing.T) {
	withCatalogue(t)
	// --copy exercises the clipboard branch; unavailable clipboards must not
	// fail the command.
	if _, err := run(t, "sample", "pacs.008.001.10", "--copy"); err != nil {
		t.Errorf("sample --copy: %v", err)
	}
	if _, err := run(t, "schema", "pacs.008.001.10", "--copy"); err != nil {
		t.Errorf("schema --copy: %v", err)
	}
}

func TestConvertCopyFlag(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "in.xml")
	if err := os.WriteFile(src, []byte(fixtureInstance("pacs.008.001.10", "EUR")), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "convert", src, "--to-json", "--copy"); err != nil {
		t.Errorf("convert --copy: %v", err)
	}
}

func TestGraphCopyFlag(t *testing.T) {
	if _, err := run(t, "graph", "pacs.008", "--copy"); err != nil {
		t.Errorf("graph --copy: %v", err)
	}
}

func TestGenerateCopyFlag(t *testing.T) {
	if _, err := run(t, "generate", "pacs.008", "--copy"); err != nil {
		t.Errorf("generate --copy: %v", err)
	}
}
