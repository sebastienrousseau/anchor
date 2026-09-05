// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package cbprworkspace

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path/filepath"
	"sort"
	"strings"
)

// AnonymisationReport records local sample transformations by hash only.
type AnonymisationReport struct {
	Release   string              `json:"release"`
	Output    string              `json:"output"`
	Processed int                 `json:"processed"`
	Changed   int                 `json:"changed"`
	Files     []AnonymisationFile `json:"files"`
}

// AnonymisationFile pins one source/output relationship without message data.
type AnonymisationFile struct {
	Source       string `json:"source"`
	Output       string `json:"output"`
	SourceSHA256 string `json:"source_sha256"`
	OutputSHA256 string `json:"output_sha256"`
	Changed      bool   `json:"changed"`
}

var (
	anonymiseLoadManifest  = LoadManifest
	anonymiseReadJSON      = readJSON
	anonymiseValidateRoots = validateRoots
	anonymiseSafeSource    = safeSourcePath
	anonymiseReadBounded   = readBounded
	anonymiseWriteSample   = writeExportedSample
)

// AnonymiseSamples creates clearly labelled local copies with common IBAN and
// long account-number shapes replaced. It is a precautionary filter, not a
// guarantee that all personal or confidential information has been removed.
func AnonymiseSamples(source, workspace, output string) (*AnonymisationReport, error) {
	manifest, err := anonymiseLoadManifest(workspace)
	if err != nil {
		return nil, err
	}
	dataRoot := manifestDataRoot(manifest, workspace)
	var suite Suite
	if err := anonymiseReadJSON(filepath.Join(dataRoot, SuiteFile), &suite); err != nil {
		return nil, err
	}
	sourceRoot, workspaceRoot, err := anonymiseValidateRoots(source, workspace)
	if err != nil {
		return nil, err
	}
	outputRoot, err := prepareSampleOutput(sourceRoot, workspaceRoot, output)
	if err != nil {
		return nil, err
	}
	report := &AnonymisationReport{Release: manifest.Release, Output: outputRoot}
	seen := map[string]bool{}
	for _, testCase := range suite.Cases {
		if testCase.Origin != "user-provided" && testCase.Origin != "" {
			continue
		}
		if seen[testCase.Sample] {
			continue
		}
		seen[testCase.Sample] = true
		path, err := anonymiseSafeSource(sourceRoot, testCase.Sample)
		if err != nil {
			return nil, err
		}
		data, err := anonymiseReadBounded(path)
		if err != nil {
			return nil, err
		}
		scrubbed := anonymiseSample(data)
		base := filepath.Base(testCase.Sample)
		ext := filepath.Ext(base)
		name := strings.TrimSuffix(base, ext) + " - askiso-anonymised" + ext
		if err := anonymiseWriteSample(filepath.Join(outputRoot, name), scrubbed); err != nil {
			return nil, err
		}
		sourceDigest, outputDigest := sha256.Sum256(data), sha256.Sum256(scrubbed)
		changed := sourceDigest != outputDigest
		report.Processed++
		if changed {
			report.Changed++
		}
		report.Files = append(report.Files, AnonymisationFile{Source: testCase.Sample, Output: name,
			SourceSHA256: hex.EncodeToString(sourceDigest[:]), OutputSHA256: hex.EncodeToString(outputDigest[:]), Changed: changed})
	}
	if report.Processed == 0 {
		return nil, errors.New("no user-provided samples were available to anonymise")
	}
	sort.Slice(report.Files, func(i, j int) bool { return report.Files[i].Source < report.Files[j].Source })
	return report, nil
}

func anonymiseSample(data []byte) []byte {
	result := ibanLikeRE.ReplaceAllFunc(data, func(value []byte) []byte {
		// ibanLikeRE guarantees a country/check prefix plus at least 11 chars.
		return []byte("ZZ00" + strings.Repeat("0", len(value)-4))
	})
	result = longNumberRE.ReplaceAllFunc(result, func(value []byte) []byte { return []byte(strings.Repeat("0", len(value))) })
	return result
}
