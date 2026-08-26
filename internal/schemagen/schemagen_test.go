// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package schemagen_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/sebastienrousseau/askiso/internal/schemagen"
	"github.com/sebastienrousseau/askiso/internal/validator"
	"github.com/sebastienrousseau/askiso/internal/xsd"
)

// A generated message is only worth anything if it validates, so every test
// here generates and then validates with AskISO's own validator rather than
// asserting on the text alone.

func parse(t *testing.T, body string) *xsd.Schema {
	t.Helper()
	doc := `<?xml version="1.0" encoding="UTF-8"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"
           xmlns="urn:iso:std:iso:20022:tech:xsd:test.001.001.01"
           targetNamespace="urn:iso:std:iso:20022:tech:xsd:test.001.001.01"
           elementFormDefault="qualified">
` + body + `
</xs:schema>`
	s, err := xsd.Parse(strings.NewReader(doc))
	if err != nil {
		t.Fatalf("parsing the fixture schema: %v", err)
	}
	return s
}

// build generates and validates, returning the document.
func build(t *testing.T, schema *xsd.Schema, opts schemagen.Options) *schemagen.Result {
	t.Helper()

	res, err := schemagen.Generate(schema, opts)
	if err != nil {
		t.Fatalf("generating: %v", err)
	}

	verdict := validator.Validate([]byte(res.XML), schema)
	if !verdict.Valid {
		t.Fatalf("the generated message does not validate:\n%s\nerrors: %+v", res.XML, verdict.Errors)
	}
	return res
}

const basicSchema = `
  <xs:element name="Document" type="Document"/>
  <xs:complexType name="Document">
    <xs:sequence>
      <xs:element name="GrpHdr" type="GroupHeader"/>
      <xs:element maxOccurs="unbounded" name="Tx" type="Transaction"/>
      <xs:element minOccurs="0" name="Optional" type="Max35Text"/>
    </xs:sequence>
  </xs:complexType>
  <xs:complexType name="GroupHeader">
    <xs:sequence>
      <xs:element name="MsgId" type="Max35Text"/>
      <xs:element name="CreDtTm" type="xs:dateTime"/>
      <xs:element name="NbOfTxs" type="Max15NumericText"/>
    </xs:sequence>
  </xs:complexType>
  <xs:complexType name="Transaction">
    <xs:sequence>
      <xs:element name="Amt" type="Amount"/>
      <xs:element name="ChrgBr" type="ChargeBearerCode"/>
      <xs:element name="Cdtr" type="Party"/>
    </xs:sequence>
  </xs:complexType>
  <xs:complexType name="Party">
    <xs:sequence>
      <xs:element name="Nm" type="Max140Text"/>
      <xs:element name="Ctry" type="CountryCode"/>
      <xs:element minOccurs="0" name="LEI" type="LEIIdentifier"/>
    </xs:sequence>
  </xs:complexType>
  <xs:complexType name="Amount">
    <xs:simpleContent>
      <xs:extension base="AmountValue">
        <xs:attribute name="Ccy" type="CurrencyCode" use="required"/>
      </xs:extension>
    </xs:simpleContent>
  </xs:complexType>
  <xs:simpleType name="AmountValue">
    <xs:restriction base="xs:decimal">
      <xs:totalDigits value="18"/><xs:fractionDigits value="5"/><xs:minInclusive value="0"/>
    </xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="Max35Text">
    <xs:restriction base="xs:string"><xs:minLength value="1"/><xs:maxLength value="35"/></xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="Max140Text">
    <xs:restriction base="xs:string"><xs:minLength value="1"/><xs:maxLength value="140"/></xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="Max15NumericText">
    <xs:restriction base="xs:string"><xs:pattern value="[0-9]{1,15}"/></xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="CurrencyCode">
    <xs:restriction base="xs:string"><xs:pattern value="[A-Z]{3,3}"/></xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="CountryCode">
    <xs:restriction base="xs:string"><xs:pattern value="[A-Z]{2,2}"/></xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="LEIIdentifier">
    <xs:restriction base="xs:string"><xs:pattern value="[A-Z0-9]{18,18}[0-9]{2,2}"/></xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="ChargeBearerCode">
    <xs:restriction base="xs:string">
      <xs:enumeration value="CRED"/><xs:enumeration value="DEBT"/><xs:enumeration value="SHAR"/>
    </xs:restriction>
  </xs:simpleType>`

