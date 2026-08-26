// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package rules_test

import (
	"strings"
	"testing"

	"github.com/sebastienrousseau/askiso/internal/rules"
)

// findingsFor runs a profile over a fixture and returns the findings by rule.
func findingsFor(t *testing.T, profile, msgID, body string) map[string][]rules.Finding {
	t.Helper()

	p, err := rules.Get(profile)
	if err != nil {
		t.Fatalf("profile %s: %v", profile, err)
	}
	res := rules.Run(p, message(t, msgID, body), msgID, "test.xml")

	out := map[string][]rules.Finding{}
	for _, f := range res.Findings {
		out[f.RuleID] = append(out[f.RuleID], f)
	}
	return out
}

func TestPurposeCodeRule(t *testing.T) {
	without := `<FIToFICstmrCdtTrf><CdtTrfTxInf>
    <PmtId><UETR>f3a1b2c4-5d6e-4f70-8a91-2b3c4d5e6f70</UETR></PmtId>
  </CdtTrfTxInf></FIToFICstmrCdtTrf>`
	got := findingsFor(t, "cbpr-2027", "pacs.008.001.10", without)
	if len(got["ENH-PURP-001"]) != 1 {
		t.Fatalf("expected one purpose finding, got %+v", got["ENH-PURP-001"])
	}
	// It is a warning: the requirement arrives in November 2027, and reporting
	// it as an error today would be wrong.
	if got["ENH-PURP-001"][0].Severity != rules.SeverityWarning {
		t.Errorf("severity = %q, want WARNING", got["ENH-PURP-001"][0].Severity)
	}
	if !strings.Contains(got["ENH-PURP-001"][0].Remediation, "SALA") {
		t.Errorf("the remediation does not give an example: %q", got["ENH-PURP-001"][0].Remediation)
	}

	with := `<FIToFICstmrCdtTrf><CdtTrfTxInf>
    <PmtId><UETR>f3a1b2c4-5d6e-4f70-8a91-2b3c4d5e6f70</UETR></PmtId>
    <Purp><Cd>SUPP</Cd></Purp>
  </CdtTrfTxInf></FIToFICstmrCdtTrf>`
	if got := findingsFor(t, "cbpr-2027", "pacs.008.001.10", with); len(got["ENH-PURP-001"]) != 0 {
		t.Errorf("a purpose code was reported as missing: %+v", got["ENH-PURP-001"])
	}
}

func TestStructuredRemittanceRule(t *testing.T) {
	free := `<FIToFICstmrCdtTrf><CdtTrfTxInf>
    <RmtInf><Ustrd>INVOICE 2026-0815</Ustrd></RmtInf>
  </CdtTrfTxInf></FIToFICstmrCdtTrf>`
	got := findingsFor(t, "cbpr-2027", "pacs.008.001.10", free)
	if len(got["ENH-RMT-001"]) != 1 {
		t.Fatalf("expected one remittance finding, got %+v", got["ENH-RMT-001"])
	}
	if got["ENH-RMT-001"][0].Found != "INVOICE 2026-0815" {
		t.Errorf("the finding does not quote the text: %+v", got["ENH-RMT-001"][0])
	}

	structured := `<FIToFICstmrCdtTrf><CdtTrfTxInf>
    <RmtInf><Ustrd>see attached</Ustrd><Strd><RfrdDocInf><Nb>INV-1</Nb></RfrdDocInf></Strd></RmtInf>
  </CdtTrfTxInf></FIToFICstmrCdtTrf>`
	if got := findingsFor(t, "cbpr-2027", "pacs.008.001.10", structured); len(got["ENH-RMT-001"]) != 0 {
		t.Errorf("structured remittance was reported: %+v", got["ENH-RMT-001"])
	}

	// An element with no remittance at all is not a finding: the rule is about
	// the form of what is there, not about requiring it.
	empty := `<FIToFICstmrCdtTrf><CdtTrfTxInf><RmtInf/></CdtTrfTxInf></FIToFICstmrCdtTrf>`
	if got := findingsFor(t, "cbpr-2027", "pacs.008.001.10", empty); len(got["ENH-RMT-001"]) != 0 {
		t.Errorf("an empty RmtInf was reported: %+v", got["ENH-RMT-001"])
	}
}

func TestUETRRule(t *testing.T) {
	without := `<FIToFICstmrCdtTrf><CdtTrfTxInf>
    <PmtId><EndToEndId>E2E-1</EndToEndId></PmtId>
    <Purp><Cd>SUPP</Cd></Purp>
  </CdtTrfTxInf></FIToFICstmrCdtTrf>`
	if got := findingsFor(t, "cbpr-2027", "pacs.008.001.10", without); len(got["ENH-UETR-001"]) != 1 {
		t.Errorf("a missing UETR was not reported: %+v", got["ENH-UETR-001"])
	}

	// A transaction with no PmtId at all is a schema failure, not a rule
	// finding; the rule does not double-report it.
	none := `<FIToFICstmrCdtTrf><CdtTrfTxInf><Purp><Cd>SUPP</Cd></Purp></CdtTrfTxInf></FIToFICstmrCdtTrf>`
	if got := findingsFor(t, "cbpr-2027", "pacs.008.001.10", none); len(got["ENH-UETR-001"]) != 0 {
		t.Errorf("a transaction without PmtId was reported: %+v", got["ENH-UETR-001"])
	}
}

