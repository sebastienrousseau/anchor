// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// batchFixture writes a directory of messages: two clean, two faulty.
func batchFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	clean := `<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">
  <MsgId>MSG-1</MsgId><Ccy>EUR</Ccy>
  <DbtrAcct><Id><IBAN>DE89370400440532013000</IBAN></Id></DbtrAcct>
  <Dbtr><PstlAdr><StrtNm>High St</StrtNm><TwnNm>London</TwnNm><Ctry>GB</Ctry></PstlAdr></Dbtr>
</Document>`

	write("clean1.xml", clean)
	write("clean2.xml", strings.Replace(clean, "MSG-1", "MSG-2", 1))
	write("bad_iban.xml", strings.Replace(clean, "DE89", "DE00", 1))
	write("bad_address.xml", strings.Replace(clean,
		`<PstlAdr><StrtNm>High St</StrtNm><TwnNm>London</TwnNm><Ctry>GB</Ctry></PstlAdr>`,
		`<PstlAdr><AdrLine>12 High Street</AdrLine></PstlAdr>`, 1))
	// A non-XML file must be ignored rather than reported.
	write("notes.txt", "ignore me")

	return dir
}

func TestBatchReportsFailures(t *testing.T) {
	dir := batchFixture(t)

	out, err := run(t, "batch", dir, "--profile", "cbpr-2026")
	if err == nil {
		t.Error("a directory containing faults should exit non-zero")
	}
	wantContains(t, out, "bad_iban.xml", "bad_address.xml", "clean1.xml", "2 passed, 2 failed")
	if strings.Contains(out, "notes.txt") {
		t.Error("non-XML files should be ignored")
	}
}

func TestBatchAllClean(t *testing.T) {
	dir := t.TempDir()
	body := `<?xml version="1.0"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10"><MsgId>M</MsgId></Document>`
	if err := os.WriteFile(filepath.Join(dir, "ok.xml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := run(t, "batch", dir)
	if err != nil {
		t.Fatalf("a clean directory should exit 0: %v\n%s", err, out)
	}
	wantContains(t, out, "1 passed, 0 failed")
}

func TestBatchJSON(t *testing.T) {
	dir := batchFixture(t)

	out, _ := run(t, "batch", dir, "--profile", "cbpr-2026", "--format", "json")

	var report struct {
		Files   int    `json:"files"`
		Passed  int    `json:"passed"`
		Failed  int    `json:"failed"`
		Errors  int    `json:"error_count"`
		Profile string `json:"profile"`
		Reports []struct {
			File      string `json:"file"`
			MessageID string `json:"message_id"`
		} `json:"reports"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &report); err != nil {
		t.Fatalf("batch --format json is not valid JSON: %v\n%s", err, out)
	}
	if report.Files != 4 || report.Passed != 2 || report.Failed != 2 {
		t.Errorf("unexpected totals: %+v", report)
	}
	if report.Profile != "cbpr-2026" {
		t.Errorf("profile = %q", report.Profile)
	}
	if report.Reports[0].MessageID != "pacs.008.001.10" {
		t.Errorf("the message type should be resolved: %+v", report.Reports[0])
	}
}

func TestBatchSARIF(t *testing.T) {
	dir := batchFixture(t)

	out, _ := run(t, "batch", dir, "--profile", "cbpr-2026", "--format", "sarif")

	var doc struct {
		Version string `json:"version"`
		Runs    []struct {
			Results []struct {
				RuleID string `json:"ruleId"`
			} `json:"results"`
		} `json:"runs"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &doc); err != nil {
		t.Fatalf("batch --format sarif is not valid JSON: %v\n%s", err, out)
	}
	if doc.Version != "2.1.0" {
		t.Errorf("version = %q", doc.Version)
	}
	if len(doc.Runs[0].Results) == 0 {
		t.Error("expected address findings in the SARIF output")
	}
}

func TestBatchWithSchemaValidation(t *testing.T) {
	root := withCatalogue(t)
	_ = root

	dir := t.TempDir()
	good := fixtureInstance("pacs.008.001.10", "EUR")
	bad := fixtureInstance("pacs.008.001.10", "EURO")
	if err := os.WriteFile(filepath.Join(dir, "good.xml"), []byte(good), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bad.xml"), []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := run(t, "batch", dir, "--schema")
	if err == nil {
		t.Error("a schema-invalid message should fail the batch")
	}
	wantContains(t, out, "bad.xml", "pattern")
}

func TestBatchSchemaNeedsCatalogue(t *testing.T) {
	isolate(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.xml"),
		[]byte(`<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10"/>`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "batch", dir, "--schema"); err == nil {
		t.Error("--schema without a catalogue should be an error")
	}
}

func TestBatchQuietListsOnlyFailures(t *testing.T) {
	dir := batchFixture(t)

	out, _ := run(t, "batch", dir, "--profile", "cbpr-2026", "-s")
	if strings.Contains(out, "clean1.xml") {
		t.Errorf("quiet mode should not list passing files:\n%s", out)
	}
	wantContains(t, out, "bad_iban.xml")
}

func TestBatchAcceptsFilesGlobsAndDirectories(t *testing.T) {
	dir := batchFixture(t)

	// A single file.
	if _, err := run(t, "batch", filepath.Join(dir, "clean1.xml")); err != nil {
		t.Errorf("a single clean file should pass: %v", err)
	}
	// A glob.
	if _, err := run(t, "batch", filepath.Join(dir, "clean*.xml")); err != nil {
		t.Errorf("a glob of clean files should pass: %v", err)
	}
	// Several arguments, deduplicated.
	out, _ := run(t, "batch", dir, filepath.Join(dir, "clean1.xml"), "--format", "json")
	var report struct {
		Files int `json:"files"`
	}
	_ = json.Unmarshal([]byte(strings.TrimSpace(out)), &report)
	if report.Files != 4 {
		t.Errorf("files should be deduplicated, got %d", report.Files)
	}
}

func TestBatchRejectsBadInput(t *testing.T) {
	if _, err := run(t, "batch", filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Error("a missing path should be an error")
	}
	if _, err := run(t, "batch", t.TempDir()); err == nil {
		t.Error("a directory with no messages should be an error")
	}
	dir := batchFixture(t)
	if _, err := run(t, "batch", dir, "--profile", "not-a-profile"); err == nil {
		t.Error("an unknown profile should be an error")
	}
}

func TestBatchUnreadableFileIsReported(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "broken.xml")
	if err := os.WriteFile(p, []byte("<not-closed>"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := run(t, "batch", dir)
	if err == nil {
		t.Error("malformed XML should fail the batch")
	}
	wantContains(t, out, "broken.xml")
}

// Reports come back in input order regardless of how work was scheduled.
func TestBatchOutputIsDeterministic(t *testing.T) {
	dir := batchFixture(t)

	var first string
	for i := 0; i < 5; i++ {
		out, _ := run(t, "batch", dir, "--profile", "cbpr-2026", "--workers", "4", "--format", "json")
		if i == 0 {
			first = out
			continue
		}
		if out != first {
			t.Fatal("batch output varies between runs")
		}
	}
}

func TestBatchSingleWorker(t *testing.T) {
	dir := batchFixture(t)
	if _, err := run(t, "batch", dir, "--workers", "1", "--format", "json"); err == nil {
		t.Error("the fixture contains faults; expected a non-zero exit")
	}
}
