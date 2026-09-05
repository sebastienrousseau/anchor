// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package validator

import (
	"strings"
	"testing"

	"github.com/sebastienrousseau/askiso/internal/xsd"
)

func TestStreamingDeclarationLookupFailures(t *testing.T) {
	schema := &xsd.Schema{
		Elements: map[string]*xsd.Element{"Document": {Name: "Document", Type: "DocumentType"}},
		ComplexTypes: map[string]*xsd.ComplexType{
			"DocumentType": {Name: "DocumentType", Content: &xsd.Sequence{Particles: []xsd.Particle{
				&xsd.Element{Name: "Known", Type: "UnknownType"},
			}}},
		},
	}
	for _, path := range []string{"", "/Missing", "/Document/Known/Child", "/Document/Other"} {
		if _, ok := declarationAt(schema, path); ok {
			t.Errorf("declarationAt(%q) unexpectedly succeeded", path)
		}
	}
	if isSpaceOnly([]byte(" \t\nX")) {
		t.Fatal("non-whitespace byte was classified as space")
	}
}

func TestLineTrackerReportsOffsetsOlderThanWindow(t *testing.T) {
	tracker := newLineTracker(strings.NewReader(""))
	for i := 0; i < lineWindow+1; i++ {
		tracker.push(int64(i + 1))
	}
	line, column := tracker.at(0)
	if line != 0 || column != 1 {
		t.Fatalf("old offset = %d:%d, want 0:1", line, column)
	}
}

func TestValidatorDefensiveNumericAndDateBranches(t *testing.T) {
	badMin, badMax := "not-a-number", "also-not-a-number"
	total := 1
	v := &validation{schema: &xsd.Schema{}, res: &Result{}}
	v.numericFacets(xsd.Facets{TotalDigits: &total, MinInclusive: &badMin, MaxInclusive: &badMax}, "decimal", "not-a-number", &node{Name: "Amount"}, "/Amount")
	if len(v.res.Errors) == 0 {
		t.Fatal("totalDigits should still be checked before an unparsable number returns")
	}
	if realDate("short") || realDateTime("short") {
		t.Fatal("short date values must be rejected")
	}
	if optional(nil) {
		t.Fatal("an unknown particle must not be optional")
	}
	max := "10"
	v.numericFacets(xsd.Facets{MaxInclusive: &max}, "decimal", "11", &node{Name: "Amount"}, "/Amount")
	if len(v.res.Errors) < 2 {
		t.Fatal("maxInclusive violation was not reported")
	}
	capped := &validation{schema: &xsd.Schema{}, res: &Result{Errors: make([]Error, maxErrors)}}
	capped.value("string", "anything", &node{Name: "Value"}, "/Value")
	if len(capped.res.Errors) != maxErrors {
		t.Fatal("value validation did not stop at the diagnostic cap")
	}
}

func TestRegexAtomDefensiveBranches(t *testing.T) {
	for _, tc := range []struct {
		s    string
		i    int
		want int
	}{
		{"", 0, 0},
		{"[abc", 0, 0},
		{`\`, 0, 0},
		{"(", 0, 0},
	} {
		if got := atomLen(tc.s, tc.i); got != tc.want {
			t.Errorf("atomLen(%q, %d) = %d, want %d", tc.s, tc.i, got, tc.want)
		}
	}
	for _, tc := range []struct {
		p    string
		end  int
		want int
	}{
		{"", 0, -1},
		{"unclosed)", 9, -1},
		{"missing]", 8, -1},
	} {
		if got := atomStartIndex(tc.p, tc.end); got != tc.want {
			t.Errorf("atomStartIndex(%q, %d) = %d, want %d", tc.p, tc.end, got, tc.want)
		}
	}
	if width := atomWidth("(|)", 3); width != 0 {
		t.Fatalf("alternating group width = %d, want unknown", width)
	}
	if width := atomWidth("([)", 3); width != 0 {
		t.Fatalf("malformed group width = %d, want unknown", width)
	}
	if got := atomStartIndex(`\)`, 2); got != -1 {
		t.Fatalf("escaped closing parenthesis start = %d", got)
	}
	if got := joinNames([]string{"A", "B", "C"}); got != "A, B, C" {
		t.Fatalf("joinNames = %q", got)
	}
}
