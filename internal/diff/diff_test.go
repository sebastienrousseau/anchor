// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package diff_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/sebastienrousseau/anchor/internal/diff"
	"github.com/sebastienrousseau/anchor/internal/xsd"
)

// schema wraps a body in the boilerplate every ISO 20022 schema carries, so the
// fixtures show only what a test is about.
func schema(t *testing.T, body string) *xsd.Schema {
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

// simple builds a one-element document whose value type is given inline.
func simple(t *testing.T, valueType string) *xsd.Schema {
	return schema(t, `
  <xs:element name="Document" type="Document"/>
  <xs:complexType name="Document">
    <xs:sequence>
      <xs:element name="Val" type="Val"/>
    </xs:sequence>
  </xs:complexType>
  `+valueType)
}

// find returns the change at a path with a given kind.
func find(t *testing.T, rep *diff.Report, path string, kind diff.Kind) diff.Change {
	t.Helper()
	for _, c := range rep.Changes {
		if c.Path == path && c.Kind == kind {
			return c
		}
	}
	t.Fatalf("no %s change at %s; report is %+v", kind, path, rep.Changes)
	return diff.Change{}
}

// absent asserts that nothing was reported at a path.
func absent(t *testing.T, rep *diff.Report, path string) {
	t.Helper()
	for _, c := range rep.Changes {
		if c.Path == path {
			t.Errorf("unexpected change at %s: %+v", path, c)
		}
	}
}

const oneField = `
  <xs:element name="Document" type="Document"/>
  <xs:complexType name="Document">
    <xs:sequence>
      <xs:element name="MsgId" type="xs:string"/>
    </xs:sequence>
  </xs:complexType>`

func TestIdenticalSchemas(t *testing.T) {
	a := schema(t, oneField)
	b := schema(t, oneField)

	rep := diff.Compare(a, b, "v1", "v2")
	if !rep.Identical() {
		t.Errorf("identical schemas reported %d change(s): %+v", len(rep.Changes), rep.Changes)
	}
	if rep.Common != 2 {
		t.Errorf("Common = %d, want 2 (the document and its one field)", rep.Common)
	}
	breaking, compatible := rep.Counts()
	if breaking != 0 || compatible != 0 {
		t.Errorf("counts = %d breaking, %d compatible", breaking, compatible)
	}
	if rep.Breaking() != nil {
		t.Errorf("Breaking() = %+v, want nil", rep.Breaking())
	}
}

func TestElementRemoved(t *testing.T) {
	a := schema(t, `
  <xs:element name="Document" type="Document"/>
  <xs:complexType name="Document">
    <xs:sequence>
      <xs:element name="MsgId" type="xs:string"/>
      <xs:element minOccurs="0" name="Legacy" type="xs:string"/>
    </xs:sequence>
  </xs:complexType>`)
	b := schema(t, oneField)

	rep := diff.Compare(a, b, "v1", "v2")
	c := find(t, rep, "/Document/Legacy", diff.KindRemoved)
	if c.Severity != diff.Breaking {
		t.Errorf("a removed element is %q, want breaking", c.Severity)
	}
	// Even an optional element is a break: a sender loses a field it may have
	// been populating, and a receiver loses data it may have been reading.
	if c.From != "0..1" {
		t.Errorf("From = %q, want the old cardinality", c.From)
	}
}

func TestElementAdded(t *testing.T) {
	a := schema(t, oneField)
	b := schema(t, `
  <xs:element name="Document" type="Document"/>
  <xs:complexType name="Document">
    <xs:sequence>
      <xs:element name="MsgId" type="xs:string"/>
      <xs:element minOccurs="0" name="Optional" type="xs:string"/>
      <xs:element name="Required" type="xs:string"/>
    </xs:sequence>
  </xs:complexType>`)

	rep := diff.Compare(a, b, "v1", "v2")

	if c := find(t, rep, "/Document/Optional", diff.KindAdded); c.Severity != diff.Compatible {
		t.Errorf("a new optional element is %q, want compatible", c.Severity)
	}
	if c := find(t, rep, "/Document/Required", diff.KindAdded); c.Severity != diff.Breaking {
		t.Errorf("a new mandatory element is %q, want breaking", c.Severity)
	}

	// Breaking changes lead the report, so a reader sees what stops them first.
	if rep.Changes[0].Severity != diff.Breaking {
		t.Errorf("the report does not lead with breaking changes: %+v", rep.Changes)
	}
}

func TestBecameMandatory(t *testing.T) {
	optional := `
  <xs:element name="Document" type="Document"/>
  <xs:complexType name="Document">
    <xs:sequence>
      <xs:element minOccurs="0" name="UETR" type="xs:string"/>
    </xs:sequence>
  </xs:complexType>`
	required := `
  <xs:element name="Document" type="Document"/>
  <xs:complexType name="Document">
    <xs:sequence>
      <xs:element name="UETR" type="xs:string"/>
    </xs:sequence>
  </xs:complexType>`

	rep := diff.Compare(schema(t, optional), schema(t, required), "v1", "v2")
	c := find(t, rep, "/Document/UETR", diff.KindCardinality)
	if c.Severity != diff.Breaking {
		t.Errorf("becoming mandatory is %q, want breaking", c.Severity)
	}
	if c.From != "0..1" || c.To != "1..1" {
		t.Errorf("got %s -> %s", c.From, c.To)
	}

	// The other direction relaxes the rule.
	back := diff.Compare(schema(t, required), schema(t, optional), "v2", "v1")
	if c := find(t, back, "/Document/UETR", diff.KindCardinality); c.Severity != diff.Compatible {
		t.Errorf("becoming optional is %q, want compatible", c.Severity)
	}
}

func TestRepeatBounds(t *testing.T) {
	build := func(max string) string {
		return `
  <xs:element name="Document" type="Document"/>
  <xs:complexType name="Document">
    <xs:sequence>
      <xs:element maxOccurs="` + max + `" name="Tx" type="xs:string"/>
    </xs:sequence>
  </xs:complexType>`
	}

	cases := []struct {
		name, from, to string
		want           diff.Severity
	}{
		{"unbounded to one", "unbounded", "1", diff.Breaking},
		{"five to two", "5", "2", diff.Breaking},
		{"two to five", "2", "5", diff.Compatible},
		{"one to unbounded", "1", "unbounded", diff.Compatible},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rep := diff.Compare(schema(t, build(tc.from)), schema(t, build(tc.to)), "a", "b")
			c := find(t, rep, "/Document/Tx", diff.KindCardinality)
			if c.Severity != tc.want {
				t.Errorf("%s -> %s is %q, want %q", tc.from, tc.to, c.Severity, tc.want)
			}
		})
	}
}

