// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package cmd

import (
	"archive/zip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Harness
// ---------------------------------------------------------------------------

// fixtureCatalogue writes a small but real catalogue: two schemas with a matching
// sample, so commands that read schema text have something to read without the
// developer having downloaded the specification.
func fixtureCatalogue(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	base := filepath.Join(root, "Payments Clearing and Settlement", "Version 11.0")
	schemas := filepath.Join(base, "Schemas")
	samples := filepath.Join(base, "Sample Messages")
	reports := filepath.Join(base, "Message Definition Reports")
	for _, d := range []string{schemas, samples, reports} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	write := func(p, body string) {
		t.Helper()
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write(filepath.Join(schemas, "pacs.008.001.10.xsd"), fixtureSchema("pacs.008.001.10"))
	write(filepath.Join(schemas, "pacs.008.001.09.xsd"), fixtureSchema("pacs.008.001.09"))
	write(filepath.Join(schemas, "pacs.009.001.10.xsd"), fixtureSchema("pacs.009.001.10"))
	write(filepath.Join(samples, "pacs.008.001.10.xml"), fixtureInstance("pacs.008.001.10", "EUR"))
	write(filepath.Join(reports, "ISO20022_MDRPart1_Payments_v11.pdf"), "%PDF-1.4 fixture")

	return root
}

func fixtureSchema(msgID string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<xs:schema xmlns="urn:iso:std:iso:20022:tech:xsd:` + msgID + `"
           xmlns:xs="http://www.w3.org/2001/XMLSchema"
           elementFormDefault="qualified"
           targetNamespace="urn:iso:std:iso:20022:tech:xsd:` + msgID + `">
  <xs:element name="Document" type="Document"/>
  <xs:complexType name="Document">
    <xs:sequence>
      <xs:element name="MsgId" type="Max35Text"/>
      <xs:element name="Ccy" type="CurrencyCode"/>
    </xs:sequence>
  </xs:complexType>
  <xs:simpleType name="Max35Text">
    <xs:restriction base="xs:string">
      <xs:minLength value="1"/><xs:maxLength value="35"/>
    </xs:restriction>
  </xs:simpleType>
  <xs:simpleType name="CurrencyCode">
    <xs:restriction base="xs:string"><xs:pattern value="[A-Z]{3,3}"/></xs:restriction>
  </xs:simpleType>
  <!-- An enumerated set, so the schema-backed code index has something to find. -->
  <xs:simpleType name="ChargeBearerType1Code">
    <xs:restriction base="xs:string">
      <xs:enumeration value="DEBT">
        <xs:annotation><xs:documentation>BorneByDebtor</xs:documentation></xs:annotation>
      </xs:enumeration>
      <xs:enumeration value="CRED">
        <xs:annotation><xs:documentation>BorneByCreditor</xs:documentation></xs:annotation>
      </xs:enumeration>
      <xs:enumeration value="SHAR">
        <xs:annotation><xs:documentation>Shared</xs:documentation></xs:annotation>
      </xs:enumeration>
    </xs:restriction>
  </xs:simpleType>
</xs:schema>`
}

func fixtureInstance(msgID, ccy string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:` + msgID + `">
  <MsgId>MSG-0001</MsgId>
  <Ccy>` + ccy + `</Ccy>
</Document>`
}

// withCatalogue points every command at a fixture catalogue for one test.
func withCatalogue(t *testing.T) string {
	t.Helper()
	root := fixtureCatalogue(t)
	t.Setenv("ANCHOR_CATALOG", root)

	prev := catalogPath
	catalogPath = ""
	t.Cleanup(func() { catalogPath = prev })
	return root
}

// run invokes a command through the real cobra tree and returns everything it
// printed. Commands write with fmt directly, so stdout is redirected.
func run(t *testing.T, args ...string) (string, error) {
	t.Helper()

	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	done := make(chan string, 1)
	go func() {
		var b strings.Builder
		buf := make([]byte, 4096)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				b.Write(buf[:n])
			}
			if err != nil {
				break
			}
		}
		done <- b.String()
	}()

	RootCmd.SetArgs(args)
	RootCmd.SetOut(w)
	RootCmd.SetErr(w)
	runErr := RootCmd.Execute()

	_ = w.Close()
	os.Stdout = orig
	out := <-done

	// Leave the shared command tree as it was found.
	RootCmd.SetArgs(nil)
	resetFlags(t)
	return out, runErr
}

