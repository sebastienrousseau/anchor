// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package iso20022_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebastienrousseau/askiso/pkg/iso20022"
)

func TestLocalCBPRPackPublicAPI(t *testing.T) {
	pack := &iso20022.CBPRPack{
		Format: "askiso-cbpr-pack/v1",
		Sources: []iso20022.CBPRPackSource{{
			Name: "private.pdf", SHA256: strings.Repeat("e", 64),
			MessageID: "pacs.008.001.08", UsageIdentifiers: []string{"swift.cbprplus.03"}, Constraints: 1,
		}},
		Constraints: []iso20022.CBPRPackConstraint{{
			Source: "private.pdf", MessageID: "pacs.008.001.08",
			UsageIdentifiers: []string{"swift.cbprplus.03"},
			Path:             []string{"Document", "FIToFICstmrCdtTrf", "GrpHdr", "LocallyRequired"}, Min: 1, Max: 1,
		}},
	}
	path := filepath.Join(t.TempDir(), "private.cbpr-pack.json")
	if err := iso20022.WriteCBPRPack(path, pack); err != nil {
		t.Fatal(err)
	}
	loaded, err := iso20022.CompileCBPRPack(path)
	if err != nil || loaded.Fingerprint == "" {
		t.Fatalf("compile/load: %+v, %v", loaded, err)
	}
	message := []byte(`<Envelope xmlns="urn:env"><AppHdr xmlns="urn:iso:std:iso:20022:tech:xsd:head.001.001.02"><BizSvc>swift.cbprplus.03</BizSvc></AppHdr><Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.08"><FIToFICstmrCdtTrf><GrpHdr/></FIToFICstmrCdtTrf></Document></Envelope>`)
	result, err := iso20022.CheckCBPRPack(message, path, "payment.xml")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, finding := range result.Findings {
		if strings.HasPrefix(finding.RuleID, "CBPR-PACK-") && strings.HasSuffix(finding.Path, "/LocallyRequired") {
			found = true
		}
	}
	if !found || result.Pack == nil {
		t.Fatalf("public pack check did not run: %+v", result)
	}
	if _, err := iso20022.SearchCBPRPack(path, "question", 5); err == nil {
		t.Error("compiled pack intentionally has no searchable prose")
	}
	if _, err := iso20022.CheckCBPRPack(message, filepath.Join(t.TempDir(), "missing.cbpr-pack.json"), "payment.xml"); err == nil {
		t.Error("missing local pack should fail")
	}
	invalid := &iso20022.CBPRPack{Constraints: []iso20022.CBPRPackConstraint{{
		MessageID: "not.a.message", Path: []string{"Document", "Value"}, Max: 1,
	}}}
	invalidPath := filepath.Join(t.TempDir(), "invalid.cbpr-pack.json")
	if err := iso20022.WriteCBPRPack(invalidPath, invalid); err != nil {
		t.Fatal(err)
	}
	if _, err := iso20022.CheckCBPRPack(message, invalidPath, "payment.xml"); err == nil || !strings.Contains(err.Error(), "not in the live") {
		t.Fatalf("invalid local constraint error = %v", err)
	}
	if _, err := iso20022.CheckCBPRPack([]byte("<broken>"), path, "broken.xml"); err == nil {
		t.Error("malformed message should fail before local rules run")
	}
}

func TestLocalCBPRWorkspacePublicAPI(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(source, 0o700); err != nil {
		t.Fatal(err)
	}
	manifest, err := iso20022.ImportCBPRWorkspace(iso20022.CBPRWorkspaceOptions{
		Source: source, Workspace: workspace, Release: "SR2025",
	})
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Release != "SR2025" || manifest.Coverage.ExpectedUsageGuidelines != 31 {
		t.Fatalf("workspace manifest = %+v", manifest)
	}
	loaded, err := iso20022.LoadCBPRWorkspace(workspace)
	if err != nil || loaded.Fingerprint != manifest.Fingerprint {
		t.Fatalf("loaded workspace = %+v, %v", loaded, err)
	}
	report, err := iso20022.VerifyCBPRWorkspace(source, workspace)
	if err != nil || report.Cases != 0 || report.Failed != 0 {
		t.Fatalf("empty workspace verification = %+v, %v", report, err)
	}
	conformance, err := iso20022.CheckCBPRConformance(iso20022.CBPRConformanceOptions{
		Source: source, Workspace: workspace,
	})
	if err != nil || conformance.Ready {
		t.Fatalf("empty strict conformance = %+v, %v", conformance, err)
	}
	runtime, err := iso20022.LoadCBPRWorkspaceRuntime(workspace)
	if err != nil || runtime.Manifest.Fingerprint != loaded.Fingerprint {
		t.Fatalf("workspace runtime = %+v, %v", runtime, err)
	}
	guide := filepath.Join(source, "guide.json")
	if err := os.WriteFile(guide, []byte(`{"description":"UETR evidence"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	search, err := iso20022.SearchCBPRSources(source, "UETR", 5)
	if err != nil || len(search.Hits) != 1 || search.Hits[0].Source != "guide.json" {
		t.Fatalf("workspace source search = %+v, %v", search, err)
	}
}

func TestExportCBPRValidSamplesPublicAPIRejectsMissingWorkspace(t *testing.T) {
	if _, err := iso20022.ExportCBPRValidSamples(t.TempDir(), filepath.Join(t.TempDir(), "missing"), filepath.Join(t.TempDir(), "out")); err == nil {
		t.Fatal("sample export without a workspace succeeded")
	}
}

func TestExtendedCBPRPublicAPIErrors(t *testing.T) {
	source, workspace, output := t.TempDir(), filepath.Join(t.TempDir(), "missing"), filepath.Join(t.TempDir(), "out")
	if _, err := iso20022.ExportCBPRValidSamplesWithOptions(source, workspace, output, iso20022.CBPRSampleExportOptions{Profile: "unknown"}); err == nil {
		t.Fatal("unknown transport accepted")
	}
	if _, err := iso20022.ExportCBPRInvalidSamples(source, workspace, output); err == nil {
		t.Fatal("missing workspace accepted for invalid export")
	}
	if _, err := iso20022.WriteCBPRReviewChecklist(workspace, output, ""); err == nil {
		t.Fatal("missing workspace accepted for checklist")
	}
	if _, err := iso20022.AuditCBPRSamples(source, workspace); err == nil {
		t.Fatal("missing workspace accepted for audit")
	}
	if _, err := iso20022.AnonymiseCBPRSamples(source, workspace, output); err == nil {
		t.Fatal("missing workspace accepted for anonymisation")
	}
	if _, err := iso20022.WriteCBPRSampleAttestation(source, workspace, output, "r", "p", "", false); err == nil {
		t.Fatal("missing acknowledgement accepted")
	}
	if _, err := iso20022.WriteCBPRExternalEvidence(workspace, output, "p", "", 1, true, false); err == nil {
		t.Fatal("missing verdict acknowledgement accepted")
	}
	if _, err := iso20022.CompileCBPROverlay(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("missing overlay accepted")
	}
	if _, err := iso20022.CompareCBPRReleaseSources(filepath.Join(t.TempDir(), "missing"), t.TempDir(), "SR2025", "SR2026"); err == nil {
		t.Fatal("missing source accepted")
	}
}
