// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package cbprworkspace

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func guidelineJSON(messageID, guideline, collection string) string {
	return fmt.Sprintf(`{
  "$schema":"http://json-schema.org/draft-04/schema#",
  "$comment":{
    "group":"Cross Border Payments and Reporting Plus (CBPR+)",
    "collection":%q,
    "usageGuideline":%q,
    "baseMessage":%q
  },
  "type":"object",
  "properties":{"message":{"$ref":"#/definitions/Message"}},
  "definitions":{"Message":{"type":"object"}}
}`, collection, guideline, messageID)
}

func TestInspectMyStandardsJSON(t *testing.T) {
	root := t.TempDir()
	if _, recognised, err := inspectMyStandardsJSON(filepath.Join(root, "missing.json")); err == nil || recognised {
		t.Fatalf("missing JSON = recognised %v, error %v", recognised, err)
	}
	valid := filepath.Join(root, "cov.json")
	writeWorkspaceFile(t, valid, guidelineJSON(
		"pacs.009.001.08",
		"CBPRPlus-pacs.009.001.08_COV_FinancialInstitutionCreditTransfer",
		"CBPRPlus SR2025 (Combined)",
	))
	metadata, recognised, err := inspectMyStandardsJSON(valid)
	if err != nil || !recognised || metadata.MessageID != "pacs.009.001.08" ||
		len(metadata.UsageIdentifiers) != 1 || metadata.UsageIdentifiers[0] != "swift.cbprplus.cov.03" {
		t.Fatalf("COV metadata = %+v, recognised %v, error %v", metadata, recognised, err)
	}

	unrelated := filepath.Join(root, "unrelated.json")
	writeWorkspaceFile(t, unrelated, `{"$schema":"https://json-schema.org/draft/2020-12/schema","$comment":"ordinary schema"}`)
	if _, recognised, err := inspectMyStandardsJSON(unrelated); err != nil || recognised {
		t.Fatalf("unrelated JSON = recognised %v, error %v", recognised, err)
	}
	noComment := filepath.Join(root, "no-comment.json")
	writeWorkspaceFile(t, noComment, `{"$schema":"draft-04","$comment":null}`)
	if _, recognised, err := inspectMyStandardsJSON(noComment); err != nil || recognised {
		t.Fatalf("null comment JSON = recognised %v, error %v", recognised, err)
	}
	otherGroup := filepath.Join(root, "other-group.json")
	writeWorkspaceFile(t, otherGroup, `{"$comment":{"group":"another market practice","usageGuideline":"guide"}}`)
	if _, recognised, err := inspectMyStandardsJSON(otherGroup); err != nil || recognised {
		t.Fatalf("other group JSON = recognised %v, error %v", recognised, err)
	}
	incomplete := filepath.Join(root, "incomplete.json")
	writeWorkspaceFile(t, incomplete, `{
  "$schema":"draft-04",
  "$comment":{
    "group":"Cross-Border Payments and Reporting Plus",
    "collection":"CBPRPlus SR2025",
    "usageGuideline":"CBPRPlus-pacs.009.001.08_COV",
    "baseMessage":"pacs.009.001.08"
  },
  "type":"object"
}`)
	if _, recognised, err := inspectMyStandardsJSON(incomplete); err == nil || !recognised {
		t.Fatalf("incomplete CBPR JSON = recognised %v, error %v", recognised, err)
	}

	malformed := filepath.Join(root, "malformed.json")
	writeWorkspaceFile(t, malformed, `{`)
	if _, recognised, err := inspectMyStandardsJSON(malformed); err == nil || recognised {
		t.Fatalf("malformed JSON = recognised %v, error %v", recognised, err)
	}
}

func TestInspectMyStandardsJSONRejectsInconsistentMetadata(t *testing.T) {
	tests := map[string]string{
		"wrong release": guidelineJSON("pacs.009.001.08", "CBPRPlus-pacs.009.001.08_COV", "CBPRPlus SR2026"),
		"bad message":   guidelineJSON("not-a-message", "CBPRPlus-not-a-message", "CBPRPlus SR2025"),
		"mismatch":      guidelineJSON("pacs.009.001.08", "CBPRPlus-pacs.008.001.08", "CBPRPlus SR2025"),
		"unsupported":   guidelineJSON("pacs.999.001.01", "CBPRPlus-pacs.999.001.01", "CBPRPlus SR2025"),
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "guide.json")
			writeWorkspaceFile(t, path, body)
			if _, recognised, err := inspectMyStandardsJSON(path); err == nil || !recognised {
				t.Fatalf("recognised = %v, error = %v", recognised, err)
			}
		})
	}
}

func TestDiscoverAndImportGuidelineJSON(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	workspace := filepath.Join(root, "workspace")
	writeWorkspaceFile(t, filepath.Join(source, "guide.json"), guidelineJSON(
		"pacs.009.001.08",
		"CBPRPlus-pacs.009.001.08_COV_FinancialInstitutionCreditTransfer",
		"CBPRPlus SR2025 (Combined)",
	))
	writeWorkspaceFile(t, filepath.Join(source, "unrelated.json"), `{}`)
	writeWorkspaceFile(t, filepath.Join(source, "broken.json"), `{`)
	writeWorkspaceFile(t, filepath.Join(source, "2Q2026_externalcodesets_v3.json"),
		`{"definitions":{"ExternalPurpose1Code":{"type":"string","enum":["SALA"]}}}`)

	files, warnings, err := discover(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || len(warnings) != 1 || !strings.Contains(warnings[0], "broken.json") {
		t.Fatalf("discovery = %+v, warnings = %v", files, warnings)
	}
	if files[1].Kind != "usage-guideline-json-schema" || files[1].MessageID != "pacs.009.001.08" ||
		len(files[1].UsageIdentifiers) != 1 || files[1].UsageIdentifiers[0] != "swift.cbprplus.cov.03" {
		t.Fatalf("guideline discovery = %+v", files[1])
	}

	manifest, err := Import(Options{Source: source, Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Coverage.JSONSchemas != 1 || manifest.Coverage.PresentUsageGuidelines != 1 ||
		manifest.Coverage.Messages != 1 || manifest.Coverage.Constraints != 0 || manifest.ExternalCodes == nil {
		t.Fatalf("JSON import coverage = %+v, external = %+v", manifest.Coverage, manifest.ExternalCodes)
	}
	joined := strings.Join(manifest.Warnings, "\n")
	if !strings.Contains(joined, "not used as XML Schemas") || !strings.Contains(joined, "broken.json") {
		t.Fatalf("JSON import warnings = %v", manifest.Warnings)
	}
}