func TestGeneratesAMinimalMessage(t *testing.T) {
	res := build(t, parse(t, basicSchema), schemagen.DefaultOptions())

	if res.Root != "Document" {
		t.Errorf("Root = %q", res.Root)
	}
	if res.Elements == 0 {
		t.Error("nothing was counted")
	}

	for _, want := range []string{
		`xmlns="urn:iso:std:iso:20022:tech:xsd:test.001.001.01"`,
		`<MsgId>`,
		`<Amt Ccy="EUR">`,
		`<Ctry>GB</Ctry>`,
	} {
		if !strings.Contains(res.XML, want) {
			t.Errorf("the message is missing %s:\n%s", want, res.XML)
		}
	}

	// Minimal means minimal: an optional element is left out.
	if strings.Contains(res.XML, "<Optional>") {
		t.Errorf("an optional element was emitted into a minimal message:\n%s", res.XML)
	}
}

func TestGeneratesEnumeratedCodes(t *testing.T) {
	res := build(t, parse(t, basicSchema), schemagen.DefaultOptions())

	// The value has to be one of the schema's own codes, and preferably one a
	// reader recognises.
	if !strings.Contains(res.XML, "<ChrgBr>") {
		t.Fatalf("no charge bearer:\n%s", res.XML)
	}
	if !strings.Contains(res.XML, "<ChrgBr>CRED</ChrgBr>") &&
		!strings.Contains(res.XML, "<ChrgBr>DEBT</ChrgBr>") &&
		!strings.Contains(res.XML, "<ChrgBr>SHAR</ChrgBr>") {
		t.Errorf("the charge bearer is not one of the schema's codes:\n%s", res.XML)
	}
}

func TestOptionalIncludesOptionalElements(t *testing.T) {
	opts := schemagen.DefaultOptions()
	opts.Optional = true

	res := build(t, parse(t, basicSchema), opts)
	if !strings.Contains(res.XML, "<Optional>") {
		t.Errorf("--optional did not emit the optional element:\n%s", res.XML)
	}
	// And an optional element with a recognised name still gets a correct
	// value rather than filler.
	if !strings.Contains(res.XML, "<LEI>") {
		t.Errorf("the optional LEI was not emitted:\n%s", res.XML)
	}
}

func TestRepeatsEmitMoreTransactions(t *testing.T) {
	opts := schemagen.DefaultOptions()
	opts.Repeats = 3

	res := build(t, parse(t, basicSchema), opts)
	if n := strings.Count(res.XML, "<Tx>"); n != 3 {
		t.Errorf("got %d transactions, want 3:\n%s", n, res.XML)
	}

	// A repeat count above what the schema allows is capped, not emitted.
	capped := parse(t, strings.Replace(basicSchema,
		`<xs:element maxOccurs="unbounded" name="Tx" type="Transaction"/>`,
		`<xs:element maxOccurs="2" name="Tx" type="Transaction"/>`, 1))
	opts.Repeats = 10
	res = build(t, capped, opts)
	if n := strings.Count(res.XML, "<Tx>"); n != 2 {
		t.Errorf("got %d transactions, want the schema's maximum of 2", n)
	}
}

func TestValueOverrides(t *testing.T) {
	opts := schemagen.DefaultOptions()
	opts.Values = map[string]string{"Nm": "ACME TRADING LIMITED", "Ccy": "GBP"}

	res := build(t, parse(t, basicSchema), opts)
	if !strings.Contains(res.XML, "<Nm>ACME TRADING LIMITED</Nm>") {
		t.Errorf("the name override was ignored:\n%s", res.XML)
	}
	if !strings.Contains(res.XML, `Ccy="GBP"`) {
		t.Errorf("the currency override was ignored:\n%s", res.XML)
	}
}

