// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package mcp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebastienrousseau/askiso/internal/mcp"
	"github.com/sebastienrousseau/askiso/pkg/iso20022"
)

// The catalogue-dependent tools were only exercised when the developer running
// the tests happened to have a catalogue installed. On CI there is none, so
// askiso_diff and the schema branch of askiso_generate — the two tools an
// assistant reaches for most once it knows a message exists — were never run
// at all.
//
// A fixture catalogue makes them testable anywhere. The schemas are small but
// real: they parse, they resolve, and the generator walks them.
func fixtureSchema(children string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<xs:schema xmlns="urn:t" xmlns:xs="http://www.w3.org/2001/XMLSchema"
           elementFormDefault="qualified" targetNamespace="urn:t">
  <xs:element name="Document" type="Doc"/>
  <xs:complexType name="Doc">
    <xs:sequence>
` + children + `
    </xs:sequence>
  </xs:complexType>
  <xs:simpleType name="Max35Text">
    <xs:restriction base="xs:string">
      <xs:minLength value="1"/>
      <xs:maxLength value="35"/>
    </xs:restriction>
  </xs:simpleType>
</xs:schema>`
}

func fixtureCatalogue(t *testing.T, schemas map[string]string) mcp.CatalogueFunc {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "Payments Clearing and Settlement", "Version 11.0", "Schemas")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for id, body := range schemas {
		if err := os.WriteFile(filepath.Join(dir, id+".xsd"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cat, err := iso20022.OpenCatalogue(root)
	if err != nil {
		t.Fatalf("OpenCatalogue: %v", err)
	}
	return func() (*iso20022.Catalogue, error) { return cat, nil }
}

func TestDiffToolReportsStructuralChanges(t *testing.T) {
	open := fixtureCatalogue(t, map[string]string{
		"pacs.008.001.09": fixtureSchema(
			`      <xs:element name="One" type="Max35Text"/>
      <xs:element name="Gone" type="Max35Text"/>`),
		"pacs.008.001.10": fixtureSchema(
			`      <xs:element name="One" type="Max35Text"/>`),
	})

	replies := sessionWith(t, open, initialize,
		call(2, "askiso_diff", map[string]any{
			"from": "pacs.008.001.09", "to": "pacs.008.001.10"}))

	res := result(t, replies[1])
	for _, key := range []string{"from", "to", "breaking_changes", "compatible_changes", "changes"} {
		if _, ok := res[key]; !ok {
			t.Errorf("the diff report has no %q: %v", key, res)
		}
	}
	// Removing a mandatory element is the case that matters: a message built
	// against the old schema can still be rejected by the new one.
	changes, _ := res["changes"].([]any)
	if len(changes) == 0 {
		t.Fatalf("two different schemas produced no changes: %v", res)
	}
	var sawRemoval bool
	for _, c := range changes {
		if m, ok := c.(map[string]any); ok {
			if path, _ := m["path"].(string); strings.Contains(path, "Gone") {
				sawRemoval = true
			}
		}
	}
	if !sawRemoval {
		t.Errorf("the removed element is not reported: %v", changes)
	}
}

// breaking_only is the flag an assistant uses when it wants the answer to
// "will this break", and it returns a different shape.
func TestDiffToolBreakingOnly(t *testing.T) {
	open := fixtureCatalogue(t, map[string]string{
		"pacs.008.001.09": fixtureSchema(
			`      <xs:element name="One" type="Max35Text"/>
      <xs:element name="Gone" type="Max35Text"/>`),
		"pacs.008.001.10": fixtureSchema(
			`      <xs:element name="One" type="Max35Text"/>`),
	})

	replies := sessionWith(t, open, initialize,
		call(2, "askiso_diff", map[string]any{
			"from": "pacs.008.001.09", "to": "pacs.008.001.10", "breaking_only": true}))

	res := result(t, replies[1])
	if _, ok := res["breaking_changes"]; !ok {
		t.Errorf("breaking_only did not return a report: %v", res)
	}
	if _, ok := res["changes"]; !ok {
		t.Errorf("breaking_only returned no changes list: %v", res)
	}
}

// Only four message types have a hand-written template. Everything else is
// built by walking its schema, which is the path that makes the other 2,841
// reachable at all.
func TestGenerateToolWalksTheSchema(t *testing.T) {
	open := fixtureCatalogue(t, map[string]string{
		"pacs.009.001.08": fixtureSchema(
			`      <xs:element name="One" type="Max35Text"/>
      <xs:element name="Two" type="Max35Text" minOccurs="0"/>`),
	})

	replies := sessionWith(t, open, initialize,
		call(2, "askiso_generate", map[string]any{"message_type": "pacs.009.001.08"}))

	res := result(t, replies[1])
	if got, _ := res["source"].(string); got != "schema" {
		t.Errorf("source = %q, want \"schema\"", got)
	}
	xml, _ := res["xml"].(string)
	if !strings.Contains(xml, "<One>") {
		t.Errorf("a mandatory element is missing:\n%s", xml)
	}
}

func TestGenerateToolPassesValuesThrough(t *testing.T) {
	open := fixtureCatalogue(t, map[string]string{
		"pacs.009.001.08": fixtureSchema(
			`      <xs:element name="One" type="Max35Text"/>
      <xs:element name="Two" type="Max35Text" minOccurs="0"/>`),
	})

	replies := sessionWith(t, open, initialize,
		call(2, "askiso_generate", map[string]any{
			"message_type": "pacs.009.001.08",
			"optional":     true,
			"amount":       "1234.56",
			"currency":     "EUR",
		}))

	// With optional elements requested, the minOccurs="0" element appears.
	xml, _ := result(t, replies[1])["xml"].(string)
	if !strings.Contains(xml, "<Two>") {
		t.Errorf("optional did not include the optional element:\n%s", xml)
	}
}
