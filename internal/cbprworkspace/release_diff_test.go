// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package cbprworkspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCompareReleaseSources(t *testing.T) {
	root := t.TempDir()
	from, to := filepath.Join(root, "from"), filepath.Join(root, "to")
	writeWorkspaceFile(t, filepath.Join(from, "same.xsd"), workspaceSchema)
	writeWorkspaceFile(t, filepath.Join(to, "same.xsd"), workspaceSchema+"\n")
	writeWorkspaceFile(t, filepath.Join(from, "removed.xml"), `<usageGuideline/>`)
	writeWorkspaceFile(t, filepath.Join(to, "added.xml"), `<usageGuideline/>`)
	writeWorkspaceFile(t, filepath.Join(from, "unchanged.xml"), `<usageGuideline id="same"/>`)
	writeWorkspaceFile(t, filepath.Join(to, "unchanged.xml"), `<usageGuideline id="same"/>`)
	report, err := CompareReleaseSources(from, to, "SR2025", "SR2026")
	if err != nil || len(report.Added) != 1 || len(report.Removed) != 1 || len(report.Changed) != 1 || report.Unchanged != 1 || len(report.Actions) != 4 {
		t.Fatalf("release diff = %+v, %v", report, err)
	}
	output := filepath.Join(root, "delta.json")
	if err := WriteReleaseDiff(output, report); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(output); err != nil {
		t.Fatal(err)
	}
	if err := WriteReleaseDiff(output, nil); err == nil {
		t.Fatal("nil diff accepted")
	}
	if _, err := CompareReleaseSources(from, to, "", "SR2026"); err == nil {
		t.Fatal("empty label accepted")
	}
	if _, err := CompareReleaseSources(from, filepath.Join(root, "missing"), "SR2025", "SR2026"); err == nil {
		t.Fatal("missing target accepted")
	}
}

func TestExternalCodeDirectorySelection(t *testing.T) {
	source, workspace, _ := workspaceFixture(t)
	dir := filepath.Join(t.TempDir(), "codes")
	writeWorkspaceFile(t, filepath.Join(dir, "1Q2026_externalcodesets_v1.json"), `{"definitions":{"ExternalPurpose1Code":{"type":"string","enum":["SALA"]}}}`)
	writeWorkspaceFile(t, filepath.Join(dir, "2Q2026_externalcodesets_v2.json"), `{"definitions":{"ExternalPurpose1Code":{"type":"string","enum":["SALA","SUPP"]}}}`)
	writeWorkspaceFile(t, filepath.Join(dir, "4Q2025_externalcodesets_v9.json"), `{"definitions":{"ExternalPurpose1Code":{"type":"string","enum":["SALA"]}}}`)
	manifest, err := Import(Options{Source: source, Workspace: workspace, ExternalCodes: dir, ExternalCodesAsOf: "2026-04-01"})
	if err != nil {
		t.Fatal(err)
	}
	if manifest.ExternalCodes == nil || manifest.ExternalCodes.Publication != "2Q2026/v2" || len(manifest.ExternalCodeHistory) != 3 {
		t.Fatalf("publication selection = %+v / %+v", manifest.ExternalCodes, manifest.ExternalCodeHistory)
	}
	if _, err := Import(Options{Source: source, Workspace: filepath.Join(t.TempDir(), "bad"), ExternalCodes: dir, ExternalCodesAsOf: "bad"}); err == nil {
		t.Fatal("bad as-of accepted")
	}
	if _, err := Import(Options{Source: source, Workspace: filepath.Join(t.TempDir(), "bad-file-date"), ExternalCodes: filepath.Join(dir, "2Q2026_externalcodesets_v2.json"), ExternalCodesAsOf: "bad"}); err == nil {
		t.Fatal("bad file as-of accepted")
	}
	if _, err := Import(Options{Source: source, Workspace: filepath.Join(t.TempDir(), "early"), ExternalCodes: dir, ExternalCodesAsOf: "2025-09-30"}); err == nil {
		t.Fatal("too-early as-of accepted")
	}
	latest, err := Import(Options{Source: source, Workspace: filepath.Join(t.TempDir(), "latest"), ExternalCodes: dir})
	if err != nil || latest.ExternalCodes.Publication != "2Q2026/v2" {
		t.Fatalf("latest = %+v, %v", latest.ExternalCodes, err)
	}
	empty := filepath.Join(t.TempDir(), "empty")
	if err := os.MkdirAll(empty, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := Import(Options{Source: source, Workspace: filepath.Join(t.TempDir(), "empty-workspace"), ExternalCodes: empty}); err == nil {
		t.Fatal("empty publication directory accepted")
	}
	unversioned := filepath.Join(t.TempDir(), "unversioned")
	writeWorkspaceFile(t, filepath.Join(unversioned, "externalcodesets.json"), `{"definitions":{"ExternalPurpose1Code":{"type":"string","enum":["SALA"]}}}`)
	one, err := Import(Options{Source: source, Workspace: filepath.Join(t.TempDir(), "unversioned-workspace"), ExternalCodes: unversioned})
	if err != nil || one.ExternalCodes.Publication != "" {
		t.Fatalf("unversioned = %+v, %v", one.ExternalCodes, err)
	}
	writeWorkspaceFile(t, filepath.Join(unversioned, "1Q2026_externalcodesets.json"), `{"definitions":{"ExternalPurpose1Code":{"type":"string","enum":["SALA"]}}}`)
	if _, err := Import(Options{Source: source, Workspace: filepath.Join(t.TempDir(), "mixed-workspace"), ExternalCodes: unversioned, ExternalCodesAsOf: "2026-03-01"}); err != nil {
		t.Fatalf("mixed publication selection: %v", err)
	}
	broken := filepath.Join(t.TempDir(), "broken")
	writeWorkspaceFile(t, filepath.Join(broken, "1Q2026_externalcodesets.json"), `{broken`)
	if _, err := Import(Options{Source: source, Workspace: filepath.Join(t.TempDir(), "broken-workspace"), ExternalCodes: broken}); err == nil {
		t.Fatal("broken publication accepted")
	}
}

func TestImportRejectsMissingRuleOverlay(t *testing.T) {
	source, workspace, _ := workspaceFixture(t)
	if _, err := Import(Options{Source: source, Workspace: workspace, RuleOverlay: filepath.Join(t.TempDir(), "missing.json")}); err == nil {
		t.Fatal("missing rule overlay accepted")
	}
}