func TestTypeRenameIsNotBreaking(t *testing.T) {
	// ISO 20022 renumbers its types on almost every version, and the content
	// difference shows up at the paths beneath. Calling the rename itself
	// breaking would bury the changes that matter.
	a := schema(t, `
  <xs:element name="Document" type="Document"/>
  <xs:complexType name="Document">
    <xs:sequence><xs:element name="Acct" type="CashAccount38"/></xs:sequence>
  </xs:complexType>
  <xs:complexType name="CashAccount38">
    <xs:sequence><xs:element name="Id" type="xs:string"/></xs:sequence>
  </xs:complexType>`)
	b := schema(t, `
  <xs:element name="Document" type="Document"/>
  <xs:complexType name="Document">
    <xs:sequence><xs:element name="Acct" type="CashAccount40"/></xs:sequence>
  </xs:complexType>
  <xs:complexType name="CashAccount40">
    <xs:sequence><xs:element minOccurs="0" name="Id" type="xs:string"/></xs:sequence>
  </xs:complexType>`)

	rep := diff.Compare(a, b, "v1", "v2")
	if c := find(t, rep, "/Document/Acct", diff.KindType); c.Severity != diff.Breaking {
		// The rename must be reported, but as compatible.
		if c.Severity != diff.Compatible {
			t.Errorf("a type rename is %q, want compatible", c.Severity)
		}
	}
	// The real change is the one underneath.
	if c := find(t, rep, "/Document/Acct/Id", diff.KindCardinality); c.Severity != diff.Compatible {
		t.Errorf("Id becoming optional is %q, want compatible", c.Severity)
	}
	if breaking, _ := rep.Counts(); breaking != 0 {
		t.Errorf("a renumbering with a relaxation reported %d breaking change(s): %+v",
			breaking, rep.Breaking())
	}
}

