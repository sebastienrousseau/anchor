// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package swift

import (
	"strings"
	"testing"
)

// A statement without its entries is a balance, not a statement. This is the
// part of the pair that used to be missing in both directions, and the part
// where MT and ISO 20022 describe a transaction most differently.

const entryStatement = `<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:camt.053.001.11">
  <BkToCstmrStmt>
    <GrpHdr><MsgId>STMT-0001</MsgId><CreDtTm>2026-08-24T09:00:00Z</CreDtTm></GrpHdr>
    <Stmt>
      <Id>STMT-0001</Id>
      <ElctrncSeqNb>123</ElctrncSeqNb>
      <Acct><Id><IBAN>GB29NWBK60161331926819</IBAN></Id>
        <Svcr><FinInstnId><BICFI>NWBKGB2LXXX</BICFI></FinInstnId></Svcr></Acct>
      <Bal><Tp><CdOrPrtry><Cd>OPBD</Cd></CdOrPrtry></Tp>
        <Amt Ccy="EUR">100000.00</Amt><CdtDbtInd>CRDT</CdtDbtInd>
        <Dt><Dt>2026-08-23</Dt></Dt></Bal>
      <Ntry>
        <NtryRef>NTRY-001</NtryRef>
        <Amt Ccy="EUR">25000.00</Amt>
        <CdtDbtInd>CRDT</CdtDbtInd>
        <Sts><Cd>BOOK</Cd></Sts>
        <BookgDt><Dt>2026-08-23</Dt></BookgDt>
        <ValDt><Dt>2026-08-24</Dt></ValDt>
        <AcctSvcrRef>SVCR-9001</AcctSvcrRef>
        <BkTxCd><Prtry><Cd>NTRF</Cd><Issr>SWIFT</Issr></Prtry></BkTxCd>
        <NtryDtls><TxDtls>
          <Refs><EndToEndId>E2E-0001</EndToEndId></Refs>
          <RltdPties><Dbtr><Pty><Nm>ACME TRADING</Nm></Pty></Dbtr></RltdPties>
          <RltdAgts><DbtrAgt><FinInstnId><BICFI>BANKGB2LXXX</BICFI></FinInstnId></DbtrAgt></RltdAgts>
          <Chrgs><TtlChrgsAndTaxAmt Ccy="EUR">1.00</TtlChrgsAndTaxAmt></Chrgs>
          <Purp><Cd>SUPP</Cd></Purp>
          <RmtInf><Ustrd>INVOICE 2026-0815</Ustrd></RmtInf>
          <AddtlTxInf>SETTLED SAME DAY</AddtlTxInf>
        </TxDtls></NtryDtls>
        <AddtlNtryInf>INCOMING TRANSFER</AddtlNtryInf>
      </Ntry>
      <Ntry>
        <Amt Ccy="EUR">150.00</Amt>
        <CdtDbtInd>DBIT</CdtDbtInd>
        <RvslInd>true</RvslInd>
        <Sts><Cd>BOOK</Cd></Sts>
        <ValDt><Dt>2026-08-24</Dt></ValDt>
        <BkTxCd><Domn><Cd>PMNT</Cd><Fmly><Cd>ICDT</Cd><SubFmlyCd>ESCT</SubFmlyCd></Fmly></Domn></BkTxCd>
        <NtryDtls><TxDtls><RtrInf><Rsn><Cd>AC04</Cd></Rsn></RtrInf></TxDtls></NtryDtls>
      </Ntry>
      <Bal><Tp><CdOrPrtry><Cd>CLBD</Cd></CdOrPrtry></Tp>
        <Amt Ccy="EUR">124850.00</Amt><CdtDbtInd>CRDT</CdtDbtInd>
        <Dt><Dt>2026-08-24</Dt></Dt></Bal>
    </Stmt>
  </BkToCstmrStmt>
</Document>`

