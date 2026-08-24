// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package codes

import (
	"strings"
	"testing"
)

func TestGetAllCodes(t *testing.T) {
	codes := GetAllCodes()
	if len(codes) == 0 {
		t.Fatal("Expected non-empty standard codes list")
	}
}

func TestLookup(t *testing.T) {
	tests := []struct {
		query    string
		expected string
	}{
		{"AC04", "Account Closed"},
		{"SALA", "Salary / Payroll Payment"},
		{"SHAR", "Shared Charges"},
		{"ACTC", "Accepted Technical Validation"},
		{"OPBD", "Opening Booked Balance"},
		{"insufficient funds", "Insufficient Funds"},
	}

	for _, tt := range tests {
		res := Lookup(tt.query)
		if len(res) == 0 {
			t.Errorf("Lookup returned 0 results for %s", tt.query)
			continue
		}
		if !strings.Contains(res[0].Name, tt.expected) {
			t.Errorf("Expected '%s' in result for query '%s', got '%s'", tt.expected, tt.query, res[0].Name)
		}
		formatted := FormatCode(res[0])
		if !strings.Contains(formatted, "Code") {
			t.Errorf("FormatCode missing header for %s", tt.query)
		}
	}
}
