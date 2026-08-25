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

// The branches a user hits when something is wrong: a missing file, a bad
// flag combination, a directory with nothing in it. These are the paths a
// happy-path test never reaches and the ones a user reaches first.

const validMessage = `<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">
  <FIToFICstmrCdtTrf>
    <GrpHdr>
      <MsgId>MSG-0001</MsgId>
      <CreDtTm>2026-08-26T10:00:00</CreDtTm>
      <NbOfTxs>1</NbOfTxs>
      <SttlmInf><SttlmMtd>INDA</SttlmMtd></SttlmInf>
    </GrpHdr>
    <CdtTrfTxInf>
      <PmtId>
        <EndToEndId>E2E-0001</EndToEndId>
        <UETR>7a8b9c0d-1e2f-4a3b-8c4d-5e6f7a8b9c0d</UETR>
      </PmtId>
      <IntrBkSttlmAmt Ccy="EUR">1000.00</IntrBkSttlmAmt>
      <Cdtr>
        <Nm>Beispiel GmbH</Nm>
        <PstlAdr><AdrLine>Musterstrasse 1</AdrLine><AdrLine>Berlin</AdrLine></PstlAdr>
      </Cdtr>
    </CdtTrfTxInf>
  </FIToFICstmrCdtTrf>
</Document>`

func writeTemp(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// SARIF is for a pipeline, and a pipeline wants findings. Asking for it
// without a profile is a mistake worth naming rather than emitting an empty
// report that reads as "nothing wrong".
func TestLintSarifWithoutAProfileExplainsItself(t *testing.T) {
	path := writeTemp(t, "m.xml", validMessage)

	_, err := run(t, "lint", path, "--format", "sarif")
	if err == nil {
		t.Fatal("sarif without a profile should be an error")
	}
	if !strings.Contains(err.Error(), "--profile") {
		t.Errorf("the error does not say how to fix it: %v", err)
	}
}

func TestLintSarifWithAProfileWritesAReport(t *testing.T) {
	path := writeTemp(t, "m.xml", validMessage)

	out, _ := run(t, "lint", path, "--profile", "cbpr-2026", "--format", "sarif")
	if !strings.Contains(out, "\"version\"") || !strings.Contains(out, "2.1.0") {
		t.Errorf("output is not SARIF 2.1.0:\n%s", out[:min(len(out), 300)])
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Errorf("SARIF is not valid JSON: %v", err)
	}
}

func TestLintOnAMissingFile(t *testing.T) {
	if _, err := run(t, "lint", filepath.Join(t.TempDir(), "nope.xml")); err == nil {
		t.Error("linting a missing file should be an error")
	}
}

func TestValidateOnAMissingFile(t *testing.T) {
	if _, err := run(t, "validate", filepath.Join(t.TempDir(), "nope.xml")); err == nil {
		t.Error("validating a missing file should be an error")
	}
}

func TestValidateOnMalformedXML(t *testing.T) {
	path := writeTemp(t, "bad.xml", "<Document><unclosed>")
	if _, err := run(t, "validate", path); err == nil {
		t.Error("malformed XML should be an error")
	}
}

func TestBatchOnAnEmptyDirectory(t *testing.T) {
	out, err := run(t, "batch", t.TempDir())
	if err != nil && !strings.Contains(err.Error(), "no ") {
		t.Fatalf("batch on an empty directory: %v", err)
	}
	if strings.Contains(out, "panic") {
		t.Errorf("unexpected output:\n%s", out)
	}
}

// A file batch cannot read is reported against that file rather than aborting
// the run: one bad file in a directory of a thousand should not hide the other
// nine hundred and ninety-nine.
func TestBatchReportsAnUnreadableFileWithoutAborting(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "good.xml"), []byte(validMessage), 0o644); err != nil {
		t.Fatal(err)
	}
	// A directory named like a message: reading it fails, but the run must not.
	if err := os.Mkdir(filepath.Join(dir, "broken.xml"), 0o755); err != nil {
		t.Fatal(err)
	}

	out, _ := run(t, "batch", dir, "--format", "json")
	if !strings.Contains(out, "good.xml") {
		t.Errorf("the readable file is missing from the report:\n%s", out)
	}
}

func TestBatchWithAProfile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "m.xml"), []byte(validMessage), 0o644); err != nil {
		t.Fatal(err)
	}

	out, _ := run(t, "batch", dir, "--profile", "cbpr-2026", "--format", "json")
	if !strings.Contains(out, "CBPR-ADDR") {
		t.Errorf("the profile findings are missing:\n%s", out)
	}
}

func TestBatchWithAnUnknownProfile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "m.xml"), []byte(validMessage), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := run(t, "batch", dir, "--profile", "not-a-profile", "--format", "json")
	if err == nil && !strings.Contains(out, "not-a-profile") {
		t.Errorf("an unknown profile should be reported: %v\n%s", err, out)
	}
}

func TestGenerateWithAnUnknownMessageType(t *testing.T) {
	if _, err := run(t, "generate", "xxxx.999.001.99"); err == nil {
		t.Error("generating an unknown message should be an error")
	}
}

func TestSampleWithoutACatalogue(t *testing.T) {
	t.Setenv("ASKISO_CATALOG", filepath.Join(t.TempDir(), "none"))

	_, err := run(t, "sample", "pacs.008.001.10")
	if err == nil {
		t.Skip("a catalogue is installed")
	}
	if !strings.Contains(err.Error(), "catalogue") && !strings.Contains(err.Error(), "iso20022.org") {
		t.Errorf("the error does not say what to download: %v", err)
	}
}

func TestSchemaWithoutACatalogue(t *testing.T) {
	t.Setenv("ASKISO_CATALOG", filepath.Join(t.TempDir(), "none"))

	if _, err := run(t, "schema", "pacs.008.001.10"); err == nil {
		t.Skip("a catalogue is installed")
	}
}
