// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package rules_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/sebastienrousseau/askiso/internal/converter"
	"github.com/sebastienrousseau/askiso/internal/rules"
)

func cbprEnvelope(msgID, service, headerID, groupID, body string) string {
	return fmt.Sprintf(`<Envelope xmlns="urn:swift:xsd:envelope">
<AppHdr xmlns="urn:iso:std:iso:20022:tech:xsd:head.001.001.02">
  <Fr><FIId><FinInstnId><BICFI>AAAAUS33XXX</BICFI></FinInstnId></FIId></Fr>
  <To><FIId><FinInstnId><BICFI>BBBBGB2LXXX</BICFI></FinInstnId></FIId></To><BizMsgIdr>%s</BizMsgIdr>
  <MsgDefIdr>%s</MsgDefIdr><BizSvc>%s</BizSvc><CreDt>2025-11-24T07:41:50Z</CreDt>
</AppHdr>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:%s"><Message><GrpHdr>
  <MsgId>%s</MsgId><NbOfTxs>1</NbOfTxs><CtrlSum>10.00</CtrlSum>
</GrpHdr>%s</Message></Document></Envelope>`, headerID, msgID, service, msgID, groupID, body)
}

func runCBPR(t *testing.T, msgID, xml string) *rules.Result {
	t.Helper()
	root, err := converter.Parse([]byte(xml))
	if err != nil {
		t.Fatalf("parse CBPR fixture: %v", err)
	}
	profile, err := rules.Get("cbpr-plus")
	if err != nil {
		t.Fatal(err)
	}
	return rules.Run(profile, root, msgID, "cbpr.xml")
}

func validPacs008() string {
	return cbprEnvelope("pacs.008.001.08", "swift.cbprplus.03", "msg-1", "msg-1",
		`<CdtTrfTxInf><PmtId><UETR>123e4567-e89b-42d3-a456-426614174000</UETR></PmtId>`+
			`<IntrBkSttlmAmt Ccy="EUR">10.00</IntrBkSttlmAmt></CdtTrfTxInf>`)
}

func TestCBPRValidEnvelopePassesEmbeddedRules(t *testing.T) {
	res := runCBPR(t, "pacs.008.001.08", validPacs008())
	if !res.Valid() {
		t.Fatalf("valid CBPR envelope failed: %+v", res.Findings)
	}
	if res.Checked < 10 {
		t.Fatalf("only %d CBPR rules ran", res.Checked)
	}
}

func TestCBPRRejectsWrongMessageVersion(t *testing.T) {
	xml := strings.ReplaceAll(validPacs008(), "pacs.008.001.08", "pacs.008.001.10")
	res := runCBPR(t, "pacs.008.001.10", xml)
	if !hasRule(res, "CBPR-SCOPE-001") {
		t.Fatalf("wrong CBPR message version was accepted: %+v", res.Findings)
	}
}

func TestCBPRRequiresBusinessApplicationHeader(t *testing.T) {
	xml := `<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.08"><Message/></Document>`
	res := runCBPR(t, "pacs.008.001.08", xml)
	if !hasRule(res, "CBPR-BAH-001") {
		t.Fatalf("headerless payload was accepted as a complete CBPR message: %+v", res.Findings)
	}
}

func TestCBPRHeaderDispatchAndConsistency(t *testing.T) {
	xml := validPacs008()
	xml = strings.Replace(xml, "<MsgDefIdr>pacs.008.001.08</MsgDefIdr>", "<MsgDefIdr>pacs.009.001.08</MsgDefIdr>", 1)
	xml = strings.Replace(xml, "<BizSvc>swift.cbprplus.03</BizSvc>", "<BizSvc>swift.cbprplus.cov.03</BizSvc>", 1)
	xml = strings.Replace(xml, "<BizMsgIdr>msg-1</BizMsgIdr>", "<BizMsgIdr>other</BizMsgIdr>", 1)
	xml = strings.Replace(xml, "2025-11-24T07:41:50Z", "2025-11-24T07:41:50", 1)
	res := runCBPR(t, "pacs.008.001.08", xml)
	for _, id := range []string{"CBPR-BAH-002", "CBPR-BAH-003", "CBPR-BAH-004", "CBPR-BAH-005"} {
		if !hasRule(res, id) {
			t.Errorf("%s did not report the bad header: %+v", id, res.Findings)
		}
	}
}

