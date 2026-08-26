// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package validator_test

import (
	"strings"
	"testing"

	"github.com/sebastienrousseau/askiso/internal/validator"
)

// The streaming path exists so a 39 MB statement does not have to be held in
// memory. It must reach the same verdict as the buffered one — including on
// the failures that happen before any transaction is read, which are the ones
// a subtree-releasing validator is most likely to get wrong.

func branchSchema(t *testing.T) string {
	t.Helper()
	return `<?xml version="1.0" encoding="UTF-8"?>
<xs:schema xmlns="urn:t" xmlns:xs="http://www.w3.org/2001/XMLSchema"
           elementFormDefault="qualified" targetNamespace="urn:t">
  <xs:element name="Document" type="Doc"/>
  <xs:complexType name="Doc">
    <xs:sequence>
      <xs:element name="Tx" type="Max4" maxOccurs="unbounded"/>
    </xs:sequence>
  </xs:complexType>
  <xs:simpleType name="Max4">
    <xs:restriction base="xs:string">
      <xs:maxLength value="4"/>
    </xs:restriction>
  </xs:simpleType>
</xs:schema>`
}

// A document in the wrong namespace is wrong before a single transaction is
// read, and the streaming path has to say so rather than reporting every
// element as unexpected.
func TestStreamingReportsAWrongNamespace(t *testing.T) {
	s := schemaFrom(t, branchSchema(t))

	body := `<?xml version="1.0" encoding="UTF-8"?><Document xmlns="urn:wrong"><Tx>a</Tx></Document>`
	res := validator.ValidateReader(strings.NewReader(body), s)
	if res.Valid {
		t.Fatal("a document in the wrong namespace was accepted")
	}
	var sawNS bool
	for _, e := range res.Errors {
		if e.Rule == "namespace" {
			sawNS = true
			if e.Actual != "urn:wrong" {
				t.Errorf("reported namespace %q, want urn:wrong", e.Actual)
			}
		}
	}
	if !sawNS {
		t.Errorf("the namespace rule is not named: %v", res.Errors)
	}
}

// A document with no namespace at all reports "(none)" rather than an empty
// string, so the message reads as a sentence.
func TestStreamingNamesAnAbsentNamespace(t *testing.T) {
	s := schemaFrom(t, branchSchema(t))

	body := `<?xml version="1.0" encoding="UTF-8"?><Document><Tx>a</Tx></Document>`
	res := validator.ValidateReader(strings.NewReader(body), s)
	for _, e := range res.Errors {
		if e.Rule == "namespace" && e.Actual != "(none)" {
			t.Errorf("an absent namespace reported as %q, want (none)", e.Actual)
		}
	}
}

// Errors found while releasing a subtree and errors found in the surrounding
// structure are recorded at different moments. They must still come back in
// document order, because that is the order the buffered path reports and the
// order a reader scanning a file expects.
func TestStreamingReportsErrorsInDocumentOrder(t *testing.T) {
	s := schemaFrom(t, branchSchema(t))

	body := `<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:t">
  <Tx>toolong1</Tx>
  <Tx>ok</Tx>
  <Tx>toolong2</Tx>
</Document>`

	res := validator.ValidateReader(strings.NewReader(body), s)
	if len(res.Errors) < 2 {
		t.Fatalf("expected both over-length values to fail: %v", res.Errors)
	}
	for i := 1; i < len(res.Errors); i++ {
		if res.Errors[i].Line < res.Errors[i-1].Line {
			t.Errorf("errors are out of document order: %d after %d",
				res.Errors[i].Line, res.Errors[i-1].Line)
		}
	}
}

// Malformed XML is a finding, not a crash: the streaming path has to report it
// the way the buffered path does rather than returning half a verdict.
func TestStreamingOnMalformedXML(t *testing.T) {
	s := schemaFrom(t, branchSchema(t))
	res := validator.ValidateReader(strings.NewReader(`<Document><unclosed>`), s)
	if res.Valid {
		t.Error("malformed XML was reported as valid")
	}
	if len(res.Errors) == 0 {
		t.Error("malformed XML produced no diagnostic")
	}
}
