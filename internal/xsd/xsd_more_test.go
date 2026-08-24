// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package xsd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func parse(t *testing.T, doc string) *Schema {
	t.Helper()
	s, err := Parse(strings.NewReader(doc))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return s
}

const full = `<?xml version="1.0" encoding="UTF-8"?>
<xs:schema xmlns="urn:t" xmlns:xs="http://www.w3.org/2001/XMLSchema"
           elementFormDefault="qualified" targetNamespace="urn:t">
  <xs:annotation><xs:documentation>ignored</xs:documentation></xs:annotation>
  <xs:element name="Document" type="Doc"/>
  <xs:element name="AppHdr" type="Doc"/>

  <xs:complexType name="Doc">
    <xs:annotation><xs:documentation>also ignored</xs:documentation></xs:annotation>
    <xs:sequence>
      <xs:element name="One" type="Max35Text"/>
      <xs:element name="Many" type="Max35Text" minOccurs="0" maxOccurs="unbounded"/>
      <xs:choice>
        <xs:element name="A" type="Max35Text"/>
        <xs:element name="B" type="Max35Text"/>
      </xs:choice>
      <xs:any namespace="##any" processContents="lax"/>
    </xs:sequence>
  </xs:complexType>

  <xs:complexType name="Amount">
    <xs:simpleContent>
      <xs:extension base="AmtType">
        <xs:attribute name="Ccy" type="Max35Text" use="required"/>
        <xs:attribute name="Opt" type="Max35Text"/>
      </xs:extension>
    </xs:simpleContent>
  </xs:complexType>

  <xs:complexType name="Empty"/>

  <xs:simpleType name="AmtType">
    <xs:restriction base="xs:decimal">
      <xs:fractionDigits value="5"/><xs:totalDigits value="18"/>
      <xs:minInclusive value="0"/><xs:maxInclusive value="999"/>
    </xs:restriction>
  </xs:simpleType>

  <xs:simpleType name="Max35Text">
    <xs:annotation><xs:documentation>doc</xs:documentation></xs:annotation>
    <xs:restriction base="BaseText">
      <xs:maxLength value="35"/>
    </xs:restriction>
  </xs:simpleType>

  <xs:simpleType name="BaseText">
    <xs:restriction base="xs:string">
      <xs:minLength value="1"/><xs:length value="10"/>
      <xs:pattern value="[A-Z]+"/>
      <xs:enumeration value="AAA"/>
    </xs:restriction>
  </xs:simpleType>
</xs:schema>`

func TestParseFullVocabulary(t *testing.T) {
	s := parse(t, full)

	if s.TargetNamespace != "urn:t" {
		t.Errorf("targetNamespace = %q", s.TargetNamespace)
	}
	if len(s.Elements) != 2 {
		t.Errorf("got %d global elements, want 2", len(s.Elements))
	}
	if len(s.ElementOrder) != 2 || s.ElementOrder[0] != "Document" {
		t.Errorf("declaration order not preserved: %v", s.ElementOrder)
	}

	doc, ok := s.ResolveComplex("Doc")
	if !ok {
		t.Fatal("Doc did not resolve")
	}
	seq, ok := doc.Content.(*Sequence)
	if !ok {
		t.Fatalf("content is %T, want *Sequence", doc.Content)
	}
	if len(seq.Particles) != 4 {
		t.Fatalf("got %d particles, want 4", len(seq.Particles))
	}

	one := seq.Particles[0].(*Element)
	if one.MinOccurs != 1 || one.MaxOccurs != 1 {
		t.Errorf("One occurs %d..%d", one.MinOccurs, one.MaxOccurs)
	}
	many := seq.Particles[1].(*Element)
	if many.MinOccurs != 0 || many.MaxOccurs != Unbounded {
		t.Errorf("Many occurs %d..%d", many.MinOccurs, many.MaxOccurs)
	}
	if _, ok := seq.Particles[2].(*Choice); !ok {
		t.Errorf("particle 3 is %T, want *Choice", seq.Particles[2])
	}
	any, ok := seq.Particles[3].(*Any)
	if !ok {
		t.Fatalf("particle 4 is %T, want *Any", seq.Particles[3])
	}
	if any.Namespace != "##any" || any.ProcessContents != "lax" {
		t.Errorf("wildcard attributes lost: %+v", any)
	}

	amount, _ := s.ResolveComplex("Amount")
	if amount.SimpleBase != "AmtType" {
		t.Errorf("simpleContent base = %q", amount.SimpleBase)
	}
	if len(amount.Attributes) != 2 {
		t.Fatalf("got %d attributes, want 2", len(amount.Attributes))
	}
	if !amount.Attributes[0].Required || amount.Attributes[1].Required {
		t.Errorf("attribute use not read: %+v", amount.Attributes)
	}

	if empty, ok := s.ResolveComplex("Empty"); !ok || empty.Content != nil {
		t.Error("an empty complexType should parse with no content")
	}
}