func TestStatementEntriesToMT940(t *testing.T) {
	c := convertBack(t, entryStatement)

	for _, want := range []string{
		":61:2608240823C25000,00NTRFE2E-0001//SVCR-9001",
		":86:INCOMING TRANSFER",
		"INVOICE 2026-0815",
		"SETTLED SAME DAY",
		":61:260824RD150,00NMSCNONREF",
	} {
		if !strings.Contains(c.XML, want) {
			t.Errorf("output is missing %q\n%s", want, c.XML)
		}
	}

	// The entries sit between the two balances, which is where MT940 puts them.
	opening := strings.Index(c.XML, ":60F:")
	first := strings.Index(c.XML, ":61:")
	closing := strings.Index(c.XML, ":62F:")
	if opening >= first || first >= closing {
		t.Errorf("the entries are not between the balances\n%s", c.XML)
	}
}

func TestProprietaryTransactionCodeIsUsedExactly(t *testing.T) {
	// A proprietary code already shaped like an MT transaction type is the
	// original code coming back, not a guess, and is reported as carried.
	c := convertBack(t, entryStatement)

	var exact bool
	for _, r := range c.Report {
		if strings.HasSuffix(r.Tag, "/BkTxCd/Prtry/Cd") && r.Fidelity == FidelityMapped {
			exact = true
			if r.Value != "NTRF" {
				t.Errorf("the proprietary code became %q", r.Value)
			}
		}
	}
	if !exact {
		t.Errorf("the proprietary transaction code was not used: %+v", c.Report)
	}
}

func TestStructuredTransactionCodeIsReportedAsLost(t *testing.T) {
	// MT wants a code from its own vocabulary and no verifiable mapping exists,
	// so NMSC is used and the structured code is named in the report.
	c := convertBack(t, entryStatement)

	var reported bool
	for _, r := range c.Report {
		if strings.HasSuffix(r.Tag, "/BkTxCd") && r.Fidelity == FidelityTruncated {
			reported = true
			if !strings.Contains(r.Value, "PMNT/ICDT/ESCT") {
				t.Errorf("the structured code was not named: %+v", r)
			}
			if !strings.Contains(r.Note, "NMSC") {
				t.Errorf("the note does not say what was used: %q", r.Note)
			}
		}
	}
	if !reported {
		t.Errorf("the structured transaction code was not reported: %+v", c.Report)
	}
}

func TestEntryDetailLossesAreReported(t *testing.T) {
	c := convertBack(t, entryStatement)

	lost := map[string]bool{}
	for _, r := range c.Report {
		if r.Fidelity == FidelityUnmapped {
			lost[r.Tag] = true
		}
	}
	for _, want := range []string{"RltdPties", "RltdAgts", "Chrgs", "Purp", "RtrInf"} {
		found := false
		for tag := range lost {
			if strings.HasSuffix(tag, "/"+want) {
				found = true
			}
		}
		if !found {
			t.Errorf("%s was not reported as lost: %+v", want, c.Report)
		}
	}
}

func TestEntryWithoutAnAmount(t *testing.T) {
	// Amt is mandatory in camt.053, so this can only arrive from a document
	// that never validated. It is reported rather than producing a broken line.
	doc := strings.Replace(entryStatement,
		`<Amt Ccy="EUR">25000.00</Amt>
        <CdtDbtInd>CRDT</CdtDbtInd>`, "<CdtDbtInd>CRDT</CdtDbtInd>", 1)

	c := convertBack(t, doc)
	if strings.Count(c.XML, ":61:") != 1 {
		t.Errorf("an entry with no amount produced a statement line\n%s", c.XML)
	}
	var reported bool
	for _, r := range c.Report {
		if strings.Contains(r.Note, "no amount") {
			reported = true
		}
	}
	if !reported {
		t.Errorf("the entry with no amount was not reported: %+v", c.Report)
	}
}