func TestSimpleToComplexIsBreaking(t *testing.T) {
	// pain.001.001.08 -> 09 did exactly this to AdrTp: a code became a choice.
	a := schema(t, `
  <xs:element name="Document" type="Document"/>
  <xs:complexType name="Document">
    <xs:sequence><xs:element name="AdrTp" type="AddressType2Code"/></xs:sequence>
  </xs:complexType>
  <xs:simpleType name="AddressType2Code">
    <xs:restriction base="xs:string"><xs:enumeration value="ADDR"/></xs:restriction>
  </xs:simpleType>`)
	b := schema(t, `
  <xs:element name="Document" type="Document"/>
  <xs:complexType name="Document">
    <xs:sequence><xs:element name="AdrTp" type="AddressType3Choice"/></xs:sequence>
  </xs:complexType>
  <xs:complexType name="AddressType3Choice">
    <xs:choice><xs:element name="Cd" type="xs:string"/></xs:choice>
  </xs:complexType>`)

	rep := diff.Compare(a, b, "v8", "v9")
	c := find(t, rep, "/Document/AdrTp", diff.KindType)
	if c.Severity != diff.Breaking {
		t.Errorf("a value becoming a structure is %q, want breaking", c.Severity)
	}
	if !strings.Contains(c.Detail, "simple value and a structured type") {
		t.Errorf("detail = %q", c.Detail)
	}
}

func TestFacetTightening(t *testing.T) {
	cases := []struct {
		name, from, to string
		want           diff.Severity
	}{
		{
			"maxLength shortened",
			`<xs:simpleType name="Val"><xs:restriction base="xs:string"><xs:maxLength value="140"/></xs:restriction></xs:simpleType>`,
			`<xs:simpleType name="Val"><xs:restriction base="xs:string"><xs:maxLength value="35"/></xs:restriction></xs:simpleType>`,
			diff.Breaking,
		},
		{
			"maxLength lengthened",
			`<xs:simpleType name="Val"><xs:restriction base="xs:string"><xs:maxLength value="35"/></xs:restriction></xs:simpleType>`,
			`<xs:simpleType name="Val"><xs:restriction base="xs:string"><xs:maxLength value="140"/></xs:restriction></xs:simpleType>`,
			diff.Compatible,
		},
		{
			"minLength raised",
			`<xs:simpleType name="Val"><xs:restriction base="xs:string"><xs:minLength value="1"/></xs:restriction></xs:simpleType>`,
			`<xs:simpleType name="Val"><xs:restriction base="xs:string"><xs:minLength value="4"/></xs:restriction></xs:simpleType>`,
			diff.Breaking,
		},
		{
			"a new bound where none existed",
			`<xs:simpleType name="Val"><xs:restriction base="xs:string"/></xs:simpleType>`,
			`<xs:simpleType name="Val"><xs:restriction base="xs:string"><xs:maxLength value="35"/></xs:restriction></xs:simpleType>`,
			diff.Breaking,
		},
		{
			"a bound removed",
			`<xs:simpleType name="Val"><xs:restriction base="xs:string"><xs:maxLength value="35"/></xs:restriction></xs:simpleType>`,
			`<xs:simpleType name="Val"><xs:restriction base="xs:string"/></xs:simpleType>`,
			diff.Compatible,
		},
		{
			"fractionDigits reduced",
			`<xs:simpleType name="Val"><xs:restriction base="xs:decimal"><xs:fractionDigits value="5"/></xs:restriction></xs:simpleType>`,
			`<xs:simpleType name="Val"><xs:restriction base="xs:decimal"><xs:fractionDigits value="2"/></xs:restriction></xs:simpleType>`,
			diff.Breaking,
		},
		{
			"totalDigits reduced",
			`<xs:simpleType name="Val"><xs:restriction base="xs:decimal"><xs:totalDigits value="18"/></xs:restriction></xs:simpleType>`,
			`<xs:simpleType name="Val"><xs:restriction base="xs:decimal"><xs:totalDigits value="9"/></xs:restriction></xs:simpleType>`,
			diff.Breaking,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rep := diff.Compare(simple(t, tc.from), simple(t, tc.to), "a", "b")
			c := find(t, rep, "/Document/Val", diff.KindFacet)
			if c.Severity != tc.want {
				t.Errorf("severity = %q, want %q (%s)", c.Severity, tc.want, c.Detail)
			}
		})
	}
}

