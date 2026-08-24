// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package swift

import (
	"strings"
	"testing"
)

// The initiation messages carry many transactions in one instruction, which is
// the case MT expresses with a repeating sequence B. These check that the
// sequence comes out in the order a receiver reads it, and that the round trip
// through MT still describes the same payments.

const pain001Doc = `<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pain.001.001.09">
  <CstmrCdtTrfInitn>
    <GrpHdr>
      <MsgId>REQ-20260824-001</MsgId>
      <CreDtTm>2026-08-24T09:00:00Z</CreDtTm>
      <NbOfTxs>2</NbOfTxs>
      <InitgPty><Nm>ACME TREASURY</Nm></InitgPty>
    </GrpHdr>
    <PmtInf>
      <PmtInfId>PAY-001</PmtInfId>
      <PmtMtd>TRF</PmtMtd>
      <NbOfTxs>1</NbOfTxs>
      <ReqdExctnDt><Dt>2026-08-26</Dt></ReqdExctnDt>
      <Dbtr><Nm>ACME TRADING LIMITED</Nm>
        <PstlAdr><TwnNm>LONDON</TwnNm><Ctry>GB</Ctry></PstlAdr></Dbtr>
      <DbtrAcct><Id><IBAN>GB29NWBK60161331926819</IBAN></Id></DbtrAcct>
      <DbtrAgt><FinInstnId><BICFI>BANKGB2LXXX</BICFI></FinInstnId></DbtrAgt>
      <UltmtDbtr><Nm>ACME HOLDINGS</Nm></UltmtDbtr>
      <CdtTrfTxInf>
        <PmtId><InstrId>FX-4471</InstrId><EndToEndId>E2E-001</EndToEndId></PmtId>
        <Amt><InstdAmt Ccy="EUR">12500.00</InstdAmt></Amt>
        <XchgRateInf><XchgRate>1.0840</XchgRate></XchgRateInf>
        <CdtrAgt><FinInstnId><BICFI>BANKDEFFXXX</BICFI></FinInstnId></CdtrAgt>
        <Cdtr><Nm>MUELLER GMBH</Nm></Cdtr>
        <CdtrAcct><Id><IBAN>DE89370400440532013000</IBAN></Id></CdtrAcct>
        <InstrForDbtrAgt>URGP</InstrForDbtrAgt>
        <RmtInf><Ustrd>INVOICE 2026-0815</Ustrd></RmtInf>
      </CdtTrfTxInf>
      <CdtTrfTxInf>
        <PmtId><EndToEndId>E2E-002</EndToEndId></PmtId>
        <Amt><EqvtAmt><Amt Ccy="USD">8000.00</Amt>
          <CcyOfTrf>EUR</CcyOfTrf></EqvtAmt></Amt>
        <CdtrAgt><FinInstnId><BICFI>CHASUS33XXX</BICFI></FinInstnId></CdtrAgt>
        <Cdtr><Nm>NORTHWIND INC</Nm></Cdtr>
        <CdtrAcct><Id><IBAN>US64SVBKUS6S3300958879</IBAN></Id></CdtrAcct>
      </CdtTrfTxInf>
    </PmtInf>
  </CstmrCdtTrfInitn>
</Document>`

