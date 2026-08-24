// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package swift

import (
	"strings"
	"testing"
)

// Going MX to MT is the direction that loses what a regulator added, so these
// tests are as much about what the report says as about what comes out.

func convertBack(t *testing.T, document string) *Conversion {
	t.Helper()
	c, err := ConvertMX([]byte(document))
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	// Whatever comes out has to parse as an MT message, or the next system in
	// the chain cannot read it.
	if _, err := Parse([]byte(c.XML)); err != nil {
		t.Fatalf("the generated MT does not parse: %v\n%s", err, c.XML)
	}
	return c
}

const pacs008Doc = `<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">
  <FIToFICstmrCdtTrf>
    <GrpHdr>
      <MsgId>MSG-20260824-0001</MsgId>
      <CreDtTm>2026-08-24T09:00:00Z</CreDtTm>
      <NbOfTxs>1</NbOfTxs>
    </GrpHdr>
    <CdtTrfTxInf>
      <PmtId>
        <InstrId>INSTR-0001</InstrId>
        <EndToEndId>E2E-0001</EndToEndId>
        <UETR>f3a1b2c4-5d6e-4f70-8a91-2b3c4d5e6f70</UETR>
      </PmtId>
      <IntrBkSttlmAmt Ccy="EUR">25000.00</IntrBkSttlmAmt>
      <IntrBkSttlmDt>2026-08-24</IntrBkSttlmDt>
      <InstdAmt Ccy="GBP">21000.00</InstdAmt>
      <XchgRate>1.1880</XchgRate>
      <ChrgBr>DEBT</ChrgBr>
      <ChrgsInf><Amt Ccy="EUR">25.00</Amt></ChrgsInf>
      <ChrgsInf><Amt Ccy="EUR">15.00</Amt></ChrgsInf>
      <Dbtr>
        <Nm>ACME TRADING LIMITED</Nm>
        <PstlAdr>
          <StrtNm>GRESHAM STREET</StrtNm>
          <BldgNb>14</BldgNb>
          <PstCd>EC2V 7NN</PstCd>
          <TwnNm>LONDON</TwnNm>
          <Ctry>GB</Ctry>
        </PstlAdr>
        <Id><OrgId><LEI>7LTWFZYICNSX8D621K86</LEI></OrgId></Id>
      </Dbtr>
      <DbtrAcct><Id><IBAN>GB29NWBK60161331926819</IBAN></Id></DbtrAcct>
      <DbtrAgt><FinInstnId><BICFI>BANKGB2LXXX</BICFI></FinInstnId></DbtrAgt>
      <CdtrAgt><FinInstnId><BICFI>BANKDEFFXXX</BICFI></FinInstnId></CdtrAgt>
      <Cdtr>
        <Nm>MUELLER GMBH</Nm>
        <PstlAdr><AdrLine>HAUPTSTRASSE 12</AdrLine></PstlAdr>
      </Cdtr>
      <CdtrAcct><Id><IBAN>DE89370400440532013000</IBAN></Id></CdtrAcct>
      <InstrForNxtAgt><Cd>PHOA</Cd><InstrInf>CALL TREASURY</InstrInf></InstrForNxtAgt>
      <Purp><Cd>SUPP</Cd></Purp>
      <RmtInf><Ustrd>INVOICE 2026-0815</Ustrd></RmtInf>
    </CdtTrfTxInf>
  </FIToFICstmrCdtTrf>
</Document>`

func TestConvertPacs008ToMT103(t *testing.T) {
	c := convertBack(t, pacs008Doc)

	if c.SourceType != "pacs.008.001.10" || c.TargetType != "MT103" {
		t.Errorf("got %s -> %s", c.SourceType, c.TargetType)
	}

	for _, want := range []string{
		"{1:F01BANKGB2LAXXX0000000000}",
		"{2:I103BANKDEFFXXXXN}",
		"{3:{121:f3a1b2c4-5d6e-4f70-8a91-2b3c4d5e6f70}}",
		":20:INSTR-0001",
		":23B:CRED",
		":23E:PHOA/CALL TREASURY",
		":32A:260824EUR25000,00",
		":33B:GBP21000,00",
		":36:1,1880",
		":50K:/GB29NWBK60161331926819",
		"ACME TRADING LIMITED",
		":52A:BANKGB2LXXX",
		":57A:BANKDEFFXXX",
		":59:/DE89370400440532013000",
		"MUELLER GMBH",
		":70:INVOICE 2026-0815",
		":71A:OUR",
		":71F:EUR25,00",
		":71G:EUR15,00",
	} {
		if !strings.Contains(c.XML, want) {
			t.Errorf("output is missing %q\n%s", want, c.XML)
		}
	}
}

