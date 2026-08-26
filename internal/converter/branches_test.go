// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package converter_test

import (
	"strings"
	"testing"

	"github.com/sebastienrousseau/askiso/internal/converter"
)

// Go's XML decoder is more permissive than the specification. A name it
// accepts on the way in but that cannot be re-emitted would turn a round trip
// into silent corruption, so the parser refuses it up front. Fuzzing found
// this for element names; attributes take the same path and deserve the same
// guarantee.
func TestParseRejectsAnInvalidAttributeName(t *testing.T) {
	_, err := converter.Parse([]byte(`<Document A:0="x"/>`))
	if err == nil {
		t.Fatal("an unemittable attribute name was accepted")
	}
	if !strings.Contains(err.Error(), "attribute name") {
		t.Errorf("the error does not say what was wrong: %v", err)
	}
}

func TestParseRejectsAnInvalidElementName(t *testing.T) {
	if _, err := converter.Parse([]byte(`<A:0/>`)); err == nil {
		t.Error("an unemittable element name was accepted")
	}
}

// An element carrying both an attribute and text cannot be a bare string in
// JSON, so it becomes an object with "@attr" and "#text". A plain leaf stays a
// string, and an empty one an empty string — an ISO 20022 amount is the case
// that matters, since the currency is an attribute and the value is the text.
func TestJSONShapesLeavesByWhatTheyCarry(t *testing.T) {
	out, err := converter.XMLToJSON([]byte(
		`<Document><Amt Ccy="EUR">100.00</Amt><Full>x</Full><Empty/></Document>`))
	if err != nil {
		t.Fatal(err)
	}

	s := string(out)
	for _, want := range []string{`"@Ccy": "EUR"`, `"#text": "100.00"`, `"Full": "x"`, `"Empty": ""`} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %s in:\n%s", want, s)
		}
	}
	// Only the element with both gets the #text form.
	if strings.Count(s, `"#text"`) != 1 {
		t.Errorf("expected exactly one #text:\n%s", s)
	}
}
