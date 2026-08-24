// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package rules_test

import (
	"strings"
	"testing"

	"github.com/sebastienrousseau/anchor/internal/rules"
)

func TestInvestigationMustIdentifyItsPayment(t *testing.T) {
	without := `<InvstgtnReq>
    <InvstgtnReq><MsgId>INV-1</MsgId><InvstgtnTp><Cd>REQI</Cd></InvstgtnTp></InvstgtnReq>
  </InvstgtnReq>`
	got := findingsFor(t, "investigations", "camt.110.001.01", without)
	if len(got["INV-001"]) != 1 {
		t.Fatalf("an investigation with no underlying payment was accepted: %+v", got)
	}
	if got["INV-001"][0].Severity != rules.SeverityError {
		t.Errorf("severity = %q, want ERROR", got["INV-001"][0].Severity)
	}
	// INV-002 must not also fire: reporting the same gap twice helps nobody.
	if len(got["INV-002"]) != 0 {
		t.Errorf("INV-002 double-reported the missing reference: %+v", got["INV-002"])
	}

	withUETR := `<InvstgtnReq>
    <InvstgtnReq>
      <MsgId>INV-1</MsgId>
      <Undrlyg><IntrBk><OrgnlGrpInf><OrgnlMsgId>MSG-1</OrgnlMsgId></OrgnlGrpInf>
        <OrgnlUETR>f3a1b2c4-5d6e-4f70-8a91-2b3c4d5e6f70</OrgnlUETR>
        <UETR>f3a1b2c4-5d6e-4f70-8a91-2b3c4d5e6f70</UETR></IntrBk></Undrlyg>
    </InvstgtnReq>
  </InvstgtnReq>`
	if got := findingsFor(t, "investigations", "camt.110.001.01", withUETR); len(got) != 0 {
		t.Errorf("a complete investigation was reported: %+v", got)
	}
}

func TestInvestigationWithoutATrackingReference(t *testing.T) {
	// The payment is identified, but only by a message identifier -- which is
	// unique to a sender rather than to a payment, so it cannot be matched
	// automatically.
	body := `<InvstgtnReq>
    <InvstgtnReq>
      <MsgId>INV-1</MsgId>
      <Undrlyg><IntrBk><OrgnlGrpInf><OrgnlMsgId>MSG-1</OrgnlMsgId></OrgnlGrpInf></IntrBk></Undrlyg>
    </InvstgtnReq>
  </InvstgtnReq>`
	got := findingsFor(t, "investigations", "camt.110.001.01", body)
	if len(got["INV-001"]) != 0 {
		t.Errorf("INV-001 fired on a message that does identify its payment: %+v", got["INV-001"])
	}
	if len(got["INV-002"]) != 1 {
		t.Fatalf("a missing UETR was not reported: %+v", got["INV-002"])
	}
	if got["INV-002"][0].Severity != rules.SeverityWarning {
		t.Errorf("severity = %q, want WARNING", got["INV-002"][0].Severity)
	}
}

func TestInvestigationResponseMustQuoteItsRequest(t *testing.T) {
	without := `<InvstgtnRspn>
    <InvstgtnRspn><Sts><Cd>CLSD</Cd></Sts></InvstgtnRspn>
  </InvstgtnRspn>`
	got := findingsFor(t, "investigations", "camt.111.001.02", without)
	if len(got["INV-003"]) != 1 {
		t.Fatalf("a response with no original request was accepted: %+v", got)
	}

	with := `<InvstgtnRspn>
    <InvstgtnRspn><Sts><Cd>CLSD</Cd></Sts></InvstgtnRspn>
    <OrgnlInvstgtnReq><MsgId>INV-1</MsgId></OrgnlInvstgtnReq>
  </InvstgtnRspn>`
	if got := findingsFor(t, "investigations", "camt.111.001.02", with); len(got["INV-003"]) != 0 {
		t.Errorf("a complete response was reported: %+v", got["INV-003"])
	}
}

func TestInvestigationRulesOnlyApplyToInvestigations(t *testing.T) {
	// The element names are generic enough to appear elsewhere; the rules must
	// not fire on a payment.
	body := `<FIToFICstmrCdtTrf><CdtTrfTxInf><InvstgtnReq/></CdtTrfTxInf></FIToFICstmrCdtTrf>`
	if got := findingsFor(t, "investigations", "pacs.008.001.10", body); len(got) != 0 {
		t.Errorf("investigation rules fired on a payment: %+v", got)
	}
}

