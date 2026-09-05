// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package cmd

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebastienrousseau/askiso/internal/cbprworkspace"
	"github.com/sebastienrousseau/askiso/pkg/iso20022"
	"github.com/spf13/cobra"
)

const commandWorkspaceSchema = `<?xml version="1.0"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"
 targetNamespace="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.08"
 xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.08" elementFormDefault="qualified">
 <xs:simpleType name="ExternalPurpose1Code"><xs:restriction base="xs:string"><xs:minLength value="1"/><xs:maxLength value="4"/></xs:restriction></xs:simpleType>
 <xs:complexType name="DocumentType"><xs:sequence><xs:element name="Purpose" type="ExternalPurpose1Code"/></xs:sequence></xs:complexType>
 <xs:element name="Document" type="DocumentType"/>
</xs:schema>`

const commandHeaderSchema = `<?xml version="1.0"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"
 targetNamespace="urn:iso:std:iso:20022:tech:xsd:head.001.001.02"
 xmlns="urn:iso:std:iso:20022:tech:xsd:head.001.001.02" elementFormDefault="qualified">
 <xs:simpleType name="Text"><xs:restriction base="xs:string"><xs:minLength value="1"/></xs:restriction></xs:simpleType>
 <xs:complexType name="Party"><xs:sequence><xs:element name="FIId" type="Text"/></xs:sequence></xs:complexType>
 <xs:complexType name="Header"><xs:sequence><xs:element name="Fr" type="Party"/><xs:element name="To" type="Party"/>
  <xs:element name="BizMsgIdr" type="Text"/><xs:element name="MsgDefIdr" type="Text"/>
  <xs:element name="BizSvc" type="Text"/><xs:element name="CreDt" type="xs:dateTime"/>
 </xs:sequence></xs:complexType><xs:element name="AppHdr" type="Header"/>
</xs:schema>`