func TestPatternChangeIsAlwaysBreaking(t *testing.T) {
	// Deciding whether one regular expression accepts everything another does is
	// not something a tool should guess at.
	a := simple(t, `<xs:simpleType name="Val"><xs:restriction base="xs:string"><xs:pattern value="[A-Z]{4,4}"/></xs:restriction></xs:simpleType>`)
	b := simple(t, `<xs:simpleType name="Val"><xs:restriction base="xs:string"><xs:pattern value="[A-Z]{4,4}[0-9]{2,2}"/></xs:restriction></xs:simpleType>`)

	rep := diff.Compare(a, b, "a", "b")
	c := find(t, rep, "/Document/Val", diff.KindFacet)
	if c.Severity != diff.Breaking {
		t.Errorf("a pattern change is %q, want breaking", c.Severity)
	}
}

func TestEnumerationChanges(t *testing.T) {
	a := simple(t, `<xs:simpleType name="Val"><xs:restriction base="xs:string">
      <xs:enumeration value="DEBT"/><xs:enumeration value="CRED"/><xs:enumeration value="SLEV"/>
    </xs:restriction></xs:simpleType>`)
	b := simple(t, `<xs:simpleType name="Val"><xs:restriction base="xs:string">
      <xs:enumeration value="DEBT"/><xs:enumeration value="CRED"/><xs:enumeration value="SHAR"/>
    </xs:restriction></xs:simpleType>`)

	rep := diff.Compare(a, b, "a", "b")

	var withdrawn, introduced *diff.Change
	for i := range rep.Changes {
		c := &rep.Changes[i]
		if c.Kind != diff.KindEnumeration {
			continue
		}
		if c.Severity == diff.Breaking {
			withdrawn = c
		} else {
			introduced = c
		}
	}
	if withdrawn == nil || !strings.Contains(withdrawn.From, "SLEV") {
		t.Errorf("the withdrawn code was not reported: %+v", rep.Changes)
	}
	if introduced == nil || !strings.Contains(introduced.To, "SHAR") {
		t.Errorf("the new code was not reported: %+v", rep.Changes)
	}
}

func TestOptionalityPropagatesIntoNestedTypes(t *testing.T) {
	// A mandatory field inside an optional branch is not a field anyone has to
	// populate, so adding one must not be reported as breaking.
	a := schema(t, oneField)
	b := schema(t, `
  <xs:element name="Document" type="Document"/>
  <xs:complexType name="Document">
    <xs:sequence>
      <xs:element name="MsgId" type="xs:string"/>
      <xs:element minOccurs="0" name="AgrdRate" type="AgreedRate"/>
    </xs:sequence>
  </xs:complexType>
  <xs:complexType name="AgreedRate">
    <xs:sequence><xs:element name="PreAgrdXchgRate" type="xs:decimal"/></xs:sequence>
  </xs:complexType>`)

	rep := diff.Compare(a, b, "v1", "v2")
	for _, c := range rep.Changes {
		if c.Severity == diff.Breaking {
			t.Errorf("adding an optional branch reported a breaking change: %+v", c)
		}
	}
	if c := find(t, rep, "/Document/AgrdRate/PreAgrdXchgRate", diff.KindAdded); c.Severity != diff.Compatible {
		t.Errorf("a mandatory field inside an optional branch is %q, want compatible", c.Severity)
	}
}

