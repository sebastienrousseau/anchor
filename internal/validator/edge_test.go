// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package validator

import (
	"strings"
	"testing"

	"github.com/sebastienrousseau/anchor/internal/xsd"
)

func schemaFrom(t *testing.T, doc string) *xsd.Schema {
	t.Helper()
	s, err := xsd.Parse(strings.NewReader(doc))
	if err != nil {
		t.Fatalf("parsing schema: %v", err)
	}
	return s
}

func TestErrorStringWithoutPosition(t *testing.T) {
	e := Error{Path: "/Document", Message: "something"}
	if got := e.String(); !strings.Contains(got, "/Document") || strings.Contains(got, ":0:") {
		t.Errorf("String() on a positionless error = %q", got)
	}
	e.Line, e.Column = 3, 5
	if got := e.String(); !strings.Contains(got, "3:5") {
		t.Errorf("String() should carry the position: %q", got)
	}
}

func TestMissingNamespaceIsReported(t *testing.T) {
	s := schemaFrom(t, `<?xml version="1.0"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:t" xmlns="urn:t">
  <xs:element name="Document" type="xs:string"/>
</xs:schema>`)

	res := Validate([]byte(`<Document/>`), s)
	if res.Valid {
		t.Fatal("a document with no namespace should be rejected")
	}
	if res.Errors[0].Actual != "(none)" {
		t.Errorf("the absent namespace should read as (none): %+v", res.Errors[0])
	}
}

func TestEmptyComplexTypeRejectsChildren(t *testing.T) {
	s := schemaFrom(t, `<?xml version="1.0"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:t" xmlns="urn:t">
  <xs:element name="Document" type="Empty"/>
  <xs:complexType name="Empty"/>
</xs:schema>`)

	if res := Validate([]byte(`<Document xmlns="urn:t"/>`), s); !res.Valid {
		t.Errorf("an empty element should be valid: %v", res.Errors)
	}
	res := Validate([]byte(`<Document xmlns="urn:t"><Child/></Document>`), s)
	if res.Valid {
		t.Fatal("children under an empty type should be rejected")
	}
	if res.Errors[0].Rule != "content model" {
		t.Errorf("rule = %q", res.Errors[0].Rule)
	}
}

func TestSimpleContentRejectsChildren(t *testing.T) {
	s := schemaFrom(t, `<?xml version="1.0"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:t" xmlns="urn:t">
  <xs:element name="Document" type="Amt"/>
  <xs:complexType name="Amt">
    <xs:simpleContent><xs:extension base="xs:decimal">
      <xs:attribute name="Ccy" type="xs:string" use="required"/>
    </xs:extension></xs:simpleContent>
  </xs:complexType>
</xs:schema>`)

	res := Validate([]byte(`<Document xmlns="urn:t" Ccy="EUR"><Nested/></Document>`), s)
	if res.Valid {
		t.Fatal("children under simple content should be rejected")
	}
}

func TestMaxOccursIsEnforced(t *testing.T) {
	s := schemaFrom(t, `<?xml version="1.0"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:t" xmlns="urn:t">
  <xs:element name="Document" type="Doc"/>
  <xs:complexType name="Doc"><xs:sequence>
    <xs:element name="A" type="xs:string" maxOccurs="2"/>
  </xs:sequence></xs:complexType>
</xs:schema>`)

	if res := Validate([]byte(`<Document xmlns="urn:t"><A>1</A><A>2</A></Document>`), s); !res.Valid {
		t.Errorf("two occurrences should be valid: %v", res.Errors)
	}
	if res := Validate([]byte(`<Document xmlns="urn:t"><A>1</A><A>2</A><A>3</A></Document>`), s); res.Valid {
		t.Error("a third occurrence should be rejected")
	}
}

func TestRepeatableChoice(t *testing.T) {
	s := schemaFrom(t, `<?xml version="1.0"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:t" xmlns="urn:t">
  <xs:element name="Document" type="Doc"/>
  <xs:complexType name="Doc">
    <xs:choice maxOccurs="unbounded">
      <xs:element name="A" type="xs:string"/>
      <xs:element name="B" type="xs:string"/>
    </xs:choice>
  </xs:complexType>
</xs:schema>`)

	if res := Validate([]byte(`<Document xmlns="urn:t"><A>1</A><B>2</B><A>3</A></Document>`), s); !res.Valid {
		t.Errorf("a repeatable choice should accept a mixed run: %v", res.Errors)
	}
}