func TestConvertPain001ToMT101(t *testing.T) {
	c := convertBack(t, pain001Doc)

	if c.SourceType != "pain.001.001.09" || c.TargetType != "MT101" {
		t.Errorf("got %s -> %s", c.SourceType, c.TargetType)
	}

	for _, want := range []string{
		"{2:I101",
		":20:REQ-20260824-001",
		":28D:1/1",
		":30:260826",
		":50L:ACME TREASURY",
		":50H:/GB29NWBK60161331926819",
		":52A:BANKGB2LXXX",
		":21:E2E-001",
		":21F:FX-4471",
		":23E:URGP",
		":32B:EUR12500,00",
		":57A:BANKDEFFXXX",
		":59:/DE89370400440532013000",
		":70:INVOICE 2026-0815",
		":36:1,0840",
		":21:E2E-002",
		":32B:USD8000,00",
	} {
		if !strings.Contains(c.XML, want) {
			t.Errorf("output is missing %q\n%s", want, c.XML)
		}
	}

	// MT fixes its field order and a receiver reads it: :23E: comes before
	// :32B:, and :36: closes the sequence.
	if strings.Index(c.XML, ":23E:") > strings.Index(c.XML, ":32B:") {
		t.Errorf("the sequence B fields are out of order\n%s", c.XML)
	}
	if strings.Index(c.XML, ":36:") < strings.Index(c.XML, ":70:") {
		t.Errorf("the exchange rate does not close the sequence\n%s", c.XML)
	}

	// The execution date is wrapped in a date-or-date-time choice, and reading
	// it as a plain element would silently substitute today's date.
	if strings.Contains(c.XML, ":30:"+todayMT()) && todayMT() != "260826" {
		t.Errorf("the execution date fell back to today\n%s", c.XML)
	}
}

// todayMT renders today the way MT writes a date, for the check above.
func todayMT() string { return isoDateToMT(today()) }

func TestConvertPain001ReportsWhatMTCannotCarry(t *testing.T) {
	c := convertBack(t, pain001Doc)

	var sawUltimate bool
	for _, r := range c.Report {
		if strings.HasSuffix(r.Tag, "/UltmtDbtr") && r.Fidelity == FidelityUnmapped {
			sawUltimate = true
		}
	}
	if !sawUltimate {
		t.Errorf("the ultimate debtor was not reported: %+v", c.Report)
	}
	if c.Lossless() {
		t.Error("a conversion that dropped an ultimate debtor reported itself as lossless")
	}
}

func TestConvertPain001EquivalentAmount(t *testing.T) {
	// The amount is a choice: an instructed amount, or an equivalent in another
	// currency. Both have to produce a field 32B.
	c := convertBack(t, pain001Doc)
	if !strings.Contains(c.XML, ":32B:USD8000,00") {
		t.Errorf("an equivalent amount produced no field 32B\n%s", c.XML)
	}
}

