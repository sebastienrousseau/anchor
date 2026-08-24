// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package lsp

import (
	"strings"
	"testing"

	"github.com/sebastienrousseau/anchor/internal/xsd"
)

// The schema helpers are the part of the server a client never exercises
// directly, and the part most likely to be wrong: a content model can nest
// choices inside sequences, and a facet can be inherited through a chain of
// restrictions. These test them against real schema shapes.

func parseSchema(t *testing.T, body string) *xsd.Schema {
	t.Helper()
	doc := `<?xml version="1.0" encoding="UTF-8"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"
           xmlns="urn:test" targetNamespace="urn:test" elementFormDefault="qualified">
` + body + `
</xs:schema>`
	s, err := xsd.Parse(strings.NewReader(doc))
	if err != nil {
		t.Fatalf("parsing fixture: %v", err)
	}
	return s
}

const contentModel = `
  <xs:element name="Document" type="Document"/>
  <xs:complexType name="Document">
    <xs:sequence>
      <xs:element name="GrpHdr" type="GroupHeader"/>
      <xs:element maxOccurs="unbounded" name="Tx" type="Transaction"/>
    </xs:sequence>
  </xs:complexType>
  <xs:complexType name="GroupHeader">
    <xs:sequence>
      <xs:element name="MsgId" type="Max35Text"/>
      <xs:element minOccurs="0" name="Ccy" type="CurrencyCode"/>
    </xs:sequence>
  </xs:complexType>
  <xs:complexType name="Transaction">
    <xs:sequence>
      <xs:element name="Amt" type="Amount"/>
      <xs:choice>
        <xs:element name="IBAN" type="Max35Text"/>
        <xs:element name="Othr" type="Max35Text"/>
      </xs:choice>
      <xs:element minOccurs="0" name="ChrgBr" type="ChargeBearerCode"/>
    </xs:sequence>
  </xs:complexType>
  <xs:simpleType name="Max35Text">
    <xs:restriction base="xs:string">
      <xs:minLength value="1"/><xs:maxLength value="35"/>
    </xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="CurrencyCode">
    <xs:restriction base="xs:string"><xs:pattern value="[A-Z]{3,3}"/></xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="Amount">
    <xs:restriction base="xs:decimal">
      <xs:totalDigits value="18"/><xs:fractionDigits value="5"/>
    </xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="ChargeBearerCode">
    <xs:restriction base="xs:string">
      <xs:enumeration value="DEBT"/><xs:enumeration value="CRED"/><xs:enumeration value="SHAR"/>
    </xs:restriction>
  </xs:simpleType>`

func TestDeclarationFor(t *testing.T) {
	schema := parseSchema(t, contentModel)

	cases := map[string]string{
		"/Document":              "Document",
		"/Document/GrpHdr":       "GroupHeader",
		"/Document/GrpHdr/MsgId": "Max35Text",
		"/Document/Tx/IBAN":      "Max35Text",
	}
	for path, wantType := range cases {
		decl, ok := declarationFor(schema, path)
		if !ok {
			t.Errorf("%s did not resolve", path)
			continue
		}
		if decl.Type != wantType {
			t.Errorf("%s has type %q, want %q", path, decl.Type, wantType)
		}
	}

	for _, path := range []string{
		"",                           // nothing to resolve
		"/Wrong",                     // not the document root
		"/Document/NoSuchChild",      // not declared here
		"/Document/GrpHdr/MsgId/Sub", // a value has no children
	} {
		if _, ok := declarationFor(schema, path); ok {
			t.Errorf("%q resolved but should not have", path)
		}
	}
	if _, ok := declarationFor(nil, "/Document"); ok {
		t.Error("a nil schema resolved a path")
	}
}

func TestChildElementsFlattensChoices(t *testing.T) {
	schema := parseSchema(t, contentModel)
	ct, ok := schema.ResolveComplex("Transaction")
	if !ok {
		t.Fatal("Transaction is missing")
	}

	var names []string
	for _, el := range childElements(ct.Content) {
		names = append(names, el.Name)
	}
	// A choice contributes every branch, in declaration order, because any of
	// them may legitimately be the next element.
	want := "Amt,IBAN,Othr,ChrgBr"
	if strings.Join(names, ",") != want {
		t.Errorf("children = %v, want %s", names, want)
	}

	if got := childElements(&xsd.Any{}); got != nil {
		t.Errorf("a wildcard contributed %v", got)
	}
	if _, ok := findChild(ct.Content, "NoSuchElement"); ok {
		t.Error("findChild matched an element that is not there")
	}
}

