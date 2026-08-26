// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package iso20022_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebastienrousseau/askiso/pkg/iso20022"
)

// The schema-driven half of the public API — Diff, GenerateFromSchema and the
// external code sets — used to be exercised only when the developer happened to
// have a real catalogue installed. On a clean checkout every one of these
// methods was reached zero times, so a regression in any of them would have
// travelled all the way to a release unnoticed. These fixtures are small, but
// they are real schemas, so the paths are genuinely walked.

// schemaWith builds a valid schema whose Document has the given child elements.
func schemaWith(children string) string {
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

// realCatalogue writes actual schema documents rather than the stubs
// fixtureCatalogue uses, so the parser and generator have something to walk.
func realCatalogue(t *testing.T, schemas map[string]string) *iso20022.Catalogue {
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
	c, err := iso20022.OpenCatalogue(root)
	if err != nil {
		t.Fatalf("OpenCatalogue: %v", err)
	}
	return c
}

func TestGenerateFromSchemaBuildsEveryMandatoryElement(t *testing.T) {
	c := realCatalogue(t, map[string]string{
		"pacs.008.001.10": schemaWith(
			`      <xs:element name="One" type="Max35Text"/>
      <xs:element name="Two" type="Max35Text" minOccurs="0"/>`),
	})

	res, err := c.GenerateFromSchema("pacs.008.001.10", iso20022.DefaultSchemaGenOptions())
	if err != nil {
		t.Fatalf("GenerateFromSchema: %v", err)
	}
	if !strings.Contains(res.XML, "<One>") {
		t.Errorf("a mandatory element is missing:\n%s", res.XML)
	}
	if strings.Contains(res.XML, "<Two>") {
		t.Errorf("the default options must not emit optional elements:\n%s", res.XML)
	}
}

func TestGenerateFromSchemaRejectsAMessageThatIsNotInstalled(t *testing.T) {
	c := realCatalogue(t, map[string]string{"pacs.008.001.10": schemaWith(
		`      <xs:element name="One" type="Max35Text"/>`)})

	if _, err := c.GenerateFromSchema("pacs.999.001.01", iso20022.DefaultSchemaGenOptions()); err == nil {
		t.Error("generating an uninstalled message should be an error")
	}
}

func TestGenerateFromSchemaReportsAnUnparseableSchema(t *testing.T) {
	c := realCatalogue(t, map[string]string{"pacs.008.001.10": "{{{ not xml"})

	_, err := c.GenerateFromSchema("pacs.008.001.10", iso20022.DefaultSchemaGenOptions())
	if err == nil {
		t.Fatal("an unparseable schema should be an error")
	}
	if !strings.Contains(err.Error(), "pacs.008.001.10") {
		t.Errorf("the error should name the message: %v", err)
	}
}

// Removing a mandatory element is the case that matters: a message built
// against the old schema can still be rejected by the new one.
func TestDiffClassifiesAStructuralChange(t *testing.T) {
	c := realCatalogue(t, map[string]string{
		"pacs.008.001.09": schemaWith(
			`      <xs:element name="One" type="Max35Text"/>
      <xs:element name="Gone" type="Max35Text"/>`),
		"pacs.008.001.10": schemaWith(
			`      <xs:element name="One" type="Max35Text"/>
      <xs:element name="Added" type="Max35Text" minOccurs="0"/>`),
	})

	rep, err := c.Diff("pacs.008.001.09", "pacs.008.001.10")
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if len(rep.Changes) == 0 {
		t.Fatal("two different schemas must produce changes")
	}

	var sawRemoval bool
	for _, ch := range rep.Changes {
		if strings.Contains(ch.Path, "Gone") {
			sawRemoval = true
		}
	}
	if !sawRemoval {
		t.Errorf("the removed element is not reported: %+v", rep.Changes)
	}
}

func TestDiffRequiresBothSchemas(t *testing.T) {
	c := realCatalogue(t, map[string]string{"pacs.008.001.10": schemaWith(
		`      <xs:element name="One" type="Max35Text"/>`)})

	if _, err := c.Diff("pacs.008.001.01", "pacs.008.001.10"); err == nil {
		t.Error("a missing 'from' schema should be an error")
	}
	if _, err := c.Diff("pacs.008.001.10", "pacs.008.001.99"); err == nil {
		t.Error("a missing 'to' schema should be an error")
	}
}

// The external code sets are stored beside the catalogue, because AskISO does
// not redistribute the Registration Authority's publication.
func TestExternalCodesRoundTripThroughImport(t *testing.T) {
	c := realCatalogue(t, map[string]string{"pacs.008.001.10": schemaWith(
		`      <xs:element name="One" type="Max35Text"/>`)})

	if got := c.ExternalCodes(); len(got) != 0 {
		t.Errorf("nothing is imported yet, got %d codes", len(got))
	}
	if got := c.LookupExternalCode("SALA"); len(got) != 0 {
		t.Errorf("nothing is imported yet, got %d matches", len(got))
	}

	src := filepath.Join(t.TempDir(), "codes.json")
	body, err := json.Marshal([]map[string]string{
		{"set": "ExternalPurposeCode", "code": "SALA", "name": "Salary",
			"definition": "Salary payment"},
		{"set": "ExternalPurposeCode", "code": "SUPP", "name": "Supplier",
			"definition": "Supplier payment"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, body, 0o644); err != nil {
		t.Fatal(err)
	}

	n, err := c.ImportExternalCodes(src)
	if err != nil {
		t.Fatalf("ImportExternalCodes: %v", err)
	}
	if n != 2 {
		t.Errorf("imported %d codes, want 2", n)
	}

	if got := len(c.ExternalCodes()); got != 2 {
		t.Errorf("ExternalCodes returned %d, want 2", got)
	}
	hits := c.LookupExternalCode("SALA")
	if len(hits) != 1 {
		t.Fatalf("LookupExternalCode returned %d matches, want 1", len(hits))
	}
	if hits[0].Definition != "Salary payment" {
		t.Errorf("definition = %q", hits[0].Definition)
	}
}

func TestImportExternalCodesRefusesAnUnknownFormat(t *testing.T) {
	c := realCatalogue(t, map[string]string{"pacs.008.001.10": schemaWith(
		`      <xs:element name="One" type="Max35Text"/>`)})

	src := filepath.Join(t.TempDir(), "codes.csv")
	if err := os.WriteFile(src, []byte("set,code\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ImportExternalCodes(src); err == nil {
		t.Error("a CSV is neither the spreadsheet nor the JSON form")
	}
}

// Light mode has no catalogue to store anything beside, and must say so rather
// than panic.
func TestExternalCodesOnANilCatalogue(t *testing.T) {
	var c *iso20022.Catalogue

	if got := c.ExternalCodes(); got != nil {
		t.Errorf("ExternalCodes on nil = %v, want nil", got)
	}
	if got := c.LookupExternalCode("SALA"); got != nil {
		t.Errorf("LookupExternalCode on nil = %v, want nil", got)
	}
	if _, err := c.ImportExternalCodes("anything.json"); err == nil {
		t.Error("importing with no catalogue open should be an error")
	}
}