func TestVerificationRequestMustIdentifyTheAccount(t *testing.T) {
	without := `<IdVrfctnReq>
    <Assgnmt><MsgId>VOP-1</MsgId></Assgnmt>
    <Vrfctn><Id>V-1</Id></Vrfctn>
  </IdVrfctnReq>`
	got := findingsFor(t, "verification-of-payee", "acmt.023.001.03", without)
	if len(got["VOP-002"]) != 1 {
		t.Fatalf("a request with nothing to verify was accepted: %+v", got)
	}

	with := `<IdVrfctnReq>
    <Assgnmt><MsgId>VOP-1</MsgId></Assgnmt>
    <Vrfctn><Id>V-1</Id><PtyAndAcctId><Pty><Nm>MUELLER GMBH</Nm></Pty>
      <Acct><Id><IBAN>DE89370400440532013000</IBAN></Id></Acct></PtyAndAcctId></Vrfctn>
  </IdVrfctnReq>`
	if got := findingsFor(t, "verification-of-payee", "acmt.023.001.03", with); len(got["VOP-002"]) != 0 {
		t.Errorf("a complete request was reported: %+v", got["VOP-002"])
	}
}

func TestVerificationReportMustExplainAMismatch(t *testing.T) {
	silent := `<IdVrfctnRpt>
    <Assgnmt><MsgId>VOP-1</MsgId></Assgnmt>
    <Rpt><OrgnlId>V-1</OrgnlId><Vrfctn>false</Vrfctn></Rpt>
  </IdVrfctnRpt>`
	got := findingsFor(t, "verification-of-payee", "acmt.024.001.03", silent)
	if len(got["VOP-001"]) != 1 {
		t.Fatalf("a bare rejection was accepted: %+v", got)
	}
	if !strings.Contains(got["VOP-001"][0].Message, "neither why nor what") {
		t.Errorf("message = %q", got["VOP-001"][0].Message)
	}

	// A reason is enough, and so are corrected details.
	for _, body := range []string{
		`<IdVrfctnRpt><Rpt><OrgnlId>V-1</OrgnlId><Vrfctn>false</Vrfctn>
       <Rsn><Cd>NMTC</Cd></Rsn></Rpt></IdVrfctnRpt>`,
		`<IdVrfctnRpt><Rpt><OrgnlId>V-1</OrgnlId><Vrfctn>false</Vrfctn>
       <UpdtdPtyAndAcctId><Pty><Nm>MUELLER GMBH</Nm></Pty></UpdtdPtyAndAcctId></Rpt></IdVrfctnRpt>`,
	} {
		if got := findingsFor(t, "verification-of-payee", "acmt.024.001.03", body); len(got["VOP-001"]) != 0 {
			t.Errorf("an explained rejection was reported: %+v", got["VOP-001"])
		}
	}

	// A positive report has nothing to explain.
	for _, indicator := range []string{"true", "1"} {
		body := `<IdVrfctnRpt><Rpt><OrgnlId>V-1</OrgnlId><Vrfctn>` + indicator + `</Vrfctn></Rpt></IdVrfctnRpt>`
		if got := findingsFor(t, "verification-of-payee", "acmt.024.001.03", body); len(got["VOP-001"]) != 0 {
			t.Errorf("a successful verification was reported: %+v", got["VOP-001"])
		}
	}

	// An absent indicator is a schema failure, not a rule finding.
	none := `<IdVrfctnRpt><Rpt><OrgnlId>V-1</OrgnlId></Rpt></IdVrfctnRpt>`
	if got := findingsFor(t, "verification-of-payee", "acmt.024.001.03", none); len(got["VOP-001"]) != 0 {
		t.Errorf("a report with no indicator was reported: %+v", got["VOP-001"])
	}
}

func TestVerificationRulesOnlyApplyToVerification(t *testing.T) {
	body := `<FIToFICstmrCdtTrf><CdtTrfTxInf><Vrfctn>false</Vrfctn></CdtTrfTxInf></FIToFICstmrCdtTrf>`
	if got := findingsFor(t, "verification-of-payee", "pacs.008.001.10", body); len(got) != 0 {
		t.Errorf("verification rules fired on a payment: %+v", got)
	}
}

func TestAllProfileGathersEveryRule(t *testing.T) {
	all, err := rules.Get("all")
	if err != nil {
		t.Fatal(err)
	}

	seen := map[string]bool{}
	for _, r := range all.Rules {
		if seen[r.ID] {
			t.Errorf("%s appears twice in the all profile", r.ID)
		}
		seen[r.ID] = true
	}

	// Every rule reachable through another profile has to be in it.
	for _, name := range rules.Names() {
		if name == "all" {
			continue
		}
		p, err := rules.Get(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range p.Rules {
			if !seen[r.ID] {
				t.Errorf("%s is in the %s profile but not in all", r.ID, name)
			}
		}
	}
}

func TestEveryRuleIsDescribed(t *testing.T) {
	// A finding a reader cannot act on is a finding that gets ignored, so every
	// rule has to carry a description and a remediation.
	all, err := rules.Get("all")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range all.Rules {
		if r.ID == "" || r.Name == "" {
			t.Errorf("a rule has no identity: %+v", r)
		}
		if len(r.Description) < 40 {
			t.Errorf("%s has a thin description: %q", r.ID, r.Description)
		}
		if len(r.Remediation) < 20 {
			t.Errorf("%s has no usable remediation: %q", r.ID, r.Remediation)
		}
		if r.Check == nil {
			t.Errorf("%s has no check", r.ID)
		}
	}
}
