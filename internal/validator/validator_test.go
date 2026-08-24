// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package validator_test

import (
	"strings"
	"testing"

	"github.com/sebastienrousseau/askiso/internal/validator"
	"github.com/sebastienrousseau/askiso/internal/xsd"
)

// A self-contained schema exercising every construct the catalogue uses, so
// these tests need no downloaded specification.
const testSchema = `<?xml version="1.0" encoding="UTF-8"?>
<xs:schema xmlns="urn:test" xmlns:xs="http://www.w3.org/2001/XMLSchema"
           elementFormDefault="qualified" targetNamespace="urn:test">
  <xs:element name="Document" type="Document"/>

  <xs:complexType name="Document">
    <xs:sequence>
      <xs:element name="Hdr" type="Header"/>
      <xs:element name="Tx" type="Transaction" maxOccurs="unbounded"/>
      <xs:element name="Note" type="Max35Text" minOccurs="0"/>
    </xs:sequence>
  </xs:complexType>

  <xs:complexType name="Header">
    <xs:sequence>
      <xs:element name="MsgId" type="Max35Text"/>
      <xs:element name="CreDtTm" type="ISODateTime"/>
      <xs:element name="NbOfTxs" type="Max15NumericText" minOccurs="0"/>
    </xs:sequence>
  </xs:complexType>

  <xs:complexType name="Transaction">
    <xs:sequence>
      <xs:element name="Amt" type="ActiveCurrencyAndAmount"/>
      <xs:element name="Acct" type="AccountChoice"/>
      <xs:element name="Sts" type="StatusCode" minOccurs="0"/>
    </xs:sequence>
  </xs:complexType>

  <xs:complexType name="AccountChoice">
    <xs:choice>
      <xs:element name="IBAN" type="IBANIdentifier"/>
      <xs:element name="Othr" type="Max35Text"/>
    </xs:choice>
  </xs:complexType>

  <xs:complexType name="ActiveCurrencyAndAmount">
    <xs:simpleContent>
      <xs:extension base="AmountSimpleType">
        <xs:attribute name="Ccy" type="CurrencyCode" use="required"/>
      </xs:extension>
    </xs:simpleContent>
  </xs:complexType>

  <xs:simpleType name="AmountSimpleType">
    <xs:restriction base="xs:decimal">
      <xs:minInclusive value="0"/>
      <xs:fractionDigits value="5"/>
      <xs:totalDigits value="18"/>
    </xs:restriction>
  </xs:simpleType>

  <xs:simpleType name="CurrencyCode">
    <xs:restriction base="xs:string">
      <xs:pattern value="[A-Z]{3,3}"/>
    </xs:restriction>
  </xs:simpleType>

  <xs:simpleType name="IBANIdentifier">
    <xs:restriction base="xs:string">
      <xs:pattern value="[A-Z]{2,2}[0-9]{2,2}[a-zA-Z0-9]{1,30}"/>
    </xs:restriction>
  </xs:simpleType>

  <xs:simpleType name="Max35Text">
    <xs:restriction base="xs:string">
      <xs:minLength value="1"/>
      <xs:maxLength value="35"/>
    </xs:restriction>
  </xs:simpleType>

  <xs:simpleType name="Max15NumericText">
    <xs:restriction base="xs:string">
      <xs:pattern value="[0-9]{1,15}"/>
    </xs:restriction>
  </xs:simpleType>

  <xs:simpleType name="ISODateTime">
    <xs:restriction base="xs:dateTime"/>
  </xs:simpleType>

  <xs:simpleType name="StatusCode">
    <xs:restriction base="xs:string">
      <xs:enumeration value="ACCP"/>
      <xs:enumeration value="RJCT"/>
      <xs:enumeration value="PDNG"/>
    </xs:restriction>
  </xs:simpleType>
</xs:schema>`

const validDoc = `<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:test">
  <Hdr>
    <MsgId>MSG-001</MsgId>
    <CreDtTm>2026-08-23T10:00:00Z</CreDtTm>
    <NbOfTxs>1</NbOfTxs>
  </Hdr>
  <Tx>
    <Amt Ccy="EUR">25000.00</Amt>
    <Acct>
      <IBAN>DE89370400440532013000</IBAN>
    </Acct>
    <Sts>ACCP</Sts>
  </Tx>
</Document>`