func TestEntryWithNoDateAnywhere(t *testing.T) {
	// No entry date, and no closing balance to fall back on.
	doc := `<Document xmlns="urn:iso:std:iso:20022:tech:xsd:camt.053.001.11">
  <BkToCstmrStmt><Stmt>
    <Id>S-1</Id>
    <Acct><Id><IBAN>GB29NWBK60161331926819</IBAN></Id></Acct>
    <Bal><Tp><CdOrPrtry><Cd>OPBD</Cd></CdOrPrtry></Tp><Amt Ccy="EUR">1.00</Amt></Bal>
    <Bal><Tp><CdOrPrtry><Cd>CLBD</Cd></CdOrPrtry></Tp><Amt Ccy="EUR">2.00</Amt></Bal>
    <Ntry><Amt Ccy="EUR">1.00</Amt><CdtDbtInd>CRDT</CdtDbtInd></Ntry>
  </Stmt></BkToCstmrStmt></Document>`

	c := convertBack(t, doc)
	if strings.Contains(c.XML, ":61:") {
		t.Errorf("a statement line was written with no date\n%s", c.XML)
	}
	var reported bool
	for _, r := range c.Report {
		if strings.Contains(r.Note, "no date") {
			reported = true
		}
	}
	if !reported {
		t.Errorf("the undated entry was not reported: %+v", c.Report)
	}
}