func TestConvertPain001Rejects(t *testing.T) {
	base := `<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pain.001.001.09">%s</Document>`
	for name, body := range map[string]string{
		"no body":        `<Nothing/>`,
		"no instruction": `<CstmrCdtTrfInitn><GrpHdr><MsgId>M</MsgId></GrpHdr></CstmrCdtTrfInitn>`,
		"no debtor": `<CstmrCdtTrfInitn><PmtInf><PmtInfId>P</PmtInfId>
      <CdtTrfTxInf><PmtId><EndToEndId>E</EndToEndId></PmtId>
      <Amt><InstdAmt Ccy="EUR">1.00</InstdAmt></Amt>
      <Cdtr><Nm>C</Nm></Cdtr></CdtTrfTxInf></PmtInf></CstmrCdtTrfInitn>`,
		"no transaction identification": `<CstmrCdtTrfInitn><PmtInf><Dbtr><Nm>D</Nm></Dbtr>
      <CdtTrfTxInf><Amt><InstdAmt Ccy="EUR">1.00</InstdAmt></Amt>
      <Cdtr><Nm>C</Nm></Cdtr></CdtTrfTxInf></PmtInf></CstmrCdtTrfInitn>`,
		"no amount": `<CstmrCdtTrfInitn><PmtInf><Dbtr><Nm>D</Nm></Dbtr>
      <CdtTrfTxInf><PmtId><EndToEndId>E</EndToEndId></PmtId>
      <Cdtr><Nm>C</Nm></Cdtr></CdtTrfTxInf></PmtInf></CstmrCdtTrfInitn>`,
		"no creditor": `<CstmrCdtTrfInitn><PmtInf><Dbtr><Nm>D</Nm></Dbtr>
      <CdtTrfTxInf><PmtId><EndToEndId>E</EndToEndId></PmtId>
      <Amt><InstdAmt Ccy="EUR">1.00</InstdAmt></Amt></CdtTrfTxInf></PmtInf></CstmrCdtTrfInitn>`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ConvertMX([]byte(strings.Replace(base, "%s", body, 1))); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestConvertPain001WithoutAnExecutionDate(t *testing.T) {
	doc := strings.Replace(pain001Doc, "<ReqdExctnDt><Dt>2026-08-26</Dt></ReqdExctnDt>", "", 1)

	c := convertBack(t, doc)
	var reported bool
	for _, r := range c.Report {
		if r.Path == ":30:" && r.Fidelity == FidelityDerived {
			reported = true
		}
	}
	if !reported {
		t.Errorf("the derived execution date was not reported: %+v", c.Report)
	}
}

const pain008Doc = `<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pain.008.001.07">
  <CstmrDrctDbtInitn>
    <GrpHdr>
      <MsgId>DD-20260824-001</MsgId>
      <CreDtTm>2026-08-24T09:00:00Z</CreDtTm>
      <NbOfTxs>1</NbOfTxs>
      <InitgPty><Nm>ACME TREASURY</Nm></InitgPty>
    </GrpHdr>
    <PmtInf>
      <PmtInfId>COLL-001</PmtInfId>
      <PmtMtd>DD</PmtMtd>
      <ReqdColltnDt>2026-09-01</ReqdColltnDt>
      <Cdtr><Nm>ACME UTILITIES LIMITED</Nm></Cdtr>
      <CdtrAcct><Id><IBAN>GB29NWBK60161331926819</IBAN></Id></CdtrAcct>
      <CdtrAgt><FinInstnId><BICFI>BANKGB2LXXX</BICFI></FinInstnId></CdtrAgt>
      <ChrgBr>SHAR</ChrgBr>
      <CdtrSchmeId><Id><PrvtId><Othr><Id>GB98ZZZ0001</Id></Othr></PrvtId></Id></CdtrSchmeId>
      <DrctDbtTxInf>
        <PmtId><EndToEndId>E2E-001</EndToEndId></PmtId>
        <InstdAmt Ccy="EUR">120.50</InstdAmt>
        <DrctDbtTx><MndtRltdInf><MndtId>MANDATE-4471</MndtId></MndtRltdInf></DrctDbtTx>
        <DbtrAgt><FinInstnId><BICFI>BANKDEFFXXX</BICFI></FinInstnId></DbtrAgt>
        <Dbtr><Nm>MUELLER GMBH</Nm></Dbtr>
        <DbtrAcct><Id><IBAN>DE89370400440532013000</IBAN></Id></DbtrAcct>
        <InstrForCdtrAgt>OTHR</InstrForCdtrAgt>
        <RmtInf><Ustrd>INVOICE 2026-0901</Ustrd></RmtInf>
      </DrctDbtTxInf>
    </PmtInf>
  </CstmrDrctDbtInitn>
</Document>`

func TestConvertPain008ToMT104(t *testing.T) {
	c := convertBack(t, pain008Doc)

	if c.TargetType != "MT104" {
		t.Errorf("TargetType = %q", c.TargetType)
	}
	for _, want := range []string{
		"{2:I104",
		":20:DD-20260824-001",
		":30:260901",
		":50C:ACME TREASURY",
		":50K:/GB29NWBK60161331926819",
		"ACME UTILITIES LIMITED",
		":52A:BANKGB2LXXX",
		":71A:SHA",
		":21:E2E-001",
		":23E:OTHR",
		":21C:MANDATE-4471",
		":32B:EUR120,50",
		":57A:BANKDEFFXXX",
		":59:/DE89370400440532013000",
		":70:INVOICE 2026-0901",
	} {
		if !strings.Contains(c.XML, want) {
			t.Errorf("output is missing %q\n%s", want, c.XML)
		}
	}

	// MT104 sequence B is :21: :23E: :21C: :32B:, in that order.
	if strings.Index(c.XML, ":21C:") > strings.Index(c.XML, ":32B:") {
		t.Errorf("the sequence B fields are out of order\n%s", c.XML)
	}

	// A creditor scheme identification has no MT field, and saying so is what
	// stops a direct debit being sent without the identifier a scheme needs.
	var reported bool
	for _, r := range c.Report {
		if strings.HasSuffix(r.Tag, "/CdtrSchmeId") && r.Fidelity == FidelityUnmapped {
			reported = true
		}
	}
	if !reported {
		t.Errorf("the creditor scheme identification was not reported: %+v", c.Report)
	}
}

func TestConvertPain008Rejects(t *testing.T) {
	base := `<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pain.008.001.07">%s</Document>`
	for name, body := range map[string]string{
		"no body":        `<Nothing/>`,
		"no instruction": `<CstmrDrctDbtInitn><GrpHdr><MsgId>M</MsgId></GrpHdr></CstmrDrctDbtInitn>`,
		"no creditor": `<CstmrDrctDbtInitn><PmtInf><PmtInfId>P</PmtInfId>
      <DrctDbtTxInf><PmtId><EndToEndId>E</EndToEndId></PmtId>
      <InstdAmt Ccy="EUR">1.00</InstdAmt><Dbtr><Nm>D</Nm></Dbtr></DrctDbtTxInf></PmtInf></CstmrDrctDbtInitn>`,
		"no transaction identification": `<CstmrDrctDbtInitn><PmtInf><Cdtr><Nm>C</Nm></Cdtr>
      <DrctDbtTxInf><InstdAmt Ccy="EUR">1.00</InstdAmt>
      <Dbtr><Nm>D</Nm></Dbtr></DrctDbtTxInf></PmtInf></CstmrDrctDbtInitn>`,
		"no amount": `<CstmrDrctDbtInitn><PmtInf><Cdtr><Nm>C</Nm></Cdtr>
      <DrctDbtTxInf><PmtId><EndToEndId>E</EndToEndId></PmtId>
      <Dbtr><Nm>D</Nm></Dbtr></DrctDbtTxInf></PmtInf></CstmrDrctDbtInitn>`,
		"no debtor": `<CstmrDrctDbtInitn><PmtInf><Cdtr><Nm>C</Nm></Cdtr>
      <DrctDbtTxInf><PmtId><EndToEndId>E</EndToEndId></PmtId>
      <InstdAmt Ccy="EUR">1.00</InstdAmt></DrctDbtTxInf></PmtInf></CstmrDrctDbtInitn>`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ConvertMX([]byte(strings.Replace(base, "%s", body, 1))); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestConvertPain008WithoutACollectionDate(t *testing.T) {
	doc := strings.Replace(pain008Doc, "<ReqdColltnDt>2026-09-01</ReqdColltnDt>", "", 1)

	c := convertBack(t, doc)
	var reported bool
	for _, r := range c.Report {
		if r.Path == ":30:" && r.Fidelity == FidelityDerived {
			reported = true
		}
	}
	if !reported {
		t.Errorf("the derived collection date was not reported: %+v", c.Report)
	}
}

const pacs010Doc = `<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.010.001.06">
  <FIDrctDbt>
    <GrpHdr><MsgId>FMDD-20260824</MsgId><CreDtTm>2026-08-24T09:00:00Z</CreDtTm><NbOfTxs>2</NbOfTxs></GrpHdr>
    <CdtInstr>
      <CdtId>FMDD-20260824</CdtId>
      <TtlIntrBkSttlmAmt>250000.00</TtlIntrBkSttlmAmt>
      <IntrBkSttlmDt>2026-08-26</IntrBkSttlmDt>
      <Cdtr><FinInstnId><BICFI>BANKGB2LXXX</BICFI></FinInstnId></Cdtr>
      <DrctDbtTxInf>
        <PmtId><EndToEndId>REL-001</EndToEndId></PmtId>
        <IntrBkSttlmAmt Ccy="EUR">150000.00</IntrBkSttlmAmt>
        <Dbtr><FinInstnId><BICFI>DEUTDEFFXXX</BICFI></FinInstnId></Dbtr>
        <RmtInf><Ustrd>MARGIN CALL</Ustrd></RmtInf>
      </DrctDbtTxInf>
      <DrctDbtTxInf>
        <PmtId><EndToEndId>REL-002</EndToEndId></PmtId>
        <IntrBkSttlmAmt Ccy="EUR">100000.00</IntrBkSttlmAmt>
        <Dbtr><FinInstnId><BICFI>BNPAFRPPXXX</BICFI></FinInstnId></Dbtr>
      </DrctDbtTxInf>
    </CdtInstr>
  </FIDrctDbt>
</Document>`

func TestConvertPacs010ToMT204(t *testing.T) {
	c := convertBack(t, pacs010Doc)

	if c.TargetType != "MT204" {
		t.Errorf("TargetType = %q", c.TargetType)
	}
	for _, want := range []string{
		"{2:I204",
		":20:FMDD-20260824",
		":19:250000,00",
		":30:260826",
		":58A:BANKGB2LXXX",
		":20:REL-001",
		":32B:EUR150000,00",
		":53A:DEUTDEFFXXX",
		":72:MARGIN CALL",
		":20:REL-002",
		":53A:BNPAFRPPXXX",
	} {
		if !strings.Contains(c.XML, want) {
			t.Errorf("output is missing %q\n%s", want, c.XML)
		}
	}

	// Sequence A's reference comes first and sequence B repeats after it, which
	// is what the parser relies on to tell the two apart.
	if strings.Index(c.XML, ":20:FMDD-20260824") > strings.Index(c.XML, ":20:REL-001") {
		t.Errorf("the message reference does not lead the message\n%s", c.XML)
	}

	// And converting it back finds two transactions, not three.
	back, err := Convert(mustParse(t, c.XML))
	if err != nil {
		t.Fatalf("converting back: %v\n%s", err, c.XML)
	}
	if n := strings.Count(back.XML, "<DrctDbtTxInf>"); n != 2 {
		t.Errorf("the round trip produced %d transactions, want 2\n%s", n, back.XML)
	}
}

func TestConvertPacs010Rejects(t *testing.T) {
	base := `<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.010.001.06">%s</Document>`
	for name, body := range map[string]string{
		"no body":        `<Nothing/>`,
		"no instruction": `<FIDrctDbt><GrpHdr><MsgId>M</MsgId></GrpHdr></FIDrctDbt>`,
		"no creditor": `<FIDrctDbt><CdtInstr><CdtId>C</CdtId>
      <DrctDbtTxInf><PmtId><EndToEndId>E</EndToEndId></PmtId>
      <IntrBkSttlmAmt Ccy="EUR">1.00</IntrBkSttlmAmt>
      <Dbtr><FinInstnId><BICFI>DEUTDEFFXXX</BICFI></FinInstnId></Dbtr></DrctDbtTxInf></CdtInstr></FIDrctDbt>`,
		"no transaction": `<FIDrctDbt><CdtInstr><CdtId>C</CdtId>
      <Cdtr><FinInstnId><BICFI>BANKGB2LXXX</BICFI></FinInstnId></Cdtr></CdtInstr></FIDrctDbt>`,
		"no transaction identification": `<FIDrctDbt><CdtInstr><CdtId>C</CdtId>
      <Cdtr><FinInstnId><BICFI>BANKGB2LXXX</BICFI></FinInstnId></Cdtr>
      <DrctDbtTxInf><IntrBkSttlmAmt Ccy="EUR">1.00</IntrBkSttlmAmt>
      <Dbtr><FinInstnId><BICFI>DEUTDEFFXXX</BICFI></FinInstnId></Dbtr></DrctDbtTxInf></CdtInstr></FIDrctDbt>`,
		"no amount": `<FIDrctDbt><CdtInstr><CdtId>C</CdtId>
      <Cdtr><FinInstnId><BICFI>BANKGB2LXXX</BICFI></FinInstnId></Cdtr>
      <DrctDbtTxInf><PmtId><EndToEndId>E</EndToEndId></PmtId>
      <Dbtr><FinInstnId><BICFI>DEUTDEFFXXX</BICFI></FinInstnId></Dbtr></DrctDbtTxInf></CdtInstr></FIDrctDbt>`,
		"no debtor": `<FIDrctDbt><CdtInstr><CdtId>C</CdtId>
      <Cdtr><FinInstnId><BICFI>BANKGB2LXXX</BICFI></FinInstnId></Cdtr>
      <DrctDbtTxInf><PmtId><EndToEndId>E</EndToEndId></PmtId>
      <IntrBkSttlmAmt Ccy="EUR">1.00</IntrBkSttlmAmt></DrctDbtTxInf></CdtInstr></FIDrctDbt>`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ConvertMX([]byte(strings.Replace(base, "%s", body, 1))); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestConvertPacs010MultipleInstructions(t *testing.T) {
	two := strings.Replace(pacs010Doc, "</CdtInstr>\n  </FIDrctDbt>",
		`</CdtInstr>
    <CdtInstr>
      <CdtId>FMDD-0002</CdtId>
      <Cdtr><FinInstnId><BICFI>MIDLGB22XXX</BICFI></FinInstnId></Cdtr>
      <DrctDbtTxInf><PmtId><EndToEndId>REL-003</EndToEndId></PmtId>
        <IntrBkSttlmAmt Ccy="EUR">1.00</IntrBkSttlmAmt>
        <Dbtr><FinInstnId><BICFI>BNPAFRPPXXX</BICFI></FinInstnId></Dbtr></DrctDbtTxInf>
    </CdtInstr>
  </FIDrctDbt>`, 1)

	c := convertBack(t, two)
	if strings.Contains(c.XML, "REL-003") {
		t.Errorf("the second instruction leaked into the message\n%s", c.XML)
	}
	var reported bool
	for _, r := range c.Report {
		if strings.Contains(r.Note, "only the first was converted") {
			reported = true
		}
	}
	if !reported {
		t.Errorf("the dropped instruction was not reported: %+v", c.Report)
	}
}

func TestConvertPacs010FallsBackToTheMessageIdentifier(t *testing.T) {
	doc := strings.Replace(pacs010Doc, "<CdtId>FMDD-20260824</CdtId>", "", 1)
	c := convertBack(t, doc)
	if !strings.Contains(c.XML, ":20:FMDD-20260824") {
		t.Errorf("the message identifier was not used as the reference\n%s", c.XML)
	}
}

func TestEveryMXPairRoundTrips(t *testing.T) {
	// The property coexistence rests on: a message converted to MT and back
	// still describes the same payment.
	cases := []struct {
		name, doc string
		wants     []string
	}{
		{"pain.001", pain001Doc, []string{
			"<InstdAmt Ccy=\"EUR\">12500.00</InstdAmt>",
			"<IBAN>GB29NWBK60161331926819</IBAN>",
			"<Nm>MUELLER GMBH</Nm>",
		}},
		{"pain.008", pain008Doc, []string{
			"<InstdAmt Ccy=\"EUR\">120.50</InstdAmt>",
			"<MndtId>MANDATE-4471</MndtId>",
			"<Nm>MUELLER GMBH</Nm>",
		}},
		{"pacs.010", pacs010Doc, []string{
			"<IntrBkSttlmAmt Ccy=\"EUR\">150000.00</IntrBkSttlmAmt>",
			"<BICFI>DEUTDEFFXXX</BICFI>",
			"<BICFI>BANKGB2LXXX</BICFI>",
		}},
		{"pacs.008", pacs008Doc, []string{
			"<IntrBkSttlmAmt Ccy=\"EUR\">25000.00</IntrBkSttlmAmt>",
			"<Nm>ACME TRADING LIMITED</Nm>",
		}},
		{"pacs.009", pacs009Doc, []string{
			"<IntrBkSttlmAmt Ccy=\"EUR\">25000.00</IntrBkSttlmAmt>",
			"<BICFI>DEUTDEFFXXX</BICFI>",
		}},
		{"camt.053", camt053Doc, []string{
			"<Amt Ccy=\"EUR\">100000.00</Amt>",
			"<IBAN>GB29NWBK60161331926819</IBAN>",
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			toMT := convertBack(t, tc.doc)

			back, err := Convert(mustParse(t, toMT.XML))
			if err != nil {
				t.Fatalf("converting back: %v\n%s", err, toMT.XML)
			}
			for _, want := range tc.wants {
				if !strings.Contains(back.XML, want) {
					t.Errorf("the round trip lost %s\n%s", want, back.XML)
				}
			}
		})
	}
}

func TestConvertPain001SkipsAnEmptyInstruction(t *testing.T) {
	// A payment instruction with no transactions contributes nothing rather
	// than an empty sequence B.
	doc := strings.Replace(pain001Doc, "</PmtInf>\n  </CstmrCdtTrfInitn>",
		`</PmtInf>
    <PmtInf>
      <PmtInfId>PAY-002</PmtInfId>
      <PmtMtd>TRF</PmtMtd>
      <Dbtr><Nm>ACME</Nm></Dbtr>
    </PmtInf>
  </CstmrCdtTrfInitn>`, 1)

	c := convertBack(t, doc)
	// Two transactions from the first instruction, none from the second.
	if n := strings.Count(c.XML, ":32B:"); n != 2 {
		t.Errorf("got %d transactions, want 2\n%s", n, c.XML)
	}
}

func TestConvertPain001StructuredRemittanceIsReported(t *testing.T) {
	doc := strings.Replace(pain001Doc,
		"<RmtInf><Ustrd>INVOICE 2026-0815</Ustrd></RmtInf>",
		`<RmtInf><Ustrd>INVOICE 2026-0815</Ustrd>
          <Strd><RfrdDocInf><Nb>INV-0815</Nb></RfrdDocInf></Strd></RmtInf>`, 1)

	c := convertBack(t, doc)
	var reported bool
	for _, r := range c.Report {
		if strings.HasSuffix(r.Tag, "/RmtInf/Strd") && r.Fidelity == FidelityUnmapped {
			reported = true
		}
	}
	if !reported {
		t.Errorf("structured remittance was not reported: %+v", c.Report)
	}
}

func TestConvertPain001AmountChoiceWithNothingUsable(t *testing.T) {
	// An Amt element carrying neither branch of the choice is a schema failure,
	// and the conversion refuses rather than emitting an empty field.
	doc := strings.Replace(pain001Doc,
		"<Amt><InstdAmt Ccy=\"EUR\">12500.00</InstdAmt></Amt>", "<Amt/>", 1)

	if _, err := ConvertMX([]byte(doc)); err == nil {
		t.Error("a transaction with an empty amount choice was accepted")
	}
}

func TestConvertPain008MultipleTransactionsInOneInstruction(t *testing.T) {
	// One creditor collecting from two debtors becomes two sequence B blocks.
	doc := strings.Replace(pain008Doc, "</DrctDbtTxInf>\n    </PmtInf>",
		`</DrctDbtTxInf>
      <DrctDbtTxInf>
        <PmtId><EndToEndId>E2E-002</EndToEndId></PmtId>
        <InstdAmt Ccy="EUR">75.00</InstdAmt>
        <DbtrAgt><FinInstnId><BICFI>CHASUS33XXX</BICFI></FinInstnId></DbtrAgt>
        <Dbtr><Nm>NORTHWIND INC</Nm></Dbtr>
        <DbtrAcct><Id><IBAN>US64SVBKUS6S3300958879</IBAN></Id></DbtrAcct>
      </DrctDbtTxInf>
    </PmtInf>`, 1)

	c := convertBack(t, doc)
	if n := strings.Count(c.XML, ":32B:"); n != 2 {
		t.Errorf("got %d collections, want 2\n%s", n, c.XML)
	}
	if !strings.Contains(c.XML, ":21:E2E-002") {
		t.Errorf("the second collection is missing\n%s", c.XML)
	}

	// Converting back finds both, in the same order.
	back, err := Convert(mustParse(t, c.XML))
	if err != nil {
		t.Fatalf("converting back: %v\n%s", err, c.XML)
	}
	if n := strings.Count(back.XML, "<DrctDbtTxInf>"); n != 2 {
		t.Errorf("the round trip produced %d collections, want 2", n)
	}
}
