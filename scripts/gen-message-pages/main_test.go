// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebastienrousseau/askiso/internal/registry"
	"github.com/sebastienrousseau/askiso/internal/translator"
)

func testRegistry(t *testing.T) *registry.Registry {
	t.Helper()
	reg, err := registry.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return reg
}

// The pages exist to be indexed, so the front matter is not decoration: a
// missing title or description is a page that ranks for nothing.
func TestPageCarriesTheFrontMatterSearchNeeds(t *testing.T) {
	reg := testRegistry(t)
	m, ok := reg.Lookup("pacs.008.001.10")
	if !ok {
		t.Fatal("pacs.008.001.10 is not in the registry")
	}

	page := buildPage(reg, m, []string{"pacs.008.001.09", "pacs.008.001.10"},
		nil, false, true, "2026-08-25")

	if !strings.HasPrefix(page, "---\n") {
		t.Fatal("the page does not open with front matter")
	}
	for _, key := range []string{
		"title:", "description:", "keywords:", "layout:", "date:", "headline:", "lead:",
	} {
		if !strings.Contains(page, key) {
			t.Errorf("front matter is missing %s", key)
		}
	}
	if !strings.Contains(page, `"pacs.008.001.10 — ISO 20022 Payments Clearing and Settlement message"`) {
		t.Error("the title does not name the message and its business area")
	}
}

// The whole premise is that no specification content is redistributed. A
// generator that started describing what messages mean would break it silently
// across 2,845 pages at once.
func TestPagePointsAtTheRegistrationAuthorityRatherThanReproducingIt(t *testing.T) {
	reg := testRegistry(t)
	m, ok := reg.Lookup("pacs.008.001.10")
	if !ok {
		t.Fatal("pacs.008.001.10 is not in the registry")
	}

	page := buildPage(reg, m, []string{"pacs.008.001.10"}, nil, false, true, "2026-08-25")

	if !strings.Contains(page, "AskIso does not reproduce the specification") {
		t.Error("the page does not state that the specification is not reproduced")
	}
	if !strings.Contains(page, "iso20022.org") {
		t.Error("the page does not link to the Registration Authority")
	}
	if !strings.Contains(page, "iso20022.org/message-set/") {
		t.Error("the page does not carry a download link for the message set")
	}
}

func TestVersionListMarksThePageYouAreOn(t *testing.T) {
	reg := testRegistry(t)
	m, _ := reg.Lookup("pacs.008.001.10")

	page := buildPage(reg, m,
		[]string{"pacs.008.001.09", "pacs.008.001.10", "pacs.008.001.11"},
		nil, false, true, "2026-08-25")

	if !strings.Contains(page, "[`pacs.008.001.10`](/messages/pacs.008.001.10/) — this page") {
		t.Error("the current version is not marked in the version list")
	}
	if !strings.Contains(page, "3 versions of this definition are published") {
		t.Error("the version count is wrong or missing")
	}
	if !strings.Contains(page, "[`pacs.008.001.09`](/messages/pacs.008.001.09/)") {
		t.Error("a sibling version is not linked")
	}
}

// A single message set must not read "published in message set".
func TestMessageSetCountReadsAsEnglish(t *testing.T) {
	reg := testRegistry(t)
	m, _ := reg.Lookup("pacs.008.001.10")

	page := buildPage(reg, m, []string{"pacs.008.001.10"}, nil, false, true, "2026-08-25")

	if strings.Contains(page, "published in message set") {
		t.Error("the message set count is missing from the sentence")
	}
	if !strings.Contains(page, "published in 1 message set.") {
		t.Errorf("expected a singular count; got:\n%s", extract(page, "Where to get"))
	}
}

func TestMTEquivalenceAppearsOnlyWhenThereIsOne(t *testing.T) {
	reg := testRegistry(t)
	m, _ := reg.Lookup("pacs.008.001.10")

	with := buildPage(reg, m, []string{"pacs.008.001.10"},
		[]translator.Mapping{{
			MTCode: "MT103", MTTitle: "Single Customer Credit Transfer",
			MXCode: "pacs.008.001.10", Description: "Direct customer payment instruction.",
		}}, true, true, "2026-08-25")

	if !strings.Contains(with, "## SWIFT MT equivalence") {
		t.Error("the MT section is missing when a mapping exists")
	}
	if !strings.Contains(with, "**MT103** — Single Customer Credit Transfer") {
		t.Error("the MT mapping does not carry its title")
	}
	if !strings.Contains(with, "askiso translate payment.mt103") {
		t.Error("the MT section does not show the command")
	}

	without := buildPage(reg, m, []string{"pacs.008.001.10"}, nil, false, true, "2026-08-25")
	if strings.Contains(without, "## SWIFT MT equivalence") {
		t.Error("the MT section appears when no mapping exists")
	}
}