func TestChoiceBranchesAreOptional(t *testing.T) {
	// Picking one branch of a choice means not picking the others, so no branch
	// can be treated as mandatory.
	a := schema(t, oneField)
	b := schema(t, `
  <xs:element name="Document" type="Document"/>
  <xs:complexType name="Document">
    <xs:sequence>
      <xs:element name="MsgId" type="xs:string"/>
      <xs:element minOccurs="0" name="Acct" type="AccountChoice"/>
    </xs:sequence>
  </xs:complexType>
  <xs:complexType name="AccountChoice">
    <xs:choice>
      <xs:element name="IBAN" type="xs:string"/>
      <xs:element name="Othr" type="xs:string"/>
    </xs:choice>
  </xs:complexType>`)

	rep := diff.Compare(a, b, "v1", "v2")
	if breaking, _ := rep.Counts(); breaking != 0 {
		t.Errorf("adding a choice reported %d breaking change(s): %+v", breaking, rep.Breaking())
	}
}

func TestRecursiveTypeTerminates(t *testing.T) {
	// A self-referencing type must be recorded once, not walked forever.
	s := schema(t, `
  <xs:element name="Document" type="Document"/>
  <xs:complexType name="Document">
    <xs:sequence><xs:element name="Node" type="Node"/></xs:sequence>
  </xs:complexType>
  <xs:complexType name="Node">
    <xs:sequence>
      <xs:element name="Name" type="xs:string"/>
      <xs:element minOccurs="0" name="Child" type="Node"/>
    </xs:sequence>
  </xs:complexType>`)

	m := diff.Flatten(s)
	if _, ok := m.Nodes["/Document/Node/Child"]; !ok {
		t.Error("the recursive element was not recorded")
	}
	if _, ok := m.Nodes["/Document/Node/Child/Child"]; ok {
		t.Error("the walk descended into a type already on the path")
	}
	if diff.Compare(s, s, "a", "a").Identical() != true {
		t.Error("a recursive schema does not compare equal to itself")
	}
}

func TestFlattenHandlesMissingRoot(t *testing.T) {
	if m := diff.Flatten(nil); m.Root != "" || len(m.Nodes) != 0 {
		t.Errorf("Flatten(nil) = %+v", m)
	}
	// A schema declaring no global element has no document to walk.
	s := schema(t, `<xs:complexType name="Orphan"><xs:sequence/></xs:complexType>`)
	if m := diff.Flatten(s); m.Root != "" {
		t.Errorf("Root = %q, want empty", m.Root)
	}
	// A root whose type is not declared yields the root and nothing else.
	dangling := schema(t, `<xs:element name="Document" type="Missing"/>`)
	if m := diff.Flatten(dangling); len(m.Nodes) != 1 {
		t.Errorf("got %d nodes, want just the root", len(m.Nodes))
	}
}

func TestPathsAreInDocumentOrder(t *testing.T) {
	s := schema(t, `
  <xs:element name="Document" type="Document"/>
  <xs:complexType name="Document">
    <xs:sequence>
      <xs:element name="GrpHdr" type="xs:string"/>
      <xs:element name="Tx" type="xs:string"/>
    </xs:sequence>
  </xs:complexType>`)

	m := diff.Flatten(s)
	want := []string{"/Document", "/Document/GrpHdr", "/Document/Tx"}
	if strings.Join(m.Order, ",") != strings.Join(want, ",") {
		t.Errorf("Order = %v, want %v", m.Order, want)
	}
}

func TestOccursRendering(t *testing.T) {
	a := schema(t, `
  <xs:element name="Document" type="Document"/>
  <xs:complexType name="Document">
    <xs:sequence><xs:element name="Tx" type="xs:string"/></xs:sequence>
  </xs:complexType>`)
	b := schema(t, `
  <xs:element name="Document" type="Document"/>
  <xs:complexType name="Document">
    <xs:sequence>
      <xs:element maxOccurs="unbounded" minOccurs="0" name="Tx" type="xs:string"/>
    </xs:sequence>
  </xs:complexType>`)

	rep := diff.Compare(a, b, "a", "b")
	c := find(t, rep, "/Document/Tx", diff.KindCardinality)
	if c.To != "0..unbounded" {
		t.Errorf("To = %q, want 0..unbounded", c.To)
	}
	absent(t, diff.Compare(a, a, "a", "a"), "/Document/Tx")
}

