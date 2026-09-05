// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package cmd

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebastienrousseau/askiso/internal/rules"
)

func fixtureCompiledCBPRPack(t *testing.T) string {
	t.Helper()
	pack := &rules.CBPRPack{
		Format: "askiso-cbpr-pack/v1",
		Sources: []rules.CBPRPackSource{{
			Name: "local-pacs008.pdf", SHA256: strings.Repeat("d", 64),
			MessageID: "pacs.008.001.08", UsageIdentifiers: []string{"swift.cbprplus.03"}, Constraints: 1,
		}},
		Constraints: []rules.CBPRPackConstraint{{
			Source: "local-pacs008.pdf", MessageID: "pacs.008.001.08",
			UsageIdentifiers: []string{"swift.cbprplus.03"},
			Path:             []string{"Document", "FIToFICstmrCdtTrf", "GrpHdr", "PackRequired"},
			Min:              1, Max: 1,
		}},
		Warnings: []string{"local fixture has intentionally partial coverage"},
	}
	path := filepath.Join(t.TempDir(), "sr2025.cbpr-pack.json")
	if err := rules.WriteCBPRPack(path, pack); err != nil {
		t.Fatal(err)
	}
	return path
}

const cbprPackMessage = `<Envelope xmlns="urn:env">
<AppHdr xmlns="urn:iso:std:iso:20022:tech:xsd:head.001.001.02">
  <Fr><FIId><FinInstnId><BICFI>AAAAGB2LXXX</BICFI></FinInstnId></FIId></Fr>
  <To><FIId><FinInstnId><BICFI>BBBBUS33XXX</BICFI></FinInstnId></FIId></To>
  <BizMsgIdr>M-1</BizMsgIdr><MsgDefIdr>pacs.008.001.08</MsgDefIdr>
  <BizSvc>swift.cbprplus.03</BizSvc><CreDt>2025-11-24T07:41:50Z</CreDt>
</AppHdr>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.08">
  <FIToFICstmrCdtTrf><GrpHdr><MsgId>M-1</MsgId></GrpHdr></FIToFICstmrCdtTrf>
</Document></Envelope>`

