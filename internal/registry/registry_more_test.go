// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package registry

import (
	"strings"
	"testing"
)

func TestSearchRankingTiers(t *testing.T) {
	r := MustLoad()

	// Every scoring tier must be reachable.
	tiers := map[string]string{
		"pacs.008.001.10": "exact identifier",
		"pacs.008":        "base code",
		"pacs.00":         "prefix",
		"008.001.10":      "substring",
		"pacs":            "domain",
	}
	for q, tier := range tiers {
		t.Run(tier, func(t *testing.T) {
			if got := r.Search(q); len(got) == 0 {
				t.Errorf("%q (%s) returned nothing", q, tier)
			}
		})
	}

	// A base-code substring that is not a whole domain.
	if got := r.Search("cs.008"); len(got) == 0 {
		t.Error("a mid-identifier substring should still match")
	}
}

func TestDecodeRejectsRecordOutsideSection(t *testing.T) {
	blob := "pacs.008.001.10\t1\n"
	if _, err := decode(gzipBytes(t, blob)); err == nil {
		t.Error("a record before any section header should be rejected")
	}
}

func TestDecodeSkipsBlankLines(t *testing.T) {
	blob := "#SETS\n\n7\tName\tslug\tv1\t2\n\n#MESSAGES\n\npacs.008.001.10\t7\n\n"
	r, err := decode(gzipBytes(t, blob))
	if err != nil {
		t.Fatalf("blank lines should be skipped: %v", err)
	}
	if len(r.Sets) != 1 || len(r.Messages) != 1 {
		t.Errorf("got %d sets and %d messages", len(r.Sets), len(r.Messages))
	}
}

func TestDecodeRejectsCorruptGzip(t *testing.T) {
	if _, err := decode([]byte("not gzip at all")); err == nil {
		t.Error("a non-gzip blob should be rejected")
	}
}

func TestBaseCodeAndDomainEdgeCases(t *testing.T) {
	if got := baseCode("nodots"); got != "nodots" {
		t.Errorf("baseCode(nodots) = %q", got)
	}
	if got := domain("nodots"); got != "nodots" {
		t.Errorf("domain(nodots) = %q", got)
	}
	if got := baseCode("a.b.c.d"); got != "a.b" {
		t.Errorf("baseCode = %q", got)
	}
}

func TestSetStringAndURL(t *testing.T) {
	s := Set{ID: "42", Name: "Some Set", Version: "v03"}
	if s.String() != "Some Set v03" {
		t.Errorf("String() = %q", s.String())
	}
	if !strings.HasSuffix(s.DownloadURL(), "/message-set/42/download") {
		t.Errorf("DownloadURL() = %q", s.DownloadURL())
	}
}

func TestMustLoadReturnsRegistry(t *testing.T) {
	if r := MustLoad(); r == nil || len(r.Messages) == 0 {
		t.Error("MustLoad should return the embedded registry")
	}
}

func TestLookupTrimsAndLowercases(t *testing.T) {
	r := MustLoad()
	if _, ok := r.Lookup("  PACS.008.001.10  "); !ok {
		t.Error("Lookup should trim and lower-case its argument")
	}
}

func TestSetLookupMiss(t *testing.T) {
	if _, ok := MustLoad().Set("no-such-set"); ok {
		t.Error("an unknown set id should not resolve")
	}
}

func TestSearchDomainTier(t *testing.T) {
	r := MustLoad()
	// An exact domain match is the lowest scoring tier.
	hits := r.Search("seev")
	if len(hits) == 0 {
		t.Fatal("a domain query should return results")
	}
	for _, m := range hits[:1] {
		if m.Domain != "seev" && !strings.Contains(m.ID, "seev") {
			t.Errorf("unexpected top hit: %s", m.ID)
		}
	}
}

func TestLoadIsIdempotent(t *testing.T) {
	a, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	b := MustLoad()
	if a != b {
		t.Error("Load and MustLoad should share the cached registry")
	}
}

func TestCustomRegistrySortsSetsAndRanksSetNameMatches(t *testing.T) {
	r := &Registry{
		Sets: []Set{
			{ID: "old", Name: "Obscure Clearing Widget", Version: "v01"},
			{ID: "new", Name: "Obscure Clearing Widget", Version: "v09"},
		},
		Messages: []Message{
			{ID: "pacs.008.001.10", BaseCode: "pacs.008", Domain: "pacs", SetIDs: []string{"old", "new"}},
			{ID: "pacs.008-extra", BaseCode: "pacs.other", Domain: "pacs"},
			{ID: "admi.024.001.01", BaseCode: "admi.024", Domain: "admi", SetIDs: []string{"old"}},
		},
		setByID: map[string]Set{
			"old": {ID: "old", Name: "Obscure Clearing Widget", Version: "v01"},
			"new": {ID: "new", Name: "Obscure Clearing Widget", Version: "v09"},
		},
		messageMap: map[string]Message{},
	}
	for _, m := range r.Messages {
		r.messageMap[m.ID] = m
	}
	sets := r.SetsFor("pacs.008.001.10")
	if len(sets) != 2 || sets[0].Version != "v09" {
		t.Fatalf("publishing sets were not sorted newest first: %+v", sets)
	}
	hits := r.Search("obscure clearing widget")
	if len(hits) != 2 {
		t.Fatalf("set-name search returned %+v", hits)
	}
	// Mix an exact and a prefix hit so the score comparator, rather than only
	// the identifier tie-breaker, determines the order.
	hits = r.Search("pacs.008")
	if len(hits) != 2 || hits[0].BaseCode != "pacs.008" {
		t.Fatalf("score-ranked search returned %+v", hits)
	}
}
