// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package validator

import (
	"bufio"
	"os"
	"strings"
	"testing"
)

// Every pattern the catalogue uses must compile, or documents would be rejected
// for a tooling limitation rather than a real fault.
func TestCatalogPatternsCompile(t *testing.T) {
	f, err := os.Open("testdata/patterns.txt")
	if err != nil {
		t.Skip("no pattern corpus")
	}
	defer func() { _ = f.Close() }()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<16), 1<<20)
	var n, bad int
	for sc.Scan() {
		p := strings.TrimSpace(sc.Text())
		if p == "" {
			continue
		}
		n++
		if _, err := compilePattern(p); err != nil {
			bad++
			t.Errorf("pattern failed to compile: %q", p)
		}
	}
	if n == 0 {
		t.Fatal("pattern corpus is empty")
	}
	t.Logf("compiled %d/%d catalogue patterns", n-bad, n)
}

func TestPatternAnchoring(t *testing.T) {
	m, err := compilePattern(`[A-Z]{3,3}`)
	if err != nil {
		t.Fatal(err)
	}
	if !m.match("EUR") {
		t.Error("EUR should match [A-Z]{3,3}")
	}
	// XSD patterns match the entire value, so a longer string must not pass.
	for _, bad := range []string{"EURO", "XEUR", "eur", "EU"} {
		if m.match(bad) {
			t.Errorf("%q should not match [A-Z]{3,3}", bad)
		}
	}
}

// A bound above Go's repeat cap is relaxed in the expression and reinstated as
// a length limit, so the effective constraint still holds.
func TestOversizedRepeatKeepsItsBound(t *testing.T) {
	m, err := compilePattern(`([0-9A-F][0-9A-F]){1,10000}`)
	if err != nil {
		t.Fatalf("oversized repeat should still compile: %v", err)
	}
	if m.maxRunes != 20000 {
		t.Errorf("maxRunes = %d, want 20000 (10000 repeats of a 2-char atom)", m.maxRunes)
	}
	if !m.match("00FF") {
		t.Error("a short hex string should match")
	}
	if m.match("0") {
		t.Error("an odd-length hex string should not match")
	}
	if m.match(strings.Repeat("AB", 10001)) {
		t.Error("a value past the original bound should be rejected")
	}
	if !m.match(strings.Repeat("AB", 10000)) {
		t.Error("a value exactly at the original bound should be accepted")
	}
}

func TestSingleCharAtomBound(t *testing.T) {
	m, err := compilePattern(`[0-9a-zA-Z]{1,2048}`)
	if err != nil {
		t.Fatal(err)
	}
	if m.maxRunes != 2048 {
		t.Errorf("maxRunes = %d, want 2048", m.maxRunes)
	}
	if !m.match(strings.Repeat("a", 2048)) {
		t.Error("a value at the bound should be accepted")
	}
	if m.match(strings.Repeat("a", 2049)) {
		t.Error("a value past the bound should be rejected")
	}
}

func TestAtomWidth(t *testing.T) {
	cases := []struct {
		pattern string
		want    int
	}{
		{`([0-9A-F][0-9A-F]){1,10000}`, 2},
		{`[0-9a-zA-Z]{1,2048}`, 1},
		{`(ab){1,2000}`, 2},
		{`(a|b){1,2000}`, 0}, // alternation: not a fixed width
		{`(a*){1,2000}`, 0},  // nested quantifier: not a fixed width
	}
	for _, tc := range cases {
		loc := repeatRe.FindStringIndex(tc.pattern)
		if loc == nil {
			t.Fatalf("no repeat found in %q", tc.pattern)
		}
		if got := atomWidth(tc.pattern, loc[0]); got != tc.want {
			t.Errorf("atomWidth(%q) = %d, want %d", tc.pattern, got, tc.want)
		}
	}
}

func TestUncompilablePatternIsIgnored(t *testing.T) {
	if _, err := compilePattern(`[unterminated`); err == nil {
		t.Error("a malformed pattern should report an error")
	}
}