func TestChildNamesTruncates(t *testing.T) {
	schema := parseSchema(t, contentModel)
	if got := childNames(schema, "GroupHeader"); strings.Join(got, ",") != "MsgId,Ccy" {
		t.Errorf("childNames = %v", got)
	}
	// A value type has no children.
	if got := childNames(schema, "Max35Text"); got != nil {
		t.Errorf("childNames of a simple type = %v", got)
	}

	// A long content model is summarised rather than dumped.
	var body strings.Builder
	body.WriteString(`
  <xs:element name="Document" type="Wide"/>
  <xs:complexType name="Wide"><xs:sequence>`)
	for i := 0; i < 30; i++ {
		body.WriteString("<xs:element name=\"E" + string(rune('a'+i%26)) + string(rune('0'+i/26)) + "\" type=\"xs:string\"/>")
	}
	body.WriteString(`</xs:sequence></xs:complexType>`)

	wide := parseSchema(t, body.String())
	got := childNames(wide, "Wide")
	if len(got) != 17 {
		t.Fatalf("got %d entries, want 16 names plus a summary", len(got))
	}
	if !strings.Contains(got[16], "more") {
		t.Errorf("the last entry is %q, want a count of what was omitted", got[16])
	}
}

func TestOccursOf(t *testing.T) {
	schema := parseSchema(t, contentModel)

	mandatory, _ := declarationFor(schema, "/Document/GrpHdr")
	if got := occursOf(mandatory); got != "1..1 (mandatory)" {
		t.Errorf("occursOf = %q", got)
	}
	optional, _ := declarationFor(schema, "/Document/GrpHdr/Ccy")
	if got := occursOf(optional); got != "0..1 (optional)" {
		t.Errorf("occursOf = %q", got)
	}
	repeating, _ := declarationFor(schema, "/Document/Tx")
	if got := occursOf(repeating); got != "1..unbounded (mandatory)" {
		t.Errorf("occursOf = %q", got)
	}
}

func TestDescribeFacets(t *testing.T) {
	schema := parseSchema(t, contentModel)

	length, base := schema.EffectiveFacets("Max35Text")
	got := describeFacets(length, base)
	if !strings.Contains(got, "1 to 35 characters") {
		t.Errorf("describeFacets = %q", got)
	}
	// The base type is only worth naming when it is not the default.
	if strings.Contains(got, "base `string`") {
		t.Errorf("the default base type was named: %q", got)
	}

	amount, base := schema.EffectiveFacets("Amount")
	got = describeFacets(amount, base)
	for _, want := range []string{"18 digits", "5 decimal place", "base `decimal`"} {
		if !strings.Contains(got, want) {
			t.Errorf("describeFacets is missing %q: %q", want, got)
		}
	}

	pattern, base := schema.EffectiveFacets("CurrencyCode")
	if got := describeFacets(pattern, base); !strings.Contains(got, "pattern") {
		t.Errorf("describeFacets = %q", got)
	}

	codes, base := schema.EffectiveFacets("ChargeBearerCode")
	if got := describeFacets(codes, base); !strings.Contains(got, "one of `DEBT`, `CRED`, `SHAR`") {
		t.Errorf("describeFacets = %q", got)
	}

	// A type with nothing to say produces nothing at all.
	if got := describeFacets(xsd.Facets{}, ""); got != "" {
		t.Errorf("describeFacets of an unconstrained type = %q", got)
	}
}

