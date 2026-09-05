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

	"github.com/sebastienrousseau/askiso/internal/codes"
	"github.com/sebastienrousseau/askiso/internal/validator"
	"github.com/sebastienrousseau/askiso/internal/xsd"
)

// NegativeExport reports deliberately invalid synthetic fixtures.
type NegativeExport struct {
	Release   string         `json:"release"`
	Output    string         `json:"output"`
	Generated int            `json:"generated"`
	Scenarios map[string]int `json:"scenarios"`
	Files     []string       `json:"files"`
	Warnings  []string       `json:"warnings,omitempty"`
}

var leafTextRE = regexp.MustCompile(`<([A-Za-z_][A-Za-z0-9_.-]*)(?:\s[^>]*)?>([^<]+)</([A-Za-z_][A-Za-z0-9_.-]*)>`)

var negativeVerify = Verify

// ExportNegativeSamples derives validated rejection fixtures from the
// AskISO-generated positive envelopes already present in the private source.
// Their filenames retain synthetic provenance and cannot satisfy independent
// sample gates.
func ExportNegativeSamples(source, workspace, output string) (*NegativeExport, error) {
	verification, err := negativeVerify(source, workspace)
	if err != nil {
		return nil, err
	}
	if verification.Failed > 0 {
		return nil, fmt.Errorf("cannot export from a suite with %d failed case(s)", verification.Failed)
	}
	manifest, err := exportLoadManifest(workspace)
	if err != nil {
		return nil, err
	}
	if err := ensureWorkspaceSnapshot(verification.workspaceFingerprint, manifest); err != nil {
		return nil, err
	}
	dataRoot := manifestDataRoot(manifest, workspace)
	var suite Suite
	if err := exportReadJSON(filepath.Join(dataRoot, SuiteFile), &suite); err != nil {
		return nil, err
	}
	sourceRoot, workspaceRoot, err := exportValidateRoots(source, workspace)
	if err != nil {
		return nil, err
	}
	outputRoot, err := prepareSampleOutput(sourceRoot, workspaceRoot, output)
	if err != nil {
		return nil, err
	}
	external, err := exportLoadExternalSets(dataRoot)
	if err != nil {
		return nil, err
	}

	report := &NegativeExport{Release: manifest.Release, Output: outputRoot, Scenarios: map[string]int{}}
	var pending []pendingSample
	for _, testCase := range suite.Cases {
		if testCase.Origin != "askiso-generated" || testCase.Expected != "valid" {
			continue
		}
		samplePath, err := exportSafeSource(sourceRoot, testCase.Sample)
		if err != nil {
			return nil, err
		}
		sample, err := exportReadBounded(samplePath)
		if err != nil {
			return nil, err
		}
		schemaPath, err := exportSafeSource(sourceRoot, testCase.Schema)
		if err != nil {
			return nil, err
		}
		schema, err := exportParseSchema(schemaPath)
		if err != nil {
			return nil, err
		}
		mutations := negativeMutations(sample, schema, external, testCase.MessageID, testCase.BusinessService)
		for _, scenario := range representativeScenarios {
			mutation, ok := mutations[scenario]
			if !ok {
				report.Warnings = append(report.Warnings, fmt.Sprintf("%s/%s: no reliable %s mutation was available", testCase.MessageID, testCase.BusinessService, scenario))
				continue
			}
			name := fmt.Sprintf("%s - %s - %s - askiso-generated.invalid.xml", testCase.MessageID, testCase.BusinessService, scenario)
			pending = append(pending, pendingSample{name: name, data: mutation})
			report.Generated++
			report.Scenarios[scenario]++
			report.Files = append(report.Files, name)
		}
	}
	if report.Generated == 0 {
		return nil, errors.New("no AskISO-generated positive envelopes were found; run export-valid-samples and re-import first")
	}
	sort.Strings(report.Files)
	sort.Strings(report.Warnings)
	for _, sample := range pending {
		if err := exportWriteSample(filepath.Join(outputRoot, sample.name), sample.data); err != nil {
			return nil, err
		}
	}
	return report, nil
}