func TestEntryReferenceFallbacks(t *testing.T) {
	cases := []struct {
		name, details, want string
	}{
		{"end to end", `<NtryDtls><TxDtls><Refs><EndToEndId>E2E-1</EndToEndId></Refs></TxDtls></NtryDtls>`, "E2E-1"},
		{"instruction", `<NtryDtls><TxDtls><Refs><InstrId>INSTR-1</InstrId></Refs></TxDtls></NtryDtls>`, "INSTR-1"},
		{"transaction", `<NtryDtls><TxDtls><Refs><TxId>TX-1</TxId></Refs></TxDtls></NtryDtls>`, "TX-1"},
		{"entry reference", ``, "NTRY-9"},
		{"nothing at all", ``, "NONREF"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			entryRef := ""
			if tc.want == "NTRY-9" {
				entryRef = "<NtryRef>NTRY-9</NtryRef>"
			}
			doc := `<Document xmlns="urn:iso:std:iso:20022:tech:xsd:camt.053.001.11">
  <BkToCstmrStmt><Stmt>
    <Id>S-1</Id>
    <Acct><Id><IBAN>GB29NWBK60161331926819</IBAN></Id></Acct>
    <Bal><Tp><CdOrPrtry><Cd>OPBD</Cd></CdOrPrtry></Tp><Amt Ccy="EUR">1.00</Amt>
      <Dt><Dt>2026-08-23</Dt></Dt></Bal>
    <Bal><Tp><CdOrPrtry><Cd>CLBD</Cd></CdOrPrtry></Tp><Amt Ccy="EUR">2.00</Amt>
      <Dt><Dt>2026-08-24</Dt></Dt></Bal>
    <Ntry>` + entryRef + `<Amt Ccy="EUR">1.00</Amt><CdtDbtInd>CRDT</CdtDbtInd>
      <ValDt><Dt>2026-08-24</Dt></ValDt>` + tc.details + `</Ntry>
  </Stmt></BkToCstmrStmt></Document>`

			c := convertBack(t, doc)
			if !strings.Contains(c.XML, "NMSC"+tc.want) {
				t.Errorf("the reference is not %q\n%s", tc.want, c.XML)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// The other direction
// ---------------------------------------------------------------------------

const mt940WithEntries = `{1:F01NWBKGB2LAXXX0000000000}{2:I940BANKDEFFXXXXN}{4:
:20:STMT20260824
:25:GB29NWBK60161331926819
:28C:00123/001
:60F:C260823EUR100000,00
:61:2608240823C25000,00NTRFE2E-0001//SVCR-9001
:86:INCOMING TRANSFER
INVOICE 2026-0815
:61:260824RD150,00NMSCNONREF
:86:REVERSED CHARGE
:62F:C260824EUR124850,00
-}`

func TestMT940EntriesBecomeCamt053Entries(t *testing.T) {
	c := convert(t, mt940WithEntries)

	if n := strings.Count(c.XML, "<Ntry>"); n != 2 {
		t.Errorf("got %d entries, want 2\n%s", n, c.XML)
	}
	for _, want := range []string{
		"<NtryRef>E2E-0001</NtryRef>",
		`<Amt Ccy="EUR">25000.00</Amt>`,
		"<CdtDbtInd>CRDT</CdtDbtInd>",
		"<AcctSvcrRef>SVCR-9001</AcctSvcrRef>",
		"<Cd>NTRF</Cd>",
		"<AddtlNtryInf>INCOMING TRANSFER INVOICE 2026-0815</AddtlNtryInf>",
		"<RvslInd>true</RvslInd>",
		"<AddtlNtryInf>REVERSED CHARGE</AddtlNtryInf>",
	} {
		if !strings.Contains(c.XML, want) {
			t.Errorf("output is missing %s\n%s", want, c.XML)
		}
	}

	// NONREF is how MT says there is no reference; carrying it across would
	// turn an absence into data.
	if strings.Contains(c.XML, "NONREF") {
		t.Errorf("NONREF was carried into the message\n%s", c.XML)
	}

	// The entry date differs from the value date on the first line, and both
	// have to survive.
	if !strings.Contains(c.XML, "<BookgDt>\n          <Dt>2026-08-23</Dt>") {
		t.Errorf("the booking date was lost\n%s", c.XML)
	}
	if !strings.Contains(c.XML, "<ValDt>\n          <Dt>2026-08-24</Dt>") {
		t.Errorf("the value date was lost\n%s", c.XML)
	}
}

func TestStatementRoundTripKeepsItsEntries(t *testing.T) {
	// A statement that survives a trip through MT and back still describes the
	// same movements, which is the whole point of converting entries at all.
	toMX := convert(t, mt940WithEntries)

	back, err := ConvertMX([]byte(toMX.XML))
	if err != nil {
		t.Fatalf("converting back: %v\n%s", err, toMX.XML)
	}
	for _, want := range []string{
		":61:2608240823C25000,00NTRFE2E-0001//SVCR-9001",
		":61:260824RD150,00NMSC",
		":86:INCOMING TRANSFER INVOICE 2026-0815",
	} {
		if !strings.Contains(back.XML, want) {
			t.Errorf("the round trip lost %q\n%s", want, back.XML)
		}
	}
}

func TestMT940RejectsAMalformedStatementLine(t *testing.T) {
	raw := strings.Replace(mt940WithEntries,
		":61:2608240823C25000,00NTRFE2E-0001//SVCR-9001", ":61:NONSENSE", 1)

	if _, err := Convert(mustParse(t, raw)); err == nil {
		t.Fatal("a malformed statement line was accepted")
	}
}

func TestInformationBeforeAnyStatementLine(t *testing.T) {
	// An :86: with no :61: before it belongs to nothing, and saying so is
	// better than attaching it to the next entry.
	raw := `{1:F01NWBKGB2LAXXX0000000000}{2:I940BANKDEFFXXXXN}{4:
:20:STMT-1
:25:GB29NWBK60161331926819
:60F:C260823EUR1,00
:86:ORPHANED INFORMATION
:61:260824C1,00NTRFREF-1
:62F:C260824EUR2,00
-}`
	c := convert(t, raw)
	if got := fidelityOf(t, c, "86").Fidelity; got != FidelityUnmapped {
		t.Errorf(":86: fidelity = %q, want unmapped", got)
	}
	if strings.Contains(c.XML, "ORPHANED INFORMATION") {
		t.Errorf("the orphaned information was attached to an entry\n%s", c.XML)
	}
}

func TestParseStatementLine(t *testing.T) {
	cases := []struct {
		name, in string
		want     StatementLine
	}{
		{
			"with an entry date and a servicer reference",
			"2608240823C25000,00NTRFE2E-0001//SVCR-9001",
			StatementLine{
				ValueDate: "2026-08-24", BookingDate: "2026-08-23", Credit: true,
				Amount: "25000.00", TransactionType: "NTRF",
				Reference: "E2E-0001", ServicerReference: "SVCR-9001",
			},
		},
		{
			"a debit with no entry date",
			"260824D150,00NMSCNONREF",
			StatementLine{
				ValueDate: "2026-08-24", Amount: "150.00",
				TransactionType: "NMSC", Reference: "NONREF",
			},
		},
		{
			"a reversal",
			"260824RC25,00NTRFREF-1",
			StatementLine{
				ValueDate: "2026-08-24", Credit: true, Reversal: true,
				Amount: "25.00", TransactionType: "NTRF", Reference: "REF-1",
			},
		},
		{
			"with a funds code",
			"260824CR25,00NTRFREF-1",
			StatementLine{
				ValueDate: "2026-08-24", Credit: true,
				Amount: "25.00", TransactionType: "NTRF", Reference: "REF-1",
			},
		},
		{
			"a whole amount",
			"260824C5000,NTRFREF-1",
			StatementLine{
				ValueDate: "2026-08-24", Credit: true,
				Amount: "5000", TransactionType: "NTRF", Reference: "REF-1",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseStatementLine(tc.in)
			if err != nil {
				t.Fatalf("ParseStatementLine: %v", err)
			}
			if got != tc.want {
				t.Errorf("got  %+v\nwant %+v", got, tc.want)
			}
		})
	}

	// Only the first line matters: the supplementary details line that may
	// follow is not part of the statement line itself.
	multi, err := ParseStatementLine("260824C1,00NTRFREF-1\nSUPPLEMENTARY DETAILS")
	if err != nil {
		t.Fatal(err)
	}
	if multi.Reference != "REF-1" {
		t.Errorf("the supplementary line leaked into the reference: %+v", multi)
	}
}

func TestParseStatementLineRejects(t *testing.T) {
	for _, in := range []string{
		"",
		"NONSENSE",
		"260824C25000,00",          // no transaction type
		"269999C25000,00NTRFREF-1", // an impossible value date
		"2608240899C1,00NTRFREF-1", // an impossible entry date
		"260824C,NTRFREF-1",        // no amount
		"260824X25000,00NTRFREF-1", // not a debit or credit mark
	} {
		if _, err := ParseStatementLine(in); err == nil {
			t.Errorf("ParseStatementLine(%q) was accepted", in)
		}
	}
}

func TestTransactionCodeWithNeitherBranch(t *testing.T) {
	// BankTransactionCodeStructure has two optional branches, so a code
	// carrying neither is possible and describes nothing.
	doc := `<Document xmlns="urn:iso:std:iso:20022:tech:xsd:camt.053.001.11">
  <BkToCstmrStmt><Stmt>
    <Id>S-1</Id>
    <Acct><Id><IBAN>GB29NWBK60161331926819</IBAN></Id></Acct>
    <Bal><Tp><CdOrPrtry><Cd>OPBD</Cd></CdOrPrtry></Tp><Amt Ccy="EUR">1.00</Amt>
      <Dt><Dt>2026-08-23</Dt></Dt></Bal>
    <Bal><Tp><CdOrPrtry><Cd>CLBD</Cd></CdOrPrtry></Tp><Amt Ccy="EUR">2.00</Amt>
      <Dt><Dt>2026-08-24</Dt></Dt></Bal>
    <Ntry><Amt Ccy="EUR">1.00</Amt><CdtDbtInd>CRDT</CdtDbtInd>
      <ValDt><Dt>2026-08-24</Dt></ValDt>
      <BkTxCd/></Ntry>
  </Stmt></BkToCstmrStmt></Document>`

	c := convertBack(t, doc)
	if !strings.Contains(c.XML, "NMSC") {
		t.Errorf("no fallback transaction type was written\n%s", c.XML)
	}
	// And a proprietary code that is not MT-shaped is not passed through.
	other := strings.Replace(doc, "<BkTxCd/>",
		"<BkTxCd><Prtry><Cd>NOT-AN-MT-CODE</Cd></Prtry></BkTxCd>", 1)
	c = convertBack(t, other)
	if strings.Contains(c.XML, "NOT-AN-MT-CODE") {
		t.Errorf("a code of the wrong shape was written into field 61\n%s", c.XML)
	}
	if !strings.Contains(c.XML, "NMSC") {
		t.Errorf("no fallback transaction type was written\n%s", c.XML)
	}
}