func TestCBPRRejectsWrongHeaderNamespace(t *testing.T) {
	xml := strings.Replace(validPacs008(), cbprHeaderNamespaceForTest, "urn:example:not-head", 1)
	res := runCBPR(t, "pacs.008.001.08", xml)
	if !hasRule(res, "CBPR-NS-001") {
		t.Fatalf("foreign AppHdr namespace was accepted: %+v", res.Findings)
	}
}

func TestCBPRRejectsNamespaceSpoofedBusinessElement(t *testing.T) {
	xml := strings.Replace(validPacs008(), "<UETR>", `<UETR xmlns="urn:foreign">`, 1)
	res := runCBPR(t, "pacs.008.001.08", xml)
	if !hasRule(res, "CBPR-NS-001") {
		t.Fatalf("foreign UETR namespace was accepted: %+v", res.Findings)
	}
}

const cbprHeaderNamespaceForTest = "urn:iso:std:iso:20022:tech:xsd:head.001.001.02"

func TestCBPRLiveAddressRules(t *testing.T) {
	// Fully unstructured is still legal in the live SR2025 profile.
	xml := strings.Replace(validPacs008(), "</CdtTrfTxInf>",
		`<Cdtr><Nm>Receiver</Nm><PstlAdr><AdrLine>1 Main Street</AdrLine><AdrLine>London GB</AdrLine></PstlAdr></Cdtr></CdtTrfTxInf>`, 1)
	res := runCBPR(t, "pacs.008.001.08", xml)
	if hasRule(res, "CBPR-ADDR-006") {
		t.Fatalf("SR2025 incorrectly rejected a fully unstructured address: %+v", res.Findings)
	}

	bad := strings.Replace(validPacs008(), "</CdtTrfTxInf>",
		`<Cdtr><Id><OrgId><Othr><Id>123</Id></Othr></OrgId></Id><PstlAdr><StrtNm>Main Street</StrtNm><Ctry>XX</Ctry></PstlAdr></Cdtr></CdtTrfTxInf>`, 1)
	res = runCBPR(t, "pacs.008.001.08", bad)
	for _, id := range []string{"CBPR-PTY-001", "CBPR-PTY-002", "CBPR-ADDR-006", "CBPR-CTRY-001"} {
		if !hasRule(res, id) {
			t.Errorf("%s did not report the bad structured address: %+v", id, res.Findings)
		}
	}
}

func TestCBPRHeaderRequiresBICs(t *testing.T) {
	xml := strings.Replace(validPacs008(), "<BICFI>AAAAUS33XXX</BICFI>", "", 1)
	res := runCBPR(t, "pacs.008.001.08", xml)
	if !hasRule(res, "CBPR-BAH-006") {
		t.Fatalf("header sender without BICFI was accepted: %+v", res.Findings)
	}
}

func TestCBPRTotalsCurrencyAndUETR(t *testing.T) {
	xml := validPacs008()
	xml = strings.Replace(xml, "<NbOfTxs>1</NbOfTxs>", "<NbOfTxs>2</NbOfTxs>", 1)
	xml = strings.Replace(xml, "<CtrlSum>10.00</CtrlSum>", "<CtrlSum>9.99</CtrlSum>", 1)
	xml = strings.Replace(xml, ` Ccy="EUR"`, ` Ccy="XAU"`, 1)
	xml = strings.Replace(xml, "<PmtId><UETR>123e4567-e89b-42d3-a456-426614174000</UETR></PmtId>", "<PmtId/>", 1)
	res := runCBPR(t, "pacs.008.001.08", xml)
	for _, id := range []string{"CBPR-GRP-001", "CBPR-GRP-002", "CBPR-CCY-001", "CBPR-PMT-001"} {
		if !hasRule(res, id) {
			t.Errorf("%s did not report its violation: %+v", id, res.Findings)
		}
	}
}