// Facets accumulate down a restriction chain, tightest first.
func TestEffectiveFacets(t *testing.T) {
	s := parse(t, full)

	f, base := s.EffectiveFacets("Max35Text")
	if base != "string" {
		t.Errorf("base = %q, want string", base)
	}
	if f.MaxLength == nil || *f.MaxLength != 35 {
		t.Errorf("maxLength not inherited: %+v", f.MaxLength)
	}
	if f.MinLength == nil || *f.MinLength != 1 {
		t.Errorf("minLength from the parent not merged: %+v", f.MinLength)
	}
	if f.Length == nil || *f.Length != 10 {
		t.Error("length facet not merged")
	}
	if len(f.Pattern) == 0 || len(f.Enumeration) == 0 {
		t.Error("pattern and enumeration should accumulate")
	}

	f, base = s.EffectiveFacets("AmtType")
	if base != "decimal" {
		t.Errorf("base = %q, want decimal", base)
	}
	for name, p := range map[string]*int{"fractionDigits": f.FractionDigits, "totalDigits": f.TotalDigits} {
		if p == nil {
			t.Errorf("%s missing", name)
		}
	}
	if f.MinInclusive == nil || f.MaxInclusive == nil {
		t.Error("inclusive bounds missing")
	}

	// A builtin, and an unknown name, both terminate the walk.
	if _, b := s.EffectiveFacets("string"); b != "string" {
		t.Errorf("builtin should return itself, got %q", b)
	}
	if _, b := s.EffectiveFacets("NoSuchType"); b != "NoSuchType" {
		t.Errorf("unknown type should return itself, got %q", b)
	}
}

// A restriction chain that loops must terminate rather than hang.
func TestEffectiveFacetsStopsOnCycle(t *testing.T) {
	s := parse(t, `<?xml version="1.0"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:t" xmlns="urn:t">
  <xs:element name="Document" type="A"/>
  <xs:simpleType name="A"><xs:restriction base="B"/></xs:simpleType>
  <xs:simpleType name="B"><xs:restriction base="A"/></xs:simpleType>
</xs:schema>`)

	done := make(chan struct{})
	go func() {
		s.EffectiveFacets("A")
		close(done)
	}()
	select {
	case <-done:
	case <-timeoutAfterSeconds(5):
		t.Fatal("EffectiveFacets did not terminate on a cyclic chain")
	}
}

func TestRootElementPreference(t *testing.T) {
	s := parse(t, full)
	root, ok := s.RootElement()
	if !ok || root.Name != "Document" {
		t.Errorf("root = %+v, want Document", root)
	}

	// AppHdr alone.
	s2 := parse(t, `<?xml version="1.0"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:t" xmlns="urn:t">
  <xs:element name="AppHdr" type="xs:string"/>
</xs:schema>`)
	if r, ok := s2.RootElement(); !ok || r.Name != "AppHdr" {
		t.Errorf("AppHdr should be the root, got %+v", r)
	}

	// A name outside the preferred list falls back to declaration order.
	s3 := parse(t, `<?xml version="1.0"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:t" xmlns="urn:t">
  <xs:element name="Custom" type="xs:string"/>
</xs:schema>`)
	if r, ok := s3.RootElement(); !ok || r.Name != "Custom" {
		t.Errorf("declaration order should decide, got %+v", r)
	}

	// No elements at all.
	empty := &Schema{Elements: map[string]*Element{}}
	if _, ok := empty.RootElement(); ok {
		t.Error("a schema with no elements has no root")
	}
}

func TestParseRejectsBadInput(t *testing.T) {
	for name, doc := range map[string]string{
		"not xml":         `{{{`,
		"not a schema":    `<?xml version="1.0"?><html/>`,
		"truncated":       `<?xml version="1.0"?><xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">`,
		"unnamed type":    `<?xml version="1.0"?><xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:t"><xs:complexType/></xs:schema>`,
		"unnamed simple":  `<?xml version="1.0"?><xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:t"><xs:simpleType/></xs:schema>`,
		"unnamed element": `<?xml version="1.0"?><xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:t"><xs:element type="xs:string"/></xs:schema>`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(strings.NewReader(doc)); err == nil {
				t.Errorf("Parse should reject %s", name)
			}
		})
	}
}

