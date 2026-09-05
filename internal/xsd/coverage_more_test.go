// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package xsd

import (
	"encoding/xml"
	"strings"
	"testing"
)

func decoderAfterStart(t *testing.T, input string) (*xml.Decoder, xml.StartElement) {
	t.Helper()
	dec := xml.NewDecoder(strings.NewReader(input))
	tok, err := dec.Token()
	if err != nil {
		t.Fatal(err)
	}
	return dec, tok.(xml.StartElement)
}

func TestDirectParserEOFBranches(t *testing.T) {
	if err := parseSchemaBody(xml.NewDecoder(strings.NewReader("")), &Schema{}); err == nil {
		t.Fatal("empty schema body should report unexpected EOF")
	}
	dec, start := decoderAfterStart(t, `<simpleContent><annotation>`)
	if err := parseSimpleContent(dec, &ComplexType{}); err == nil {
		t.Fatal("truncated simpleContent annotation should fail")
	}
	dec, start = decoderAfterStart(t, `<extension><attribute name="A">`)
	if err := parseExtensionBody(dec, &ComplexType{}, start.Name.Local); err == nil {
		t.Fatal("truncated extension attribute should fail")
	}
	dec, start = decoderAfterStart(t, `<sequence><element name="A">`)
	if _, err := parseParticle(dec, start); err == nil {
		t.Fatal("truncated particle element should fail")
	}
	dec, start = decoderAfterStart(t, `<restriction><pattern value="x">`)
	if err := parseFacets(dec, &Facets{}); err == nil {
		t.Fatal("truncated facet should fail")
	}
}

func TestEffectiveFacetsStopsAtTypeWithoutBase(t *testing.T) {
	s := &Schema{SimpleTypes: map[string]*SimpleType{"Leaf": {Name: "Leaf"}}}
	_, base := s.EffectiveFacets("Leaf")
	if base != "Leaf" {
		t.Fatalf("base = %q, want Leaf", base)
	}
}

func TestSkipToEndHandlesNestedSameName(t *testing.T) {
	dec, start := decoderAfterStart(t, `<node><node></node></node>`)
	if err := skipToEnd(dec, start.Name.Local); err != nil {
		t.Fatalf("nested same-name element was not skipped: %v", err)
	}
}
