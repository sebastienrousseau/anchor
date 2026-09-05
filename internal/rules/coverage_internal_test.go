// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package rules

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSmallDefensiveRuleBranches(t *testing.T) {
	if Walk(nil) != nil {
		t.Fatal("Walk(nil) should be empty")
	}
	if got := mod97("12-"); got != -1 {
		t.Fatalf("mod97 foreign-character result = %d", got)
	}

	file := filepath.Join(t.TempDir(), "not-a-pdf.txt")
	if err := os.WriteFile(file, []byte("text"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := SearchCBPRPack(file, "question", 1); err == nil || !strings.Contains(err.Error(), "must be a PDF") {
		t.Fatalf("non-PDF search error = %v", err)
	}
}

func TestSARIFUsesFindingRemediationFallback(t *testing.T) {
	var out bytes.Buffer
	err := WriteSARIF(&out, &Result{Findings: []Finding{{
		RuleID: "unknown-rule", Rule: "Unknown", Severity: SeverityWarning,
		Message: "message", Remediation: "fallback help",
	}}})
	if err != nil || !strings.Contains(out.String(), "fallback help") {
		t.Fatalf("SARIF fallback = %v\n%s", err, out.String())
	}
}