// Unknown constructs are skipped rather than fatal, so an unexpected schema
// still yields a usable model.
func TestUnknownConstructsAreSkipped(t *testing.T) {
	s := parse(t, `<?xml version="1.0"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:t" xmlns="urn:t">
  <xs:notation name="n" public="p"/>
  <xs:element name="Document" type="Doc"/>
  <xs:complexType name="Doc">
    <xs:sequence>
      <xs:element name="A" type="xs:string"/>
      <xs:unknownThing/>
    </xs:sequence>
  </xs:complexType>
</xs:schema>`)
	if _, ok := s.Elements["Document"]; !ok {
		t.Error("parsing should continue past an unknown construct")
	}
}

func TestParseFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "s.xsd")
	if err := os.WriteFile(p, []byte(full), 0o644); err != nil {
		t.Fatal(err)
	}

	s, err := ParseFile(p)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if s.Path != p {
		t.Errorf("Path = %q, want %q", s.Path, p)
	}

	if _, err := ParseFile(filepath.Join(dir, "missing.xsd")); err == nil {
		t.Error("a missing file should be an error")
	}

	bad := filepath.Join(dir, "bad.xsd")
	if err := os.WriteFile(bad, []byte("{{{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseFile(bad); err == nil {
		t.Error("an unparseable file should be an error")
	}
}

func TestIsBuiltin(t *testing.T) {
	for _, n := range []string{"string", "decimal", "dateTime", "boolean", "gYear", "unsignedByte"} {
		if !IsBuiltin(n) {
			t.Errorf("%s should be builtin", n)
		}
	}
	for _, n := range []string{"Max35Text", "", "Document"} {
		if IsBuiltin(n) {
			t.Errorf("%s should not be builtin", n)
		}
	}
}

func TestResolveMissingTypes(t *testing.T) {
	s := parse(t, full)
	if _, ok := s.ResolveComplex("NoSuchType"); ok {
		t.Error("an unknown complex type should not resolve")
	}
	if _, ok := s.ResolveSimple("NoSuchType"); ok {
		t.Error("an unknown simple type should not resolve")
	}
}

func TestParticleMarkers(t *testing.T) {
	// The marker methods exist to close the Particle interface.
	var ps = []Particle{&Element{}, &Sequence{}, &Choice{}, &Any{}}
	for _, p := range ps {
		p.particle()
	}
}

func timeoutAfterSeconds(n int) <-chan time.Time {
	return time.After(time.Duration(n) * time.Second)
}

// Truncating the document inside each construct exercises the error paths that
// a malformed schema would take.
func TestTruncatedSchemasReportErrors(t *testing.T) {
	head := `<?xml version="1.0"?><xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:t" xmlns="urn:t">`

	cases := map[string]string{
		"inside schema":        head,
		"inside element":       head + `<xs:element name="Document" type="Doc">`,
		"inside complexType":   head + `<xs:complexType name="Doc">`,
		"inside sequence":      head + `<xs:complexType name="Doc"><xs:sequence>`,
		"inside choice":        head + `<xs:complexType name="Doc"><xs:choice>`,
		"inside simpleType":    head + `<xs:simpleType name="T">`,
		"inside restriction":   head + `<xs:simpleType name="T"><xs:restriction base="xs:string">`,
		"inside simpleContent": head + `<xs:complexType name="A"><xs:simpleContent>`,
		"inside extension":     head + `<xs:complexType name="A"><xs:simpleContent><xs:extension base="xs:string">`,
		"inside attribute":     head + `<xs:complexType name="A"><xs:attribute name="x" type="xs:string">`,
		"inside any":           head + `<xs:complexType name="A"><xs:sequence><xs:any>`,
		"inside annotation":    head + `<xs:annotation>`,
		"inside foreign":       head + `<other:thing xmlns:other="urn:other">`,
	}

	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(strings.NewReader(doc)); err == nil {
				t.Errorf("a schema truncated %s should be an error", name)
			}
		})
	}
}

// Foreign-namespace elements at the top level are skipped, not fatal.
func TestForeignNamespaceIsSkipped(t *testing.T) {
	s := parse(t, `<?xml version="1.0"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" xmlns:o="urn:other"
           targetNamespace="urn:t" xmlns="urn:t">
  <o:vendorExtension><o:nested>ignored</o:nested></o:vendorExtension>
  <xs:element name="Document" type="xs:string"/>
</xs:schema>`)
	if _, ok := s.Elements["Document"]; !ok {
		t.Error("a foreign-namespace element should be skipped, not fatal")
	}
}

