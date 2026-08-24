// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package schemagen_test

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/sebastienrousseau/askiso/internal/schemagen"
)

// The corpus is every distinct pattern in the published standard, collected
// from all 4,746 schemas. A sampler that cannot produce a matching value for
// one of them cannot generate that message, so this is the bar.
func TestSamplerHandlesTheWholeCorpus(t *testing.T) {
	path := filepath.Join("..", "validator", "testdata", "patterns.txt")
	f, err := os.Open(path)
	if err != nil {
		t.Skipf("no pattern corpus at %s", path)
	}
	defer func() { _ = f.Close() }()

	var checked, failed int
	seen := map[string]bool{}

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for sc.Scan() {
		pattern := strings.TrimRight(sc.Text(), " \t")
		if pattern == "" || seen[pattern] {
			continue
		}
		seen[pattern] = true
		checked++

		got, err := schemagen.SamplePattern(pattern, 1)
		if err != nil {
			failed++
			t.Errorf("pattern %q could not be sampled: %v", pattern, err)
			continue
		}
		if got == "" {
			failed++
			t.Errorf("pattern %q produced an empty value", pattern)
			continue
		}

		// The value has to match the pattern it came from. XSD patterns are
		// implicitly anchored, so the check anchors them.
		re, err := regexp.Compile("^(?:" + pattern + ")$")
		if err != nil {
			// A pattern Go's engine will not compile cannot be verified here;
			// the validator has its own handling for those.
			t.Logf("skipping %q: Go cannot compile it (%v)", pattern, err)
			continue
		}
		if !re.MatchString(got) {
			failed++
			t.Errorf("pattern %q produced %q, which does not match it", pattern, got)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}

	if checked == 0 {
		t.Fatal("the corpus is empty")
	}
	t.Logf("sampled %d distinct pattern(s), %d failure(s)", checked, failed)
}

func TestSamplerRespectsMinimumLength(t *testing.T) {
	cases := []struct {
		pattern string
		minimum int
	}{
		{`[0-9]{1,15}`, 10},
		{`[a-zA-Z0-9]{1,30}`, 25},
		{`[0-9a-zA-Z/\-\?:\(\)\.,'\+ ]{1,35}`, 35},
		{`[A-Z]{2,2}[0-9]{2,2}[a-zA-Z0-9]{1,30}`, 22},
	}
	for _, tc := range cases {
		got, err := schemagen.SamplePattern(tc.pattern, tc.minimum)
		if err != nil {
			t.Errorf("%s: %v", tc.pattern, err)
			continue
		}
		if len([]rune(got)) < tc.minimum {
			t.Errorf("%s produced %q (%d runes), want at least %d",
				tc.pattern, got, len([]rune(got)), tc.minimum)
		}
		re := regexp.MustCompile("^(?:" + tc.pattern + ")$")
		if !re.MatchString(got) {
			t.Errorf("%s produced %q, which does not match", tc.pattern, got)
		}
	}
}

func TestSamplerPrefersReadableCharacters(t *testing.T) {
	// A value made of slashes and question marks is valid and useless.
	got, err := schemagen.SamplePattern(`[0-9a-zA-Z/\-\?:\(\)\.,'\+ ]{1,35}`, 4)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range got {
		alnum := (r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')
		if !alnum {
			t.Errorf("the sample %q contains punctuation where a letter would do", got)
			break
		}
	}
}

func TestSamplerRejectsWhatItCannotParse(t *testing.T) {
	for _, pattern := range []string{
		`[a-z`,   // unclosed class
		`(abc`,   // unclosed group
		`abc)`,   // unbalanced
		`a{1,`,   // unclosed quantifier
		`a{x}`,   // non-numeric quantifier
		`a{1,x}`, // non-numeric bound
		`[]`,     // empty class
		`abc\`,   // trailing backslash
		`[a\`,    // trailing backslash in a class
	} {
		if _, err := schemagen.SamplePattern(pattern, 1); err == nil {
			t.Errorf("%q was accepted", pattern)
		}
	}
}

func TestSamplerEscapesAndClasses(t *testing.T) {
	cases := map[string]string{
		// Escapes outside a character class.
		`\d{4}`: `^\d{4}$`,
		`\w{3}`: `^\w{3}$`,
		`a\.b`:  `^a\.b$`,
		`x\ny`:  "^x\ny$",
		`x\ry`:  "^x\ry$",
		`x\ty`:  "^x\ty$",
		`a\s b`: `^a\s b$`,
		// A wildcard.
		`.{3}`: `^.{3}$`,
		// A negated class.
		`[^0-9]{2}`: `^[^0-9]{2}$`,
		// A class of punctuation only, where no preferred character fits.
		`[/:\?]{2}`: `^[/:\?]{2}$`,
		// A hyphen as the last member of a class is a literal.
		`[abc-]{2}`: `^[abc-]{2}$`,
		// Escapes inside a class.
		`[\d\-\.]{3}`: `^[\d\-\.]{3}$`,
		`[\n\r\t]{1}`: "^[\n\r\t]{1}$",
		`[\s]{2}`:     `^[\s]{2}$`,
		`[\w]{2}`:     `^[\w]{2}$`,
		// Quantifiers.
		`ab?c`:  `^ab?c$`,
		`ab*c`:  `^ab*c$`,
		`ab+c`:  `^ab+c$`,
		`a{2,}`: `^a{2,}$`,
		// Alternation inside a group.
		`(AB|CD)[0-9]{2}`: `^(AB|CD)[0-9]{2}$`,
	}

	for pattern, verify := range cases {
		got, err := schemagen.SamplePattern(pattern, 1)
		if err != nil {
			t.Errorf("%s: %v", pattern, err)
			continue
		}
		re, err := regexp.Compile(verify)
		if err != nil {
			t.Fatalf("the verification pattern %q is wrong: %v", verify, err)
		}
		if !re.MatchString(got) {
			t.Errorf("%s produced %q, which does not match", pattern, got)
		}
	}
}

func TestSamplerOnAPatternThatMatchesOnlyEmpty(t *testing.T) {
	// Legal, and useless as a sample. It must not loop or error.
	got, err := schemagen.SamplePattern(`a?`, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got != "" && got != "a" {
		t.Errorf("SamplePattern = %q", got)
	}
}

func TestSamplerCannotLengthenAFixedPattern(t *testing.T) {
	// A pattern of fixed length cannot satisfy a longer minimum; asking for one
	// returns what the pattern allows rather than looping.
	got, err := schemagen.SamplePattern(`[A-Z]{3,3}`, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Errorf("SamplePattern = %q, want three characters", got)
	}
}
