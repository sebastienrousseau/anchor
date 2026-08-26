// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package validator_test

import (
	"strings"
	"testing"

	"github.com/sebastienrousseau/askiso/internal/validator"
	"github.com/sebastienrousseau/askiso/internal/xsd"
)

// The validator is the part of AskISO most likely to be pointed at a file
// nobody vetted: a message that arrived over a wire, or one an editor is
// half-way through writing. It must always return a verdict rather than
// panicking, and its verdict must be internally consistent.
//
//	go test ./internal/validator/ -fuzz FuzzValidate -fuzztime 60s

const fuzzSchema = `<?xml version="1.0"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"
           xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10"
           targetNamespace="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10"
           elementFormDefault="qualified">
  <xs:element name="Document" type="Document"/>
  <xs:complexType name="Document">
    <xs:sequence>
      <xs:element name="CstmrCdtTrf" type="CustomerCreditTransfer"/>
    </xs:sequence>
  </xs:complexType>
  <!-- The real ISO 20022 shape: a document element, one wrapper, then the
       repeating transactions. The streaming validator releases at that third
       level, so the fixture has to have it. -->
  <xs:complexType name="CustomerCreditTransfer">
    <xs:sequence>
      <xs:element name="GrpHdr" type="GroupHeader"/>
      <xs:element maxOccurs="unbounded" name="Tx" type="Transaction"/>
    </xs:sequence>
  </xs:complexType>
  <xs:complexType name="GroupHeader">
    <xs:sequence>
      <xs:element name="MsgId" type="Max35Text"/>
      <xs:element minOccurs="0" name="CreDtTm" type="xs:dateTime"/>
    </xs:sequence>
  </xs:complexType>
  <xs:complexType name="Transaction">
    <xs:sequence>
      <xs:element name="Amt" type="Amount"/>
      <xs:choice>
        <xs:element name="IBAN" type="IBANIdentifier"/>
        <xs:element name="Othr" type="Max35Text"/>
      </xs:choice>
      <xs:element minOccurs="0" name="ChrgBr" type="ChargeBearerCode"/>
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
  <xs:simpleType name="CurrencyCode">
    <xs:restriction base="xs:string"><xs:pattern value="[A-Z]{3,3}"/></xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="IBANIdentifier">
    <xs:restriction base="xs:string"><xs:pattern value="[A-Z]{2,2}[0-9]{2,2}[a-zA-Z0-9]{1,30}"/></xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="ChargeBearerCode">
    <xs:restriction base="xs:string">
      <xs:enumeration value="DEBT"/><xs:enumeration value="CRED"/><xs:enumeration value="SHAR"/>
    </xs:restriction>
  </xs:simpleType>
</xs:schema>`

func FuzzValidate(f *testing.F) {
	schema, err := xsd.Parse(strings.NewReader(fuzzSchema))
	if err != nil {
		f.Fatalf("the fuzz schema does not parse: %v", err)
	}

	f.Add(`<?xml version="1.0"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">
  <CstmrCdtTrf>
    <GrpHdr><MsgId>MSG-1</MsgId></GrpHdr>
    <Tx><Amt Ccy="EUR">25000.00</Amt><IBAN>GB29NWBK60161331926819</IBAN><ChrgBr>SHAR</ChrgBr></Tx>
  </CstmrCdtTrf>
</Document>`)

	// The shapes that have broken validators before: wrong order, missing
	// mandatory elements, unexpected children, empty values, unclosed tags, a
	// namespace that does not match, and text where structure is expected.
	f.Add(`<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10"><CstmrCdtTrf><Tx/><GrpHdr/></CstmrCdtTrf></Document>`)
	f.Add(`<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10"></Document>`)
	f.Add(`<Document xmlns="urn:wrong"><GrpHdr/></Document>`)
	f.Add(`<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10"><CstmrCdtTrf>
  <GrpHdr><MsgId></MsgId></GrpHdr><Tx><Amt>x</Amt><Othr>a</Othr></Tx></CstmrCdtTrf></Document>`)
	f.Add(`<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10"><CstmrCdtTrf><GrpHdr>`)
	f.Add(`<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">text</Document>`)
	f.Add("")
	f.Add("<!DOCTYPE x [<!ENTITY e SYSTEM \"file:///etc/passwd\">]><Document>&e;</Document>")

	f.Fuzz(func(t *testing.T, data string) {
		res := validator.Validate([]byte(data), schema)
		if res == nil {
			t.Fatal("Validate returned nil")
		}

		// The verdict and the error list must agree; a caller reads one or the
		// other and they cannot disagree.
		if res.Valid && len(res.Errors) > 0 {
			t.Fatalf("valid with %d error(s): %+v", len(res.Errors), res.Errors)
		}
		if !res.Valid && len(res.Errors) == 0 {
			t.Fatal("invalid with no errors to explain why")
		}

		// Every error has to be reportable: a message a user can read, and a
		// position an editor can point at.
		for _, e := range res.Errors {
			if e.Message == "" {
				t.Fatalf("an error carries no message: %+v", e)
			}
			if e.Line < 0 || e.Column < 0 {
				t.Fatalf("an error carries a negative position: %+v", e)
			}
		}
	})
}
