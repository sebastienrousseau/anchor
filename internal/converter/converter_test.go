// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package converter

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestXMLToJSONAndBack(t *testing.T) {
	originalXML := `<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">
  <FIToFICstmrCdtTrf>
    <GrpHdr>
      <MsgId>MSG-12345</MsgId>
      <CreDtTm>2026-08-23T12:00:00Z</CreDtTm>
    </GrpHdr>
    <CdtTrfTxInf>
      <PmtId>
        <EndToEndId>E2E-999</EndToEndId>
      </PmtId>
      <IntrBkSttlmAmt Ccy="EUR">5000.00</IntrBkSttlmAmt>
    </CdtTrfTxInf>
  </FIToFICstmrCdtTrf>
</Document>`

	jsonBytes, err := XMLToJSON([]byte(originalXML))
	if err != nil {
		t.Fatalf("XMLToJSON failed: %v", err)
	}

	jsonStr := string(jsonBytes)
	if !strings.Contains(jsonStr, "MSG-12345") {
		t.Errorf("JSON missing MsgId: %s", jsonStr)
	}
	if !strings.Contains(jsonStr, "@Ccy") {
		t.Errorf("JSON missing @Ccy attribute: %s", jsonStr)
	}

	xmlBytes, err := JSONToXML(jsonBytes)
	if err != nil {
		t.Fatalf("JSONToXML failed: %v", err)
	}

	xmlStr := string(xmlBytes)
	if !strings.Contains(xmlStr, "MSG-12345") {
		t.Errorf("XML missing MsgId: %s", xmlStr)
	}
}

func TestAttributeValuesAreEscapedAsXML(t *testing.T) {
	// Go's %q verb quotes for Go, not for XML: an ampersand would pass through
	// unescaped and a control character would come out as the literal text of a
	// Go escape. Found by fuzzing.
	jsonDoc := `{
  "Document": {
    "Amt": {
      "@Ccy": "A&B<C>\"D\"",
      "#text": "1.00"
    }
  }
}`
	xmlDoc, err := JSONToXML([]byte(jsonDoc))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(xmlDoc), `"A&B`) {
		t.Errorf("an ampersand was emitted unescaped:\n%s", xmlDoc)
	}

	// And the result has to be readable again, with the value intact. The JSON
	// encoder escapes &, < and > as \u sequences, so the comparison is made
	// after decoding rather than on the bytes.
	back, err := XMLToJSON(xmlDoc)
	if err != nil {
		t.Fatalf("the generated XML could not be read back: %v\n%s", err, xmlDoc)
	}

	var decoded map[string]map[string]map[string]string
	if err := json.Unmarshal(back, &decoded); err != nil {
		t.Fatalf("the round trip did not produce valid JSON: %v\n%s", err, back)
	}
	if got := decoded["Document"]["Amt"]["@Ccy"]; got != `A&B<C>"D"` {
		t.Errorf("the attribute value became %q", got)
	}
}

func TestNamesThatCannotBeXML(t *testing.T) {
	// Go's decoder accepts names the specification does not. AskISO must not
	// accept one it could never emit.
	if _, err := XMLToJSON([]byte("<A:0/>")); err == nil {
		t.Error("an element name starting with a digit was accepted")
	}
	if _, err := XMLToJSON([]byte(`<A 0="x"/>`)); err == nil {
		t.Error("an attribute name starting with a digit was accepted")
	}

	// And a JSON key that cannot become an element is refused rather than
	// producing XML nobody can parse.
	for _, doc := range []string{
		`{"0": {"#text": "x"}}`,
		`{"A": {"@1": "x"}}`,
		`{"A": {"B": {"9C": {"#text": "x"}}}}`,
		`{"A": [{"0": {"#text": "x"}}]}`,
		`{"": {"#text": "x"}}`,
	} {
		if _, err := JSONToXML([]byte(doc)); err == nil {
			t.Errorf("%s produced XML from an invalid name", doc)
		}
	}

	// Names that are unusual but legal still work: a namespace prefix, a
	// leading underscore, digits and hyphens after the first character.
	for _, doc := range []string{
		`{"ns:Document": {"#text": "x"}}`,
		`{"_Private": {"#text": "x"}}`,
		`{"A1-b.c": {"#text": "x"}}`,
		`{"Überweisung": {"#text": "x"}}`,
	} {
		if _, err := JSONToXML([]byte(doc)); err != nil {
			t.Errorf("%s was refused: %v", doc, err)
		}
	}
}