func TestConvertPacs008ReportsWhatMTCannotCarry(t *testing.T) {
	c := convertBack(t, pacs008Doc)

	// The things a regulator added and MT has no room for.
	lost := map[string]bool{}
	for _, r := range c.Report {
		if r.Fidelity == FidelityUnmapped {
			lost[r.Tag] = true
		}
	}
	if !lost["/Document/FIToFICstmrCdtTrf/CdtTrfTxInf/Purp"] {
		t.Errorf("the purpose code was not reported as lost: %+v", c.Report)
	}
	if !lost["/Document/FIToFICstmrCdtTrf/CdtTrfTxInf/Dbtr/Id/OrgId/LEI"] {
		t.Errorf("the legal entity identifier was not reported as lost: %+v", c.Report)
	}

	// And the structured address, which is the whole point of the 2026 rules.
	var sawAddress bool
	for _, r := range c.Report {
		if strings.HasSuffix(r.Tag, "/Dbtr/PstlAdr") {
			sawAddress = true
			if r.Fidelity != FidelityTruncated {
				t.Errorf("the structured address is %q, want truncated", r.Fidelity)
			}
			if !strings.Contains(r.Note, "TwnNm") {
				t.Errorf("the note does not say what is lost: %q", r.Note)
			}
		}
	}
	if !sawAddress {
		t.Errorf("the structured address was not reported: %+v", c.Report)
	}

	// The bank operation code has no source at all.
	var sawDerived bool
	for _, r := range c.Report {
		if r.Path == ":23B:" && r.Fidelity == FidelityDerived {
			sawDerived = true
		}
	}
	if !sawDerived {
		t.Errorf("the derived bank operation code was not reported: %+v", c.Report)
	}
	if c.Lossless() {
		t.Error("a conversion that lost a purpose code reported itself as lossless")
	}
}

func TestConvertPacs008TruncatesALongReference(t *testing.T) {
	// Field 20 permits 16 characters; the source permits 35. That is the
	// clearest loss in this direction and it has to be reported.
	long := strings.Replace(pacs008Doc,
		"<InstrId>INSTR-0001</InstrId>",
		"<InstrId>INSTRUCTION-IDENTIFICATION-0001</InstrId>", 1)

	c := convertBack(t, long)
	if strings.Contains(c.XML, "INSTRUCTION-IDENTIFICATION-0001") {
		t.Errorf("a 31-character reference was emitted into a 16-character field\n%s", c.XML)
	}

	var reported bool
	for _, r := range c.Report {
		if r.Path == ":20:" {
			reported = true
			if r.Fidelity != FidelityTruncated {
				t.Errorf(":20: is %q, want truncated", r.Fidelity)
			}
			if !strings.Contains(r.Note, "16") {
				t.Errorf("the note does not give the limit: %q", r.Note)
			}
		}
	}
	if !reported {
		t.Errorf(":20: was not reported: %+v", c.Report)
	}
}

func TestConvertPacs008WrapsLongText(t *testing.T) {
	long := strings.Replace(pacs008Doc,
		"<Ustrd>INVOICE 2026-0815</Ustrd>",
		"<Ustrd>"+strings.Repeat("A", 100)+"</Ustrd>", 1)

	c := convertBack(t, long)
	// Field 70 is four lines of 35, so 100 characters fit across three lines.
	// Only the text block is line-bounded; the header blocks are one line.
	_, body, found := strings.Cut(c.XML, "{4:\n")
	if !found {
		t.Fatalf("no text block:\n%s", c.XML)
	}
	for _, line := range strings.Split(body, "\n") {
		if !strings.Contains(line, "A") {
			continue
		}
		if len([]rune(strings.TrimPrefix(line, ":70:"))) > 35 {
			t.Errorf("a line exceeds the MT width: %q", line)
		}
	}

	// More than fits is dropped, and said so.
	tooLong := strings.Replace(pacs008Doc,
		"<Ustrd>INVOICE 2026-0815</Ustrd>",
		"<Ustrd>"+strings.Repeat("A", 400)+"</Ustrd>", 1)
	c = convertBack(t, tooLong)

	var reported bool
	for _, r := range c.Report {
		if r.Path == ":70:" && r.Fidelity == FidelityTruncated {
			reported = true
			if !strings.Contains(r.Note, "did not fit") {
				t.Errorf("the note does not say what was dropped: %q", r.Note)
			}
		}
	}
	if !reported {
		t.Errorf("the dropped remittance lines were not reported: %+v", c.Report)
	}
}