// The command block is what a reader copies. Offering `generate --from-schema`
// for a message that has a template, or the reverse, sends them down the path
// that needs a catalogue they may not have.
func TestGenerateCommandMatchesWhetherATemplateExists(t *testing.T) {
	reg := testRegistry(t)
	m, _ := reg.Lookup("pacs.008.001.10")

	templated := buildPage(reg, m, []string{"pacs.008.001.10"}, nil, false, true, "2026-08-25")
	if !strings.Contains(templated, "askiso generate pacs.008") {
		t.Error("a templated message should show the base-code generate command")
	}
	if strings.Contains(templated, "--from-schema") {
		t.Error("a templated message should not be sent down the schema path")
	}

	walked := buildPage(reg, m, []string{"pacs.008.001.10"}, nil, false, false, "2026-08-25")
	if !strings.Contains(walked, "askiso generate pacs.008.001.10 --from-schema") {
		t.Error("a message without a template should walk the schema")
	}
}

func TestCommandCommentsAreAligned(t *testing.T) {
	reg := testRegistry(t)
	m, _ := reg.Lookup("pacs.008.001.10")
	page := buildPage(reg, m, []string{"pacs.008.001.10"}, nil, false, true, "2026-08-25")

	var cols []int
	for _, line := range strings.Split(page, "\n") {
		if strings.HasPrefix(line, "askiso ") && strings.Contains(line, "  # ") {
			cols = append(cols, strings.Index(line, "  # "))
		}
	}
	if len(cols) < 2 {
		t.Fatalf("expected a command block, found %d commented lines", len(cols))
	}
	for i, c := range cols {
		if c != cols[0] {
			t.Errorf("comment %d starts at column %d, want %d", i, c, cols[0])
		}
	}
}

func TestBaseCode(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"pacs.008.001.10", "pacs.008"},
		{"camt.053.001.08", "camt.053"},
		{"pacs.008", "pacs.008"},
		{"nodots", ""},
		{"", ""},
	} {
		if got := baseCode(tc.in); got != tc.want {
			t.Errorf("baseCode(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestVersionOf(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"pacs.008.001.10", "10"},
		{"camt.053.001.08", "08"},
		{"nodots", "nodots"},
	} {
		if got := versionOf(tc.in); got != tc.want {
			t.Errorf("versionOf(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestPlural(t *testing.T) {
	if got := plural(1, "set", "sets"); got != "set" {
		t.Errorf("plural(1) = %q", got)
	}
	if got := plural(2, "set", "sets"); got != "sets" {
		t.Errorf("plural(2) = %q", got)
	}
	if got := plural(0, "set", "sets"); got != "sets" {
		t.Errorf("plural(0) = %q", got)
	}
}

// mtSources is inverted from the translator's table so that a conversion added
// there reaches the pages without a second list being updated.
func TestMTSourcesIsDerivedFromTheTranslator(t *testing.T) {
	got := mtSources()
	if len(got) == 0 {
		t.Fatal("no MT sources derived at all")
	}
	if _, ok := got["pacs.008"]; !ok {
		t.Error("pacs.008 has no MT source; MT103 should map to it")
	}
	for base, maps := range got {
		if !strings.Contains(base, ".") {
			t.Errorf("%q is not a base code", base)
		}
		for _, m := range maps {
			if m.MTCode == "" {
				t.Errorf("%s has a mapping with no MT code", base)
			}
		}
	}
}

// The end-to-end shape: one file per message, named so ssg maps it to
// /messages/<id>/.
func TestRunWritesOnePagePerMessage(t *testing.T) {
	if testing.Short() {
		t.Skip("writes 2,845 files")
	}
	dir := t.TempDir()
	if err := run(dir, "2026-08-25"); err != nil {
		t.Fatalf("run: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	reg := testRegistry(t)
	if len(entries) != len(reg.Messages) {
		t.Errorf("wrote %d files, want %d", len(entries), len(reg.Messages))
	}

	body, err := os.ReadFile(filepath.Join(dir, "pacs.008.001.10.md"))
	if err != nil {
		t.Fatalf("expected a page for pacs.008.001.10: %v", err)
	}
	if !strings.Contains(string(body), "pacs.008.001.10") {
		t.Error("the page does not name its own message")
	}
}

func TestRunRejectsAnUnwritableDirectory(t *testing.T) {
	dir := t.TempDir()
	blocked := filepath.Join(dir, "nope")
	if err := os.WriteFile(blocked, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run(filepath.Join(blocked, "sub"), "2026-08-25"); err == nil {
		t.Error("writing into a file path should be an error")
	}
}

// extract returns the section of a page starting at the given heading, for
// readable failure output.
func extract(page, heading string) string {
	i := strings.Index(page, heading)
	if i < 0 {
		return page
	}
	end := i + 400
	if end > len(page) {
		end = len(page)
	}
	return page[i:end]
}