func TestOptionalChoiceMayBeAbsent(t *testing.T) {
	s := schemaFrom(t, `<?xml version="1.0"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:t" xmlns="urn:t">
  <xs:element name="Document" type="Doc"/>
  <xs:complexType name="Doc"><xs:sequence>
    <xs:element name="A" type="xs:string"/>
    <xs:choice minOccurs="0">
      <xs:element name="X" type="xs:string"/>
    </xs:choice>
  </xs:sequence></xs:complexType>
</xs:schema>`)

	if res := Validate([]byte(`<Document xmlns="urn:t"><A>1</A></Document>`), s); !res.Valid {
		t.Errorf("an optional choice may be omitted: %v", res.Errors)
	}
}

func TestWildcardAcceptsAnything(t *testing.T) {
	s := schemaFrom(t, `<?xml version="1.0"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:t" xmlns="urn:t">
  <xs:element name="Document" type="Doc"/>
  <xs:complexType name="Doc"><xs:sequence>
    <xs:any namespace="##any" processContents="lax" maxOccurs="unbounded"/>
  </xs:sequence></xs:complexType>
</xs:schema>`)

	if res := Validate([]byte(`<Document xmlns="urn:t"><Anything/><Else/></Document>`), s); !res.Valid {
		t.Errorf("a wildcard should accept anything: %v", res.Errors)
	}

	// A required wildcard with nothing to match.
	s2 := schemaFrom(t, `<?xml version="1.0"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:t" xmlns="urn:t">
  <xs:element name="Document" type="Doc"/>
  <xs:complexType name="Doc"><xs:sequence>
    <xs:any namespace="##any" processContents="lax"/>
  </xs:sequence></xs:complexType>
</xs:schema>`)
	if res := Validate([]byte(`<Document xmlns="urn:t"/>`), s2); res.Valid {
		t.Error("a required wildcard needs one element")
	}
}

func TestNumericAndTemporalTypes(t *testing.T) {
	s := schemaFrom(t, `<?xml version="1.0"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:t" xmlns="urn:t">
  <xs:element name="Document" type="Doc"/>
  <xs:complexType name="Doc"><xs:sequence>
    <xs:element name="B" type="xs:boolean"/>
    <xs:element name="I" type="xs:integer"/>
    <xs:element name="N" type="xs:nonNegativeInteger"/>
    <xs:element name="D" type="xs:date"/>
    <xs:element name="T" type="xs:time"/>
    <xs:element name="Y" type="xs:gYear"/>
    <xs:element name="YM" type="xs:gYearMonth"/>
    <xs:element name="M" type="xs:gMonth"/>
    <xs:element name="U" type="xs:anyURI"/>
  </xs:sequence></xs:complexType>
</xs:schema>`)

	good := `<Document xmlns="urn:t">
  <B>true</B><I>-42</I><N>7</N><D>2026-08-24</D><T>10:30:00Z</T>
  <Y>2026</Y><YM>2026-08</YM><M>--08</M><U>urn:x</U>
</Document>`
	if res := Validate([]byte(good), s); !res.Valid {
		t.Fatalf("all values should be valid: %v", res.Errors)
	}

	bad := map[string]string{
		"B":  "maybe",
		"I":  "1.5",
		"N":  "-1",
		"D":  "2026-13-01",
		"T":  "25:00",
		"Y":  "20XX",
		"YM": "2026",
		"M":  "08",
	}
	for elem, val := range bad {
		t.Run(elem, func(t *testing.T) {
			doc := strings.Replace(good, ">"+map[string]string{
				"B": "true", "I": "-42", "N": "7", "D": "2026-08-24",
				"T": "10:30:00Z", "Y": "2026", "YM": "2026-08", "M": "--08",
			}[elem]+"<", ">"+val+"<", 1)
			if res := Validate([]byte(doc), s); res.Valid {
				t.Errorf("%s=%q should be rejected", elem, val)
			}
		})
	}
}