func TestCBPRWorkspaceCommands(t *testing.T) {
	pack := fixtureCompiledCBPRPack(t)
	source := filepath.Dir(pack)
	write := func(name, body string) string {
		path := filepath.Join(source, name)
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	write("pacs.008.001.08.xsd", commandWorkspaceSchema)
	write("payment.xml", `<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.08"><Purpose>SALA</Purpose></Document>`)
	external := write("2Q2026_externalcodesets_v3.json",
		`{"definitions":{"ExternalPurpose1Code":{"type":"string","enum":["SALA"]}}}`)
	workspace := filepath.Join(t.TempDir(), "workspace")

	out, err := run(t, "cbpr-pack", "import", source, "--workspace", workspace, "--external-codes", external)
	if err != nil {
		t.Fatalf("workspace import: %v\n%s", err, out)
	}
	wantContains(t, out, "private workspace ready", "Guideline JSON", "External codes", "Local only")

	out, err = run(t, "cbpr-pack", "status", workspace, "--json")
	if err != nil {
		t.Fatalf("workspace status: %v\n%s", err, out)
	}
	var manifest cbprworkspace.Manifest
	if err := json.Unmarshal([]byte(out), &manifest); err != nil || manifest.SuiteCases != 1 {
		t.Fatalf("status manifest: %v, %+v\n%s", err, manifest, out)
	}
	out, err = run(t, "cbpr-pack", "status", workspace)
	if err != nil {
		t.Fatalf("workspace status text: %v\n%s", err, out)
	}
	wantContains(t, out, "workspace", "Usage Guidelines", "Constraints", "Guideline JSON", "Suite cases")

	cbprMessage := writeTemp(t, "cbpr-payment.xml", cbprPackMessage)
	out, err = run(t, "lint", cbprMessage, "--cbpr-workspace", workspace, "--json")
	if err == nil || !strings.Contains(out, `"cbpr_pack"`) || !strings.Contains(out, "PackRequired") {
		t.Fatalf("workspace lint did not load its pack: %v\n%s", err, out)
	}
	out, err = run(t, "batch", cbprMessage, "--cbpr-workspace", workspace, "--format", "json")
	if err == nil || !strings.Contains(out, `"cbpr_pack"`) || !strings.Contains(out, "PackRequired") {
		t.Fatalf("workspace batch did not load its pack: %v\n%s", err, out)
	}

	out, err = run(t, "cbpr-pack", "verify", source, "--workspace", workspace)
	if err != nil || !strings.Contains(out, "1 passed, 0 failed") {
		t.Fatalf("workspace verify: %v\n%s", err, out)
	}

	// A normal-looking sample with a disallowed external code is expected valid
	// by the filename convention, so it exercises the visible failed-case path.
	write("payment.xml", `<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.08"><Purpose>NOPE</Purpose></Document>`)
	out, err = run(t, "cbpr-pack", "import", source, "--workspace", workspace, "--json")
	if err != nil || !strings.Contains(out, `"format": "askiso-cbpr-workspace/v1"`) {
		t.Fatalf("workspace JSON import: %v\n%s", err, out)
	}
	out, err = run(t, "cbpr-pack", "verify", source, "--workspace", workspace)
	if err == nil || !strings.Contains(out, "FAIL") || !strings.Contains(err.Error(), "1 local conformance") {
		t.Fatalf("failed workspace case: %v\n%s", err, out)
	}
}

func TestCBPRWorkspaceGeneratesLocalFixtures(t *testing.T) {
	previous, previousOutput := cbprGenerateSamples, cbprSampleOutput
	t.Cleanup(func() { cbprGenerateSamples, cbprSampleOutput = previous, previousOutput })
	source, _, external := commandWorkspaceFixture(t)
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(source, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("pacs.008.001.08.xsd", strings.Replace(commandWorkspaceSchema,
		"<xs:simpleType", "<xs:annotation><xs:documentation>swift.cbprplus.03</xs:documentation></xs:annotation><xs:simpleType", 1))
	write("head.001.001.02.xsd", commandHeaderSchema)
	workspace := filepath.Join(t.TempDir(), "generated-workspace")
	out, err := run(t, "cbpr-pack", "import", source, "--workspace", workspace,
		"--external-codes", external, "--generate-samples")
	if err != nil {
		t.Fatalf("generated workspace import: %v\n%s", err, out)
	}
	wantContains(t, out, "Executable XML UGs: 1/31", "2 workspace-generated", "Suite cases      : 2")
	out, err = run(t, "cbpr-pack", "verify", source, "--workspace", workspace)
	if err != nil || !strings.Contains(out, "2 passed, 0 failed") {
		t.Fatalf("generated workspace verify: %v\n%s", err, out)
	}
	output := filepath.Join(source, "04 Conformance Samples", "Valid")
	out, err = run(t, "cbpr-pack", "export-valid-samples", source, "--workspace", workspace, "--output", output)
	if err != nil {
		t.Fatalf("valid sample export: %v\n%s", err, out)
	}
	wantContains(t, out, "exported 1 AskISO-generated valid samples", "not independent conformance evidence")
	entries, err := os.ReadDir(output)
	if err != nil || len(entries) != 1 {
		t.Fatalf("exported files = %v, %v", entries, err)
	}
	out, err = run(t, "cbpr-pack", "export-valid-samples", source, "--workspace", workspace, "--output", output, "--json")
	if err != nil || !strings.Contains(out, `"generated": 1`) {
		t.Fatalf("valid sample JSON export: %v\n%s", err, out)
	}
}

func TestCBPRStrictConformanceCommandReportsGaps(t *testing.T) {
	previousGenerate, previousAck := cbprGenerateSamples, cbprAcknowledgeEntitlement
	previousAsOf, previousRequire := cbprAsOf, cbprRequireUserSamples
	t.Cleanup(func() {
		cbprGenerateSamples, cbprAcknowledgeEntitlement = previousGenerate, previousAck
		cbprAsOf, cbprRequireUserSamples = previousAsOf, previousRequire
	})
	source, _, external := commandWorkspaceFixture(t)
	schema := strings.Replace(commandWorkspaceSchema,
		"<xs:simpleType", "<xs:annotation><xs:documentation>swift.cbprplus.03</xs:documentation></xs:annotation><xs:simpleType", 1)
	if err := os.WriteFile(filepath.Join(source, "pacs.008.001.08.xsd"), []byte(schema), 0o600); err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(t.TempDir(), "strict-workspace")
	if out, err := run(t, "cbpr-pack", "import", source, "--workspace", workspace,
		"--external-codes", external, "--generate-samples", "--acknowledge-entitlement"); err != nil {
		t.Fatalf("strict fixture import: %v\n%s", err, out)
	}
	out, err := run(t, "cbpr-pack", "conformance", source, "--workspace", workspace,
		"--as-of", "2026-09-05", "--require-user-samples=false")
	if err == nil || !strings.Contains(out, "NOT READY") ||
		!strings.Contains(out, "PASS entitlement") || !strings.Contains(out, "FAIL executable-usage-guidelines") {
		t.Fatalf("strict gap report: %v\n%s", err, out)
	}
}

func TestWorkspaceProfileSelectionAndBatchExternalCodes(t *testing.T) {
	if _, _, err := resolveRuleProfileWithWorkspace("cbpr-plus", "pack", "workspace"); err == nil ||
		!strings.Contains(err.Error(), "cannot be used together") {
		t.Fatalf("conflicting CBPR sources error = %v", err)
	}

	source, workspace, external := commandWorkspaceFixture(t)
	if _, err := cbprworkspace.Import(cbprworkspace.Options{
		Source: source, Workspace: workspace, ExternalCodes: external,
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := resolveRuleProfileWithWorkspace("base", "", workspace); err == nil ||
		!strings.Contains(err.Error(), "requires --profile cbpr-plus") {
		t.Fatalf("wrong workspace profile error = %v", err)
	}

	catalogueRoot := filepath.Join(t.TempDir(), "catalogue")
	schemaDir := filepath.Join(catalogueRoot, "Payments", "Version 1", "Schemas")
	if err := os.MkdirAll(schemaDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(schemaDir, "pacs.008.001.08.xsd"), []byte(commandWorkspaceSchema), 0o600); err != nil {
		t.Fatal(err)
	}
	cat, err := iso20022.OpenCatalogue(catalogueRoot)
	if err != nil {
		t.Fatal(err)
	}
	workspaceRuntime, err := cbprworkspace.LoadRuntime(workspace)
	if err != nil {
		t.Fatal(err)
	}
	message := filepath.Join(t.TempDir(), "payment.xml")
	if err := os.WriteFile(message, []byte(
		`<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.08"><Purpose>NOPE</Purpose></Document>`), 0o600); err != nil {
		t.Fatal(err)
	}
	report := checkOneWithRuntime(message, cat, nil, workspaceRuntime.ExternalCodes)
	if report.Schema == nil || report.Schema.Valid {
		t.Fatalf("workspace external codes were not enforced: %+v", report)
	}
	found := false
	for _, issue := range report.Schema.Errors {
		if issue.Rule == "external code set" {
			found = true
		}
	}
	if !found {
		t.Fatalf("external-code finding absent: %+v", report.Schema.Errors)
	}
}

func commandWorkspaceFixture(t *testing.T) (string, string, string) {
	t.Helper()
	pack := fixtureCompiledCBPRPack(t)
	source := filepath.Dir(pack)
	workspace := filepath.Join(t.TempDir(), "workspace")
	external := filepath.Join(source, "2Q2026_externalcodesets_v3.json")
	if err := os.WriteFile(external,
		[]byte(`{"definitions":{"ExternalPurpose1Code":{"type":"string","enum":["SALA"]}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	return source, workspace, external
}

func TestCBPRWorkspaceCommandFailuresAndJSON(t *testing.T) {
	source := t.TempDir()
	workspace := filepath.Join(t.TempDir(), "workspace")
	if _, err := run(t, "cbpr-pack", "import", source, "--workspace", workspace, "--release", "SR2099"); err == nil {
		t.Fatal("unsupported workspace release succeeded")
	}
	if _, err := run(t, "cbpr-pack", "import", source); err == nil {
		t.Fatal("workspace import without --workspace succeeded")
	}

	if _, err := cbprworkspace.Import(cbprworkspace.Options{Source: source, Workspace: workspace}); err != nil {
		t.Fatal(err)
	}
	out, err := run(t, "cbpr-pack", "verify", source, "--workspace", workspace, "--json")
	if err != nil {
		t.Fatalf("empty JSON suite: %v\n%s", err, out)
	}
	var report cbprworkspace.Verification
	if err := json.Unmarshal([]byte(out), &report); err != nil || report.Cases != 0 {
		t.Fatalf("JSON verification = %v, %+v", err, report)
	}
}

func TestCBPRWorkspaceGenerationCommands(t *testing.T) {
	source, workspace, external := commandWorkspaceFixture(t)
	first, err := cbprworkspace.Import(cbprworkspace.Options{
		Source: source, Workspace: workspace, ExternalCodes: external,
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := cbprworkspace.Import(cbprworkspace.Options{
		Source: source, Workspace: workspace, ExternalCodes: external,
		EntitlementAcknowledged: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	out, err := run(t, "cbpr-pack", "generations", workspace)
	if err != nil || !strings.Contains(out, first.Fingerprint) || !strings.Contains(out, second.Fingerprint) ||
		!strings.Contains(out, "active") {
		t.Fatalf("generation inventory: %v\n%s", err, out)
	}
	out, err = run(t, "cbpr-pack", "activate", workspace, first.Fingerprint, "--json")
	if err != nil || !strings.Contains(out, first.Fingerprint) {
		t.Fatalf("generation activation: %v\n%s", err, out)
	}
	active, err := cbprworkspace.LoadManifest(workspace)
	if err != nil || active.Fingerprint != first.Fingerprint {
		t.Fatalf("active manifest = %+v, err=%v", active, err)
	}
	out, err = run(t, "cbpr-pack", "generations", workspace, "--json")
	if err != nil || !strings.Contains(out, `"active": true`) {
		t.Fatalf("JSON generation inventory: %v\n%s", err, out)
	}
	if _, err := run(t, "cbpr-pack", "activate", workspace, "invalid"); err == nil {
		t.Fatal("invalid generation activation succeeded")
	}
	if _, err := run(t, "cbpr-pack", "prune", workspace, "--keep", "1"); err == nil {
		t.Fatal("prune without confirmation succeeded")
	}
	out, err = run(t, "cbpr-pack", "prune", workspace, "--keep", "1", "--confirm")
	if err != nil || !strings.Contains(out, "Pruned 1") {
		t.Fatalf("confirmed prune: %v\n%s", err, out)
	}
}

func TestValidateWithDirectExternalCodePublication(t *testing.T) {
	schema := filepath.Join(t.TempDir(), "pacs.008.001.08.xsd")
	if err := os.WriteFile(schema, []byte(commandWorkspaceSchema), 0o600); err != nil {
		t.Fatal(err)
	}
	external := filepath.Join(t.TempDir(), "2Q2026_externalcodesets_v3.json")
	if err := os.WriteFile(external,
		[]byte(`{"definitions":{"ExternalPurpose1Code":{"type":"string","enum":["SALA"]}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	valid := writeTemp(t, "valid.xml",
		`<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.08"><Purpose>SALA</Purpose></Document>`)
	invalid := writeTemp(t, "invalid.xml",
		`<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.08"><Purpose>NOPE</Purpose></Document>`)
	if _, err := run(t, "validate", valid, schema, "--external-codes", external); err != nil {
		t.Fatalf("known direct external code: %v", err)
	}
	out, err := run(t, "validate", invalid, schema, "--external-codes", external, "--json")
	if err == nil || !strings.Contains(out, `"rule": "external code set"`) {
		t.Fatalf("unknown direct external code: %v\n%s", err, out)
	}
	if _, err := run(t, "validate", valid, schema, "--external-codes", external, "--engine", "libxml2"); err == nil ||
		!strings.Contains(err.Error(), "requires the Go") {
		t.Fatalf("libxml2 external-code error = %v", err)
	}
}

type failingCBPRWriter struct{}

func (failingCBPRWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func TestWriteCBPRJSONErrors(t *testing.T) {
	command := &cobra.Command{}
	if err := writeCBPRJSON(command, make(chan int)); err == nil {
		t.Fatal("unencodable CBPR JSON was accepted")
	}
	command.SetOut(failingCBPRWriter{})
	if err := writeCBPRJSON(command, map[string]bool{"ok": true}); err == nil {
		t.Fatal("CBPR JSON write error was ignored")
	}
}