func TestLEIRule(t *testing.T) {
	// Real identifiers, which is the only way to know the checksum is right.
	for _, lei := range []string{
		"7LTWFZYICNSX8D621K86",
		"549300GKFG0RYRRQ1414",
		"213800WAVVOPS85N2205",
	} {
		body := `<FIToFICstmrCdtTrf><CdtTrfTxInf><Dbtr><Id><OrgId><LEI>` + lei +
			`</LEI></OrgId></Id></Dbtr></CdtTrfTxInf></FIToFICstmrCdtTrf>`
		if got := findingsFor(t, "cbpr-2027", "pacs.008.001.10", body); len(got["ENH-LEI-001"]) != 0 {
			t.Errorf("%s was rejected: %+v", lei, got["ENH-LEI-001"])
		}
	}

	// A single transposed character has to fail, which is the whole point of the
	// check digits.
	bad := `<FIToFICstmrCdtTrf><CdtTrfTxInf><Dbtr><Id><OrgId>` +
		`<LEI>7LTWFZYICNSX8D612K86</LEI></OrgId></Id></Dbtr></CdtTrfTxInf></FIToFICstmrCdtTrf>`
	got := findingsFor(t, "cbpr-2027", "pacs.008.001.10", bad)
	if len(got["ENH-LEI-001"]) != 1 {
		t.Fatalf("a transposed LEI was accepted: %+v", got["ENH-LEI-001"])
	}
	// Unlike the other enhanced rules this is an error today: a failing checksum
	// is not a field awaiting a deadline, it is a wrong one.
	if got["ENH-LEI-001"][0].Severity != rules.SeverityError {
		t.Errorf("severity = %q, want ERROR", got["ENH-LEI-001"][0].Severity)
	}
}

func TestValidateLEI(t *testing.T) {
	cases := []struct {
		lei      string
		ok       bool
		mentions string
	}{
		{"7LTWFZYICNSX8D621K86", true, ""},
		{" 7ltwfzyicnsx8d621k86 ", true, ""},
		{"7LTWFZYICNSX8D621K8", false, "20 characters"},
		{"7LTWFZYICNSX8D621K866", false, "20 characters"},
		{"7LTWFZYICNSX8D612K86", false, "check digits"},
		{"7LTWFZYICNSX8D621KA6", false, "check digits"},
		{"7LTWFZYICNSX8D621K8-", false, "not permitted"},
	}
	for _, tc := range cases {
		ok, reason := rules.ValidateLEI(tc.lei)
		if ok != tc.ok {
			t.Errorf("ValidateLEI(%q) = %v (%s), want %v", tc.lei, ok, reason, tc.ok)
			continue
		}
		if tc.mentions != "" && !strings.Contains(reason, tc.mentions) {
			t.Errorf("ValidateLEI(%q) said %q, want it to mention %q", tc.lei, reason, tc.mentions)
		}
	}
}

func TestEnhancedRulesAreExemptWhereTheyDoNotApply(t *testing.T) {
	// A statement carries no payment instruction to enrich, so the enhanced
	// rules must not fire on one.
	body := `<BkToCstmrStmt><Stmt><Ntry><NtryDtls><TxDtls>
    <RmtInf><Ustrd>free text</Ustrd></RmtInf>
  </TxDtls></NtryDtls></Ntry></Stmt></BkToCstmrStmt>`

	got := findingsFor(t, "cbpr-2027", "camt.053.001.11", body)
	for id := range got {
		if strings.HasPrefix(id, "ENH-") && id != "ENH-LEI-001" {
			t.Errorf("%s fired on a statement: %+v", id, got[id])
		}
	}
}

func TestLEIRuleIsNeverExempt(t *testing.T) {
	// A wrong identifier is wrong in any message, so this rule has no exemption.
	body := `<BkToCstmrStmt><Stmt><Acct><Ownr><Id><OrgId>` +
		`<LEI>0000000000000000000X</LEI></OrgId></Id></Ownr></Acct></Stmt></BkToCstmrStmt>`
	if got := findingsFor(t, "cbpr-2027", "camt.053.001.11", body); len(got["ENH-LEI-001"]) != 1 {
		t.Errorf("a malformed LEI in a statement was not reported: %+v", got["ENH-LEI-001"])
	}

	// An empty element is not an invalid one.
	empty := `<BkToCstmrStmt><Stmt><Acct><Ownr><Id><OrgId><LEI></LEI></OrgId></Id></Ownr></Acct></Stmt></BkToCstmrStmt>`
	if got := findingsFor(t, "cbpr-2027", "camt.053.001.11", empty); len(got["ENH-LEI-001"]) != 0 {
		t.Errorf("an empty LEI was reported: %+v", got["ENH-LEI-001"])
	}
}

func TestLEIWithAnUnexpandableCharacter(t *testing.T) {
	// The checksum expands letters and digits only. Anything else is refused by
	// the format check before the arithmetic sees it, and the arithmetic refuses
	// it too rather than producing a number.
	if ok, reason := rules.ValidateLEI("7LTWFZYICNSX8D621K8 "); ok {
		t.Errorf("a space was accepted: %s", reason)
	}
	if ok, _ := rules.ValidateLEI("7LTWFZYICNSX8D621K8_"); ok {
		t.Error("an underscore was accepted")
	}
}

func TestEnhancedRulesOnAMessageWithoutAVersion(t *testing.T) {
	// A message identifier that is not four dotted parts still has to classify,
	// because a document may carry a namespace AskISO does not recognise.
	body := `<FIToFICstmrCdtTrf><CdtTrfTxInf><PmtId/></CdtTrfTxInf></FIToFICstmrCdtTrf>`
	if got := findingsFor(t, "cbpr-2027", "pacs008", body); len(got) == 0 {
		t.Error("no rules ran against an unrecognised message identifier")
	}
}