func TestUnknownBaseTypeIsAccepted(t *testing.T) {
	s := schemaFrom(t, `<?xml version="1.0"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:t" xmlns="urn:t">
  <xs:element name="Document" type="Doc"/>
  <xs:complexType name="Doc"><xs:sequence>
    <xs:element name="A" type="xs:someFutureType"/>
  </xs:sequence></xs:complexType>
</xs:schema>`)
	// An unrecognised base must not cause a false rejection.
	if res := Validate([]byte(`<Document xmlns="urn:t"><A>anything</A></Document>`), s); !res.Valid {
		t.Errorf("an unknown base type should not reject: %v", res.Errors)
	}
}

func TestLongEnumerationIsTruncatedInMessage(t *testing.T) {
	var enums strings.Builder
	for i := 0; i < 20; i++ {
		enums.WriteString(`<xs:enumeration value="V` + string(rune('A'+i)) + `"/>`)
	}
	s := schemaFrom(t, `<?xml version="1.0"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:t" xmlns="urn:t">
  <xs:element name="Document" type="E"/>
  <xs:simpleType name="E"><xs:restriction base="xs:string">`+enums.String()+`</xs:restriction></xs:simpleType>
</xs:schema>`)

	res := Validate([]byte(`<Document xmlns="urn:t">NOPE</Document>`), s)
	if res.Valid {
		t.Fatal("expected an enumeration failure")
	}
	if !strings.Contains(res.Errors[0].Expected, "…") {
		t.Errorf("a long enumeration list should be truncated: %q", res.Errors[0].Expected)
	}
}

func TestLongValueIsTruncatedInMessage(t *testing.T) {
	s := schemaFrom(t, `<?xml version="1.0"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:t" xmlns="urn:t">
  <xs:element name="Document" type="T"/>
  <xs:simpleType name="T"><xs:restriction base="xs:string"><xs:maxLength value="5"/>
    <xs:pattern value="[A-Z]{1,5}"/></xs:restriction></xs:simpleType>
</xs:schema>`)

	long := strings.Repeat("a", 200)
	res := Validate([]byte(`<Document xmlns="urn:t">`+long+`</Document>`), s)
	if res.Valid {
		t.Fatal("expected a failure")
	}
	for _, e := range res.Errors {
		if strings.Contains(e.Message, long) {
			t.Error("the message should truncate a very long value")
		}
	}
}

func TestChoiceBranchDetectionThroughNesting(t *testing.T) {
	s := schemaFrom(t, `<?xml version="1.0"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:t" xmlns="urn:t">
  <xs:element name="Document" type="Doc"/>
  <xs:complexType name="Doc">
    <xs:choice>
      <xs:sequence>
        <xs:element name="Opt" type="xs:string" minOccurs="0"/>
        <xs:element name="Deep" type="xs:string"/>
      </xs:sequence>
      <xs:element name="Other" type="xs:string"/>
    </xs:choice>
  </xs:complexType>
</xs:schema>`)

	// The branch is chosen by an element that appears after an optional one.
	if res := Validate([]byte(`<Document xmlns="urn:t"><Deep>x</Deep></Document>`), s); !res.Valid {
		t.Errorf("nested branch detection failed: %v", res.Errors)
	}
	if res := Validate([]byte(`<Document xmlns="urn:t"><Other>x</Other></Document>`), s); !res.Valid {
		t.Errorf("second branch failed: %v", res.Errors)
	}
}

func TestValidateRejectsMultipleRoots(t *testing.T) {
	s := schemaFrom(t, `<?xml version="1.0"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:t" xmlns="urn:t">
  <xs:element name="Document" type="xs:string"/>
</xs:schema>`)
	if res := Validate([]byte(`<Document xmlns="urn:t"/><Document xmlns="urn:t"/>`), s); res.Valid {
		t.Error("two root elements should be rejected")
	}
}

func TestAtomWidthEdgeCases(t *testing.T) {
	// An escape as the repeated atom.
	if _, err := compilePattern(`\d{1,3000}`); err != nil {
		t.Errorf("an escaped atom should compile: %v", err)
	}
	// A repeat with no atom before it.
	if _, err := compilePattern(`{1,3000}`); err == nil {
		t.Log("a bare repeat compiled; acceptable")
	}
	// A group ending the pattern.
	if w := atomWidth(`(ab)`, 4); w != 2 {
		t.Errorf("atomWidth on a trailing group = %d, want 2", w)
	}
	if w := atomWidth(``, 0); w != 0 {
		t.Errorf("atomWidth on empty = %d, want 0", w)
	}
}