func TestRepeatedElementNameRecordedOnce(t *testing.T) {
	// A name repeated within one container maps to one path, so the walk must
	// not record it twice or the order would carry a duplicate.
	s := schema(t, `
  <xs:element name="Document" type="Document"/>
  <xs:complexType name="Document">
    <xs:sequence>
      <xs:element name="Id" type="xs:string"/>
      <xs:element minOccurs="0" name="Id" type="xs:string"/>
    </xs:sequence>
  </xs:complexType>`)

	m := diff.Flatten(s)
	var seen int
	for _, p := range m.Order {
		if p == "/Document/Id" {
			seen++
		}
	}
	if seen != 1 {
		t.Errorf("/Document/Id appears %d times in Order", seen)
	}
}

func TestDeepNestingStops(t *testing.T) {
	// The walk is bounded, because a schema can nest further than any report is
	// useful at, and a malformed one can nest without end.
	var body strings.Builder
	body.WriteString(`
  <xs:element name="Document" type="T0"/>`)
	const levels = 40
	for i := 0; i < levels; i++ {
		fmt.Fprintf(&body, `
  <xs:complexType name="T%d">
    <xs:sequence><xs:element name="L%d" type="T%d"/></xs:sequence>
  </xs:complexType>`, i, i, i+1)
	}
	fmt.Fprintf(&body, `
  <xs:complexType name="T%d"><xs:sequence/></xs:complexType>`, levels)

	m := diff.Flatten(schema(t, body.String()))
	if len(m.Nodes) == 0 {
		t.Fatal("nothing was recorded")
	}
	if len(m.Nodes) > levels {
		t.Errorf("the walk recorded %d nodes for %d levels; it did not stop", len(m.Nodes), levels)
	}
	// Whatever the bound is, the shallow paths must still be there.
	if _, ok := m.Nodes["/Document/L0/L1/L2"]; !ok {
		t.Error("a shallow path is missing")
	}
}

func TestBreakingAndCounts(t *testing.T) {
	a := schema(t, oneField)
	b := schema(t, `
  <xs:element name="Document" type="Document"/>
  <xs:complexType name="Document">
    <xs:sequence>
      <xs:element name="MsgId" type="xs:string"/>
      <xs:element name="Required" type="xs:string"/>
      <xs:element minOccurs="0" name="Optional" type="xs:string"/>
    </xs:sequence>
  </xs:complexType>`)

	rep := diff.Compare(a, b, "v1", "v2")
	breaking, compatible := rep.Counts()
	if breaking != 1 || compatible != 1 {
		t.Errorf("counts = %d breaking, %d compatible; want 1 and 1: %+v", breaking, compatible, rep.Changes)
	}

	only := rep.Breaking()
	if len(only) != 1 || only[0].Path != "/Document/Required" {
		t.Errorf("Breaking() = %+v", only)
	}
	if rep.Identical() {
		t.Error("Identical() is true for a report with changes")
	}
}

func TestRootElementPreference(t *testing.T) {
	// A schema declaring a business application header uses AppHdr as its root
	// even when another element is declared first.
	s := schema(t, `
  <xs:element name="Other" type="Other"/>
  <xs:element name="AppHdr" type="Header"/>
  <xs:complexType name="Other"><xs:sequence/></xs:complexType>
  <xs:complexType name="Header">
    <xs:sequence><xs:element name="Fr" type="xs:string"/></xs:sequence>
  </xs:complexType>`)

	m := diff.Flatten(s)
	if m.Root != "AppHdr" {
		t.Errorf("Root = %q, want AppHdr", m.Root)
	}
	if _, ok := m.Nodes["/AppHdr/Fr"]; !ok {
		t.Error("the header body was not walked")
	}
}