// An element carrying an inline anonymous type still yields its name and type.
func TestElementWithInlineBody(t *testing.T) {
	s := parse(t, `<?xml version="1.0"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:t" xmlns="urn:t">
  <xs:element name="Document" type="Doc">
    <xs:annotation><xs:documentation>docs</xs:documentation></xs:annotation>
  </xs:element>
  <xs:complexType name="Doc"><xs:sequence/></xs:complexType>
</xs:schema>`)
	if el, ok := s.Elements["Document"]; !ok || el.Type != "Doc" {
		t.Errorf("element = %+v", el)
	}
}

// A complexType body containing an annotation, an attribute, and an unknown
// construct alongside the content model.
func TestComplexTypeMixedBody(t *testing.T) {
	s := parse(t, `<?xml version="1.0"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:t" xmlns="urn:t">
  <xs:element name="Document" type="Doc"/>
  <xs:complexType name="Doc">
    <xs:annotation><xs:documentation>d</xs:documentation></xs:annotation>
    <xs:sequence><xs:element name="A" type="xs:string"/></xs:sequence>
    <xs:attribute name="Attr" type="xs:string" use="required"/>
    <xs:somethingUnknown/>
  </xs:complexType>
</xs:schema>`)
	ct, ok := s.ResolveComplex("Doc")
	if !ok {
		t.Fatal("Doc did not resolve")
	}
	if len(ct.Attributes) != 1 || !ct.Attributes[0].Required {
		t.Errorf("attributes = %+v", ct.Attributes)
	}
	if ct.Content == nil {
		t.Error("content model was lost")
	}
}

// simpleContent with a restriction rather than an extension, plus annotations
// and unknown children.
func TestSimpleContentRestrictionAndNoise(t *testing.T) {
	s := parse(t, `<?xml version="1.0"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:t" xmlns="urn:t">
  <xs:element name="Document" type="A"/>
  <xs:complexType name="A">
    <xs:annotation><xs:documentation>d</xs:documentation></xs:annotation>
    <xs:simpleContent>
      <xs:annotation><xs:documentation>d</xs:documentation></xs:annotation>
      <xs:restriction base="xs:string">
        <xs:attribute name="Ccy" type="xs:string" use="required"/>
        <xs:unknownChild/>
      </xs:restriction>
    </xs:simpleContent>
  </xs:complexType>
</xs:schema>`)
	ct, _ := s.ResolveComplex("A")
	if ct.SimpleBase != "string" {
		t.Errorf("SimpleBase = %q", ct.SimpleBase)
	}
	if len(ct.Attributes) != 1 {
		t.Errorf("attributes = %+v", ct.Attributes)
	}
}

func TestSimpleTypeWithNoiseAndSelfClosedRestriction(t *testing.T) {
	s := parse(t, `<?xml version="1.0"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:t" xmlns="urn:t">
  <xs:element name="Document" type="A"/>
  <xs:simpleType name="A">
    <xs:annotation><xs:documentation>d</xs:documentation></xs:annotation>
    <xs:restriction base="xs:string"/>
  </xs:simpleType>
  <xs:simpleType name="B">
    <xs:unknownChild/>
    <xs:restriction base="xs:string"><xs:maxLength value="3"/></xs:restriction>
  </xs:simpleType>
</xs:schema>`)
	if st, ok := s.ResolveSimple("A"); !ok || st.Base != "string" {
		t.Errorf("A = %+v", st)
	}
	if st, ok := s.ResolveSimple("B"); !ok || st.Facets.MaxLength == nil {
		t.Errorf("B = %+v", st)
	}
}