func TestEscapedHelper(t *testing.T) {
	if !escaped(`\]`, 1) {
		t.Error(`\] should be escaped`)
	}
	if escaped(`\\]`, 2) {
		t.Error(`\\] the bracket is not escaped`)
	}
	if escaped(`]`, 0) {
		t.Error("a lone bracket is not escaped")
	}
}

func TestCountDigits(t *testing.T) {
	cases := []struct {
		in          string
		total, frac int
	}{
		{"123.45", 5, 2},
		{"-123.45", 5, 2},
		{"+0.5", 1, 1},
		{"000123", 3, 0},
		{"1.5000", 2, 1},
		{"0", 0, 0},
	}
	for _, tc := range cases {
		total, frac := countDigits(tc.in)
		if total != tc.total || frac != tc.frac {
			t.Errorf("countDigits(%q) = (%d,%d), want (%d,%d)", tc.in, total, frac, tc.total, tc.frac)
		}
	}
}

func TestErrorCapStopsNestedWalks(t *testing.T) {
	// A deeply repeated invalid structure must stop at the cap rather than
	// walking the whole document.
	s := schemaFrom(t, `<?xml version="1.0"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:t" xmlns="urn:t">
  <xs:element name="Document" type="Doc"/>
  <xs:complexType name="Doc"><xs:sequence>
    <xs:element name="Tx" type="Tx" maxOccurs="unbounded"/>
  </xs:sequence></xs:complexType>
  <xs:complexType name="Tx"><xs:sequence>
    <xs:element name="A" type="Code"/>
    <xs:element name="B" type="Code"/>
  </xs:sequence></xs:complexType>
  <xs:simpleType name="Code">
    <xs:restriction base="xs:string"><xs:pattern value="[A-Z]{3,3}"/></xs:restriction>
  </xs:simpleType>
</xs:schema>`)

	var b strings.Builder
	b.WriteString(`<Document xmlns="urn:t">`)
	for i := 0; i < 400; i++ {
		b.WriteString(`<Tx><A>bad</A><B>worse</B></Tx>`)
	}
	b.WriteString(`</Document>`)

	res := Validate([]byte(b.String()), s)
	if res.Valid {
		t.Fatal("expected failures")
	}
	if len(res.Errors) > maxErrors {
		t.Errorf("errors = %d, want at most %d", len(res.Errors), maxErrors)
	}
}

func TestBoundedRepeatsOnGroups(t *testing.T) {
	s := schemaFrom(t, `<?xml version="1.0"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:t" xmlns="urn:t">
  <xs:element name="Document" type="Doc"/>
  <xs:complexType name="Doc">
    <xs:sequence maxOccurs="2">
      <xs:element name="A" type="xs:string"/>
    </xs:sequence>
  </xs:complexType>
</xs:schema>`)

	if res := Validate([]byte(`<Document xmlns="urn:t"><A>1</A><A>2</A></Document>`), s); !res.Valid {
		t.Errorf("a repeatable sequence should accept two runs: %v", res.Errors)
	}
	// A third run exceeds the bound and leaves an unconsumed child.
	if res := Validate([]byte(`<Document xmlns="urn:t"><A>1</A><A>2</A><A>3</A></Document>`), s); res.Valid {
		t.Error("a third run should be rejected")
	}
}

func TestBoundedChoiceRepeats(t *testing.T) {
	s := schemaFrom(t, `<?xml version="1.0"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:t" xmlns="urn:t">
  <xs:element name="Document" type="Doc"/>
  <xs:complexType name="Doc">
    <xs:choice maxOccurs="2">
      <xs:element name="A" type="xs:string"/>
      <xs:element name="B" type="xs:string"/>
    </xs:choice>
  </xs:complexType>
</xs:schema>`)

	if res := Validate([]byte(`<Document xmlns="urn:t"><A>1</A><B>2</B></Document>`), s); !res.Valid {
		t.Errorf("two choice runs should be valid: %v", res.Errors)
	}
	if res := Validate([]byte(`<Document xmlns="urn:t"><A>1</A><B>2</B><A>3</A></Document>`), s); res.Valid {
		t.Error("a third choice run should be rejected")
	}
}