// resetFlags restores package-level flag variables, which cobra binds once and
// which therefore leak between tests.
func resetFlags(t *testing.T) {
	t.Helper()
	searchJSON, infoJSON, statsJSON = false, false, false
	lintJSON, lintStrict, lintProfile = false, false, ""
	validateJSON, validateEngine, validateStream = false, "go", false
	codeJSON, codeCategory = false, ""
	convertToJSON, convertToXML, convertOutput, convertCopy = false, false, "", false
	formatMinify, formatCopy, formatOutput = false, false, ""
	genAmount, genCurrency, genPreset, genWithBAH, genCopy, genOutputFile = "", "", "standard", false, false, ""
	genDebtor, genCreditor, genDebtorIBAN, genCreditorIBAN = "", "", "", ""
	genFromSchema, genOptional, genRepeats = false, false, 1
	graphPreset, graphFormat, graphCopy = "sepa", "ascii", false
	flowPreset, flowAmount, flowCurrency, flowOutputDir, flowJSON = "sepa", "15000.00", "EUR", "", false
	schemaCopy, schemaRaw, sampleCopy, sampleRaw = false, false, false, false
	catalogAddDest, catalogAddCategory, catalogAddVersion, catalogAddDryRun = "", "", "", false
	fetchWatchDir, fetchNoOpen, fetchDest = "", false, ""
	fetchTimeout = 5 * time.Minute
	catalogStatusAll, showMatrix = false, false
	diffJSON, diffBreakingOnly, diffStrict = false, false, false
	translateOut, translateReport, translateFormat = "", false, "text"
	batchProfile, batchFormat, batchSchema, batchWorkers, batchQuiet = "", "text", false, 0, false
	codeSet, codeListSets, codeLimit, codeAll, codeImport = "", false, 25, false, ""
	lintFormat = "text"
}

func wantContains(t *testing.T, out string, subs ...string) {
	t.Helper()
	for _, s := range subs {
		if !strings.Contains(out, s) {
			t.Errorf("output should contain %q:\n%s", s, out)
		}
	}
}

// ---------------------------------------------------------------------------
// Commands that need no catalogue
// ---------------------------------------------------------------------------

func TestVersionCommand(t *testing.T) {
	out, err := run(t, "version")
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	wantContains(t, out, "Anchor")
}

func TestGenerateCommand(t *testing.T) {
	for _, msg := range []string{"pacs.008", "pacs.009", "pain.001", "camt.053"} {
		t.Run(msg, func(t *testing.T) {
			out, err := run(t, "generate", msg)
			if err != nil {
				t.Fatalf("generate %s: %v", msg, err)
			}
			wantContains(t, out, "<Document", "urn:iso:std:iso:20022:tech:xsd:"+msg)
		})
	}
}

func TestGenerateFlags(t *testing.T) {
	dir := t.TempDir()
	outFile := filepath.Join(dir, "gen.xml")

	_, err := run(t, "generate", "pacs.008",
		"--preset", "sepa",
		"--amount", "1234.56",
		"--currency", "EUR",
		"--debtor", "Test Debtor",
		"--creditor", "Test Creditor",
		"--debtor-iban", "DE89370400440532013000",
		"--creditor-iban", "FR7630006000011234567890189",
		"--bah",
		"-o", outFile)
	if err != nil {
		t.Fatalf("generate with flags: %v", err)
	}

	body, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("output file: %v", err)
	}
	wantContains(t, string(body), "1234.56", "Test Debtor", "Test Creditor", "<AppHdr", "pacs.008.001.10")
}

func TestGenerateUnsupportedType(t *testing.T) {
	if _, err := run(t, "generate", "zzzz.999"); err == nil {
		t.Error("an unsupported message type should be an error")
	}
}

