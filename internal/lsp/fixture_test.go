// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package lsp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebastienrousseau/askiso/internal/lsp"
	"github.com/sebastienrousseau/askiso/pkg/iso20022"
)

// Hover and completion are the two things an editor integration exists for,
// and both need a schema. The tests for them skipped whenever the developer
// running them had no catalogue installed, which is always true on CI — so the
// features most likely to break silently were the least exercised.
//
// A fixture catalogue fixes that. The schema is small but real: it has facets,
// a choice, an unbounded element and a nested type, which is enough to drive
// every branch hover and completion have.
const fixtureXSD = `<?xml version="1.0" encoding="UTF-8"?>
<xs:schema xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10"
           xmlns:xs="http://www.w3.org/2001/XMLSchema"
           elementFormDefault="qualified"
           targetNamespace="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">
  <xs:element name="Document" type="Document"/>
  <xs:complexType name="Document">
    <xs:sequence>
      <xs:element name="GrpHdr" type="GroupHeader"/>
      <xs:element name="Tx" type="Transaction" minOccurs="0" maxOccurs="unbounded"/>
    </xs:sequence>
  </xs:complexType>
  <xs:complexType name="GroupHeader">
    <xs:sequence>
      <xs:element name="MsgId" type="Max35Text"/>
      <xs:element name="Sts" type="StatusCode" minOccurs="0"/>
    </xs:sequence>
  </xs:complexType>
  <xs:complexType name="Transaction">
    <xs:sequence>
      <xs:element name="Amt" type="Max35Text"/>
    </xs:sequence>
  </xs:complexType>
  <xs:simpleType name="Max35Text">
    <xs:restriction base="xs:string">
      <xs:minLength value="1"/>
      <xs:maxLength value="35"/>
    </xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="StatusCode">
    <xs:restriction base="xs:string">
      <xs:enumeration value="ACCP"/>
      <xs:enumeration value="RJCT"/>
    </xs:restriction>
  </xs:simpleType>
</xs:schema>`

// fixtureCatalogue writes a catalogue holding pacs.008.001.10 and returns a
// CatalogueFunc for it, so these tests run with or without a real one.
func fixtureCatalogue(t *testing.T) lsp.CatalogueFunc {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "Payments Clearing and Settlement", "Version 11.0", "Schemas")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pacs.008.001.10.xsd"), []byte(fixtureXSD), 0o644); err != nil {
		t.Fatal(err)
	}
	cat, err := iso20022.OpenCatalogue(root)
	if err != nil {
		t.Fatalf("OpenCatalogue: %v", err)
	}
	return func() (*iso20022.Catalogue, error) { return cat, nil }
}

const fixtureDoc = `<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">
  <GrpHdr>
    <MsgId>MSG-1</MsgId>
    <Sts>ACCP</Sts>
  </GrpHdr>
</Document>`

func TestHoverDescribesAnElementFromTheSchema(t *testing.T) {
	replies := sessionWith(t, fixtureCatalogue(t),
		initialize,
		openDoc(t, "file:///m.xml", fixtureDoc),
		request(t, 2, "textDocument/hover", map[string]any{
			"textDocument": map[string]any{"uri": "file:///m.xml"},
			// Inside <MsgId> on line 3.
			"position": map[string]any{"line": 3, "character": 6},
		}),
	)

	body := hoverText(t, replies)
	for _, want := range []string{"Max35Text", "Occurs", "Constraints"} {
		if !strings.Contains(body, want) {
			t.Errorf("hover does not mention %q:\n%s", want, body)
		}
	}
	// The facets are the reason to hover at all.
	if !strings.Contains(body, "35") {
		t.Errorf("hover does not carry the length facet:\n%s", body)
	}
}

// A complex type's hover should list what it contains, so a reader can see the
// shape without opening the schema.
func TestHoverListsChildElements(t *testing.T) {
	replies := sessionWith(t, fixtureCatalogue(t),
		initialize,
		openDoc(t, "file:///m.xml", fixtureDoc),
		request(t, 2, "textDocument/hover", map[string]any{
			"textDocument": map[string]any{"uri": "file:///m.xml"},
			// Inside <GrpHdr> on line 2.
			"position": map[string]any{"line": 2, "character": 5},
		}),
	)

	body := hoverText(t, replies)
	if !strings.Contains(body, "Contains") || !strings.Contains(body, "MsgId") {
		t.Errorf("hover does not list the children:\n%s", body)
	}
}