func TestDescribeFacetsBoundVariants(t *testing.T) {
	four := 4
	cases := []struct {
		name  string
		facet xsd.Facets
		want  string
	}{
		{"exact length", xsd.Facets{Length: &four}, "exactly 4 character"},
		{"only a maximum", xsd.Facets{MaxLength: &four}, "at most 4 characters"},
		{"only a minimum", xsd.Facets{MinLength: &four}, "at least 4 characters"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := describeFacets(tc.facet, ""); !strings.Contains(got, tc.want) {
				t.Errorf("describeFacets = %q, want it to mention %q", got, tc.want)
			}
		})
	}

	// A long code set is summarised: a hover listing three hundred codes helps
	// nobody.
	var many []string
	for i := 0; i < 40; i++ {
		many = append(many, "CODE"+string(rune('A'+i%26)))
	}
	got := describeFacets(xsd.Facets{Enumeration: many}, "")
	if !strings.Contains(got, "and 28 more") {
		t.Errorf("a long code set was not summarised: %q", got)
	}
}

func TestChildCompletions(t *testing.T) {
	schema := parseSchema(t, contentModel)

	items := childCompletions(schema, "/Document/GrpHdr")
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2", len(items))
	}
	if items[0]["label"] != "MsgId" || items[1]["label"] != "Ccy" {
		t.Errorf("the schema order was not preserved: %v", items)
	}
	// A container child is offered as a property, a value child as a field, so
	// an editor can show them differently.
	doc := childCompletions(schema, "/Document")
	if doc[0]["kind"] != kindProperty {
		t.Errorf("a structured child has kind %v", doc[0]["kind"])
	}
	if items[0]["kind"] != kindField {
		t.Errorf("a value child has kind %v", items[0]["kind"])
	}

	// A path that does not resolve, and a value that has no children, both
	// yield an empty list rather than a nil the client cannot read.
	for _, path := range []string{"/Nope", "/Document/GrpHdr/MsgId"} {
		if got := childCompletions(schema, path); got == nil || len(got) != 0 {
			t.Errorf("childCompletions(%q) = %v", path, got)
		}
	}
}

func TestValueCompletions(t *testing.T) {
	schema := parseSchema(t, contentModel)

	items := valueCompletions(schema, "/Document/Tx/ChrgBr")
	if len(items) != 3 || items[0]["label"] != "DEBT" {
		t.Errorf("valueCompletions = %v", items)
	}
	// An element with no code set has nothing to offer, which is different from
	// offering an empty list.
	if got := valueCompletions(schema, "/Document/GrpHdr/MsgId"); got != nil {
		t.Errorf("valueCompletions for a free-text element = %v", got)
	}
	if got := valueCompletions(schema, "/Nope"); got != nil {
		t.Errorf("valueCompletions for an unknown path = %v", got)
	}
}

func TestParentOf(t *testing.T) {
	cases := map[string]string{
		"/Document/GrpHdr/MsgId": "/Document/GrpHdr",
		"/Document":              "",
		"":                       "",
		"Document":               "",
	}
	for path, want := range cases {
		if got := parentOf(path); got != want {
			t.Errorf("parentOf(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestTrimNamespace(t *testing.T) {
	if got := trimNamespace("ns:IBAN"); got != "IBAN" {
		t.Errorf("trimNamespace = %q", got)
	}
	if got := trimNamespace("IBAN"); got != "IBAN" {
		t.Errorf("trimNamespace = %q", got)
	}
}

func TestDescribeElementWithoutADeclaration(t *testing.T) {
	schema := parseSchema(t, contentModel)
	s := New(strings.NewReader(""), &nopWriter{}, &nopWriter{})

	el := Element{Name: "Nope", Path: "/Document/Nope"}
	got := s.describeElement(el, schema, "pacs.008.001.10", true)
	if !strings.Contains(got, "not declared") {
		t.Errorf("describeElement = %q", got)
	}
}

func TestDescribeElementWithoutANamespace(t *testing.T) {
	s := New(strings.NewReader(""), &nopWriter{}, &nopWriter{})
	el := Element{Name: "Nm", Path: "/Document/Nm", Value: "ACME",
		ValueStart: 0, ValueEnd: 4}

	got := s.describeElement(el, nil, "", false)
	if !strings.Contains(got, "does not name an ISO 20022 message") {
		t.Errorf("describeElement = %q", got)
	}
	if !strings.Contains(got, "ACME") {
		t.Errorf("the value was not shown: %q", got)
	}
}

type nopWriter struct{}

func (*nopWriter) Write(p []byte) (int, error) { return len(p), nil }