func mustSchema(t *testing.T) *xsd.Schema {
	t.Helper()
	s, err := xsd.Parse(strings.NewReader(testSchema))
	if err != nil {
		t.Fatalf("parsing the test schema: %v", err)
	}
	return s
}

func TestValidDocumentPasses(t *testing.T) {
	res := validator.Validate([]byte(validDoc), mustSchema(t))
	if !res.Valid {
		for _, e := range res.Errors {
			t.Errorf("unexpected error: %s", e)
		}
		t.Fatal("the reference document should validate")
	}
}

// Each case breaks the valid document in exactly one way. The rule field is
// asserted, so a check that fires for the wrong reason still fails.
func TestMutationsAreCaught(t *testing.T) {
	cases := []struct {
		name string
		from string
		to   string
		rule string
	}{
		{"missing mandatory element", "<MsgId>MSG-001</MsgId>", "", "cardinality"},
		{"unexpected element", "<Sts>ACCP</Sts>", "<Sts>ACCP</Sts><Bogus>x</Bogus>", "content model"},
		{"elements out of order", "<MsgId>MSG-001</MsgId>\n    <CreDtTm>2026-08-23T10:00:00Z</CreDtTm>",
			"<CreDtTm>2026-08-23T10:00:00Z</CreDtTm>\n    <MsgId>MSG-001</MsgId>", "cardinality"},
		{"value not in enumeration", "<Sts>ACCP</Sts>", "<Sts>NOPE</Sts>", "enumeration"},
		{"pattern violation", `Ccy="EUR"`, `Ccy="EURO"`, "pattern"},
		{"pattern violation on IBAN", "DE89370400440532013000", "12345", "pattern"},
		{"too many fraction digits", "25000.00", "25000.123456", "fractionDigits"},
		{"below minInclusive", "25000.00", "-1.00", "minInclusive"},
		{"not a decimal", "25000.00", "not-a-number", "type"},
		{"bad dateTime", "2026-08-23T10:00:00Z", "2026-13-45T99:00:00Z", "type"},
		{"impossible date", "2026-08-23T10:00:00Z", "2026-02-30T10:00:00Z", "type"},
		{"missing required attribute", ` Ccy="EUR"`, "", "required attribute"},
		{"exceeds maxLength", "<MsgId>MSG-001</MsgId>",
			"<MsgId>" + strings.Repeat("X", 36) + "</MsgId>", "maxLength"},
		{"below minLength", "<MsgId>MSG-001</MsgId>", "<MsgId></MsgId>", "minLength"},
		{"child under a simple element", "<MsgId>MSG-001</MsgId>", "<MsgId><Nested>x</Nested></MsgId>", "content model"},
		{"neither branch of a choice", "<IBAN>DE89370400440532013000</IBAN>", "<Wrong>x</Wrong>", "choice"},
	}

	schema := mustSchema(t)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := strings.Replace(validDoc, tc.from, tc.to, 1)
			if doc == validDoc {
				t.Fatalf("mutation did not apply; %q not found in the document", tc.from)
			}

			res := validator.Validate([]byte(doc), schema)
			if res.Valid {
				t.Fatalf("mutation was not caught")
			}

			var rules []string
			for _, e := range res.Errors {
				rules = append(rules, e.Rule)
				if e.Rule == tc.rule {
					if e.Line <= 0 {
						t.Errorf("error should carry a line number: %+v", e)
					}
					if e.Path == "" {
						t.Errorf("error should carry a path: %+v", e)
					}
					return
				}
			}
			t.Errorf("expected rule %q, got %v\n  first: %s", tc.rule, rules, res.Errors[0])
		})
	}
}

func TestRepeatedElementsAllowed(t *testing.T) {
	twoTx := strings.Replace(validDoc, "</Tx>\n</Document>",
		"</Tx>\n  <Tx><Amt Ccy=\"GBP\">10.00</Amt><Acct><Othr>ACC-1</Othr></Acct></Tx>\n</Document>", 1)
	if twoTx == validDoc {
		t.Fatal("mutation did not apply")
	}
	res := validator.Validate([]byte(twoTx), mustSchema(t))
	if !res.Valid {
		t.Errorf("maxOccurs=unbounded should permit a second <Tx>: %v", res.Errors)
	}
}