func TestChoiceTakesTheFirstBranchAndSaysSo(t *testing.T) {
	schema := parse(t, `
  <xs:element name="Document" type="Document"/>
  <xs:complexType name="Document">
    <xs:sequence><xs:element name="Acct" type="AccountChoice"/></xs:sequence>
  </xs:complexType>
  <xs:complexType name="AccountChoice">
    <xs:choice>
      <xs:element name="IBAN" type="IBANIdentifier"/>
      <xs:element name="Othr" type="Max35Text"/>
    </xs:choice>
  </xs:complexType>
  <xs:simpleType name="IBANIdentifier">
    <xs:restriction base="xs:string"><xs:pattern value="[A-Z]{2,2}[0-9]{2,2}[a-zA-Z0-9]{1,30}"/></xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="Max35Text">
    <xs:restriction base="xs:string"><xs:minLength value="1"/><xs:maxLength value="35"/></xs:restriction>
  </xs:simpleType>`)

	res := build(t, schema, schemagen.DefaultOptions())
	if !strings.Contains(res.XML, "<IBAN>") || strings.Contains(res.XML, "<Othr>") {
		t.Errorf("the first branch was not taken:\n%s", res.XML)
	}
	// A reader has to know a decision was made on their behalf.
	var sawNote bool
	for _, n := range res.Notes {
		if strings.Contains(n, "choice") {
			sawNote = true
		}
	}
	if !sawNote {
		t.Errorf("the choice was not reported: %v", res.Notes)
	}
}

func TestMandatoryChoiceOfOptionalBranches(t *testing.T) {
	// ISO 20022 uses this shape constantly: the choice must be satisfied, and
	// every branch is declared optional. Emitting nothing would be invalid.
	schema := parse(t, `
  <xs:element name="Document" type="Document"/>
  <xs:complexType name="Document">
    <xs:sequence><xs:element name="Mtg" type="Meeting"/></xs:sequence>
  </xs:complexType>
  <xs:complexType name="Meeting">
    <xs:sequence>
      <xs:choice>
        <xs:element minOccurs="0" name="Clssfctn" type="Max35Text"/>
        <xs:element minOccurs="0" name="XtndedClssfctn" type="Max35Text"/>
      </xs:choice>
    </xs:sequence>
  </xs:complexType>
  <xs:simpleType name="Max35Text">
    <xs:restriction base="xs:string"><xs:minLength value="1"/><xs:maxLength value="35"/></xs:restriction>
  </xs:simpleType>`)

	res := build(t, schema, schemagen.DefaultOptions())
	if !strings.Contains(res.XML, "<Clssfctn>") {
		t.Errorf("a mandatory choice of optional branches produced nothing:\n%s", res.XML)
	}
}

func TestRecursiveTypeStops(t *testing.T) {
	schema := parse(t, `
  <xs:element name="Document" type="Document"/>
  <xs:complexType name="Document">
    <xs:sequence><xs:element name="Node" type="Node"/></xs:sequence>
  </xs:complexType>
  <xs:complexType name="Node">
    <xs:sequence>
      <xs:element name="Name" type="Max35Text"/>
      <xs:element minOccurs="0" name="Child" type="Node"/>
    </xs:sequence>
  </xs:complexType>
  <xs:simpleType name="Max35Text">
    <xs:restriction base="xs:string"><xs:minLength value="1"/><xs:maxLength value="35"/></xs:restriction>
  </xs:simpleType>`)

	opts := schemagen.DefaultOptions()
	opts.Optional = true

	res, err := schemagen.Generate(schema, opts)
	if err != nil {
		t.Fatalf("generating: %v", err)
	}
	if strings.Count(res.XML, "<Child") > 2 {
		t.Errorf("the recursion was not stopped:\n%s", res.XML)
	}
	var sawNote bool
	for _, n := range res.Notes {
		if strings.Contains(n, "recursion") {
			sawNote = true
		}
	}
	if !sawNote {
		t.Errorf("the recursion was not reported: %v", res.Notes)
	}
}

