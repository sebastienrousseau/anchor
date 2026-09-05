// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package rules

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebastienrousseau/askiso/internal/converter"
)

func TestCompileCBPROverlayAndConditionalDispatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conditional.json")
	body := `{"format":"askiso-cbpr-rule-overlay/v1","release":"sr2025","constraints":[{"id":"LOCAL-1","source":"","message_id":"pacs.008.001.08","path":["Document","RequiredWhenUrgent"],"min":1,"max":1,"when_path":["Document","Priority"],"when_values":["URGT"]}]}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	pack, err := CompileCBPROverlay(path)
	if err != nil || len(pack.Constraints) != 1 || len(pack.Fingerprint) != 24 || pack.Constraints[0].Source != "conditional.json" {
		t.Fatalf("compiled overlay = %+v, %v", pack, err)
	}
	for _, test := range []struct {
		xml      string
		findings int
	}{
		{`<Document><Priority>URGT</Priority></Document>`, 1},
		{`<Document><Priority>NORM</Priority></Document>`, 0},
		{`<Document><Priority>URGT</Priority><RequiredWhenUrgent>x</RequiredWhenUrgent></Document>`, 0},
	} {
		root, parseErr := converter.Parse([]byte(test.xml))
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		if got := len(checkPackConstraints(root, pack.Constraints)); got != test.findings {
			t.Fatalf("findings for %s = %d, want %d", test.xml, got, test.findings)
		}
	}
}

func TestCBPROverlayRejectsMalformedInput(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		"unknown.json":  `{ "format":"askiso-cbpr-rule-overlay/v1","release":"SR2025","extra":true,"constraints":[]}`,
		"trailing.json": `{"format":"askiso-cbpr-rule-overlay/v1","release":"SR2025","constraints":[]} {}`,
		"release.json":  `{"format":"askiso-cbpr-rule-overlay/v1","release":"SR2026","constraints":[{}]}`,
		"empty.json":    `{"format":"askiso-cbpr-rule-overlay/v1","release":"SR2025","constraints":[]}`,
	} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := CompileCBPROverlay(path); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	if _, err := CompileCBPROverlay(filepath.Join(dir, "missing.json")); err == nil || !strings.Contains(err.Error(), "reading") {
		t.Fatalf("missing overlay error = %v", err)
	}
	if _, err := CompileCBPROverlay(dir); err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("directory overlay error = %v", err)
	}
	badConstraint := filepath.Join(dir, "bad-constraint.json")
	if err := os.WriteFile(badConstraint, []byte(`{"format":"askiso-cbpr-rule-overlay/v1","release":"SR2025","constraints":[{"message_id":"bad","path":["Document"],"min":1,"max":1}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := CompileCBPROverlay(badConstraint); err == nil || !strings.Contains(err.Error(), "constraint 1") {
		t.Fatalf("constraint error = %v", err)
	}
}

func TestConstraintAppliesWhenAbsent(t *testing.T) {
	root, err := converter.Parse([]byte(`<Document/>`))
	if err != nil {
		t.Fatal(err)
	}
	constraint := CBPRPackConstraint{WhenPath: []string{"Document", "Optional"}, WhenAbsent: true}
	if !constraintApplies(root, constraint) {
		t.Fatal("absent condition did not apply")
	}
	constraint.WhenAbsent = false
	if constraintApplies(root, constraint) {
		t.Fatal("present condition applied to absent path")
	}
	root, err = converter.Parse([]byte(`<Document><Optional/></Document>`))
	if err != nil {
		t.Fatal(err)
	}
	if !constraintApplies(root, constraint) {
		t.Fatal("present condition without values did not apply")
	}
}

func TestMergeCBPRPacks(t *testing.T) {
	pack := &CBPRPack{Format: cbprPackFormat, Sources: []CBPRPackSource{{Name: "a", SHA256: strings.Repeat("a", 64)}}, Warnings: []string{"same"}}
	merged, err := MergeCBPRPacks(nil, pack, pack)
	if err != nil || len(merged.Sources) != 2 || len(merged.Warnings) != 1 || merged.Fingerprint == "" {
		t.Fatalf("merged = %+v, %v", merged, err)
	}
	if _, err := MergeCBPRPacks(); err == nil {
		t.Fatal("empty merge accepted")
	}
	if _, err := MergeCBPRPacks(&CBPRPack{Format: "future"}); err == nil {
		t.Fatal("future pack accepted")
	}
}