func TestBoundedWildcardRepeats(t *testing.T) {
	s := schemaFrom(t, `<?xml version="1.0"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:t" xmlns="urn:t">
  <xs:element name="Document" type="Doc"/>
  <xs:complexType name="Doc"><xs:sequence>
    <xs:any namespace="##any" processContents="lax" maxOccurs="2"/>
  </xs:sequence></xs:complexType>
</xs:schema>`)

	if res := Validate([]byte(`<Document xmlns="urn:t"><X/><Y/></Document>`), s); !res.Valid {
		t.Errorf("two wildcard matches should be valid: %v", res.Errors)
	}
	if res := Validate([]byte(`<Document xmlns="urn:t"><X/><Y/><Z/></Document>`), s); res.Valid {
		t.Error("a third wildcard match should be rejected")
	}
}

// A choice whose branches are themselves groups exercises branch naming.
func TestChoiceOfGroupsReportsBranchNames(t *testing.T) {
	s := schemaFrom(t, `<?xml version="1.0"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:t" xmlns="urn:t">
  <xs:element name="Document" type="Doc"/>
  <xs:complexType name="Doc">
    <xs:choice>
      <xs:sequence><xs:element name="A" type="xs:string"/></xs:sequence>
      <xs:choice>
        <xs:element name="B" type="xs:string"/>
        <xs:element name="C" type="xs:string"/>
      </xs:choice>
      <xs:any namespace="##any" processContents="lax"/>
    </xs:choice>
  </xs:complexType>
</xs:schema>`)

	// The wildcard branch accepts anything, so nothing is rejected here; the
	// point is that branch enumeration handles nested groups without panicking.
	for _, doc := range []string{
		`<Document xmlns="urn:t"><A>1</A></Document>`,
		`<Document xmlns="urn:t"><B>1</B></Document>`,
		`<Document xmlns="urn:t"><C>1</C></Document>`,
		`<Document xmlns="urn:t"><Z>1</Z></Document>`,
	} {
		if res := Validate([]byte(doc), s); !res.Valid {
			t.Errorf("%s should be valid: %v", doc, res.Errors)
		}
	}
}

func TestManyBranchesAreTruncatedInMessage(t *testing.T) {
	var branches strings.Builder
	for i := 0; i < 15; i++ {
		branches.WriteString(`<xs:element name="E` + string(rune('A'+i)) + `" type="xs:string"/>`)
	}
	s := schemaFrom(t, `<?xml version="1.0"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:t" xmlns="urn:t">
  <xs:element name="Document" type="Doc"/>
  <xs:complexType name="Doc"><xs:choice>`+branches.String()+`</xs:choice></xs:complexType>
</xs:schema>`)

	res := Validate([]byte(`<Document xmlns="urn:t"><Nope/></Document>`), s)
	if res.Valid {
		t.Fatal("expected a choice failure")
	}
	if !strings.Contains(res.Errors[0].Expected, "…") {
		t.Errorf("a long branch list should be truncated: %q", res.Errors[0].Expected)
	}
}

func TestUnexpectedElementListsAlternatives(t *testing.T) {
	var elems strings.Builder
	for i := 0; i < 15; i++ {
		elems.WriteString(`<xs:element name="E` + string(rune('A'+i)) + `" type="xs:string" minOccurs="0"/>`)
	}
	s := schemaFrom(t, `<?xml version="1.0"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:t" xmlns="urn:t">
  <xs:element name="Document" type="Doc"/>
  <xs:complexType name="Doc"><xs:sequence>`+elems.String()+`</xs:sequence></xs:complexType>
</xs:schema>`)

	res := Validate([]byte(`<Document xmlns="urn:t"><Unexpected/></Document>`), s)
	if res.Valid {
		t.Fatal("an unexpected element should be rejected")
	}
	if res.Errors[0].Rule != "content model" {
		t.Errorf("rule = %q", res.Errors[0].Rule)
	}
}

func TestFailStopsAtTheCap(t *testing.T) {
	v := &validation{res: &Result{}}
	for i := 0; i < maxErrors+50; i++ {
		v.fail(nil, "/p", "rule", "want", "got", "message")
	}
	if len(v.res.Errors) != maxErrors {
		t.Errorf("errors = %d, want the cap of %d", len(v.res.Errors), maxErrors)
	}
	if !v.tooMany() {
		t.Error("tooMany should report the cap was reached")
	}
}