func negativeMutations(envelope []byte, schema *xsd.Schema, external *codes.ExternalSets, messageID, service string) map[string][]byte {
	out := map[string][]byte{}
	start, innerStart, innerEnd, end, ok := documentBounds(envelope)
	if !ok {
		return out
	}
	prefix, inner, suffix := envelope[:innerStart], envelope[innerStart:innerEnd], envelope[innerEnd:]
	trimmed := strings.TrimSpace(string(inner))
	candidates := map[string][]byte{
		"missing-mandatory": appendEnvelopeParts(prefix, nil, suffix),
		"forbidden-element": appendEnvelopeParts(prefix, []byte(trimmed+"\n<AskISOForbidden xmlns=\"urn:askiso:invalid\"/>"), suffix),
		"cardinality":       appendEnvelopeParts(prefix, []byte(trimmed+"\n"+trimmed), suffix),
		"lexical":           replaceElementText(envelope, "CreDt", "not-a-date"),
		"restricted-code":   replaceElementText(envelope, "BizSvc", "swift.cbprplus.invalid.99"),
		"business-service":  replaceElementText(envelope, "BizSvc", "swift.cbprplus.invalid.99"),
		"bah-payload":       replaceElementText(envelope, "MsgDefIdr", "admi.999.999.99"),
	}
	_ = start
	_ = end
	for scenario, candidate := range candidates {
		payload, envelopeErrors := validationPayload(candidate, messageID, service)
		verdict := validator.ValidateWithExternalSets(payload, schema, external)
		if !verdict.Valid || envelopeErrors > 0 {
			out[scenario] = candidate
		}
	}

	document := envelope[start:end]
	for _, target := range []struct {
		scenario string
		value    string
		rules    map[string]bool
	}{
		{scenario: "lexical", value: strings.Repeat("X", 4096), rules: map[string]bool{
			"type": true, "length": true, "minLength": true, "maxLength": true, "pattern": true,
			"totalDigits": true, "fractionDigits": true, "minInclusive": true, "maxInclusive": true,
		}},
		{scenario: "restricted-code", value: "ASKISO_INVALID_CODE", rules: map[string]bool{"enumeration": true}},
		{scenario: "external-code", value: "ASKISO_INVALID_CODE", rules: map[string]bool{"external code set": true}},
	} {
		if mutation, ok := mutateLeafForRule(envelope, document, schema, external, target.value, target.rules); ok {
			out[target.scenario] = mutation
		}
	}
	return out
}

func documentBounds(data []byte) (start, innerStart, innerEnd, end int, ok bool) {
	text := string(data)
	start = strings.Index(text, "<Document")
	if start < 0 {
		return 0, 0, 0, 0, false
	}
	relOpen := strings.Index(text[start:], ">")
	if relOpen < 0 {
		return 0, 0, 0, 0, false
	}
	innerStart = start + relOpen + 1
	relClose := strings.Index(text[innerStart:], "</Document>")
	if relClose < 0 {
		return 0, 0, 0, 0, false
	}
	innerEnd = innerStart + relClose
	end = innerEnd + len("</Document>")
	return start, innerStart, innerEnd, end, true
}

func appendEnvelopeParts(prefix, inner, suffix []byte) []byte {
	out := make([]byte, 0, len(prefix)+len(inner)+len(suffix)+2)
	out = append(out, prefix...)
	out = append(out, '\n')
	out = append(out, inner...)
	out = append(out, '\n')
	out = append(out, suffix...)
	return out
}

func replaceElementText(data []byte, element, value string) []byte {
	text := string(data)
	open, close := "<"+element+">", "</"+element+">"
	start := strings.Index(text, open)
	if start < 0 {
		return append([]byte(nil), data...)
	}
	start += len(open)
	end := strings.Index(text[start:], close)
	if end < 0 {
		return append([]byte(nil), data...)
	}
	return []byte(text[:start] + value + text[start+end:])
}

func mutateLeafForRule(envelope, document []byte, schema *xsd.Schema, external *codes.ExternalSets, value string, wanted map[string]bool) ([]byte, bool) {
	documentOffset := strings.Index(string(envelope), string(document))
	for _, match := range leafTextRE.FindAllSubmatchIndex(document, -1) {
		if string(document[match[2]:match[3]]) != string(document[match[6]:match[7]]) {
			continue
		}
		start, end := documentOffset+match[4], documentOffset+match[5]
		candidate := append([]byte(nil), envelope...)
		candidate = append(candidate[:start], append([]byte(value), candidate[end:]...)...)
		payload, _ := validationPayload(candidate, "", "")
		verdict := validator.ValidateWithExternalSets(payload, schema, external)
		for _, issue := range verdict.Errors {
			if wanted[issue.Rule] {
				return candidate, true
			}
		}
	}
	return nil, false
}
