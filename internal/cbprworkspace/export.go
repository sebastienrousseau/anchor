// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package cbprworkspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sebastienrousseau/askiso/internal/codes"
	"github.com/sebastienrousseau/askiso/internal/schemagen"
	"github.com/sebastienrousseau/askiso/internal/validator"
	"github.com/sebastienrousseau/askiso/internal/xsd"
)

// SampleExport reports synthetic positive fixtures written by AskISO.
type SampleExport struct {
	Release          string   `json:"release"`
	Output           string   `json:"output"`
	Profile          string   `json:"profile"`
	NetworkValidated bool     `json:"network_validated"`
	Generated        int      `json:"generated"`
	Files            []string `json:"files"`
}

type pendingSample struct {
	name string
	data []byte
}

var (
	exportVerify           = Verify
	exportLoadManifest     = LoadManifest
	exportReadJSON         = readJSON
	exportValidateRoots    = validateRoots
	exportLoadExternalSets = codes.LoadExternalSets
	exportSafeGenerated    = safeGeneratedPath
	exportReadBounded      = readBounded
	exportSafeSource       = safeSourcePath
	exportParseSchema      = xsd.ParseFile
	exportWriteSample      = writeExportedSample
	sampleAbs              = filepath.Abs
	sampleLstat            = os.Lstat
	sampleMkdirAll         = os.MkdirAll
	sampleChmod            = os.Chmod
	sampleWriteFile        = os.WriteFile
)

// ExportValidSamples writes one BAH-plus-payload synthetic positive fixture
// for every generated executable Usage Guideline case. The filenames retain
// explicit AskISO provenance, so a later import cannot count them as
// independent user-provided conformance evidence.
func ExportValidSamples(source, workspace, output string) (*SampleExport, error) {
	return ExportValidSamplesWithOptions(source, workspace, output, SampleExportOptions{})
}

// ExportValidSamplesWithOptions is ExportValidSamples with an explicit local
// transport wrapper.
func ExportValidSamplesWithOptions(source, workspace, output string, options SampleExportOptions) (*SampleExport, error) {
	options, err := normaliseTransportOptions(options)
	if err != nil {
		return nil, err
	}
	verification, err := exportVerify(source, workspace)
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

	report := &SampleExport{Release: manifest.Release, Output: outputRoot, Profile: options.Profile}
	seen := map[string]bool{}
	var pending []pendingSample
	for _, testCase := range suite.Cases {
		if testCase.Origin != "generated" || testCase.Expected != "valid" {
			continue
		}
		key := testCase.MessageID + "|" + testCase.BusinessService
		if seen[key] {
			return nil, fmt.Errorf("generated suite has more than one positive case for %s", key)
		}
		seen[key] = true
		payloadPath, err := exportSafeGenerated(dataRoot, testCase.Sample)
		if err != nil {
			return nil, err
		}
		payload, err := exportReadBounded(payloadPath)
		if err != nil {
			return nil, err
		}
		payloadSchemaPath, err := exportSafeSource(sourceRoot, testCase.Schema)
		if err != nil {
			return nil, err
		}
		payloadSchema, err := exportParseSchema(payloadSchemaPath)
		if err != nil {
			return nil, err
		}

		headerRelative, err := pairedHeaderSchema(manifest.Sources, testCase.Schema)
		if err != nil {
			return nil, err
		}
		headerPath, err := exportSafeSource(sourceRoot, headerRelative)
		if err != nil {
			return nil, err
		}
		headerSchema, err := exportParseSchema(headerPath)
		if err != nil {
			return nil, err
		}
		header, err := schemagen.Generate(headerSchema, schemagen.Options{
			Repeats: 1, MaxDepth: 30,
			Values: map[string]string{
				"BICFI": "AAAAGB2LXXX", "BizMsgIdr": fmt.Sprintf("ASKISO-20260905-%04d", report.Generated+1),
				"MsgDefIdr": testCase.MessageID, "BizSvc": testCase.BusinessService,
				"CreDt": "2026-09-05T09:00:00+00:00",
			},
		})
		if err != nil {
			return nil, fmt.Errorf("generating BAH for %s: %w", key, err)
		}
		if verdict := validator.Validate([]byte(header.XML), headerSchema); !verdict.Valid {
			detail := "unknown validation error"
			if len(verdict.Errors) > 0 {
				detail = verdict.Errors[0].String()
			}
			return nil, fmt.Errorf("generated BAH for %s did not validate: %s", key, detail)
		}
		envelope, err := transportEnvelope([]byte(header.XML), payload, testCase.MessageID,
			fmt.Sprintf("ASKISO-20260905-%04d", report.Generated+1), options)
		if err != nil {
			return nil, err
		}
		validationDocument, envelopeErrors := validationPayload(envelope, testCase.MessageID, testCase.BusinessService)
		if verdict := validator.ValidateWithExternalSets(validationDocument, payloadSchema, external); !verdict.Valid || envelopeErrors != 0 {
			return nil, fmt.Errorf("generated envelope for %s did not validate (%d payload, %d BAH error(s))", key, len(verdict.Errors), envelopeErrors)
		}

		name := fmt.Sprintf("%02d %s - %s - askiso-generated.valid.xml", report.Generated+1, testCase.MessageID, testCase.BusinessService)
		report.Generated++
		report.Files = append(report.Files, name)
		pending = append(pending, pendingSample{name: name, data: envelope})
	}
	if report.Generated == 0 {
		return nil, errors.New("workspace has no generated positive cases; import again with --generate-samples")
	}
	for _, sample := range pending {
		if err := exportWriteSample(filepath.Join(outputRoot, sample.name), sample.data); err != nil {
			return nil, err
		}
	}
	return report, nil
}

