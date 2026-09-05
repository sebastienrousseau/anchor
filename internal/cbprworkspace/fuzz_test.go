// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package cbprworkspace

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func FuzzWorkspaceMetadataRoundTrip(f *testing.F) {
	f.Add("SR2025", "pacs.008.001.08", "swift.cbprplus.03", uint8(1), true)
	f.Add("release", "camt.053.001.08", "swift.cbprplus.02", uint8(8), false)

	f.Fuzz(func(t *testing.T, rawRelease, rawMessage, rawService string, rawCases uint8, valid bool) {
		release := boundedWorkspaceText(rawRelease, "SR2025")
		message := boundedWorkspaceText(rawMessage, "pacs.008.001.08")
		service := boundedWorkspaceText(rawService, "swift.cbprplus.03")
		caseCount := int(rawCases%16) + 1
		expected := "invalid"
		if valid {
			expected = "valid"
		}

		suite := &Suite{Format: SuiteFormat, Release: release}
		for i := range caseCount {
			suffix := fmt.Sprintf("%02d", i)
			suite.Cases = append(suite.Cases, SuiteCase{
				ID:              message + "-" + suffix,
				MessageID:       message,
				BusinessService: service,
				Sample:          "samples/" + suffix + ".xml",
				SampleSHA256:    strings.Repeat("a", 64),
				Schema:          "schemas/" + suffix + ".xsd",
				SchemaSHA256:    strings.Repeat("b", 64),
				Expected:        expected,
				Origin:          "user-provided",
			})
		}
		suite.Fingerprint = suiteFingerprint(suite)
		manifest := &Manifest{
			Format: ManifestFormat, Release: release,
			SuiteCases: len(suite.Cases), SuiteFingerprint: suite.Fingerprint,
			LocalOnly: true, Coverage: Coverage{Samples: len(suite.Cases)},
		}
		manifest.Fingerprint = manifestFingerprint(manifest)

		suiteJSON, err := json.Marshal(suite)
		if err != nil {
			t.Fatal(err)
		}
		manifestJSON, err := json.Marshal(manifest)
		if err != nil {
			t.Fatal(err)
		}
		var decodedSuite Suite
		if err := decodeJSON(SuiteFile, suiteJSON, &decodedSuite); err != nil {
			t.Fatal(err)
		}
		var decodedManifest Manifest
		if err := decodeJSON(ManifestFile, manifestJSON, &decodedManifest); err != nil {
			t.Fatal(err)
		}
		if decodedSuite.Fingerprint != suiteFingerprint(&decodedSuite) {
			t.Fatal("suite fingerprint did not survive JSON round trip")
		}
		if decodedManifest.Fingerprint != manifestFingerprint(&decodedManifest) {
			t.Fatal("manifest fingerprint did not survive JSON round trip")
		}
		if decodedManifest.SuiteCases != len(decodedSuite.Cases) ||
			decodedManifest.SuiteFingerprint != decodedSuite.Fingerprint {
			t.Fatal("manifest and suite ceased to describe one snapshot")
		}

		originalSuiteFingerprint := decodedSuite.Fingerprint
		decodedSuite.Cases[0].Expected += "-tampered"
		if suiteFingerprint(&decodedSuite) == originalSuiteFingerprint {
			t.Fatal("suite mutation was not detected by its fingerprint")
		}
		originalManifestFingerprint := decodedManifest.Fingerprint
		decodedManifest.SuiteCases++
		if manifestFingerprint(&decodedManifest) == originalManifestFingerprint {
			t.Fatal("manifest mutation was not detected by its fingerprint")
		}
	})
}

func boundedWorkspaceText(value, fallback string) string {
	runes := []rune(strings.ToValidUTF8(value, "X"))
	if len(runes) > 128 {
		runes = runes[:128]
	}
	value = strings.TrimSpace(string(runes))
	if value == "" {
		return fallback
	}
	return value
}