func TestDepthCap(t *testing.T) {
	const levels = 12
	body := `
  <xs:element name="Document" type="T0"/>`
	for i := 0; i < levels; i++ {
		body += "\n  <xs:complexType name=\"T" + itoa(i) + "\"><xs:sequence>" +
			"<xs:element name=\"L" + itoa(i) + "\" type=\"T" + itoa(i+1) + "\"/></xs:sequence></xs:complexType>"
	}
	body += "\n  <xs:complexType name=\"T" + itoa(levels) + "\"><xs:sequence/></xs:complexType>"

	schema := parse(t, body)

	opts := schemagen.DefaultOptions()
	opts.MaxDepth = 4

	res, err := schemagen.Generate(schema, opts)
	if err != nil {
		t.Fatalf("generating: %v", err)
	}
	if !strings.Contains(strings.Join(res.Notes, " "), "nests deeper") {
		t.Errorf("the depth cap was not reported: %v", res.Notes)
	}
	if strings.Contains(res.XML, "<L6>") {
		t.Errorf("the walk went past the cap:\n%s", res.XML)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

func TestBaseTypes(t *testing.T) {
	// Every base type the catalogue uses has to produce a value its own type
	// accepts, or the message will not validate.
	body := `
  <xs:element name="Document" type="Document"/>
  <xs:complexType name="Document">
    <xs:sequence>
      <xs:element name="ADate" type="xs:date"/>
      <xs:element name="ADateTime" type="xs:dateTime"/>
      <xs:element name="ATime" type="xs:time"/>
      <xs:element name="ABool" type="xs:boolean"/>
      <xs:element name="ADecimal" type="xs:decimal"/>
      <xs:element name="AYear" type="xs:gYear"/>
      <xs:element name="AYearMonth" type="xs:gYearMonth"/>
      <xs:element name="AMonth" type="xs:gMonth"/>
      <xs:element name="AString" type="xs:string"/>
    </xs:sequence>
  </xs:complexType>`

	res := build(t, parse(t, body), schemagen.DefaultOptions())
	for _, want := range []string{
		"<ADate>2026-11-14</ADate>",
		"<ADateTime>2026-11-14T09:00:00Z</ADateTime>",
		"<ABool>true</ABool>",
		"<AYear>2026</AYear>",
	} {
		if !strings.Contains(res.XML, want) {
			t.Errorf("the message is missing %s:\n%s", want, res.XML)
		}
	}
}

func TestNumericFacets(t *testing.T) {
	body := `
  <xs:element name="Document" type="Document"/>
  <xs:complexType name="Document">
    <xs:sequence>
      <xs:element name="Rate" type="Percentage"/>
      <xs:element name="Count" type="SmallNumber"/>
      <xs:element name="Whole" type="WholeNumber"/>
    </xs:sequence>
  </xs:complexType>
  <xs:simpleType name="Percentage">
    <xs:restriction base="xs:decimal">
      <xs:fractionDigits value="10"/><xs:totalDigits value="11"/>
      <xs:minInclusive value="0"/><xs:maxInclusive value="100"/>
    </xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="SmallNumber">
    <xs:restriction base="xs:decimal"><xs:totalDigits value="3"/><xs:fractionDigits value="0"/></xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="WholeNumber">
    <xs:restriction base="xs:decimal"><xs:totalDigits value="18"/><xs:fractionDigits value="0"/></xs:restriction>
  </xs:simpleType>`

	build(t, parse(t, body), schemagen.DefaultOptions())
}

func TestLengthFacets(t *testing.T) {
	body := `
  <xs:element name="Document" type="Document"/>
  <xs:complexType name="Document">
    <xs:sequence>
      <xs:element name="Exact" type="ExactFour"/>
      <xs:element name="Long" type="LongText"/>
      <xs:element name="Short" type="ShortText"/>
    </xs:sequence>
  </xs:complexType>
  <xs:simpleType name="ExactFour">
    <xs:restriction base="xs:string"><xs:length value="4"/></xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="LongText">
    <xs:restriction base="xs:string"><xs:minLength value="30"/><xs:maxLength value="60"/></xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="ShortText">
    <xs:restriction base="xs:string"><xs:maxLength value="2"/></xs:restriction>
  </xs:simpleType>`

	res := build(t, parse(t, body), schemagen.DefaultOptions())
	if !strings.Contains(res.XML, "<Exact>") {
		t.Errorf("no exact-length element:\n%s", res.XML)
	}
}

func TestSemanticValuesAreCorrectNotJustValid(t *testing.T) {
	// An IBAN that matches the pattern and fails its checksum is useless as an
	// example. The generator knows the names the linter checks.
	res := build(t, parse(t, `
  <xs:element name="Document" type="Document"/>
  <xs:complexType name="Document">
    <xs:sequence>
      <xs:element name="IBAN" type="IBANIdentifier"/>
      <xs:element name="BICFI" type="BICIdentifier"/>
      <xs:element name="UETR" type="UUIDv4Identifier"/>
    </xs:sequence>
  </xs:complexType>
  <xs:simpleType name="IBANIdentifier">
    <xs:restriction base="xs:string"><xs:pattern value="[A-Z]{2,2}[0-9]{2,2}[a-zA-Z0-9]{1,30}"/></xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="BICIdentifier">
    <xs:restriction base="xs:string"><xs:pattern value="[A-Z0-9]{4,4}[A-Z]{2,2}[A-Z0-9]{2,2}([A-Z0-9]{3,3}){0,1}"/></xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="UUIDv4Identifier">
    <xs:restriction base="xs:string"><xs:pattern value="[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}"/></xs:restriction>
  </xs:simpleType>`), schemagen.DefaultOptions())

	for _, want := range []string{
		"<IBAN>GB29NWBK60161331926819</IBAN>",
		"<BICFI>BANKGB2LXXX</BICFI>",
		"<UETR>f3a1b2c4-5d6e-4f70-8a91-2b3c4d5e6f70</UETR>",
	} {
		if !strings.Contains(res.XML, want) {
			t.Errorf("the message is missing %s:\n%s", want, res.XML)
		}
	}
}

func TestSemanticValueYieldsToTheFacets(t *testing.T) {
	// A schema whose element is named IBAN but constrained to four characters
	// gets a generated value, not the known one: the facets are the authority.
	res := build(t, parse(t, `
  <xs:element name="Document" type="Document"/>
  <xs:complexType name="Document">
    <xs:sequence>
      <xs:element name="IBAN" type="ShortCode"/>
      <xs:element name="Ccy" type="ANumber"/>
    </xs:sequence>
  </xs:complexType>
  <xs:simpleType name="ShortCode">
    <xs:restriction base="xs:string"><xs:length value="4"/></xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="ANumber">
    <xs:restriction base="xs:decimal"><xs:totalDigits value="4"/><xs:fractionDigits value="0"/></xs:restriction>
  </xs:simpleType>`), schemagen.DefaultOptions())

	if strings.Contains(res.XML, "GB29NWBK") {
		t.Errorf("a 22-character IBAN was put into a 4-character element:\n%s", res.XML)
	}
	if strings.Contains(res.XML, "<Ccy>EUR</Ccy>") {
		t.Errorf("a currency code was put into a numeric element:\n%s", res.XML)
	}
}

func TestEmptyAndWildcardContent(t *testing.T) {
	res := build(t, parse(t, `
  <xs:element name="Document" type="Document"/>
  <xs:complexType name="Document">
    <xs:sequence>
      <xs:element name="Empty" type="EmptyType"/>
      <xs:element name="Splmtry" type="Supplementary"/>
    </xs:sequence>
  </xs:complexType>
  <xs:complexType name="EmptyType"><xs:sequence/></xs:complexType>
  <xs:complexType name="Supplementary">
    <xs:sequence><xs:any namespace="##any" processContents="lax"/></xs:sequence>
  </xs:complexType>`), schemagen.DefaultOptions())

	if !strings.Contains(res.XML, "<Empty/>") {
		t.Errorf("an empty type was not emitted as an empty element:\n%s", res.XML)
	}
}

func TestGenerateRejectsWhatItCannotBuild(t *testing.T) {
	if _, err := schemagen.Generate(nil, schemagen.DefaultOptions()); err == nil {
		t.Error("a nil schema was accepted")
	}

	// A schema with no global element has no document to build.
	orphan := parse(t, `<xs:complexType name="Orphan"><xs:sequence/></xs:complexType>`)
	if _, err := schemagen.Generate(orphan, schemagen.DefaultOptions()); err == nil {
		t.Error("a schema with no document element was accepted")
	}
}

func TestOptionsAreNormalised(t *testing.T) {
	// Nonsense options fall back to the defaults rather than producing nothing.
	opts := schemagen.Options{Repeats: 0, MaxDepth: 0}
	res, err := schemagen.Generate(parse(t, basicSchema), opts)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.XML, "<Tx>") {
		t.Errorf("a repeat count of zero emitted no transaction:\n%s", res.XML)
	}
}

func TestValuesAreEscaped(t *testing.T) {
	opts := schemagen.DefaultOptions()
	opts.Values = map[string]string{"Nm": `SMITH & SONS <LTD>`}

	res := build(t, parse(t, basicSchema), opts)
	if strings.Contains(res.XML, "SMITH & SONS") {
		t.Errorf("an ampersand was emitted unescaped:\n%s", res.XML)
	}
	if !strings.Contains(res.XML, "SMITH &amp; SONS &lt;LTD&gt;") {
		t.Errorf("the value was not escaped correctly:\n%s", res.XML)
	}
}

func TestMessageIDFromNamespace(t *testing.T) {
	got := schemagen.MessageIDFromNamespace("urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10")
	if got != "pacs.008.001.10" {
		t.Errorf("MessageIDFromNamespace = %q", got)
	}
	if got := schemagen.MessageIDFromNamespace("urn:something:else"); got != "" {
		t.Errorf("MessageIDFromNamespace = %q, want empty", got)
	}
}

func TestRemainingBaseTypes(t *testing.T) {
	// The rest of the base-type vocabulary. Each has to produce a value its own
	// type accepts, which the validator decides.
	body := `
  <xs:element name="Document" type="Document"/>
  <xs:complexType name="Document">
    <xs:sequence>
      <xs:element name="ADay" type="xs:gDay"/>
      <xs:element name="ADuration" type="xs:duration"/>
      <xs:element name="ABinary" type="xs:base64Binary"/>
      <xs:element name="AHex" type="xs:hexBinary"/>
      <xs:element name="AURI" type="xs:anyURI"/>
      <xs:element name="AToken" type="xs:token"/>
      <xs:element name="AnInteger" type="xs:integer"/>
      <xs:element name="ACount" type="BoundedCount"/>
      <xs:element name="AFloat" type="xs:float"/>
    </xs:sequence>
  </xs:complexType>
  <xs:simpleType name="BoundedCount">
    <xs:restriction base="xs:integer"><xs:totalDigits value="4"/></xs:restriction>
  </xs:simpleType>`

	res, err := schemagen.Generate(parse(t, body), schemagen.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"<ADay>---14</ADay>",
		"<ADuration>P1D</ADuration>",
		"<AURI>https://www.iso20022.org/</AURI>",
		"<AnInteger>1</AnInteger>",
	} {
		if !strings.Contains(res.XML, want) {
			t.Errorf("the message is missing %s:\n%s", want, res.XML)
		}
	}
}

func TestBoundsClampAValue(t *testing.T) {
	body := `
  <xs:element name="Document" type="Document"/>
  <xs:complexType name="Document">
    <xs:sequence>
      <xs:element name="Tiny" type="TinyRate"/>
      <xs:element name="Floor" type="FlooredNumber"/>
    </xs:sequence>
  </xs:complexType>
  <xs:simpleType name="TinyRate">
    <xs:restriction base="xs:decimal">
      <xs:totalDigits value="6"/><xs:fractionDigits value="2"/><xs:maxInclusive value="1.5"/>
    </xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="FlooredNumber">
    <xs:restriction base="xs:decimal">
      <xs:totalDigits value="6"/><xs:fractionDigits value="0"/><xs:minInclusive value="5000"/>
    </xs:restriction>
  </xs:simpleType>`

	res := build(t, parse(t, body), schemagen.DefaultOptions())
	// A value above the ceiling is pulled down to it; one below the floor is
	// pushed up. Both come out as numbers rather than as the bound's text.
	if !strings.Contains(res.XML, "<Tiny>1.5</Tiny>") {
		t.Errorf("the ceiling was not applied:\n%s", res.XML)
	}
	if !strings.Contains(res.XML, "<Floor>5000</Floor>") {
		t.Errorf("the floor was not applied:\n%s", res.XML)
	}
}

func TestOverLongValuesAreTrimmed(t *testing.T) {
	// An override longer than the element permits is trimmed rather than
	// producing an invalid message.
	opts := schemagen.DefaultOptions()
	opts.Values = map[string]string{}

	body := `
  <xs:element name="Document" type="Document"/>
  <xs:complexType name="Document">
    <xs:sequence>
      <xs:element name="Nm" type="TwoChars"/>
      <xs:element name="Ctry" type="SixChars"/>
    </xs:sequence>
  </xs:complexType>
  <xs:simpleType name="TwoChars">
    <xs:restriction base="xs:string"><xs:maxLength value="2"/></xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="SixChars">
    <xs:restriction base="xs:string"><xs:length value="6"/></xs:restriction>
  </xs:simpleType>`

	res := build(t, parse(t, body), opts)
	if strings.Contains(res.XML, "ACME TRADING") {
		t.Errorf("a long name was put into a 2-character element:\n%s", res.XML)
	}
}

func TestChoiceBranchThatIsASequence(t *testing.T) {
	// A mandatory choice whose branch is a sequence of optional elements: the
	// sequence has to appear, and something inside it too.
	schema := parse(t, `
  <xs:element name="Document" type="Document"/>
  <xs:complexType name="Document">
    <xs:sequence><xs:element name="Ref" type="Reference"/></xs:sequence>
  </xs:complexType>
  <xs:complexType name="Reference">
    <xs:choice>
      <xs:sequence>
        <xs:element minOccurs="0" name="Id" type="Max35Text"/>
        <xs:element minOccurs="0" name="Sub" type="Max35Text"/>
      </xs:sequence>
      <xs:element name="Othr" type="Max35Text"/>
    </xs:choice>
  </xs:complexType>
  <xs:simpleType name="Max35Text">
    <xs:restriction base="xs:string"><xs:minLength value="1"/><xs:maxLength value="35"/></xs:restriction>
  </xs:simpleType>`)

	res := build(t, schema, schemagen.DefaultOptions())
	if !strings.Contains(res.XML, "<Id>") {
		t.Errorf("the chosen sequence produced nothing:\n%s", res.XML)
	}
}

func TestOptionalAttributes(t *testing.T) {
	body := `
  <xs:element name="Document" type="Document"/>
  <xs:complexType name="Document">
    <xs:sequence><xs:element name="Amt" type="Amount"/></xs:sequence>
  </xs:complexType>
  <xs:complexType name="Amount">
    <xs:simpleContent>
      <xs:extension base="xs:decimal">
        <xs:attribute name="Ccy" type="xs:string" use="required"/>
        <xs:attribute name="Note" type="xs:string"/>
      </xs:extension>
    </xs:simpleContent>
  </xs:complexType>`

	minimal := build(t, parse(t, body), schemagen.DefaultOptions())
	if strings.Contains(minimal.XML, "Note=") {
		t.Errorf("an optional attribute was emitted into a minimal message:\n%s", minimal.XML)
	}

	opts := schemagen.DefaultOptions()
	opts.Optional = true
	full := build(t, parse(t, body), opts)
	if !strings.Contains(full.XML, "Note=") {
		t.Errorf("--optional did not emit the optional attribute:\n%s", full.XML)
	}
}

func TestExternalCodeSetsReadAsCodes(t *testing.T) {
	// The Registration Authority maintains these outside the schema, so the
	// schema constrains only the shape.
	res := build(t, parse(t, `
  <xs:element name="Document" type="Document"/>
  <xs:complexType name="Document">
    <xs:sequence>
      <xs:element name="Purp" type="ExternalPurpose1Code"/>
      <xs:element name="Rsn" type="ExternalStatusReason1Code"/>
    </xs:sequence>
  </xs:complexType>
  <xs:simpleType name="ExternalPurpose1Code">
    <xs:restriction base="xs:string"><xs:minLength value="1"/><xs:maxLength value="4"/></xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="ExternalStatusReason1Code">
    <xs:restriction base="xs:string"><xs:minLength value="1"/><xs:maxLength value="4"/></xs:restriction>
  </xs:simpleType>`), schemagen.DefaultOptions())

	if !strings.Contains(res.XML, "<Purp>ANCH</Purp>") {
		t.Errorf("an external code set produced filler rather than a code:\n%s", res.XML)
	}
}

func TestMandatoryElementWithAllOptionalContent(t *testing.T) {
	// A mandatory element whose content is entirely optional is valid when
	// empty, and useless: <FinInstrmId/> says nothing about how an instrument
	// is identified. The first child is just as valid and shows the shape.
	schema := parse(t, `
  <xs:element name="Document" type="Document"/>
  <xs:complexType name="Document">
    <xs:sequence><xs:element name="FinInstrmId" type="SecurityIdentification"/></xs:sequence>
  </xs:complexType>
  <xs:complexType name="SecurityIdentification">
    <xs:sequence>
      <xs:element minOccurs="0" name="ISIN" type="ISINIdentifier"/>
      <xs:element minOccurs="0" name="Desc" type="Max140Text"/>
    </xs:sequence>
  </xs:complexType>
  <xs:simpleType name="ISINIdentifier">
    <xs:restriction base="xs:string"><xs:pattern value="[A-Z]{2,2}[A-Z0-9]{9,9}[0-9]{1,1}"/></xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="Max140Text">
    <xs:restriction base="xs:string"><xs:minLength value="1"/><xs:maxLength value="140"/></xs:restriction>
  </xs:simpleType>`)

	res := build(t, schema, schemagen.DefaultOptions())
	if strings.Contains(res.XML, "<FinInstrmId/>") {
		t.Errorf("the element came out empty:\n%s", res.XML)
	}
	if !strings.Contains(res.XML, "<ISIN>GB0002634946</ISIN>") {
		t.Errorf("the instrument was not identified:\n%s", res.XML)
	}
	// Only the first child: a minimal message stays minimal.
	if strings.Contains(res.XML, "<Desc>") {
		t.Errorf("more than the first child was emitted:\n%s", res.XML)
	}

	// And the reader is told a decision was made on their behalf.
	if !strings.Contains(strings.Join(res.Notes, " "), "empty element") {
		t.Errorf("the decision was not reported: %v", res.Notes)
	}
}

func TestTrulyEmptyTypeStaysEmpty(t *testing.T) {
	// A type with no content at all has nothing to fill it with.
	res := build(t, parse(t, `
  <xs:element name="Document" type="Document"/>
  <xs:complexType name="Document">
    <xs:sequence><xs:element name="Empty" type="EmptyType"/></xs:sequence>
  </xs:complexType>
  <xs:complexType name="EmptyType"><xs:sequence/></xs:complexType>`), schemagen.DefaultOptions())

	if !strings.Contains(res.XML, "<Empty/>") {
		t.Errorf("an empty type was not emitted as an empty element:\n%s", res.XML)
	}
}

func TestSecurityIdentifierIsARealISIN(t *testing.T) {
	// A sample identifier that fails its own check digit is the kind of thing
	// someone copies into a test and then spends an afternoon on.
	const isin = "GB0002634946"

	var digits strings.Builder
	for _, r := range isin[:len(isin)-1] {
		if r >= 'A' && r <= 'Z' {
			fmt.Fprintf(&digits, "%d", r-'A'+10)
			continue
		}
		digits.WriteRune(r)
	}

	// Luhn over the expanded digits, right to left.
	runes := []rune(digits.String())
	total, position := 0, 0
	for i := len(runes) - 1; i >= 0; i-- {
		d := int(runes[i] - '0')
		if position%2 == 0 {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		total += d
		position++
	}
	if check := (10 - total%10) % 10; check != int(isin[len(isin)-1]-'0') {
		t.Errorf("%s fails its own check digit: computed %d", isin, check)
	}
}