func TestCompletionOffersTheChildrenOfTheEnclosingElement(t *testing.T) {
	doc := `<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">
  <GrpHdr>

  </GrpHdr>
</Document>`

	replies := sessionWith(t, fixtureCatalogue(t),
		initialize,
		openDoc(t, "file:///c.xml", doc),
		request(t, 2, "textDocument/completion", map[string]any{
			"textDocument": map[string]any{"uri": "file:///c.xml"},
			"position":     map[string]any{"line": 3, "character": 4},
		}),
	)

	labels := completionLabels(t, replies)
	if len(labels) == 0 {
		t.Fatal("no completions inside GrpHdr")
	}
	joined := strings.Join(labels, ",")
	for _, want := range []string{"MsgId", "Sts"} {
		if !strings.Contains(joined, want) {
			t.Errorf("completion does not offer %q: %v", want, labels)
		}
	}
}

// A cursor inside an element's text is asking for a value, not a child. For an
// enumerated type that means the codes, which is the completion that actually
// saves someone looking up a code list.
func TestCompletionOffersEnumeratedValues(t *testing.T) {
	doc := `<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">
  <GrpHdr>
    <MsgId>M</MsgId>
    <Sts>A</Sts>
  </GrpHdr>
</Document>`

	replies := sessionWith(t, fixtureCatalogue(t),
		initialize,
		openDoc(t, "file:///v.xml", doc),
		request(t, 2, "textDocument/completion", map[string]any{
			"textDocument": map[string]any{"uri": "file:///v.xml"},
			// Inside the text of <Sts>.
			"position": map[string]any{"line": 4, "character": 10},
		}),
	)

	joined := strings.Join(completionLabels(t, replies), ",")
	if !strings.Contains(joined, "ACCP") || !strings.Contains(joined, "RJCT") {
		t.Errorf("completion did not offer the enumerated values: %s", joined)
	}
}

// Schema diagnostics are the difference between an editor that flags a real
// problem and one that only checks the document is well formed.
func TestDiagnosticsReportSchemaViolations(t *testing.T) {
	// Sts carries a value the enumeration does not allow.
	bad := `<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">
  <GrpHdr>
    <MsgId>MSG-1</MsgId>
    <Sts>NOPE</Sts>
  </GrpHdr>
</Document>`

	replies := sessionWith(t, fixtureCatalogue(t),
		initialize,
		openDoc(t, "file:///bad.xml", bad),
	)

	found := diagnosticsFor(t, replies, "file:///bad.xml")
	if len(found) == 0 {
		t.Fatal("no diagnostics for a schema-invalid document")
	}
	var sawEnum bool
	for _, d := range found {
		msg, _ := d["message"].(string)
		if strings.Contains(msg, "NOPE") || strings.Contains(strings.ToLower(msg), "enumerat") {
			sawEnum = true
		}
	}
	if !sawEnum {
		t.Errorf("the enumeration violation is not reported: %v", found)
	}
}

// hoverText pulls the markdown out of the last hover reply.
func hoverText(t *testing.T, replies []map[string]any) string {
	t.Helper()
	res, ok := replies[len(replies)-1]["result"].(map[string]any)
	if !ok {
		t.Fatalf("no hover result in %v", replies[len(replies)-1])
	}
	contents, ok := res["contents"].(map[string]any)
	if !ok {
		t.Fatalf("no contents in %v", res)
	}
	value, _ := contents["value"].(string)
	return value
}

// completionLabels pulls the item labels out of the last completion reply.
func completionLabels(t *testing.T, replies []map[string]any) []string {
	t.Helper()
	res, ok := replies[len(replies)-1]["result"].(map[string]any)
	if !ok {
		t.Fatalf("no completion result in %v", replies[len(replies)-1])
	}
	raw, _ := res["items"].([]any)
	out := make([]string, 0, len(raw))
	for _, it := range raw {
		if m, ok := it.(map[string]any); ok {
			if label, _ := m["label"].(string); label != "" {
				out = append(out, label)
			}
		}
	}
	return out
}
