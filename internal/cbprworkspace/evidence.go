// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package cbprworkspace

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/sebastienrousseau/askiso/internal/rules"
)

const (
	ReviewChecklistFormat   = "askiso-cbpr-review-checklist/v1"
	SampleAttestationFormat = "askiso-cbpr-sample-attestation/v1"
)

// ReviewChecklist is a content-free plan for independently sourced fixtures.
type ReviewChecklist struct {
	Format               string       `json:"format"`
	Release              string       `json:"release"`
	WorkspaceFingerprint string       `json:"workspace_fingerprint"`
	SuiteFingerprint     string       `json:"suite_fingerprint"`
	CreatedAt            string       `json:"created_at"`
	Items                []ReviewItem `json:"items"`
}

// ReviewItem identifies evidence needed without containing message data.
type ReviewItem struct {
	MessageID       string `json:"message_id"`
	BusinessService string `json:"business_service"`
	Expected        string `json:"expected"`
	Scenario        string `json:"scenario"`
	Status          string `json:"status"`
	SamplePath      string `json:"sample_path,omitempty"`
}

// SampleAudit is a local, content-free intake assessment.
type SampleAudit struct {
	Release               string              `json:"release"`
	Cases                 int                 `json:"cases"`
	Eligible              int                 `json:"eligible"`
	Synthetic             int                 `json:"synthetic"`
	Duplicates            map[string][]string `json:"duplicates,omitempty"`
	SensitiveDataWarnings []string            `json:"sensitive_data_warnings,omitempty"`
	Unpaired              []string            `json:"unpaired,omitempty"`
	ReadyForAttestation   bool                `json:"ready_for_attestation"`

	workspaceFingerprint string
}

// SampleAttestation records an operator's explicit independent-review claim.
// It stores hashes and case identifiers, never XML bodies.
type SampleAttestation struct {
	Format               string                  `json:"format"`
	Release              string                  `json:"release"`
	WorkspaceFingerprint string                  `json:"workspace_fingerprint"`
	SuiteFingerprint     string                  `json:"suite_fingerprint"`
	Reviewer             string                  `json:"reviewer"`
	Provider             string                  `json:"provider"`
	ReviewedAt           string                  `json:"reviewed_at"`
	Cases                []SampleAttestationCase `json:"cases"`
}

// SampleAttestationCase pins one reviewed sample without copying its content.
type SampleAttestationCase struct {
	ID              string `json:"id"`
	MessageID       string `json:"message_id"`
	BusinessService string `json:"business_service"`
	Expected        string `json:"expected"`
	Scenario        string `json:"scenario"`
	SHA256          string `json:"sha256"`
}

var (
	ibanLikeRE             = regexp.MustCompile(`(?i)\b[A-Z]{2}[0-9]{2}[A-Z0-9]{11,30}\b`)
	longNumberRE           = regexp.MustCompile(`\b[0-9]{12,34}\b`)
	evidenceVerify         = Verify
	evidenceLoadManifest   = LoadManifest
	evidenceReadJSON       = readJSON
	evidenceValidateRoots  = validateRoots
	evidenceSafeSourcePath = safeSourcePath
	evidenceReadBounded    = readBounded
	evidenceAuditSamples   = AuditSamples
	evidenceWriteJSON      = writeEvidenceJSON
)

// WriteReviewChecklist creates the complete 31-positive/31-negative review
// inventory without creating or asserting any evidence.
func WriteReviewChecklist(workspace, output, createdAt string) (*ReviewChecklist, error) {
	manifest, err := evidenceLoadManifest(workspace)
	if err != nil {
		return nil, err
	}
	when, err := evidenceTime(createdAt)
	if err != nil {
		return nil, err
	}
	report := &ReviewChecklist{
		Format: ReviewChecklistFormat, Release: manifest.Release,
		WorkspaceFingerprint: manifest.Fingerprint, SuiteFingerprint: manifest.SuiteFingerprint,
		CreatedAt: when,
	}
	for index, guideline := range rules.CBPRSR2025UsageGuidelines() {
		report.Items = append(report.Items,
			ReviewItem{MessageID: guideline.MessageID, BusinessService: guideline.UsageIdentifier, Expected: "valid", Scenario: "valid", Status: "pending"},
			ReviewItem{MessageID: guideline.MessageID, BusinessService: guideline.UsageIdentifier, Expected: "invalid", Scenario: representativeScenarios[index%len(representativeScenarios)], Status: "pending"},
		)
	}
	if err := evidenceWriteJSON(output, report); err != nil {
		return nil, err
	}
	return report, nil
}