func TestOptionalElementsMayBeOmitted(t *testing.T) {
	doc := strings.Replace(validDoc, "    <NbOfTxs>1</NbOfTxs>\n", "", 1)
	doc = strings.Replace(doc, "    <Sts>ACCP</Sts>\n", "", 1)
	res := validator.Validate([]byte(doc), mustSchema(t))
	if !res.Valid {
		t.Errorf("optional elements should be omittable: %v", res.Errors)
	}
}

func TestBothChoiceBranchesAccepted(t *testing.T) {
	doc := strings.Replace(validDoc,
		"<IBAN>DE89370400440532013000</IBAN>", "<Othr>ACCOUNT-1</Othr>", 1)
	res := validator.Validate([]byte(doc), mustSchema(t))
	if !res.Valid {
		t.Errorf("the Othr branch should be accepted: %v", res.Errors)
	}
}

func TestWrongNamespaceRejected(t *testing.T) {
	doc := strings.Replace(validDoc, `xmlns="urn:test"`, `xmlns="urn:wrong"`, 1)
	res := validator.Validate([]byte(doc), mustSchema(t))
	if res.Valid {
		t.Fatal("a document in the wrong namespace should be rejected")
	}
	if res.Errors[0].Rule != "namespace" {
		t.Errorf("rule = %q, want namespace", res.Errors[0].Rule)
	}
}

func TestUnknownRootRejected(t *testing.T) {
	doc := `<?xml version="1.0"?><Wrong xmlns="urn:test"/>`
	res := validator.Validate([]byte(doc), mustSchema(t))
	if res.Valid {
		t.Fatal("an undeclared root element should be rejected")
	}
	if res.Errors[0].Rule != "root element" {
		t.Errorf("rule = %q, want 'root element'", res.Errors[0].Rule)
	}
}

func TestMalformedXMLReported(t *testing.T) {
	for _, doc := range []string{
		`<Document xmlns="urn:test"><Hdr>`,
		`not xml at all`,
		``,
	} {
		res := validator.Validate([]byte(doc), mustSchema(t))
		if res.Valid {
			t.Errorf("malformed input should be rejected: %q", doc)
		}
	}
}

// Entity references must not be expanded: a schema validator is a natural place
// to feed hostile XML.
func TestEntityReferenceRejected(t *testing.T) {
	doc := `<?xml version="1.0"?>
<!DOCTYPE Document [<!ENTITY xxe SYSTEM "file:///etc/passwd">]>
<Document xmlns="urn:test"><Hdr><MsgId>&xxe;</MsgId></Hdr></Document>`
	res := validator.Validate([]byte(doc), mustSchema(t))
	if res.Valid {
		t.Fatal("a document with an external entity reference should be rejected")
	}
	if res.Errors[0].Rule != "well-formedness" {
		t.Errorf("rule = %q, want well-formedness", res.Errors[0].Rule)
	}
}

func TestErrorCascadeIsBounded(t *testing.T) {
	var b strings.Builder
	b.WriteString(`<Document xmlns="urn:test"><Hdr><MsgId>x</MsgId><CreDtTm>2026-08-23T10:00:00Z</CreDtTm></Hdr>`)
	for i := 0; i < 500; i++ {
		b.WriteString(`<Tx><Amt Ccy="BAD">x</Amt><Acct><IBAN>nope</IBAN></Acct></Tx>`)
	}
	b.WriteString(`</Document>`)

	res := validator.Validate([]byte(b.String()), mustSchema(t))
	if res.Valid {
		t.Fatal("expected errors")
	}
	if len(res.Errors) > 100 {
		t.Errorf("error list should be capped, got %d", len(res.Errors))
	}
}

func TestErrorFormatting(t *testing.T) {
	doc := strings.Replace(validDoc, "<Sts>ACCP</Sts>", "<Sts>NOPE</Sts>", 1)
	res := validator.Validate([]byte(doc), mustSchema(t))
	if res.Valid {
		t.Fatal("expected an error")
	}
	e := res.Errors[0]
	if !strings.Contains(e.String(), "Sts") {
		t.Errorf("formatted error should name the element: %s", e)
	}
	if e.Expected == "" || e.Actual == "" {
		t.Errorf("error should carry expected and actual: %+v", e)
	}
}