func TestCBPRPacs009VariantDispatch(t *testing.T) {
	xml := cbprEnvelope("pacs.009.001.08", "swift.cbprplus.cov.03", "msg-1", "msg-1",
		`<CdtTrfTxInf><PmtId><UETR>123e4567-e89b-42d3-a456-426614174000</UETR></PmtId>`+
			`<IntrBkSttlmAmt Ccy="EUR">10</IntrBkSttlmAmt></CdtTrfTxInf>`)
	res := runCBPR(t, "pacs.009.001.08", xml)
	if !hasRule(res, "CBPR-VAR-001") {
		t.Fatalf("COV without underlying customer transfer was accepted: %+v", res.Findings)
	}

	coreWithCover := strings.Replace(xml, "swift.cbprplus.cov.03", "swift.cbprplus.03", 1)
	coreWithCover = strings.Replace(coreWithCover, "</CdtTrfTxInf>", "<UndrlygCstmrCdtTrf/></CdtTrfTxInf>", 1)
	res = runCBPR(t, "pacs.009.001.08", coreWithCover)
	if !hasRule(res, "CBPR-VAR-001") {
		t.Fatalf("core pacs.009 with COV content was accepted: %+v", res.Findings)
	}
}

func TestCBPRRuleIDsAreUnique(t *testing.T) {
	profile, err := rules.Get("cbpr-plus")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, rule := range profile.Rules {
		if seen[rule.ID] {
			t.Errorf("duplicate CBPR rule id %q", rule.ID)
		}
		seen[rule.ID] = true
		if strings.HasPrefix(rule.ID, "CBPR-") && (rule.Description == "" || rule.Remediation == "" || rule.Reference == "") {
			t.Errorf("CBPR rule %s has incomplete metadata", rule.ID)
		}
	}
}

func parseCBPRRoot(t *testing.T, xml string) *converter.Node {
	t.Helper()
	root, err := converter.Parse([]byte(xml))
	if err != nil {
		t.Fatalf("parse CBPR rule fixture: %v", err)
	}
	return root
}

func TestCBPRScopeReportsAbsentMessageIdentifier(t *testing.T) {
	root := parseCBPRRoot(t, `<Document/>`)
	findings := rules.CBPRMessageDefinition.Check(&rules.Context{Root: root})
	if len(findings) != 1 || findings[0].Found != "(absent)" {
		t.Fatalf("absent identifier finding = %+v", findings)
	}
}

func TestCBPRNamespaceDiagnosticsAndExtensionPoints(t *testing.T) {
	msgID := "pacs.008.001.08"
	xml := `<Envelope>` +
		`<AppHdr><Sgntr><Signature xmlns="urn:signature"><Value/></Signature></Sgntr><BizSvc>swift.cbprplus.03</BizSvc></AppHdr>` +
		`<Document><Message><SplmtryData><Envlp><Extension xmlns="urn:extension"/></Envlp></SplmtryData>` +
		`<Foreign/></Message></Document></Envelope>`
	findings := rules.CBPRNamespaces.Check(&rules.Context{Root: parseCBPRRoot(t, xml), MsgID: msgID})
	if len(findings) < 3 {
		t.Fatalf("expected root and child namespace findings, got %+v", findings)
	}
	for _, finding := range findings {
		if strings.Contains(finding.Path, "/Sgntr/Signature") || strings.Contains(finding.Path, "/SplmtryData/Envlp/Extension") {
			t.Fatalf("extension namespace was incorrectly rejected: %+v", finding)
		}
		if finding.Found != "(none)" {
			t.Fatalf("empty namespace should be displayed as (none): %+v", finding)
		}
	}
}

func TestCBPRBusinessMessageIDExemptions(t *testing.T) {
	for _, msgID := range []string{"camt.029.001.09", "camt.055.001.08", "camt.056.001.08", "not.cbpr"} {
		if !rules.CBPRBusinessMessageID.Exempt(msgID) {
			t.Errorf("%s should be exempt", msgID)
		}
	}
	if rules.CBPRBusinessMessageID.Exempt("pacs.008.001.08") {
		t.Fatal("pacs.008 must not be exempt")
	}
}

