// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package schemagen

import (
	"strings"
	"testing"

	"github.com/sebastienrousseau/askiso/internal/xsd"
)

// The helpers below are reachable through Generate only in combinations no real
// schema produces. They are still the parts most likely to be wrong, so they
// are tested where they live.

func intp(n int) *int       { return &n }
func strp(s string) *string { return &s }

func TestClampLength(t *testing.T) {
	cases := []struct {
		name  string
		value string
		f     xsd.Facets
		want  string
	}{
		{"exact length trims", "ABCDEF", xsd.Facets{Length: intp(3)}, "ABC"},
		{"exact length pads", "AB", xsd.Facets{Length: intp(4)}, "ABAA"},
		{"exact length fits", "ABC", xsd.Facets{Length: intp(3)}, "ABC"},
		{"maximum trims", "ABCDEF", xsd.Facets{MaxLength: intp(2)}, "AB"},
		{"minimum pads", "AB", xsd.Facets{MinLength: intp(5)}, "ABAAA"},
		{"no facets, empty value", "", xsd.Facets{}, "A"},
		{"no facets, real value", "ASKISO", xsd.Facets{}, "ASKISO"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := clampLength(tc.value, tc.f); got != tc.want {
				t.Errorf("clampLength = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNormaliseBase(t *testing.T) {
	if got := normaliseBase("xs:decimal"); got != "decimal" {
		t.Errorf("normaliseBase = %q", got)
	}
	if got := normaliseBase("decimal"); got != "decimal" {
		t.Errorf("normaliseBase = %q", got)
	}
}

func TestPreferredCode(t *testing.T) {
	// A code a reader recognises beats the first alphabetically.
	if got := preferredCode([]string{"ZZZZ", "SHAR", "AAAA"}); got != "SHAR" {
		t.Errorf("preferredCode = %q, want SHAR", got)
	}
	// With nothing recognised, the first is as good as any.
	if got := preferredCode([]string{"ZZZZ", "AAAA"}); got != "ZZZZ" {
		t.Errorf("preferredCode = %q", got)
	}
}

func TestTrimNumber(t *testing.T) {
	if got := trimNumber(100); got != "100" {
		t.Errorf("trimNumber = %q", got)
	}
	if got := trimNumber(1.5); got != "1.5" {
		t.Errorf("trimNumber = %q", got)
	}
}

func TestApplyBoundsOnANonNumber(t *testing.T) {
	// A value that is not a number is returned unchanged rather than becoming
	// zero.
	if got := applyBounds("not a number", xsd.Facets{MinInclusive: strp("1")}); got != "not a number" {
		t.Errorf("applyBounds = %q", got)
	}
	// A bound that is not a number is ignored.
	if got := applyBounds("5", xsd.Facets{MinInclusive: strp("x"), MaxInclusive: strp("y")}); got != "5" {
		t.Errorf("applyBounds = %q", got)
	}
}

func TestIntegerValue(t *testing.T) {
	if got := integerValue(xsd.Facets{}); got != "1" {
		t.Errorf("integerValue = %q", got)
	}
	if got := integerValue(xsd.Facets{TotalDigits: intp(4)}); got != "1" {
		t.Errorf("integerValue = %q", got)
	}
}

func TestDecimalValueWithTightDigits(t *testing.T) {
	// A type with fewer total digits than the default whole part needs has to
	// shrink rather than overflow.
	got := decimalValue(xsd.Facets{TotalDigits: intp(2), FractionDigits: intp(1)})
	if len(got) > 4 {
		t.Errorf("decimalValue = %q, which is too long for two total digits", got)
	}
	// Zero fraction digits produce a whole number.
	if got := decimalValue(xsd.Facets{TotalDigits: intp(6), FractionDigits: intp(0)}); strings.Contains(got, ".") {
		t.Errorf("decimalValue = %q, want no decimal point", got)
	}
	// A fraction wider than the total is impossible; the value still has to be
	// a number.
	if got := decimalValue(xsd.Facets{TotalDigits: intp(1), FractionDigits: intp(5)}); got == "" {
		t.Error("decimalValue produced nothing")
	}
}

func TestStringValueWithoutAPattern(t *testing.T) {
	// A long minimum with no pattern is filled with readable text rather than
	// one repeated character.
	got, err := stringValue(xsd.Facets{MinLength: intp(40), MaxLength: intp(60)})
	if err != nil {
		t.Fatal(err)
	}
	if len([]rune(got)) < 40 {
		t.Errorf("stringValue = %q (%d runes), want at least 40", got, len([]rune(got)))
	}
	if !strings.Contains(got, "ASKISO") {
		t.Errorf("stringValue = %q, want something readable", got)
	}
}

func TestStringValueWithAnUnparseablePattern(t *testing.T) {
	if _, err := stringValue(xsd.Facets{Pattern: []string{"[unclosed"}}); err == nil {
		t.Error("an unparseable pattern was accepted")
	}
}

func TestPickFallsBack(t *testing.T) {
	// A class with nothing preferred in it still yields a character.
	cls := &charClass{ranges: []runeRange{{'#', '$'}}}
	if got := cls.pick(); got != '#' {
		t.Errorf("pick = %q", string(got))
	}
	// A class whose only members are unprintable falls back to the first.
	low := &charClass{ranges: []runeRange{{0x01, 0x02}}}
	if got := low.pick(); got != 0x01 {
		t.Errorf("pick = %q", string(got))
	}
	// A negated class that excludes everything preferred still yields
	// something.
	neg := &charClass{negated: true, ranges: []runeRange{{0x20, 0x7E}}}
	if got := neg.pick(); got == 0 {
		t.Error("pick produced nothing for a negated class")
	}
}

func TestPatternAcceptsHandlesEveryNodeKind(t *testing.T) {
	cases := []struct {
		pattern, value string
		want           bool
	}{
		{`[A-Z]{3}`, "ABC", true},
		{`[A-Z]{3}`, "AB", false},
		{`AB|CD`, "CD", true},
		{`AB|CD`, "EF", false},
		{`(AB)+`, "ABAB", true},
		{`(AB)+`, "ABA", false},
		{`A.C`, "ABC", true},
		{`A?B`, "B", true},
		{`A*`, "", true},
		{`[^0-9]+`, "ABC", true},
		{`[^0-9]+`, "A1C", false},
		{`[unclosed`, "x", false},
	}
	for _, tc := range cases {
		if got := patternAccepts(tc.pattern, tc.value); got != tc.want {
			t.Errorf("patternAccepts(%q, %q) = %v, want %v", tc.pattern, tc.value, got, tc.want)
		}
	}
}

func TestSemanticValueRejectsAMismatch(t *testing.T) {
	// A recognised name on a type the value does not fit falls through.
	if _, ok := semanticValue("IBAN", "xs:string", xsd.Facets{MaxLength: intp(4)}); ok {
		t.Error("a 22-character IBAN was accepted for a 4-character element")
	}
	if _, ok := semanticValue("Ccy", "xs:decimal", xsd.Facets{}); ok {
		t.Error("a currency code was accepted for a numeric element")
	}
	if _, ok := semanticValue("NotAKnownName", "xs:string", xsd.Facets{}); ok {
		t.Error("an unrecognised name produced a value")
	}
	if _, ok := semanticValue("Ctry", "xs:string", xsd.Facets{Pattern: []string{`[0-9]{2}`}}); ok {
		t.Error("a country code was accepted against a numeric pattern")
	}
	if _, ok := semanticValue("Ctry", "xs:string", xsd.Facets{MinLength: intp(10)}); ok {
		t.Error("a two-character country code satisfied a ten-character minimum")
	}
	if _, ok := semanticValue("Ctry", "xs:string", xsd.Facets{Length: intp(3)}); ok {
		t.Error("a two-character country code satisfied an exact length of three")
	}

	// And one that does fit is used.
	if got, ok := semanticValue("Ctry", "xs:string", xsd.Facets{Pattern: []string{`[A-Z]{2,2}`}}); !ok || got != "GB" {
		t.Errorf("semanticValue = %q, %v", got, ok)
	}
}

func TestMaxHelper(t *testing.T) {
	if max(1, 2) != 2 || max(3, 2) != 3 {
		t.Error("max is wrong")
	}
}

func TestPeekAtTheEnd(t *testing.T) {
	p := &patternParser{src: []rune("")}
	if p.peek() != 0 {
		t.Error("peek past the end should be zero")
	}
}

func TestExternalCodePlaceholder(t *testing.T) {
	// A type named "External...Code" is a code set the Registration Authority
	// maintains outside the schema: the schema constrains the shape only. A
	// single letter satisfies it and reads as nothing.
	if !isExternalCodeType("ExternalInvestigationType1Code") {
		t.Error("an external code type was not recognised")
	}
	if isExternalCodeType("ChargeBearerType1Code") {
		t.Error("an enumerated type was treated as external")
	}
	if isExternalCodeType("Max35Text") {
		t.Error("a text type was treated as a code")
	}

	cases := []struct {
		name string
		f    xsd.Facets
		want string
	}{
		{"no bounds", xsd.Facets{}, "ANCH"},
		{"exact length", xsd.Facets{Length: intp(6)}, "ANCHAN"},
		{"short maximum", xsd.Facets{MaxLength: intp(2)}, "AN"},
		{"long minimum", xsd.Facets{MinLength: intp(6)}, "ANCHAN"},
		{"zero length", xsd.Facets{Length: intp(0)}, "A"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := codePlaceholder(tc.f); got != tc.want {
				t.Errorf("codePlaceholder = %q, want %q", got, tc.want)
			}
		})
	}
}