func TestConvertPacs008Fallbacks(t *testing.T) {
	// No instruction identification, no settlement date, an unrecognised charge
	// bearer: each falls back and says so.
	minimal := `<?xml version="1.0"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">
  <FIToFICstmrCdtTrf>
    <GrpHdr><MsgId>MSG-1</MsgId></GrpHdr>
    <CdtTrfTxInf>
      <PmtId><EndToEndId>E2E-1</EndToEndId></PmtId>
      <IntrBkSttlmAmt Ccy="EUR">1.00</IntrBkSttlmAmt>
      <Dbtr><Nm>ACME</Nm></Dbtr>
      <Cdtr><Nm>MUELLER</Nm></Cdtr>
    </CdtTrfTxInf>
  </FIToFICstmrCdtTrf>
</Document>`

	c := convertBack(t, minimal)
	if !strings.Contains(c.XML, ":20:MSG-1") {
		t.Errorf("the message identifier was not used as the reference\n%s", c.XML)
	}
	if !strings.Contains(c.XML, ":71A:SHA") {
		t.Errorf("the mandatory charge bearer was not defaulted\n%s", c.XML)
	}
	// A whole amount is written with a trailing comma in MT.
	if !strings.Contains(c.XML, "EUR1,00") {
		t.Errorf("the amount was not written in MT form\n%s", c.XML)
	}
	// With no agent BICs the header still has to be well formed.
	if !strings.Contains(c.XML, "{1:F01NOTPROVDAXXX") {
		t.Errorf("the header has no sender placeholder\n%s", c.XML)
	}
}

func TestConvertPacs008MultipleTransactions(t *testing.T) {
	// MT103 carries one payment. A batch has to say what was left behind.
	two := strings.Replace(pacs008Doc, "</CdtTrfTxInf>\n  </FIToFICstmrCdtTrf>",
		`</CdtTrfTxInf>
    <CdtTrfTxInf>
      <PmtId><EndToEndId>E2E-0002</EndToEndId></PmtId>
      <IntrBkSttlmAmt Ccy="EUR">1.00</IntrBkSttlmAmt>
      <Dbtr><Nm>ACME</Nm></Dbtr>
      <Cdtr><Nm>SCHMIDT</Nm></Cdtr>
    </CdtTrfTxInf>
  </FIToFICstmrCdtTrf>`, 1)

	c := convertBack(t, two)
	if strings.Contains(c.XML, "SCHMIDT") {
		t.Errorf("the second payment leaked into the message\n%s", c.XML)
	}

	var reported bool
	for _, r := range c.Report {
		if strings.Contains(r.Note, "only the first was converted") {
			reported = true
		}
	}
	if !reported {
		t.Errorf("the dropped payment was not reported: %+v", c.Report)
	}
}