func TestCBPRPartyChoiceAndAnyBICAreAccepted(t *testing.T) {
	root := parseCBPRRoot(t, `<Document><Message>`+
		`<Dbtr><Pty><Id><OrgId><AnyBIC>AAAAUS33XXX</AnyBIC></OrgId></Id></Pty></Dbtr>`+
		`<Cdtr><Nm>Receiver</Nm></Cdtr></Message></Document>`)
	findings := rules.CBPRPartyNameWithoutAnyBIC.Check(&rules.Context{Root: root, MsgID: "pacs.008.001.08"})
	if len(findings) != 0 {
		t.Fatalf("choice wrapper or AnyBIC produced a finding: %+v", findings)
	}
}

func TestCBPRHybridAddressLimits(t *testing.T) {
	longLine := strings.Repeat("é", 71)
	root := parseCBPRRoot(t, `<Document><PstlAdr><StrtNm>Main Street</StrtNm><TwnNm>London</TwnNm>`+
		`<AdrLine>`+longLine+`</AdrLine><AdrLine>two</AdrLine><AdrLine>three</AdrLine></PstlAdr></Document>`)
	findings := rules.CBPRCurrentAddress.Check(&rules.Context{Root: root, MsgID: "pacs.008.001.08"})
	if len(findings) != 3 {
		t.Fatalf("hybrid address findings = %+v, want country, line-count, and line-length", findings)
	}
	if !strings.Contains(findings[2].Path, "AdrLine[1]") {
		t.Fatalf("long Unicode line path = %+v", findings[2])
	}
}

func TestCBPRTotalsIgnoreUnsupportedAndLexicallyInvalidValues(t *testing.T) {
	cases := []struct {
		name string
		rule rules.Rule
		msg  string
		xml  string
	}{
		{"count unsupported", rules.CBPRTransactionCount, "camt.029.001.09", `<Document><GrpHdr><NbOfTxs>1</NbOfTxs></GrpHdr></Document>`},
		{"count malformed", rules.CBPRTransactionCount, "pacs.008.001.08", `<Document><GrpHdr><NbOfTxs>one</NbOfTxs></GrpHdr></Document>`},
		{"sum unsupported", rules.CBPRControlSum, "camt.029.001.09", `<Document><GrpHdr><CtrlSum>1</CtrlSum></GrpHdr></Document>`},
		{"sum malformed", rules.CBPRControlSum, "pacs.008.001.08", `<Document><GrpHdr><CtrlSum>ten</CtrlSum></GrpHdr></Document>`},
		{"amount malformed", rules.CBPRControlSum, "pacs.008.001.08", `<Document><GrpHdr><CtrlSum>10</CtrlSum></GrpHdr><IntrBkSttlmAmt>ten</IntrBkSttlmAmt></Document>`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			findings := tc.rule.Check(&rules.Context{Root: parseCBPRRoot(t, tc.xml), MsgID: tc.msg})
			if len(findings) != 0 {
				t.Fatalf("schema-owned lexical error produced profile finding: %+v", findings)
			}
		})
	}
}

func TestCBPRPacs009VariantNoHeaderAndValidCover(t *testing.T) {
	withoutHeader := parseCBPRRoot(t, `<Document><CdtTrfTxInf/></Document>`)
	if findings := rules.CBPRPacs009Variant.Check(&rules.Context{Root: withoutHeader, MsgID: "pacs.009.001.08"}); len(findings) != 0 {
		t.Fatalf("header absence is owned by the BAH rule: %+v", findings)
	}
	validCover := parseCBPRRoot(t, `<Envelope><AppHdr><BizSvc>swift.cbprplus.cov.03</BizSvc></AppHdr>`+
		`<Document><CdtTrfTxInf><UndrlygCstmrCdtTrf/></CdtTrfTxInf></Document></Envelope>`)
	if findings := rules.CBPRPacs009Variant.Check(&rules.Context{Root: validCover, MsgID: "pacs.009.001.08"}); len(findings) != 0 {
		t.Fatalf("valid COV content was rejected: %+v", findings)
	}
}