func prepareSampleOutput(sourceRoot, workspaceRoot, output string) (string, error) {
	if strings.TrimSpace(output) == "" {
		return "", errors.New("sample output directory is required")
	}
	root, err := sampleAbs(filepath.Clean(output))
	if err != nil {
		return "", err
	}
	if !within(sourceRoot, root) {
		return "", errors.New("sample output must be inside the private source directory")
	}
	if within(workspaceRoot, root) {
		return "", errors.New("sample output must be outside the generated workspace")
	}
	if info, err := sampleLstat(root); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return "", errors.New("sample output must be a real directory")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	} else if err := sampleMkdirAll(root, 0o700); err != nil {
		return "", err
	}
	if err := sampleChmod(root, 0o700); err != nil {
		return "", err
	}
	return root, nil
}

func pairedHeaderSchema(sources []SourceFile, payloadSchema string) (string, error) {
	dir := filepath.ToSlash(filepath.Dir(payloadSchema))
	var matches []string
	for _, source := range sources {
		if source.Kind == "schema" && strings.HasPrefix(source.MessageID, "head.") && filepath.ToSlash(filepath.Dir(source.Path)) == dir {
			matches = append(matches, source.Path)
		}
	}
	if len(matches) != 1 {
		return "", fmt.Errorf("payload schema %s has %d paired BAH schemas", payloadSchema, len(matches))
	}
	return matches[0], nil
}

func syntheticEnvelope(header, payload []byte) []byte {
	header = stripXMLDeclaration(header)
	payload = stripXMLDeclaration(payload)
	return []byte("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<Envelope>\n" +
		strings.TrimSpace(string(header)) + "\n" + strings.TrimSpace(string(payload)) + "\n</Envelope>\n")
}

func stripXMLDeclaration(data []byte) []byte {
	text := strings.TrimSpace(string(data))
	if strings.HasPrefix(text, "<?xml") {
		if end := strings.Index(text, "?>"); end >= 0 {
			text = strings.TrimSpace(text[end+2:])
		}
	}
	return []byte(text)
}

func writeExportedSample(path string, data []byte) error {
	if info, err := sampleLstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("refusing to overwrite non-regular sample %s", filepath.Base(path))
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := sampleWriteFile(path, data, 0o600); err != nil {
		return err
	}
	return sampleChmod(path, 0o600)
}
