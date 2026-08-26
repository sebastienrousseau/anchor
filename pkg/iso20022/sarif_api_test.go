// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package iso20022

import (
	"encoding/json"
	"strings"
	"testing"
)

// The website offers a SARIF download, and the promise attached to it is that
// it is the same report the CLI writes. That promise is only worth making if
// the shape is pinned: a pipeline that ingests SARIF fails loudly on a missing
// version field and silently on a missing ruleId.

const addrMessage = `<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">
  <FIToFICstmrCdtTrf><GrpHdr><MsgId>MSG-1</MsgId></GrpHdr>
    <CdtTrfTxInf><IntrBkSttlmAmt Ccy="EUR">1000.00</IntrBkSttlmAmt>
      <Cdtr><Nm>Beispiel GmbH</Nm>
        <PstlAdr><AdrLine>Musterstrasse 1</AdrLine><AdrLine>Berlin</AdrLine></PstlAdr>
      </Cdtr></CdtTrfTxInf></FIToFICstmrCdtTrf></Document>`

func TestSARIFCarriesEveryFindingWithItsRuleID(t *testing.T) {
	res, err := CheckProfile([]byte(addrMessage), "cbpr-2026", "message.xml")
	if err != nil {
		t.Fatalf("checking the profile: %v", err)
	}
	if len(res.Findings) == 0 {
		t.Fatal("the sample message produced no findings, so this test proves nothing")
	}

	log, err := SARIF(res)
	if err != nil {
		t.Fatalf("rendering SARIF: %v", err)
	}

	var parsed struct {
		Version string `json:"version"`
		Schema  string `json:"$schema"`
		Runs    []struct {
			Tool struct {
				Driver struct {
					Name  string `json:"name"`
					Rules []struct {
						ID string `json:"id"`
					} `json:"rules"`
				} `json:"driver"`
			} `json:"tool"`
			Results []struct {
				RuleID  string `json:"ruleId"`
				Level   string `json:"level"`
				Message struct {
					Text string `json:"text"`
				} `json:"message"`
			} `json:"results"`
		} `json:"runs"`
	}
	if err := json.Unmarshal([]byte(log), &parsed); err != nil {
		t.Fatalf("the report is not JSON: %v", err)
	}

	if parsed.Version != "2.1.0" {
		t.Errorf("version is %q, want 2.1.0", parsed.Version)
	}
	if !strings.Contains(parsed.Schema, "sarif") {
		t.Errorf("$schema does not point at a SARIF schema: %q", parsed.Schema)
	}
	if len(parsed.Runs) != 1 {
		t.Fatalf("got %d runs, want 1", len(parsed.Runs))
	}
	run := parsed.Runs[0]
	if run.Tool.Driver.Name != "askiso" {
		t.Errorf("driver name is %q, want askiso", run.Tool.Driver.Name)
	}
	if len(run.Results) != len(res.Findings) {
		t.Fatalf("%d findings became %d SARIF results", len(res.Findings), len(run.Results))
	}

	// Every result must name a rule, and every rule it names must be described
	// in the driver: a ruleId with no matching rule renders as a bare string in
	// every viewer that reads these files.
	described := map[string]bool{}
	for _, r := range run.Tool.Driver.Rules {
		described[r.ID] = true
	}
	for i, r := range run.Results {
		if r.RuleID == "" {
			t.Errorf("result %d has no ruleId", i)
			continue
		}
		if !described[r.RuleID] {
			t.Errorf("result %d cites %s, which the driver does not describe", i, r.RuleID)
		}
		if r.Message.Text == "" {
			t.Errorf("result %d (%s) has no message", i, r.RuleID)
		}
		if r.Level != "error" && r.Level != "warning" && r.Level != "note" {
			t.Errorf("result %d (%s) has level %q, which SARIF does not define", i, r.RuleID, r.Level)
		}
	}
}

// A clean message still produces a well-formed log. A pipeline step that only
// emits a file when something is wrong is one that cannot distinguish "passed"
// from "did not run".
func TestSARIFOfACleanRunIsStillWellFormed(t *testing.T) {
	const clean = `<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">
  <FIToFICstmrCdtTrf><GrpHdr><MsgId>MSG-1</MsgId></GrpHdr></FIToFICstmrCdtTrf></Document>`

	res, err := CheckProfile([]byte(clean), "cbpr-2026", "clean.xml")
	if err != nil {
		t.Fatalf("checking the profile: %v", err)
	}
	log, err := SARIF(res)
	if err != nil {
		t.Fatalf("rendering SARIF: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(log), &parsed); err != nil {
		t.Fatalf("the report is not JSON: %v", err)
	}
	if parsed["version"] != "2.1.0" {
		t.Errorf("version is %v, want 2.1.0", parsed["version"])
	}
}

// Nothing at all is still a valid log, not a crash.
func TestSARIFWithNoResults(t *testing.T) {
	log, err := SARIF()
	if err != nil {
		t.Fatalf("rendering SARIF: %v", err)
	}
	if !strings.Contains(log, `"2.1.0"`) {
		t.Errorf("an empty log is not SARIF 2.1.0: %s", log)
	}
}
