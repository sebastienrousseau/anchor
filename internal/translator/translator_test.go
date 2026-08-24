// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package translator

import (
	"strings"
	"testing"
)

func TestGetAllMappings(t *testing.T) {
	mappings := GetAllMappings()
	if len(mappings) == 0 {
		t.Fatal("Expected non-empty standard mappings list")
	}
}

func TestLookup(t *testing.T) {
	tests := []struct {
		query    string
		expected string
	}{
		{"MT103", "pacs.008.001.10"},
		{"mt103", "pacs.008.001.10"},
		{"pacs.008", "MT103"},
		{"MT202", "pacs.009.001.10 (CORE)"},
		{"MT940", "camt.053.001.11"},
		{"camt.053", "MT940"},
	}

	for _, tt := range tests {
		m, ok := Lookup(tt.query)
		if !ok {
			t.Errorf("Lookup failed for query %s", tt.query)
			continue
		}
		formatted := FormatMapping(m)
		if !strings.Contains(formatted, "SWIFT") {
			t.Errorf("FormatMapping missing SWIFT header for %s", tt.query)
		}
	}
}
