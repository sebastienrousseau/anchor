// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package xsd_test

import (
	"strings"
	"testing"

	"github.com/sebastienrousseau/askiso/internal/xsd"
)

// A schema is a file the user downloaded. It is not hostile, but it is not
// verified either, and a parser that panics on a truncated or malformed one
// takes the whole tool down. These targets assert the only property that
// matters here: the parser returns, either a schema or an error.
//
//	go test ./internal/xsd/ -fuzz FuzzParse -fuzztime 60s

func FuzzParse(f *testing.F) {
	f.Add(`<?xml version="1.0"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:a" xmlns="urn:a">
  <xs:element name="Document" type="Document"/>
  <xs:complexType name="Document">
    <xs:sequence><xs:element name="MsgId" type="Max35Text"/></xs:sequence>
  </xs:complexType>
  <xs:simpleType name="Max35Text">
    <xs:restriction base="xs:string"><xs:maxLength value="35"/></xs:restriction>
  </xs:simpleType>
</xs:schema>`)

	// The shapes most likely to break a hand-written parser: unbalanced tags,
	// unknown constructs, absurd occurrence counts, and deep nesting.
	f.Add(`<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"><xs:element name="A"`)
	f.Add(`<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="u">
  <xs:complexType name="T"><xs:choice maxOccurs="99999999999999999999">
    <xs:element name="A" minOccurs="-5" type="xs:string"/></xs:choice></xs:complexType></xs:schema>`)
	f.Add(`<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="u">
  <xs:simpleType name="T"><xs:restriction base="xs:string">
    <xs:maxLength value="not-a-number"/><xs:pattern value="[["/>
  </xs:restriction></xs:simpleType></xs:schema>`)
	f.Add(`<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="u">
  <xs:complexType name="T"><xs:sequence><xs:any/></xs:sequence></xs:complexType>
  <xs:complexType name="U"><xs:simpleContent><xs:extension base="xs:decimal">
    <xs:attribute name="Ccy" type="xs:string" use="required"/>
  </xs:extension></xs:simpleContent></xs:complexType></xs:schema>`)
	f.Add("")
	f.Add("<")

	f.Fuzz(func(t *testing.T, data string) {
		schema, err := xsd.Parse(strings.NewReader(data))
		if err != nil {
			if schema != nil {
				t.Fatalf("Parse returned both a schema and an error: %v", err)
			}
			return
		}
		if schema == nil {
			t.Fatal("Parse returned neither a schema nor an error")
		}

		// Everything a caller may reach for has to be safe on whatever parsed.
		if _, ok := schema.RootElement(); ok {
			for name := range schema.ComplexTypes {
				_, _ = schema.ResolveComplex(name)
			}
			for name := range schema.SimpleTypes {
				_, _ = schema.ResolveSimple(name)
				_, _ = schema.EffectiveFacets(name)
			}
		}
		if len(schema.ElementOrder) > len(schema.Elements) {
			t.Fatalf("ElementOrder has %d entries for %d elements",
				len(schema.ElementOrder), len(schema.Elements))
		}
	})
}