func TestCodeCommand(t *testing.T) {
	out, err := run(t, "code", "AC04")
	if err != nil {
		t.Fatalf("code: %v", err)
	}
	wantContains(t, out, "AC04", "Account Closed")

	out, err = run(t, "code", "AC04", "--json")
	if err != nil {
		t.Fatalf("code --json: %v", err)
	}
	// Curated codes and codes read out of an installed catalogue are reported
	// separately, so a consumer can tell a hand-written definition from one
	// extracted from the user's own schemas.
	var payload struct {
		Curated []map[string]any `json:"curated"`
		Schema  []map[string]any `json:"schema"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &payload); err != nil {
		t.Fatalf("code --json is not valid JSON: %v\n%s", err, out)
	}
	if len(payload.Curated) == 0 || payload.Curated[0]["code"] != "AC04" {
		t.Errorf("unexpected JSON: %v", payload.Curated)
	}

	if _, err := run(t, "code"); err != nil {
		t.Errorf("code with no argument should list everything: %v", err)
	}
	if _, err := run(t, "code", "--category", "reason"); err != nil {
		t.Errorf("code --category: %v", err)
	}
	if _, err := run(t, "code", "definitely-not-a-code"); err == nil {
		t.Error("an unknown code should be an error")
	}
}

func TestTranslateCommand(t *testing.T) {
	out, err := run(t, "translate", "MT103")
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	wantContains(t, out, "MT103", "pacs.008")

	out, err = run(t, "translate", "--matrix")
	if err != nil {
		t.Fatalf("translate --matrix: %v", err)
	}
	wantContains(t, out, "MT103", "MT940")

	if _, err := run(t, "translate", "MT999999"); err == nil {
		t.Error("an unknown MT code should be an error")
	}
}

func TestGraphCommand(t *testing.T) {
	out, err := run(t, "graph", "pacs.008", "--format", "mermaid")
	if err != nil {
		t.Fatalf("graph mermaid: %v", err)
	}
	wantContains(t, out, "sequenceDiagram")

	out, err = run(t, "graph", "pacs.008", "--format", "ascii", "--preset", "chaps")
	if err != nil {
		t.Fatalf("graph ascii: %v", err)
	}
	if strings.Contains(out, "sequenceDiagram") {
		t.Error("ascii format should not emit mermaid")
	}
}

func TestFlowCommand(t *testing.T) {
	out, err := run(t, "flow", "pacs.008")
	if err != nil {
		t.Fatalf("flow: %v", err)
	}
	wantContains(t, out, "pain.001", "pacs.008", "pacs.002", "camt.053")

	out, err = run(t, "flow", "--json")
	if err != nil {
		t.Fatalf("flow --json: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &payload); err != nil {
		t.Fatalf("flow --json is not valid JSON: %v", err)
	}

	dir := t.TempDir()
	if _, err := run(t, "flow", "--output-dir", dir, "--preset", "target2",
		"--amount", "99.99", "--currency", "EUR"); err != nil {
		t.Fatalf("flow --output-dir: %v", err)
	}
	files, _ := os.ReadDir(dir)
	if len(files) < 4 {
		t.Errorf("expected four exported payloads, got %d", len(files))
	}
}

func TestLintCommand(t *testing.T) {
	dir := t.TempDir()

	good := filepath.Join(dir, "good.xml")
	gen, err := run(t, "generate", "pacs.008", "--preset", "sepa")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(good, []byte(gen), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := run(t, "lint", good)
	if err != nil {
		t.Fatalf("lint on a clean message: %v\n%s", err, out)
	}
	wantContains(t, out, "passed")

	// A broken IBAN checksum must be reported and must fail the command.
	bad := filepath.Join(dir, "bad.xml")
	if err := os.WriteFile(bad, []byte(strings.Replace(gen, "DE89", "DE00", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err = run(t, "lint", bad)
	if err == nil {
		t.Error("lint should fail on a bad IBAN")
	}
	wantContains(t, out, "IBAN")

	out, _ = run(t, "lint", bad, "--json")
	var res map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &res); err != nil {
		t.Fatalf("lint --json is not valid JSON: %v\n%s", err, out)
	}

	if _, err := run(t, "lint", filepath.Join(dir, "missing.xml")); err == nil {
		t.Error("a missing file should be an error")
	}
}

func TestLintStrict(t *testing.T) {
	dir := t.TempDir()
	// Settlement date before creation raises a warning, not an error.
	doc := `<?xml version="1.0"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">
  <CreDtTm>2026-08-23T10:00:00Z</CreDtTm>
  <IntrBkSttlmDt>2020-01-01</IntrBkSttlmDt>
</Document>`
	p := filepath.Join(dir, "warn.xml")
	if err := os.WriteFile(p, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := run(t, "lint", p); err != nil {
		t.Errorf("a warning alone should not fail lint: %v", err)
	}
	if _, err := run(t, "lint", p, "--strict"); err == nil {
		t.Error("--strict should turn a warning into a failure")
	}
}

func TestFormatCommand(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "in.xml")
	if err := os.WriteFile(src, []byte(fixtureInstance("pacs.008.001.10", "EUR")), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := run(t, "format", src); err != nil {
		t.Fatalf("format: %v", err)
	}

	minified := filepath.Join(dir, "min.xml")
	if _, err := run(t, "format", src, "--minify", "-o", minified); err != nil {
		t.Fatalf("format --minify: %v", err)
	}
	body, err := os.ReadFile(minified)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(body), "\n") > 1 {
		t.Errorf("minified output should be on one line:\n%s", body)
	}

	broken := filepath.Join(dir, "broken.xml")
	if err := os.WriteFile(broken, []byte("<not-closed>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "format", broken); err == nil {
		t.Error("malformed XML should be an error")
	}
	if _, err := run(t, "format", filepath.Join(dir, "nope.xml")); err == nil {
		t.Error("a missing file should be an error")
	}
}

func TestConvertCommand(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "in.xml")
	if err := os.WriteFile(src, []byte(fixtureInstance("pacs.008.001.10", "EUR")), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := run(t, "convert", src, "--to-json")
	if err != nil {
		t.Fatalf("convert --to-json: %v", err)
	}
	wantContains(t, out, "Document", "MsgId")

	jsonFile := filepath.Join(dir, "out.json")
	if _, err := run(t, "convert", src, "--to-json", "-o", jsonFile); err != nil {
		t.Fatalf("convert -o: %v", err)
	}
	if _, err := run(t, "convert", jsonFile, "--to-xml"); err != nil {
		t.Fatalf("convert --to-xml: %v", err)
	}

	// With no direction flag the command infers from the extension.
	if _, err := run(t, "convert", src); err != nil {
		t.Errorf("convert should infer direction from the file type: %v", err)
	}
	if _, err := run(t, "convert", filepath.Join(dir, "missing.xml"), "--to-json"); err == nil {
		t.Error("a missing file should be an error")
	}
}

func TestCompletionCommand(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
		out, err := run(t, "completion", shell)
		if err != nil {
			t.Errorf("completion %s: %v", shell, err)
		}
		if len(out) == 0 {
			t.Errorf("completion %s produced nothing", shell)
		}
	}
	if _, err := run(t, "completion", "tcsh"); err == nil {
		t.Error("an unsupported shell should be an error")
	}
}

// ---------------------------------------------------------------------------
// Commands that read the catalogue
// ---------------------------------------------------------------------------

func TestSearchWithCatalogue(t *testing.T) {
	withCatalogue(t)

	out, err := run(t, "search", "pacs.008")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	wantContains(t, out, "pacs.008.001.10")

	out, err = run(t, "search", "pacs.008", "--json")
	if err != nil {
		t.Fatalf("search --json: %v", err)
	}
	var hits []map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &hits); err != nil {
		t.Fatalf("search --json is not valid JSON: %v\n%s", err, out)
	}
	if len(hits) == 0 {
		t.Error("expected search hits")
	}
}

func TestInfoWithCatalogue(t *testing.T) {
	withCatalogue(t)

	out, err := run(t, "info", "pacs.008.001.10")
	if err != nil {
		t.Fatalf("info: %v", err)
	}
	wantContains(t, out, "pacs.008.001.10", "Payments Clearing and Settlement")

	out, err = run(t, "info", "pacs.008.001.10", "--json")
	if err != nil {
		t.Fatalf("info --json: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &payload); err != nil {
		t.Fatalf("info --json is not valid JSON: %v\n%s", err, out)
	}
}

func TestSchemaCommand(t *testing.T) {
	withCatalogue(t)

	out, err := run(t, "schema", "pacs.008.001.10")
	if err != nil {
		t.Fatalf("schema: %v", err)
	}
	wantContains(t, out, "xs:schema")

	if _, err := run(t, "schema", "pacs.008.001.10", "--raw"); err != nil {
		t.Errorf("schema --raw: %v", err)
	}
	if _, err := run(t, "schema", "zzzz.999.999.99"); err == nil {
		t.Error("an unknown identifier should be an error")
	}
}

func TestSampleCommand(t *testing.T) {
	withCatalogue(t)

	out, err := run(t, "sample", "pacs.008.001.10")
	if err != nil {
		t.Fatalf("sample: %v", err)
	}
	wantContains(t, out, "<Document")

	if _, err := run(t, "sample", "pacs.008.001.10", "--raw"); err != nil {
		t.Errorf("sample --raw: %v", err)
	}

	// No published sample for this one, so a generated one stands in.
	out, err = run(t, "sample", "pacs.009.001.10")
	if err != nil {
		t.Fatalf("sample should fall back to generation: %v", err)
	}
	wantContains(t, out, "<Document")
}

func TestDiffCommand(t *testing.T) {
	withCatalogue(t)

	out, err := run(t, "diff", "pacs.008.001.09", "pacs.008.001.10")
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	wantContains(t, out, "pacs.008.001.09", "pacs.008.001.10")

	if _, err := run(t, "diff", "zzzz.999.999.99", "pacs.008.001.10"); err == nil {
		t.Error("an unknown identifier should be an error")
	}
}

func TestStatsCommand(t *testing.T) {
	withCatalogue(t)

	out, err := run(t, "stats")
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	wantContains(t, out, "pacs")

	out, err = run(t, "stats", "--json")
	if err != nil {
		t.Fatalf("stats --json: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &payload); err != nil {
		t.Fatalf("stats --json is not valid JSON: %v\n%s", err, out)
	}
}

func TestListCommand(t *testing.T) {
	withCatalogue(t)
	out, err := run(t, "list")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	// list enumerates categories, not individual messages.
	wantContains(t, out, "Payments Clearing and Settlement", "MESSAGE CATEGORIES")
}

func TestDoctorCommand(t *testing.T) {
	withCatalogue(t)
	out, err := run(t, "doctor")
	if err != nil {
		t.Fatalf("doctor with a catalogue installed: %v\n%s", err, out)
	}
	wantContains(t, out, "Catalogue", "message definitions")
}

func TestValidateCommand(t *testing.T) {
	root := withCatalogue(t)
	dir := t.TempDir()

	good := filepath.Join(dir, "good.xml")
	if err := os.WriteFile(good, []byte(fixtureInstance("pacs.008.001.10", "EUR")), 0o644); err != nil {
		t.Fatal(err)
	}

	// The schema is resolved from the document's own namespace.
	out, err := run(t, "validate", good)
	if err != nil {
		t.Fatalf("validate: %v\n%s", err, out)
	}
	wantContains(t, out, "VALID")

	// Explicit schema path.
	schemaPath := filepath.Join(root, "Payments Clearing and Settlement", "Version 11.0",
		"Schemas", "pacs.008.001.10.xsd")
	if _, err := run(t, "validate", good, schemaPath); err != nil {
		t.Errorf("validate with an explicit schema: %v", err)
	}

	bad := filepath.Join(dir, "bad.xml")
	if err := os.WriteFile(bad, []byte(fixtureInstance("pacs.008.001.10", "EURO")), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err = run(t, "validate", bad)
	if err == nil {
		t.Error("an invalid document should fail")
	}
	wantContains(t, out, "INVALID", "pattern")

	out, _ = run(t, "validate", bad, "--json")
	var payload map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &payload); err != nil {
		t.Fatalf("validate --json is not valid JSON: %v\n%s", err, out)
	}
	if payload["valid"] != false {
		t.Errorf("valid should be false: %v", payload)
	}

	if _, err := run(t, "validate", filepath.Join(dir, "missing.xml")); err == nil {
		t.Error("a missing file should be an error")
	}
	if _, err := run(t, "validate", good, filepath.Join(dir, "missing.xsd")); err == nil {
		t.Error("a missing schema should be an error")
	}

	// A document with no ISO 20022 namespace cannot be resolved.
	plain := filepath.Join(dir, "plain.xml")
	if err := os.WriteFile(plain, []byte(`<root/>`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "validate", plain); err == nil {
		t.Error("a document with no ISO namespace should be an error")
	}
}

// ---------------------------------------------------------------------------
// catalog subcommands
// ---------------------------------------------------------------------------

func TestCatalogWhere(t *testing.T) {
	withCatalogue(t)
	out, err := run(t, "catalog", "where")
	if err != nil {
		t.Fatalf("catalog where: %v", err)
	}
	wantContains(t, out, "ANCHOR_CATALOG", "Using:")
}

func TestCatalogWhereWithoutCatalogue(t *testing.T) {
	isolate(t)
	out, err := run(t, "catalog", "where")
	if err != nil {
		t.Fatalf("catalog where should not fail: %v", err)
	}
	wantContains(t, out, "iso20022.org")
}

func TestCatalogStatus(t *testing.T) {
	withCatalogue(t)
	out, err := run(t, "catalog", "status")
	if err != nil {
		t.Fatalf("catalog status: %v", err)
	}
	wantContains(t, out, "published sets")

	if _, err := run(t, "catalog", "status", "--all"); err != nil {
		t.Errorf("catalog status --all: %v", err)
	}
}

func TestCatalogAdd(t *testing.T) {
	dest := t.TempDir()
	src := t.TempDir()

	archive := filepath.Join(src, "PaymentsClearingAndSettlement_v11.zip")
	writeTestZip(t, archive, map[string]string{
		"pacs.008.001.10.xsd": fixtureSchema("pacs.008.001.10"),
		"pacs.008.001.10.xml": fixtureInstance("pacs.008.001.10", "EUR"),
		"notes.bin":           "junk",
	})

	out, err := run(t, "catalog", "add", archive, "--to", dest)
	if err != nil {
		t.Fatalf("catalog add: %v\n%s", err, out)
	}
	wantContains(t, out, "schema")

	if _, err := os.Stat(filepath.Join(dest,
		"Payments Clearing and Settlement", "Version 11.0", "Schemas", "pacs.008.001.10.xsd")); err != nil {
		t.Errorf("schema was not imported: %v", err)
	}

	// Dry run writes nothing.
	dryDest := t.TempDir()
	if _, err := run(t, "catalog", "add", archive, "--to", dryDest, "--dry-run"); err != nil {
		t.Fatalf("catalog add --dry-run: %v", err)
	}
	if entries, _ := os.ReadDir(dryDest); len(entries) != 0 {
		t.Error("a dry run should write nothing")
	}

	// Overrides.
	ovDest := t.TempDir()
	if _, err := run(t, "catalog", "add", archive, "--to", ovDest,
		"--category", "Custom Category", "--version", "Version 9.0"); err != nil {
		t.Fatalf("catalog add with overrides: %v", err)
	}
	if _, err := os.Stat(filepath.Join(ovDest, "Custom Category", "Version 9.0", "Schemas")); err != nil {
		t.Errorf("overrides were not honoured: %v", err)
	}

	// A directory of archives.
	dirDest := t.TempDir()
	if _, err := run(t, "catalog", "add", src, "--to", dirDest); err != nil {
		t.Fatalf("catalog add on a directory: %v", err)
	}

	if _, err := run(t, "catalog", "add", filepath.Join(src, "nope.zip"), "--to", dest); err == nil {
		t.Error("a missing archive should be an error")
	}

	empty := filepath.Join(src, "empty.zip")
	writeTestZip(t, empty, map[string]string{"a.bin": "x"})
	if _, err := run(t, "catalog", "add", empty, "--to", dest); err == nil {
		t.Error("an archive with no ISO content should be an error")
	}
}

func TestRootHelp(t *testing.T) {
	out, err := run(t, "--help")
	if err != nil {
		t.Fatalf("--help: %v", err)
	}
	wantContains(t, out, "anchor", "Available Commands")
}

// writeTestZip builds a zip archive from a name/content map.
func writeTestZip(t *testing.T, path string, entries map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	zw := zip.NewWriter(f)
	for name, body := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}