func TestLintWithCompiledCBPRPackRunsLocally(t *testing.T) {
	packPath := fixtureCompiledCBPRPack(t)
	messagePath := filepath.Join(t.TempDir(), "payment.xml")
	if err := os.WriteFile(messagePath, []byte(cbprPackMessage), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := run(t, "lint", messagePath, "--cbpr-pack", packPath, "--json")
	if err == nil {
		t.Fatal("missing locally required element should fail")
	}
	var payload struct {
		Profile struct {
			Profile  string              `json:"profile"`
			Pack     *rules.CBPRPackInfo `json:"cbpr_pack"`
			Findings []rules.Finding     `json:"findings"`
		} `json:"profile"`
	}
	if jsonErr := json.Unmarshal([]byte(out), &payload); jsonErr != nil {
		t.Fatalf("lint output is not JSON: %v\n%s", jsonErr, out)
	}
	if payload.Profile.Profile != "cbpr-plus" || payload.Profile.Pack == nil || payload.Profile.Pack.Constraints != 1 {
		t.Fatalf("local pack provenance is missing: %+v", payload.Profile)
	}
	found := false
	for _, finding := range payload.Profile.Findings {
		if strings.HasPrefix(finding.RuleID, "CBPR-PACK-") && strings.HasSuffix(finding.Path, "/PackRequired") {
			found = true
		}
	}
	if !found {
		t.Fatalf("local pack finding is missing: %+v", payload.Profile.Findings)
	}
}

func TestCBPRPackCannotAugmentAnotherProfile(t *testing.T) {
	packPath := fixtureCompiledCBPRPack(t)
	messagePath := filepath.Join(t.TempDir(), "payment.xml")
	if err := os.WriteFile(messagePath, []byte(cbprPackMessage), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := run(t, "lint", messagePath, "--profile", "base", "--cbpr-pack", packPath)
	if err == nil || !strings.Contains(err.Error(), "only augment") {
		t.Fatalf("wrong profile should be rejected: %v", err)
	}
}

func TestCBPRPackCompileRequiresPrivatePackSuffix(t *testing.T) {
	_, err := run(t, "cbpr-pack", "compile", fixtureCompiledCBPRPack(t), "--output", filepath.Join(t.TempDir(), "pack.json"))
	if err == nil || !strings.Contains(err.Error(), ".cbpr-pack.json") {
		t.Fatalf("unsafe output name should be rejected: %v", err)
	}
}

func TestCBPRPackCompileCommandWritesOrPrints(t *testing.T) {
	source := fixtureCompiledCBPRPack(t)
	output := filepath.Join(t.TempDir(), "copy.cbpr-pack.json")
	out, err := run(t, "cbpr-pack", "compile", source, "--output", output)
	if err != nil {
		t.Fatalf("compile output: %v\n%s", err, out)
	}
	if _, err := os.Stat(output); err != nil {
		t.Fatalf("compiled output missing: %v", err)
	}
	out, err = run(t, "cbpr-pack", "compile", source)
	if err != nil {
		t.Fatalf("compile stdout: %v\n%s", err, out)
	}
	var decoded rules.CBPRPack
	if err := json.Unmarshal([]byte(out), &decoded); err != nil || len(decoded.Constraints) != 1 {
		t.Fatalf("stdout is not a compiled pack: %v\n%s", err, out)
	}

	blockedOutput := filepath.Join(t.TempDir(), "blocked.cbpr-pack.json")
	if err := os.Mkdir(blockedOutput, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "cbpr-pack", "compile", source, "--output", blockedOutput); err == nil {
		t.Fatal("compiler wrote a pack over a directory")
	}
}

func captureCBPRStdout(t *testing.T, fn func()) string {
	t.Helper()
	original := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	fn()
	_ = w.Close()
	os.Stdout = original
	data, err := io.ReadAll(r)
	_ = r.Close()
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestPrintLocalCBPRHitsLabelsPrivacyAndNoResults(t *testing.T) {
	hits := []rules.CBPRPackHit{{Source: "local.pdf", Page: 7, Score: 10, Snippet: "UETR local evidence"}}
	rawOutput = true
	out := captureCBPRStdout(t, func() { printLocalCBPRHits("UETR", hits) })
	if !strings.Contains(out, "no model or network") || !strings.Contains(out, "page 7") {
		t.Fatalf("plain local evidence lacks privacy/source labels:\n%s", out)
	}
	rawOutput = false
	out = captureCBPRStdout(t, func() { printLocalCBPRHits("unknown", nil) })
	if !strings.Contains(out, "No matching passage") || !strings.Contains(out, "No model or network") {
		t.Fatalf("styled empty result lacks privacy label:\n%s", out)
	}
	out = captureCBPRStdout(t, func() { printLocalCBPRHits("UETR", hits) })
	if !strings.Contains(out, "local.pdf") || !strings.Contains(out, "UETR local evidence") {
		t.Fatalf("styled hit missing source/evidence:\n%s", out)
	}
	machineHit := []rules.CBPRPackHit{{Source: "guide.json", Kind: "json", Snippet: "local rule"}}
	out = captureCBPRStdout(t, func() { printLocalCBPRHits("rule", machineHit) })
	if !strings.Contains(out, "guide.json") || !strings.Contains(out, "json") {
		t.Fatalf("machine-readable hit missing source kind:\n%s", out)
	}
	out = captureCBPRStdout(t, func() { printLocalCBPRWarnings([]string{"guide.xls: save as XLSX"}) })
	if !strings.Contains(out, "warning:") || !strings.Contains(out, "save as XLSX") {
		t.Fatalf("local warning output = %s", out)
	}
	plainOutput = true
	out = captureCBPRStdout(t, func() { printLocalCBPRHits("unknown", nil) })
	plainOutput = false
	if !strings.Contains(out, "No matching passage") {
		t.Fatalf("plain empty result missing explanation:\n%s", out)
	}
}

func TestAskCBPRPackRequiresQuestionAndPDFSource(t *testing.T) {
	pack := fixtureCompiledCBPRPack(t)
	_, err := run(t, "ask", "--cbpr-pack", pack)
	if err == nil || !strings.Contains(err.Error(), "needs a question") {
		t.Fatalf("empty local question should fail: %v", err)
	}
	_, err = run(t, "ask", "Where is UETR?", "--cbpr-pack", pack)
	if err == nil || !strings.Contains(err.Error(), "no document prose") {
		t.Fatalf("compiled pack should direct search to private PDFs: %v", err)
	}
}

func TestAskCBPRPackSearchesMachineReadableSourcesLocally(t *testing.T) {
	source := t.TempDir()
	path := filepath.Join(source, "guide.json")
	if err := os.WriteFile(path, []byte(`{
  "$comment":{"description":"The UETR is mandatory for this transaction."},
  "definitions":{"uetr":{"type":"string"}}
}`), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := run(t, "ask", "Where is UETR mandatory?", "--cbpr-pack", source, "--text")
	if err != nil {
		t.Fatalf("local JSON question: %v\n%s", err, out)
	}
	if !strings.Contains(out, "guide.json") || !strings.Contains(out, "UETR is mandatory") ||
		!strings.Contains(out, "no model or network") {
		t.Fatalf("local JSON evidence output = %s", out)
	}
}