func TestConvertPacs008Rejects(t *testing.T) {
	cases := map[string]string{
		"no body": `<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10"/>`,
		"no transaction": `<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">
  <FIToFICstmrCdtTrf><GrpHdr><MsgId>M</MsgId></GrpHdr></FIToFICstmrCdtTrf></Document>`,
		"no amount": `<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">
  <FIToFICstmrCdtTrf><CdtTrfTxInf><Dbtr><Nm>A</Nm></Dbtr><Cdtr><Nm>B</Nm></Cdtr></CdtTrfTxInf></FIToFICstmrCdtTrf></Document>`,
		"no debtor": `<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">
  <FIToFICstmrCdtTrf><CdtTrfTxInf><IntrBkSttlmAmt Ccy="EUR">1.00</IntrBkSttlmAmt>
  <Cdtr><Nm>B</Nm></Cdtr></CdtTrfTxInf></FIToFICstmrCdtTrf></Document>`,
		"no creditor": `<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">
  <FIToFICstmrCdtTrf><CdtTrfTxInf><IntrBkSttlmAmt Ccy="EUR">1.00</IntrBkSttlmAmt>
  <Dbtr><Nm>A</Nm></Dbtr></CdtTrfTxInf></FIToFICstmrCdtTrf></Document>`,
	}
	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ConvertMX([]byte(doc)); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

const pacs009Doc = `<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.009.001.10">
  <FICdtTrf>
    <GrpHdr><MsgId>COVER-0001</MsgId><CreDtTm>2026-08-24T09:00:00Z</CreDtTm><NbOfTxs>1</NbOfTxs></GrpHdr>
    <CdtTrfTxInf>
      <PmtId><EndToEndId>REL-0001</EndToEndId><UETR>f3a1b2c4-5d6e-4f70-8a91-2b3c4d5e6f70</UETR></PmtId>
      <IntrBkSttlmAmt Ccy="EUR">25000.00</IntrBkSttlmAmt>
      <IntrBkSttlmDt>2026-08-24</IntrBkSttlmDt>
      <InstgAgt><FinInstnId><BICFI>CHASGB2LXXX</BICFI></FinInstnId></InstgAgt>
      <InstdAgt><FinInstnId><BICFI>BANKDEFFXXX</BICFI></FinInstnId></InstdAgt>
      <Dbtr><FinInstnId><BICFI>BANKGB2LXXX</BICFI></FinInstnId></Dbtr>
      <Cdtr><FinInstnId><BICFI>DEUTDEFFXXX</BICFI></FinInstnId></Cdtr>
    </CdtTrfTxInf>
  </FICdtTrf>
</Document>`

func TestConvertPacs009ToMT202(t *testing.T) {
	c := convertBack(t, pacs009Doc)

	if c.TargetType != "MT202" {
		t.Errorf("TargetType = %q", c.TargetType)
	}
	for _, want := range []string{
		":20:COVER-0001",
		":21:REL-0001",
		":32A:260824EUR25000,00",
		":52A:BANKGB2LXXX",
		":53A:CHASGB2LXXX",
		":57A:BANKDEFFXXX",
		":58A:DEUTDEFFXXX",
		"{3:{121:f3a1b2c4-5d6e-4f70-8a91-2b3c4d5e6f70}}",
	} {
		if !strings.Contains(c.XML, want) {
			t.Errorf("output is missing %q\n%s", want, c.XML)
		}
	}
}

func TestConvertPacs009Fallbacks(t *testing.T) {
	// No end-to-end identification: :21: is mandatory, so the sender's
	// reference is reused and the report says so.
	minimal := `<?xml version="1.0"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.009.001.10">
  <FICdtTrf>
    <GrpHdr><MsgId>COVER-1</MsgId></GrpHdr>
    <CdtTrfTxInf>
      <PmtId/>
      <IntrBkSttlmAmt Ccy="EUR">1.00</IntrBkSttlmAmt>
      <InstgAgt><FinInstnId><BICFI>CHASGB2LXXX</BICFI></FinInstnId></InstgAgt>
      <Cdtr><FinInstnId><BICFI>DEUTDEFFXXX</BICFI></FinInstnId></Cdtr>
    </CdtTrfTxInf>
  </FICdtTrf>
</Document>`

	c := convertBack(t, minimal)
	if !strings.Contains(c.XML, ":21:COVER-1") {
		t.Errorf("the mandatory related reference was not defaulted\n%s", c.XML)
	}
	// With no Dbtr the instructing agent stands in.
	if !strings.Contains(c.XML, ":52A:CHASGB2LXXX") {
		t.Errorf("the instructing agent was not used as the debtor\n%s", c.XML)
	}
}

func TestConvertPacs009Rejects(t *testing.T) {
	for name, doc := range map[string]string{
		"no body":        `<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.009.001.10"/>`,
		"no transaction": `<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.009.001.10"><FICdtTrf><GrpHdr><MsgId>M</MsgId></GrpHdr></FICdtTrf></Document>`,
		"no amount": `<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.009.001.10"><FICdtTrf><CdtTrfTxInf>
  <Cdtr><FinInstnId><BICFI>DEUTDEFFXXX</BICFI></FinInstnId></Cdtr></CdtTrfTxInf></FICdtTrf></Document>`,
		"no creditor": `<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.009.001.10"><FICdtTrf><CdtTrfTxInf>
  <IntrBkSttlmAmt Ccy="EUR">1.00</IntrBkSttlmAmt></CdtTrfTxInf></FICdtTrf></Document>`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ConvertMX([]byte(doc)); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

const camt053Doc = `<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:camt.053.001.11">
  <BkToCstmrStmt>
    <GrpHdr><MsgId>STMT-0001</MsgId><CreDtTm>2026-08-24T09:00:00Z</CreDtTm></GrpHdr>
    <Stmt>
      <Id>STMT-0001</Id>
      <ElctrncSeqNb>123</ElctrncSeqNb>
      <Acct>
        <Id><IBAN>GB29NWBK60161331926819</IBAN></Id>
        <Svcr><FinInstnId><BICFI>NWBKGB2LXXX</BICFI></FinInstnId></Svcr>
      </Acct>
      <Bal>
        <Tp><CdOrPrtry><Cd>OPBD</Cd></CdOrPrtry></Tp>
        <Amt Ccy="EUR">100000.00</Amt><CdtDbtInd>CRDT</CdtDbtInd>
        <Dt><Dt>2026-08-23</Dt></Dt>
      </Bal>
      <Bal>
        <Tp><CdOrPrtry><Cd>CLBD</Cd></CdOrPrtry></Tp>
        <Amt Ccy="EUR">125000.00</Amt><CdtDbtInd>DBIT</CdtDbtInd>
        <Dt><Dt>2026-08-24</Dt></Dt>
      </Bal>
      <Ntry><Amt Ccy="EUR">25000.00</Amt></Ntry>
    </Stmt>
  </BkToCstmrStmt>
</Document>`

func TestConvertCamt053ToMT940(t *testing.T) {
	c := convertBack(t, camt053Doc)

	if c.TargetType != "MT940" {
		t.Errorf("TargetType = %q", c.TargetType)
	}
	for _, want := range []string{
		"{1:F01NWBKGB2LAXXX",
		":20:STMT-0001",
		":25:GB29NWBK60161331926819",
		":28C:123",
		":60F:C260823EUR100000,00",
		":62F:D260824EUR125000,00",
	} {
		if !strings.Contains(c.XML, want) {
			t.Errorf("output is missing %q\n%s", want, c.XML)
		}
	}

	// The entry becomes a statement line. Its date is missing from the fixture,
	// so the statement's closing date stands in and the report says so.
	if !strings.Contains(c.XML, ":61:260824C25000,00") {
		t.Errorf("the entry did not become a statement line\n%s", c.XML)
	}
	var derivedDate bool
	for _, r := range c.Report {
		if r.Path == ":61:" && r.Fidelity == FidelityDerived &&
			strings.Contains(r.Note, "closing date") {
			derivedDate = true
		}
	}
	if !derivedDate {
		t.Errorf("the derived value date was not reported: %+v", c.Report)
	}
}

func TestConvertCamt053Fallbacks(t *testing.T) {
	// No sequence number and no statement identifier: both are mandatory in
	// MT940, so both are defaulted and reported.
	doc := `<?xml version="1.0"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:camt.053.001.11">
  <BkToCstmrStmt>
    <GrpHdr><MsgId>STMT-1</MsgId></GrpHdr>
    <Stmt>
      <Acct><Id><Othr><Id>12345678</Id></Othr></Id></Acct>
      <Bal><Tp><CdOrPrtry><Cd>PRCD</Cd></CdOrPrtry></Tp>
        <Amt Ccy="EUR">1.00</Amt><Dt><DtTm>2026-08-23T00:00:00Z</DtTm></Dt></Bal>
      <Bal><Tp><CdOrPrtry><Cd>CLBD</Cd></CdOrPrtry></Tp>
        <Amt Ccy="EUR">2.00</Amt><Dt><Dt>2026-08-24</Dt></Dt></Bal>
    </Stmt>
  </BkToCstmrStmt>
</Document>`

	c := convertBack(t, doc)
	if !strings.Contains(c.XML, ":20:STMT-1") {
		t.Errorf("the message identifier was not used\n%s", c.XML)
	}
	if !strings.Contains(c.XML, ":28C:1") {
		t.Errorf("the mandatory sequence number was not defaulted\n%s", c.XML)
	}
	// A non-IBAN account still identifies the statement.
	if !strings.Contains(c.XML, ":25:12345678") {
		t.Errorf("a domestic account number was dropped\n%s", c.XML)
	}
	// PRCD is a previously closing balance, which is MT940's opening balance.
	if !strings.Contains(c.XML, ":60F:") {
		t.Errorf("PRCD was not treated as the opening balance\n%s", c.XML)
	}
}

func TestConvertCamt053Rejects(t *testing.T) {
	for name, doc := range map[string]string{
		"no body":      `<Document xmlns="urn:iso:std:iso:20022:tech:xsd:camt.053.001.11"/>`,
		"no statement": `<Document xmlns="urn:iso:std:iso:20022:tech:xsd:camt.053.001.11"><BkToCstmrStmt><GrpHdr><MsgId>M</MsgId></GrpHdr></BkToCstmrStmt></Document>`,
		"no account": `<Document xmlns="urn:iso:std:iso:20022:tech:xsd:camt.053.001.11"><BkToCstmrStmt><Stmt>
  <Id>S</Id></Stmt></BkToCstmrStmt></Document>`,
		"no balances": `<Document xmlns="urn:iso:std:iso:20022:tech:xsd:camt.053.001.11"><BkToCstmrStmt><Stmt>
  <Id>S</Id><Acct><Id><IBAN>GB29NWBK60161331926819</IBAN></Id></Acct></Stmt></BkToCstmrStmt></Document>`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ConvertMX([]byte(doc)); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestConvertMXRejectsWhatItCannotRead(t *testing.T) {
	if _, err := ConvertMX([]byte("<unclosed>")); err == nil {
		t.Error("malformed XML was accepted")
	}
	if _, err := ConvertMX([]byte("<Document/>")); err == nil {
		t.Error("a document with no ISO 20022 namespace was accepted")
	}

	// A message Anchor knows but cannot convert names what it can.
	_, err := ConvertMX([]byte(`<Document xmlns="urn:iso:std:iso:20022:tech:xsd:seev.031.001.09"/>`))
	if err == nil {
		t.Fatal("an unsupported message was accepted")
	}
	if !strings.Contains(err.Error(), "pacs.008") {
		t.Errorf("the error does not list what is supported: %v", err)
	}
}

func TestRoundTripThroughBothDirections(t *testing.T) {
	// The property that makes coexistence work: a message converted to MT and
	// back must still be the same payment.
	toMT := convertBack(t, pacs008Doc)

	back, err := Convert(mustParse(t, toMT.XML))
	if err != nil {
		t.Fatalf("converting back: %v\n%s", err, toMT.XML)
	}

	for _, want := range []string{
		"<IntrBkSttlmAmt Ccy=\"EUR\">25000.00</IntrBkSttlmAmt>",
		"<IBAN>GB29NWBK60161331926819</IBAN>",
		"<IBAN>DE89370400440532013000</IBAN>",
		"<BICFI>BANKGB2LXXX</BICFI>",
		"<BICFI>BANKDEFFXXX</BICFI>",
		"<Nm>ACME TRADING LIMITED</Nm>",
		"<Nm>MUELLER GMBH</Nm>",
		"<UETR>f3a1b2c4-5d6e-4f70-8a91-2b3c4d5e6f70</UETR>",
		"<ChrgBr>DEBT</ChrgBr>",
	} {
		if !strings.Contains(back.XML, want) {
			t.Errorf("the round trip lost %s\n%s", want, back.XML)
		}
	}

	// And what it could not keep: the structured address is now free text,
	// which is exactly what the 2026 rules are about.
	if strings.Contains(back.XML, "<TwnNm>") {
		t.Error("a structured address survived a trip through MT, which it cannot")
	}
	if !strings.Contains(back.XML, "<AdrLine>") {
		t.Errorf("the address was lost entirely\n%s", back.XML)
	}
}

func TestSupportedMX(t *testing.T) {
	got := SupportedMX()
	if len(got) != 6 {
		t.Errorf("SupportedMX() = %v", got)
	}
	// Every message listed has to actually convert, or the list is a promise
	// the tool does not keep.
	for _, want := range []string{"pain.001", "pacs.008", "pain.008", "pacs.009", "pacs.010", "camt.053"} {
		found := false
		for _, g := range got {
			if g == want {
				found = true
			}
		}
		if !found {
			t.Errorf("%s is missing from %v", want, got)
		}
	}
}

func TestMTAmountFormatting(t *testing.T) {
	cases := map[string]string{
		"25000.00": "25000,00",
		"1000":     "1000,",
		"":         "",
		" 1.5 ":    "1,5",
	}
	for in, want := range cases {
		if got := mtAmount(in); got != want {
			t.Errorf("mtAmount(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestISODateToMT(t *testing.T) {
	if got := isoDateToMT("2026-08-24"); got != "260824" {
		t.Errorf("isoDateToMT = %q", got)
	}
	if got := isoDateToMT("2026-08"); got != "" {
		t.Errorf("isoDateToMT of a partial date = %q", got)
	}
}

func TestLogicalTerminal(t *testing.T) {
	cases := []struct{ bic, terminal, want string }{
		{"BANKGB2LXXX", "A", "BANKGB2LAXXX"},
		{"BANKGB2L", "A", "BANKGB2LAXXX"},
		{"BANK", "X", "BANKXXXXXXXX"},
		{"", "A", "NOTPROVDAXXX"},
	}
	for _, tc := range cases {
		if got := logicalTerminal(tc.bic, tc.terminal); got != tc.want {
			t.Errorf("logicalTerminal(%q) = %q, want %q", tc.bic, got, tc.want)
		}
	}
}

func TestReverseChargeBearer(t *testing.T) {
	for iso, mt := range map[string]string{"DEBT": "OUR", "CRED": "BEN", "SHAR": "SHA"} {
		if got, ok := reverseChargeBearer(iso); !ok || got != mt {
			t.Errorf("reverseChargeBearer(%q) = %q, %v", iso, got, ok)
		}
	}
	if _, ok := reverseChargeBearer("SLEV"); ok {
		t.Error("an unmappable charge bearer was accepted")
	}
}

func TestWrapLines(t *testing.T) {
	got := wrapLines([]string{"", "  ", strings.Repeat("A", 80)}, 35)
	if len(got) != 3 {
		t.Fatalf("wrapLines produced %d lines: %v", len(got), got)
	}
	for _, line := range got {
		if len([]rune(line)) > 35 {
			t.Errorf("line %q is too long", line)
		}
	}
	if got := wrapLines(nil, 35); got != nil {
		t.Errorf("wrapLines(nil) = %v", got)
	}
}

func TestMXHelpersOnAbsentContent(t *testing.T) {
	// Every reader has to cope with an element that is not there, because a
	// document arriving from another institution routinely omits what is
	// optional.
	if _, ok := child(nil, "Anything"); ok {
		t.Error("child of a nil node matched")
	}
	if got := childrenNamed(nil, "Anything"); got != nil {
		t.Errorf("childrenNamed of a nil node = %v", got)
	}
	if got := attr(nil, "Ccy"); got != "" {
		t.Errorf("attr of a nil node = %q", got)
	}

	empty := `<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">
  <FIToFICstmrCdtTrf><CdtTrfTxInf>
    <IntrBkSttlmAmt Ccy="EUR">1.00</IntrBkSttlmAmt>
    <Dbtr><Nm>A</Nm></Dbtr>
    <DbtrAcct><Id/></DbtrAcct>
    <Cdtr><Nm>B</Nm></Cdtr>
    <RmtInf/>
  </CdtTrfTxInf></FIToFICstmrCdtTrf></Document>`

	c := convertBack(t, empty)
	// An account element with nothing usable in it contributes no account line.
	if strings.Contains(c.XML, ":50K:/") {
		t.Errorf("an empty account produced an account line\n%s", c.XML)
	}
	// An empty remittance element produces no field 70 rather than an empty one.
	if strings.Contains(c.XML, ":70:") {
		t.Errorf("an empty RmtInf produced a field 70\n%s", c.XML)
	}
	// An attribute that is not there is not invented.
	if strings.Contains(c.XML, ":36:") {
		t.Errorf("an exchange rate was invented\n%s", c.XML)
	}
}

func TestMXAttributeLookupMisses(t *testing.T) {
	// An amount carrying some other attribute must not have it read as the
	// currency.
	doc := `<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">
  <FIToFICstmrCdtTrf><CdtTrfTxInf>
    <IntrBkSttlmAmt Other="x">1.00</IntrBkSttlmAmt>
    <Dbtr><Nm>A</Nm></Dbtr><Cdtr><Nm>B</Nm></Cdtr>
  </CdtTrfTxInf></FIToFICstmrCdtTrf></Document>`

	c := convertBack(t, doc)
	if strings.Contains(c.XML, ":32A:") && strings.Contains(c.XML, "x1,00") {
		t.Errorf("an unrelated attribute was read as the currency\n%s", c.XML)
	}
}

func TestSetLinesOnEmptyInput(t *testing.T) {
	b := &mtBuilder{}
	b.setLines("59", "/Document/Cdtr", []string{"", "   "}, 4, 35)
	if len(b.fields) != 0 {
		t.Errorf("an empty party produced a field: %v", b.fields)
	}
	if len(b.reports) != 0 {
		t.Errorf("an empty party produced a report: %+v", b.reports)
	}
}

func TestSetOnEmptyValue(t *testing.T) {
	b := &mtBuilder{}
	b.set("20", "/Document/GrpHdr/MsgId", "   ", 16)
	if len(b.fields) != 0 {
		t.Errorf("an empty value produced a field: %v", b.fields)
	}
}

func TestBalanceDateVariants(t *testing.T) {
	// A balance whose date is written directly on the wrapper, and one with no
	// date at all.
	doc := `<Document xmlns="urn:iso:std:iso:20022:tech:xsd:camt.053.001.11">
  <BkToCstmrStmt><Stmt>
    <Id>S-1</Id>
    <Acct><Id><IBAN>GB29NWBK60161331926819</IBAN></Id></Acct>
    <Bal><Tp><CdOrPrtry><Cd>OPBD</Cd></CdOrPrtry></Tp>
      <Amt Ccy="EUR">1.00</Amt><Dt>2026-08-23</Dt></Bal>
    <Bal><Tp><CdOrPrtry><Cd>CLBD</Cd></CdOrPrtry></Tp>
      <Amt Ccy="EUR">2.00</Amt></Bal>
    <Bal><Tp><CdOrPrtry><Cd>ITBD</Cd></CdOrPrtry></Tp>
      <Amt Ccy="EUR">3.00</Amt><Dt><Dt>2026-08-24</Dt></Dt></Bal>
    <Bal><Tp><CdOrPrtry><Cd>OTHR</Cd></CdOrPrtry></Tp></Bal>
  </Stmt></BkToCstmrStmt></Document>`

	c := convertBack(t, doc)
	if !strings.Contains(c.XML, ":60F:C260823EUR1,00") {
		t.Errorf("a date written on the wrapper was not read\n%s", c.XML)
	}
	// A closing balance with no date still produces the field, without one.
	if !strings.Contains(c.XML, ":62F:CEUR2,00") {
		t.Errorf("a balance with no date was dropped\n%s", c.XML)
	}
	// A balance type MT940 has no field for is simply not used.
	if strings.Contains(c.XML, "3,00") {
		t.Errorf("an interim balance was emitted\n%s", c.XML)
	}
}

func TestConvertPacs008WithoutASettlementDate(t *testing.T) {
	// The date is mandatory in field 32A. With none in the source, today's is
	// used and the report says so.
	doc := strings.Replace(pacs008Doc, "<IntrBkSttlmDt>2026-08-24</IntrBkSttlmDt>", "", 1)

	c := convertBack(t, doc)
	var reported bool
	for _, r := range c.Report {
		if r.Path == ":32A:" && r.Fidelity == FidelityDerived {
			reported = true
			if !strings.Contains(r.Note, "today") {
				t.Errorf("the note does not say what was used: %q", r.Note)
			}
		}
	}
	if !reported {
		t.Errorf("the derived settlement date was not reported: %+v", c.Report)
	}
}

func TestConvertPacs009WithoutASettlementDate(t *testing.T) {
	doc := strings.Replace(pacs009Doc, "<IntrBkSttlmDt>2026-08-24</IntrBkSttlmDt>", "", 1)

	c := convertBack(t, doc)
	var reported bool
	for _, r := range c.Report {
		if r.Path == ":32A:" && r.Fidelity == FidelityDerived {
			reported = true
		}
	}
	if !reported {
		t.Errorf("the derived settlement date was not reported: %+v", c.Report)
	}
}

func TestConvertPacs009MultipleTransactions(t *testing.T) {
	two := strings.Replace(pacs009Doc, "</CdtTrfTxInf>\n  </FICdtTrf>",
		`</CdtTrfTxInf>
    <CdtTrfTxInf>
      <PmtId><EndToEndId>REL-0002</EndToEndId></PmtId>
      <IntrBkSttlmAmt Ccy="EUR">1.00</IntrBkSttlmAmt>
      <Cdtr><FinInstnId><BICFI>BNPAFRPPXXX</BICFI></FinInstnId></Cdtr>
    </CdtTrfTxInf>
  </FICdtTrf>`, 1)

	c := convertBack(t, two)
	if strings.Contains(c.XML, "BNPAFRPPXXX") {
		t.Errorf("the second transfer leaked into the message\n%s", c.XML)
	}
	var reported bool
	for _, r := range c.Report {
		if strings.Contains(r.Note, "only the first was converted") {
			reported = true
		}
	}
	if !reported {
		t.Errorf("the dropped transfer was not reported: %+v", c.Report)
	}
}

func TestConvertCamt053MultipleStatements(t *testing.T) {
	two := strings.Replace(camt053Doc, "</Stmt>\n  </BkToCstmrStmt>",
		`</Stmt>
    <Stmt>
      <Id>STMT-0002</Id>
      <Acct><Id><IBAN>DE89370400440532013000</IBAN></Id></Acct>
    </Stmt>
  </BkToCstmrStmt>`, 1)

	c := convertBack(t, two)
	if strings.Contains(c.XML, "DE89370400440532013000") {
		t.Errorf("the second statement leaked into the message\n%s", c.XML)
	}
	var reported bool
	for _, r := range c.Report {
		if strings.Contains(r.Note, "only the first was converted") {
			reported = true
		}
	}
	if !reported {
		t.Errorf("the dropped statement was not reported: %+v", c.Report)
	}
}

func TestConvertPacs008InstructionCodeOnly(t *testing.T) {
	// An instruction with a code and no narrative, and one with narrative and
	// no code: both have to produce a usable field 23E.
	code := strings.Replace(pacs008Doc,
		"<InstrForNxtAgt><Cd>PHOA</Cd><InstrInf>CALL TREASURY</InstrInf></InstrForNxtAgt>",
		"<InstrForNxtAgt><Cd>TELA</Cd></InstrForNxtAgt>", 1)
	if c := convertBack(t, code); !strings.Contains(c.XML, ":23E:TELA") {
		t.Errorf("a bare instruction code was dropped\n%s", c.XML)
	}

	narrative := strings.Replace(pacs008Doc,
		"<InstrForNxtAgt><Cd>PHOA</Cd><InstrInf>CALL TREASURY</InstrInf></InstrForNxtAgt>",
		"<InstrForNxtAgt><InstrInf>CALL TREASURY</InstrInf></InstrForNxtAgt>", 1)
	if c := convertBack(t, narrative); !strings.Contains(c.XML, ":23E:CALL TREASURY") {
		t.Errorf("a bare instruction narrative was dropped\n%s", c.XML)
	}
}

func TestConvertPacs008StructuredRemittanceIsReported(t *testing.T) {
	// Structured remittance is what makes a payment reconcilable automatically,
	// and MT has nowhere to put it. Saying so is the whole value of the report.
	doc := strings.Replace(pacs008Doc,
		"<RmtInf><Ustrd>INVOICE 2026-0815</Ustrd></RmtInf>",
		`<RmtInf><Ustrd>INVOICE 2026-0815</Ustrd>
        <Strd><RfrdDocInf><Tp><CdOrPrtry><Cd>CINV</Cd></CdOrPrtry></Tp><Nb>INV-0815</Nb></RfrdDocInf></Strd>
      </RmtInf>`, 1)

	c := convertBack(t, doc)
	var reported bool
	for _, r := range c.Report {
		if strings.HasSuffix(r.Tag, "/RmtInf/Strd") {
			reported = true
			if r.Fidelity != FidelityUnmapped {
				t.Errorf("structured remittance is %q, want unmapped", r.Fidelity)
			}
		}
	}
	if !reported {
		t.Errorf("structured remittance was not reported: %+v", c.Report)
	}
	// The free-text part still comes across.
	if !strings.Contains(c.XML, ":70:INVOICE 2026-0815") {
		t.Errorf("the unstructured remittance was dropped\n%s", c.XML)
	}
}

func TestConvertPacs008ChargesWithoutAnAmount(t *testing.T) {
	// A charges element with no amount contributes nothing rather than an empty
	// field.
	doc := strings.Replace(pacs008Doc,
		"<ChrgsInf><Amt Ccy=\"EUR\">25.00</Amt></ChrgsInf>",
		"<ChrgsInf><Agt><FinInstnId><BICFI>BANKGB2LXXX</BICFI></FinInstnId></Agt></ChrgsInf>", 1)

	c := convertBack(t, doc)
	if strings.Contains(c.XML, ":71F:\n") || strings.Contains(c.XML, ":71F:EUR25,00") {
		t.Errorf("a charges element with no amount produced a field\n%s", c.XML)
	}
}
