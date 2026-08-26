package catalog

import (
	"testing"
)

// Uses a fixture rather than an installed catalogue: AskISO no longer ships one,
// and a test that silently depends on the developer's own copy is a test that
// passes for the wrong reason.
func TestCatalogLoadAndSearch(t *testing.T) {
	root := fakeCatalog(t,
		"pacs.008.001.09",
		"pacs.008.001.10",
		"pacs.009.001.10",
		"camt.053.001.11",
	)

	idx, err := Load(root)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(idx.Categories) == 0 {
		t.Fatal("expected at least one category")
	}
	if got, want := len(idx.Messages), 4; got != want {
		t.Fatalf("got %d messages, want %d", got, want)
	}

	results := idx.Search("pacs.008")
	if len(results) != 2 {
		t.Fatalf("got %d results for pacs.008, want 2", len(results))
	}
	for _, r := range results {
		if r.BaseCode != "pacs.008" {
			t.Errorf("unexpected match: %s", r.ID)
		}
	}

	// An exact identifier must outrank a base-code match.
	if top := idx.Search("pacs.008.001.10"); len(top) == 0 || top[0].ID != "pacs.008.001.10" {
		t.Errorf("exact ID should rank first, got %v", top)
	}

	if got := idx.Search("nothing-matches-this"); len(got) != 0 {
		t.Errorf("expected no results, got %d", len(got))
	}
}