func TestOptionalHelperCoversEveryParticle(t *testing.T) {
	cases := []struct {
		p    xsd.Particle
		want bool
	}{
		{&xsd.Element{MinOccurs: 0}, true},
		{&xsd.Element{MinOccurs: 1}, false},
		{&xsd.Sequence{MinOccurs: 0}, true},
		{&xsd.Sequence{MinOccurs: 1}, false},
		{&xsd.Choice{MinOccurs: 0}, true},
		{&xsd.Choice{MinOccurs: 1}, false},
		{&xsd.Any{MinOccurs: 0}, true},
		{&xsd.Any{MinOccurs: 1}, false},
	}
	for _, tc := range cases {
		if got := optional(tc.p); got != tc.want {
			t.Errorf("optional(%T min=%v) = %v, want %v", tc.p, tc.p, got, tc.want)
		}
	}
}

func TestStartsWithCoversEveryParticle(t *testing.T) {
	if !startsWith(&xsd.Any{}, "anything") {
		t.Error("a wildcard starts with anything")
	}
	if !startsWith(&xsd.Element{Name: "A"}, "A") || startsWith(&xsd.Element{Name: "A"}, "B") {
		t.Error("element matching failed")
	}

	// A sequence whose first particle is mandatory stops the search.
	seq := &xsd.Sequence{Particles: []xsd.Particle{
		&xsd.Element{Name: "A", MinOccurs: 1},
		&xsd.Element{Name: "B", MinOccurs: 1},
	}}
	if !startsWith(seq, "A") {
		t.Error("a sequence starts with its first element")
	}
	if startsWith(seq, "B") {
		t.Error("a mandatory first element should stop the search")
	}

	// An optional first particle lets the search continue.
	optSeq := &xsd.Sequence{Particles: []xsd.Particle{
		&xsd.Element{Name: "A", MinOccurs: 0},
		&xsd.Element{Name: "B", MinOccurs: 1},
	}}
	if !startsWith(optSeq, "B") {
		t.Error("an optional first element should let the search continue")
	}

	ch := &xsd.Choice{Particles: []xsd.Particle{
		&xsd.Element{Name: "X"}, &xsd.Element{Name: "Y"},
	}}
	if !startsWith(ch, "Y") {
		t.Error("a choice starts with any branch")
	}
	if startsWith(ch, "Z") {
		t.Error("an unlisted name should not match")
	}

	// An empty sequence matches nothing.
	if startsWith(&xsd.Sequence{}, "A") {
		t.Error("an empty sequence matches nothing")
	}
}

func TestFirstNamesCoversEveryParticle(t *testing.T) {
	if got := firstNames(&xsd.Any{}); len(got) != 1 || got[0] != "(any)" {
		t.Errorf("firstNames(Any) = %v", got)
	}
	if got := firstNames(&xsd.Element{Name: "A"}); len(got) != 1 || got[0] != "A" {
		t.Errorf("firstNames(Element) = %v", got)
	}
	if got := firstNames(&xsd.Sequence{Particles: []xsd.Particle{&xsd.Element{Name: "A"}}}); len(got) != 1 {
		t.Errorf("firstNames(Sequence) = %v", got)
	}
	if got := firstNames(&xsd.Choice{Particles: []xsd.Particle{
		&xsd.Element{Name: "A"}, &xsd.Element{Name: "B"},
	}}); len(got) != 2 {
		t.Errorf("firstNames(Choice) = %v", got)
	}
	if got := firstNames(&xsd.Sequence{}); got != nil {
		t.Errorf("an empty sequence should yield nothing, got %v", got)
	}
}

func TestParticleFallThrough(t *testing.T) {
	// An unrecognised particle type leaves the position untouched rather than
	// panicking.
	v := &validation{res: &Result{}}
	if got := v.particle(nil, &node{Name: "P"}, nil, 3, "/p"); got != 3 {
		t.Errorf("position = %d, want 3", got)
	}
}

func TestPatternCacheReturnsSameMatcher(t *testing.T) {
	a, err := compilePattern(`[A-Z]{2,2}`)
	if err != nil {
		t.Fatal(err)
	}
	b, err := compilePattern(`[A-Z]{2,2}`)
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Error("a compiled pattern should be cached")
	}

	// A failure is cached too.
	if _, err := compilePattern(`[bad`); err == nil {
		t.Fatal("expected a compile failure")
	}
	if _, err := compilePattern(`[bad`); err == nil {
		t.Error("the cached failure should still report an error")
	}
}