// AuditSamples checks locally indexed fixtures for duplicates, unpaired files,
// and common live-data shapes before an operator attests to independent review.
func AuditSamples(source, workspace string) (*SampleAudit, error) {
	verification, err := evidenceVerify(source, workspace)
	if err != nil {
		return nil, err
	}
	manifest, err := evidenceLoadManifest(workspace)
	if err != nil {
		return nil, err
	}
	if err := ensureWorkspaceSnapshot(verification.workspaceFingerprint, manifest); err != nil {
		return nil, err
	}
	dataRoot := manifestDataRoot(manifest, workspace)
	var suite Suite
	if err := evidenceReadJSON(filepath.Join(dataRoot, SuiteFile), &suite); err != nil {
		return nil, err
	}
	root, _, err := evidenceValidateRoots(source, workspace)
	if err != nil {
		return nil, err
	}
	report := &SampleAudit{
		Release: manifest.Release, Cases: len(suite.Cases), Duplicates: map[string][]string{},
		workspaceFingerprint: manifest.Fingerprint,
	}
	hashes := map[string][]string{}
	for _, testCase := range suite.Cases {
		if testCase.Origin == "generated" || testCase.Origin == "askiso-generated" || testCase.Origin == "askiso-anonymised" {
			report.Synthetic++
			continue
		}
		if testCase.BusinessService == "" {
			report.Unpaired = append(report.Unpaired, testCase.ID)
			continue
		}
		report.Eligible++
		hashes[testCase.SampleSHA256] = append(hashes[testCase.SampleSHA256], testCase.ID)
		path, err := evidenceSafeSourcePath(root, testCase.Sample)
		if err != nil {
			return nil, err
		}
		data, err := evidenceReadBounded(path)
		if err != nil {
			return nil, err
		}
		if ibanLikeRE.Match(data) || longNumberRE.Match(data) {
			report.SensitiveDataWarnings = append(report.SensitiveDataWarnings, testCase.ID+": contains an IBAN/account-number-like value")
		}
	}
	for hash, ids := range hashes {
		if len(ids) > 1 {
			sort.Strings(ids)
			report.Duplicates[hash] = ids
		}
	}
	sort.Strings(report.SensitiveDataWarnings)
	sort.Strings(report.Unpaired)
	report.ReadyForAttestation = verification.Failed == 0 && report.Eligible > 0 && len(report.Duplicates) == 0 && len(report.SensitiveDataWarnings) == 0 && len(report.Unpaired) == 0
	return report, nil
}

// WriteSampleAttestation records a deliberate human assertion. AskISO cannot
// independently establish the truth of reviewer/provider strings.
func WriteSampleAttestation(source, workspace, output, reviewer, provider, reviewedAt string, acknowledge bool) (*SampleAttestation, error) {
	if !acknowledge {
		return nil, errors.New("independent-review acknowledgement is required")
	}
	if strings.TrimSpace(reviewer) == "" || strings.TrimSpace(provider) == "" {
		return nil, errors.New("reviewer and provider are required")
	}
	audit, err := evidenceAuditSamples(source, workspace)
	if err != nil {
		return nil, err
	}
	if !audit.ReadyForAttestation {
		return nil, errors.New("sample audit is not ready for attestation")
	}
	when, err := evidenceTime(reviewedAt)
	if err != nil {
		return nil, err
	}
	manifest, err := evidenceLoadManifest(workspace)
	if err != nil {
		return nil, err
	}
	if err := ensureWorkspaceSnapshot(audit.workspaceFingerprint, manifest); err != nil {
		return nil, err
	}
	dataRoot := manifestDataRoot(manifest, workspace)
	var suite Suite
	if err := evidenceReadJSON(filepath.Join(dataRoot, SuiteFile), &suite); err != nil {
		return nil, err
	}
	attestation := &SampleAttestation{
		Format: SampleAttestationFormat, Release: manifest.Release,
		WorkspaceFingerprint: manifest.Fingerprint, SuiteFingerprint: manifest.SuiteFingerprint,
		Reviewer: strings.TrimSpace(reviewer), Provider: strings.TrimSpace(provider), ReviewedAt: when,
	}
	for _, testCase := range suite.Cases {
		if testCase.Origin == "generated" || testCase.Origin == "askiso-generated" || testCase.Origin == "askiso-anonymised" {
			continue
		}
		attestation.Cases = append(attestation.Cases, SampleAttestationCase{
			ID: testCase.ID, MessageID: testCase.MessageID, BusinessService: testCase.BusinessService,
			Expected: testCase.Expected, Scenario: testCase.Scenario, SHA256: testCase.SampleSHA256,
		})
	}
	if err := evidenceWriteJSON(output, attestation); err != nil {
		return nil, err
	}
	return attestation, nil
}

// WriteExternalEvidence records an externally obtained verdict without its
// request or response bodies.
func WriteExternalEvidence(workspace, output, provider, testedAt string, cases int, passed, acknowledge bool) (*ExternalEvidence, error) {
	if !acknowledge {
		return nil, errors.New("external-verdict acknowledgement is required")
	}
	if strings.TrimSpace(provider) == "" || cases < 1 {
		return nil, errors.New("provider and a positive case count are required")
	}
	when, err := evidenceTime(testedAt)
	if err != nil {
		return nil, err
	}
	manifest, err := evidenceLoadManifest(workspace)
	if err != nil {
		return nil, err
	}
	evidence := &ExternalEvidence{
		Format: EvidenceFormat, Provider: strings.TrimSpace(provider),
		WorkspaceFingerprint: manifest.Fingerprint, SuiteFingerprint: manifest.SuiteFingerprint,
		TestedAt: when, Cases: cases, Passed: passed,
	}
	if err := evidenceWriteJSON(output, evidence); err != nil {
		return nil, err
	}
	return evidence, nil
}

func evidenceTime(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return time.Now().UTC().Format(time.RFC3339), nil
	}
	when, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return "", fmt.Errorf("time must be RFC3339: %w", err)
	}
	return when.UTC().Format(time.RFC3339), nil
}

func writeEvidenceJSON(output string, value any) error {
	if strings.TrimSpace(output) == "" {
		return errors.New("output file is required")
	}
	path, err := filepath.Abs(filepath.Clean(output))
	if err != nil {
		return err
	}
	if err := protectWorkspace(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return writeJSON(path, value)
}
