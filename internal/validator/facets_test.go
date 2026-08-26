// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package validator_test

import (
	"strings"
	"testing"

	"github.com/sebastienrousseau/askiso/internal/validator"
	"github.com/sebastienrousseau/askiso/internal/xsd"
)

// Facets and cardinality are where a validator is most easily wrong in a way
// nobody notices: a length rule off by one, or an "expected" message that
// describes a different constraint from the one that failed. ISO 20022 leans
// heavily on both, so each branch is worth a case of its own.

func schemaFrom(t *testing.T, body string) *xsd.Schema {
	t.Helper()
	s, err := xsd.Parse(strings.NewReader(body))
	if err != nil {
		t.Fatalf("parsing the schema: %v", err)
	}
	return s
}

func wrap(children, types string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<xs:schema xmlns="urn:t" xmlns:xs="http://www.w3.org/2001/XMLSchema"
           elementFormDefault="qualified" targetNamespace="urn:t">
  <xs:element name="Document" type="Doc"/>
  <xs:complexType name="Doc">
    <xs:sequence>
` + children + `
    </xs:sequence>
  </xs:complexType>
` + types + `
</xs:schema>`
}

func doc(body string) []byte {
	return []byte(`<?xml version="1.0" encoding="UTF-8"?>` +
		`<Document xmlns="urn:t">` + body + `</Document>`)
}

// An exact length is a different rule from a minimum and a maximum that happen
// to coincide, and it reports differently.
func TestExactLengthFacet(t *testing.T) {
	s := schemaFrom(t, wrap(
		`      <xs:element name="Code" type="Fixed4"/>`,
		`  <xs:simpleType name="Fixed4">
    <xs:restriction base="xs:string">
      <xs:length value="4"/>
    </xs:restriction>
  </xs:simpleType>`))

	if res := validator.Validate(doc("<Code>ABCD</Code>"), s); !res.Valid {
		t.Errorf("a value of exactly the right length was rejected: %v", res.Errors)
	}

	res := validator.Validate(doc("<Code>ABC</Code>"), s)
	if res.Valid {
		t.Fatal("a value of the wrong length was accepted")
	}
	var sawLength bool
	for _, e := range res.Errors {
		if e.Rule == "length" {
			sawLength = true
			if e.Expected != "4" || e.Actual != "3" {
				t.Errorf("expected 4 got 3, reported expected %q actual %q", e.Expected, e.Actual)
			}
		}
	}
	if !sawLength {
		t.Errorf("the length rule is not named in %v", res.Errors)
	}
}

// Length counts characters, not bytes. An ISO 20022 name field holding
// accented text would otherwise fail a rule it satisfies.
func TestLengthCountsRunesNotBytes(t *testing.T) {
	s := schemaFrom(t, wrap(
		`      <xs:element name="Nm" type="Max4"/>`,
		`  <xs:simpleType name="Max4">
    <xs:restriction base="xs:string">
      <xs:maxLength value="4"/>
    </xs:restriction>
  </xs:simpleType>`))

	// Four characters, seven bytes.
	if res := validator.Validate(doc("<Nm>Zürïç</Nm>"), s); res.Valid {
		t.Error("five characters passed a maxLength of four")
	}
	if res := validator.Validate(doc("<Nm>Zürï</Nm>"), s); !res.Valid {
		t.Errorf("four characters failed a maxLength of four: %v", res.Errors)
	}
}

// A pattern the engine cannot compile must not fail an otherwise valid
// document. Refusing a message because AskISO could not read the rule would be
// the worst possible failure mode for a validator.
func TestAnUnusablePatternDoesNotFailTheDocument(t *testing.T) {
	s := schemaFrom(t, wrap(
		`      <xs:element name="Ref" type="Weird"/>`,
		`  <xs:simpleType name="Weird">
    <xs:restriction base="xs:string">
      <xs:pattern value="[a-z"/>
    </xs:restriction>
  </xs:simpleType>`))

	if res := validator.Validate(doc("<Ref>anything</Ref>"), s); !res.Valid {
		t.Errorf("an uncompilable pattern rejected a document: %v", res.Errors)
	}
}

// "at least N" is the message for an unbounded element, and it is a different
// sentence from the bounded case.
func TestUnboundedCardinalityMessage(t *testing.T) {
	s := schemaFrom(t, wrap(
		`      <xs:element name="Tx" type="Max4" minOccurs="2" maxOccurs="unbounded"/>`,
		`  <xs:simpleType name="Max4">
    <xs:restriction base="xs:string">
      <xs:maxLength value="4"/>
    </xs:restriction>
  </xs:simpleType>`))

	res := validator.Validate(doc("<Tx>a</Tx>"), s)
	if res.Valid {
		t.Fatal("one occurrence satisfied minOccurs=2")
	}
	var msg string
	for _, e := range res.Errors {
		msg += e.Expected + " " + e.Message + " "
	}
	if !strings.Contains(msg, "at least") {
		t.Errorf("the unbounded case does not say \"at least\": %v", res.Errors)
	}
}

func TestBoundedCardinalityMessage(t *testing.T) {
	s := schemaFrom(t, wrap(
		`      <xs:element name="Tx" type="Max4" minOccurs="2" maxOccurs="3"/>`,
		`  <xs:simpleType name="Max4">
    <xs:restriction base="xs:string">
      <xs:maxLength value="4"/>
    </xs:restriction>
  </xs:simpleType>`))

	res := validator.Validate(doc("<Tx>a</Tx>"), s)
	if res.Valid {
		t.Fatal("one occurrence satisfied minOccurs=2")
	}
	var msg string
	for _, e := range res.Errors {
		msg += e.Expected + " " + e.Message + " "
	}
	if !strings.Contains(msg, "2 to 3") {
		t.Errorf("the bounded range is not reported: %v", res.Errors)
	}
}

// An optional wildcard is satisfied by nothing at all, which is the branch a
// document without extension content takes.
func TestOptionalWildcardAcceptsAnEmptyDocument(t *testing.T) {
	s := schemaFrom(t, wrap(
		`      <xs:element name="Id" type="Max4"/>
      <xs:any namespace="##any" processContents="lax" minOccurs="0"/>`,
		`  <xs:simpleType name="Max4">
    <xs:restriction base="xs:string">
      <xs:maxLength value="4"/>
    </xs:restriction>
  </xs:simpleType>`))

	if res := validator.Validate(doc("<Id>a</Id>"), s); !res.Valid {
		t.Errorf("an absent optional wildcard was treated as missing content: %v", res.Errors)
	}
	// And it still accepts something when present.
	if res := validator.Validate(doc("<Id>a</Id><Ext/>"), s); !res.Valid {
		t.Errorf("the wildcard rejected extension content: %v", res.Errors)
	}
}

// A required wildcard is not satisfied by nothing.
func TestRequiredWildcardNeedsContent(t *testing.T) {
	s := schemaFrom(t, wrap(
		`      <xs:element name="Id" type="Max4"/>
      <xs:any namespace="##any" processContents="lax"/>`,
		`  <xs:simpleType name="Max4">
    <xs:restriction base="xs:string">
      <xs:maxLength value="4"/>
    </xs:restriction>
  </xs:simpleType>`))

	if res := validator.Validate(doc("<Id>a</Id>"), s); res.Valid {
		t.Error("a required wildcard was satisfied by nothing")
	}
}