// Sequences and choices carrying annotations, unknown children, and nesting.
func TestParticleNoiseAndNesting(t *testing.T) {
	s := parse(t, `<?xml version="1.0"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:t" xmlns="urn:t">
  <xs:element name="Document" type="Doc"/>
  <xs:complexType name="Doc">
    <xs:sequence>
      <xs:annotation><xs:documentation>d</xs:documentation></xs:annotation>
      <xs:unknownParticle/>
      <xs:element name="A" type="xs:string"/>
      <xs:sequence minOccurs="0" maxOccurs="3">
        <xs:element name="B" type="xs:string"/>
      </xs:sequence>
      <xs:any namespace="##other" processContents="skip" minOccurs="0" maxOccurs="unbounded"/>
    </xs:sequence>
  </xs:complexType>
</xs:schema>`)
	ct, _ := s.ResolveComplex("Doc")
	seq := ct.Content.(*Sequence)
	if len(seq.Particles) != 3 {
		t.Fatalf("got %d particles, want 3: %#v", len(seq.Particles), seq.Particles)
	}
	inner, ok := seq.Particles[1].(*Sequence)
	if !ok {
		t.Fatalf("particle 2 is %T, want a nested *Sequence", seq.Particles[1])
	}
	if inner.MinOccurs != 0 || inner.MaxOccurs != 3 {
		t.Errorf("nested sequence occurs %d..%d", inner.MinOccurs, inner.MaxOccurs)
	}
	any := seq.Particles[2].(*Any)
	if any.MaxOccurs != Unbounded {
		t.Errorf("wildcard maxOccurs = %d", any.MaxOccurs)
	}
}

func TestOccursParsing(t *testing.T) {
	cases := []struct {
		in   string
		def  int
		want int
	}{
		{"", 1, 1},
		{"unbounded", 1, Unbounded},
		{"0", 1, 0},
		{"7", 1, 7},
		{"-3", 1, 1},  // negative falls back to the default
		{"abc", 5, 5}, // unparseable falls back
	}
	for _, tc := range cases {
		if got := occurs(tc.in, tc.def); got != tc.want {
			t.Errorf("occurs(%q, %d) = %d, want %d", tc.in, tc.def, got, tc.want)
		}
	}
}

func TestLocalNameAndIntPtr(t *testing.T) {
	if localName("xs:string") != "string" || localName("Max35Text") != "Max35Text" {
		t.Error("localName failed")
	}
	if intPtr("abc") != nil {
		t.Error("intPtr should reject non-numeric input")
	}
	if p := intPtr("42"); p == nil || *p != 42 {
		t.Error("intPtr failed on a number")
	}
}

func TestAttrLookup(t *testing.T) {
	s := parse(t, `<?xml version="1.0"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:t" xmlns="urn:t">
  <xs:element name="Document" type="xs:string"/>
</xs:schema>`)
	if s.TargetNamespace != "urn:t" {
		t.Errorf("attr lookup failed: %q", s.TargetNamespace)
	}
}

// Malformed markup inside a construct that is skipped wholesale makes the skip
// itself fail, which must be reported rather than silently swallowed.
func TestSkipFailuresAreReported(t *testing.T) {
	head := `<?xml version="1.0"?><xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:t" xmlns="urn:t">`
	cases := map[string]string{
		"top-level annotation":   head + `<xs:annotation><xs:documentation>`,
		"top-level unknown":      head + `<xs:unknown><inner>`,
		"complexType annotation": head + `<xs:complexType name="A"><xs:annotation><xs:doc>`,
		"complexType unknown":    head + `<xs:complexType name="A"><xs:unknown><inner>`,
		"particle annotation":    head + `<xs:complexType name="A"><xs:sequence><xs:annotation><xs:doc>`,
		"particle unknown":       head + `<xs:complexType name="A"><xs:sequence><xs:unknown><inner>`,
		"simpleType annotation":  head + `<xs:simpleType name="A"><xs:annotation><xs:doc>`,
		"simpleType unknown":     head + `<xs:simpleType name="A"><xs:unknown><inner>`,
		"simpleContent unknown":  head + `<xs:complexType name="A"><xs:simpleContent><xs:unknown><inner>`,
		"extension unknown":      head + `<xs:complexType name="A"><xs:simpleContent><xs:extension base="xs:string"><xs:unknown><inner>`,
	}
	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(strings.NewReader(doc)); err == nil {
				t.Errorf("a truncated %s should be an error", name)
			}
		})
	}
}

// Content before the schema element is ignored; a stream that never contains
// one is rejected.
func TestPreambleIsSkipped(t *testing.T) {
	s, err := Parse(strings.NewReader(`<?xml version="1.0"?>
<!-- a comment -->
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:t" xmlns="urn:t">
  <xs:element name="Document" type="xs:string"/>
</xs:schema>`))
	if err != nil {
		t.Fatalf("a leading comment should be skipped: %v", err)
	}
	if s.TargetNamespace != "urn:t" {
		t.Errorf("targetNamespace = %q", s.TargetNamespace)
	}

	if _, err := Parse(strings.NewReader(`<?xml version="1.0"?><root><child/></root>`)); err == nil {
		t.Error("a document with no schema element should be rejected")
	}
}
