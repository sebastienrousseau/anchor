// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebastienrousseau/askiso/internal/cbprworkspace"
)

func TestCBPRWorkflowCommands(t *testing.T) {
	source, _, external := commandWorkspaceFixture(t)
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(source, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("pacs.008.001.08.xsd", strings.Replace(commandWorkspaceSchema,
		"<xs:simpleType", "<xs:annotation><xs:documentation>swift.cbprplus.03</xs:documentation></xs:annotation><xs:simpleType", 1))
	write("head.001.001.02.xsd", commandHeaderSchema)
	workspace := filepath.Join(t.TempDir(), "workspace-one")
	if _, err := cbprworkspace.Import(cbprworkspace.Options{Source: source, Workspace: workspace, ExternalCodes: external, GenerateSamples: true}); err != nil {
		t.Fatal(err)
	}
	valid := filepath.Join(source, "valid")
	if _, err := cbprworkspace.ExportValidSamples(source, workspace, valid); err != nil {
		t.Fatal(err)
	}
	workspace = filepath.Join(t.TempDir(), "workspace-two")
	if _, err := cbprworkspace.Import(cbprworkspace.Options{Source: source, Workspace: workspace, ExternalCodes: external, GenerateSamples: true}); err != nil {
		t.Fatal(err)
	}

	invalid := filepath.Join(source, "invalid")
	out, err := run(t, "cbpr-pack", "export-invalid-samples", source, "--workspace", workspace, "--output", invalid)
	if err != nil || !strings.Contains(out, "synthetic rejection fixtures") {
		t.Fatalf("invalid export: %v\n%s", err, out)
	}
	if out, err = run(t, "cbpr-pack", "export-invalid-samples", source, "--workspace", workspace, "--output", invalid, "--json"); err != nil || !strings.Contains(out, `"scenarios"`) {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	workspace = filepath.Join(t.TempDir(), "workspace-three")
	if _, err := cbprworkspace.Import(cbprworkspace.Options{Source: source, Workspace: workspace, ExternalCodes: external, GenerateSamples: true}); err != nil {
		t.Fatal(err)
	}

	checklist := filepath.Join(t.TempDir(), "review.json")
	out, err = run(t, "cbpr-pack", "review-checklist", workspace, "--output", checklist, "--created-at", "2026-09-05T09:00:00Z")
	if err != nil || !strings.Contains(out, "62 pending") {
		t.Fatalf("checklist: %v\n%s", err, out)
	}
	if out, err = run(t, "cbpr-pack", "review-checklist", workspace, "--output", checklist, "--json"); err != nil || !strings.Contains(out, `"items"`) {
		t.Fatalf("checklist JSON: %v\n%s", err, out)
	}
	out, err = run(t, "cbpr-pack", "audit-samples", source, "--workspace", workspace)
	if err != nil || !strings.Contains(out, "ready=false") {
		t.Fatalf("synthetic audit: %v\n%s", err, out)
	}
	if out, err = run(t, "cbpr-pack", "audit-samples", source, "--workspace", workspace, "--json"); err != nil || !strings.Contains(out, `"synthetic"`) {
		t.Fatalf("audit JSON: %v\n%s", err, out)
	}

	write("user - swift.cbprplus.03 - valid.xml", `<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.08"><Purpose>SALA</Purpose></Document>`)
	workspace = filepath.Join(t.TempDir(), "workspace-four")
	if _, err := cbprworkspace.Import(cbprworkspace.Options{Source: source, Workspace: workspace, ExternalCodes: external}); err != nil {
		t.Fatal(err)
	}
	out, err = run(t, "cbpr-pack", "anonymise-samples", source, "--workspace", workspace, "--output", filepath.Join(source, "anonymised"))
	if err != nil || !strings.Contains(out, "Review required") {
		t.Fatalf("anonymise: %v\n%s", err, out)
	}
	if out, err = run(t, "cbpr-pack", "anonymise-samples", source, "--workspace", workspace, "--output", filepath.Join(source, "anonymised-json"), "--json"); err != nil || !strings.Contains(out, `"processed"`) {
		t.Fatalf("anonymise JSON: %v\n%s", err, out)
	}
	out, err = run(t, "cbpr-pack", "attest-samples", source, "--workspace", workspace, "--output", filepath.Join(t.TempDir(), "attestation.json"), "--reviewer", "reviewer", "--provider", "provider", "--acknowledge-independent-review")
	if err != nil || !strings.Contains(out, "reviewed sample hashes") {
		t.Fatalf("attest: %v\n%s", err, out)
	}
	if out, err = run(t, "cbpr-pack", "attest-samples", source, "--workspace", workspace, "--output", filepath.Join(t.TempDir(), "attestation-json.json"), "--reviewer", "reviewer", "--provider", "provider", "--acknowledge-independent-review", "--json"); err != nil || !strings.Contains(out, `"reviewer"`) {
		t.Fatalf("attest JSON: %v\n%s", err, out)
	}
	out, err = run(t, "cbpr-pack", "record-external-evidence", workspace, "--output", filepath.Join(t.TempDir(), "evidence.json"), "--provider", "test-service", "--cases", "1", "--passed", "--acknowledge-external-verdict")
	if err != nil || !strings.Contains(out, "external verdict") {
		t.Fatalf("evidence: %v\n%s", err, out)
	}
	if out, err = run(t, "cbpr-pack", "record-external-evidence", workspace, "--output", filepath.Join(t.TempDir(), "evidence-json.json"), "--provider", "test-service", "--cases", "1", "--passed", "--acknowledge-external-verdict", "--json"); err != nil || !strings.Contains(out, `"provider"`) {
		t.Fatalf("evidence JSON: %v\n%s", err, out)
	}

	target := filepath.Join(t.TempDir(), "target")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "new.xsd"), []byte(commandWorkspaceSchema), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err = run(t, "cbpr-pack", "diff", source, target, "--output", filepath.Join(t.TempDir(), "delta.json"))
	if err != nil || !strings.Contains(out, "SR2025 to SR2026") {
		t.Fatalf("diff: %v\n%s", err, out)
	}
	if out, err = run(t, "cbpr-pack", "diff", source, target, "--json"); err != nil || !strings.Contains(out, `"from_release"`) {
		t.Fatalf("diff JSON: %v\n%s", err, out)
	}
	if _, err = run(t, "cbpr-pack", "diff", source, target, "--output", filepath.Join(t.TempDir(), "missing", "delta.json")); err == nil {
		t.Fatal("unwritable diff output accepted")
	}
	if out, err = run(t, "cbpr-pack", "conformance", source, "--workspace", workspace, "--as-of", "2026-09-05", "--json"); err == nil || !strings.Contains(out, `"ready": false`) {
		t.Fatalf("conformance JSON gap: %v\n%s", err, out)
	}
	if out, err = run(t, "cbpr-pack", "conformance", source, "--workspace", workspace, "--as-of", "2026-09-05"); err == nil || !strings.Contains(out, "Missing sample:") || !strings.Contains(out, "Missing scenario:") {
		t.Fatalf("conformance text gaps: %v\n%s", err, out)
	}

	write("sensitive - swift.cbprplus.03 - x.invalid.xml", `<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.08"><Purpose>GB82WEST12345698765432</Purpose></Document>`)
	sensitiveWorkspace := filepath.Join(t.TempDir(), "sensitive-workspace")
	if _, err := cbprworkspace.Import(cbprworkspace.Options{Source: source, Workspace: sensitiveWorkspace, ExternalCodes: external}); err != nil {
		t.Fatal(err)
	}
	if out, err = run(t, "cbpr-pack", "audit-samples", source, "--workspace", sensitiveWorkspace); err != nil || !strings.Contains(out, "Sensitive-data warning:") {
		t.Fatalf("sensitive audit: %v\n%s", err, out)
	}

	missing := filepath.Join(t.TempDir(), "missing")
	for _, args := range [][]string{
		{"cbpr-pack", "export-invalid-samples", source, "--workspace", missing, "--output", invalid},
		{"cbpr-pack", "review-checklist", missing, "--output", checklist},
		{"cbpr-pack", "audit-samples", source, "--workspace", missing},
		{"cbpr-pack", "anonymise-samples", source, "--workspace", missing, "--output", filepath.Join(source, "x")},
		{"cbpr-pack", "attest-samples", source, "--workspace", workspace, "--output", filepath.Join(t.TempDir(), "x.json")},
		{"cbpr-pack", "record-external-evidence", workspace, "--output", filepath.Join(t.TempDir(), "y.json")},
		{"cbpr-pack", "diff", missing, target},
		{"cbpr-pack", "conformance", source, "--workspace", workspace, "--as-of", "bad"},
	} {
		if _, err := run(t, args...); err == nil {
			t.Errorf("expected error for %v", args)
		}
	}
}

func TestCBPROverlayAndTransportCommands(t *testing.T) {
	overlay := filepath.Join(t.TempDir(), "overlay.json")
	body := `{"format":"askiso-cbpr-rule-overlay/v1","release":"SR2025","constraints":[{"message_id":"pacs.008.001.08","path":["Document","Required"],"min":1,"max":1,"when_path":["Document","Priority"],"when_values":["URGT"]}]}`
	if err := os.WriteFile(overlay, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := run(t, "cbpr-pack", "compile-overlay", overlay)
	if err != nil || !strings.Contains(out, "explicitly authored by the operator") {
		t.Fatalf("overlay stdout: %v\n%s", err, out)
	}
	out, err = run(t, "cbpr-pack", "compile-overlay", overlay, "--output", filepath.Join(t.TempDir(), "overlay.cbpr-pack.json"))
	if err != nil || !strings.Contains(out, "compiled 1") {
		t.Fatalf("overlay file: %v\n%s", err, out)
	}
	if _, err := run(t, "cbpr-pack", "compile-overlay", overlay, "--output", filepath.Join(t.TempDir(), "bad.json")); err == nil {
		t.Fatal("bad overlay suffix accepted")
	}
	blocked := filepath.Join(t.TempDir(), "blocked.cbpr-pack.json")
	if err := os.MkdirAll(blocked, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "cbpr-pack", "compile-overlay", overlay, "--output", blocked); err == nil {
		t.Fatal("overlay directory output accepted")
	}
	if _, err := run(t, "cbpr-pack", "compile-overlay", filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("missing overlay accepted")
	}

	source, _, external := commandWorkspaceFixture(t)
	if err := os.WriteFile(filepath.Join(source, "pacs.008.001.08.xsd"), []byte(strings.Replace(commandWorkspaceSchema,
		"<xs:simpleType", "<xs:annotation><xs:documentation>swift.cbprplus.03</xs:documentation></xs:annotation><xs:simpleType", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "head.001.001.02.xsd"), []byte(commandHeaderSchema), 0o600); err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(t.TempDir(), "workspace")
	if _, err := cbprworkspace.Import(cbprworkspace.Options{Source: source, Workspace: workspace, ExternalCodes: external, GenerateSamples: true, RuleOverlay: overlay}); err != nil {
		t.Fatal(err)
	}
	out, err = run(t, "cbpr-pack", "export-valid-samples", source, "--workspace", workspace, "--output", filepath.Join(source, "request"), "--transport", "request-payload")
	if err != nil || !strings.Contains(out, "exported 1") {
		t.Fatalf("request payload: %v\n%s", err, out)
	}
	if _, err := run(t, "cbpr-pack", "export-valid-samples", source, "--workspace", workspace, "--output", filepath.Join(source, "pdu"), "--transport", "swift-datapdu"); err == nil {
		t.Fatal("DataPDU without DNs accepted")
	}
}
