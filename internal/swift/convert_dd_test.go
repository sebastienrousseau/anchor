// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package swift

import (
	"strings"
	"testing"
)

const mt104 = `{1:F01ACMEGB2LAXXX0000000000}{2:I104BANKGB2LXXXXN}{4:
:20:DD-20260824-001
:23E:AUTH
:26T:UTIL
:30:260901
:50C:ACMEGB2LXXX
:50K:/GB29NWBK60161331926819
ACME UTILITIES LIMITED
14 GRESHAM STREET
LONDON
:52A:BANKGB2LXXX
:71A:SHA
:77B:/ORDERRES/GB
:21:COLL-001
:23E:OTHR
:21C:MANDATE-4471
:21D:DD-REF-1
:21E:REG-1
:32B:EUR120,50
:57A:BANKDEFFXXX
:59:/DE89370400440532013000
MUELLER GMBH
:70:INVOICE 2026-0901
:21:COLL-002
:21C:MANDATE-4472
:32B:EUR75,00
:71A:OUR
:25A:/GB29NWBK60161331926819
:57A:CHASUS33XXX
:59:/US64SVBKUS6S3300958879
NORTHWIND INC
:70:INVOICE 2026-0902
-}`

func TestConvert104(t *testing.T) {
	c := convert(t, mt104)

	if c.SourceType != "104" || c.TargetType != "pain.008.001.07" {
		t.Errorf("got %s -> %s", c.SourceType, c.TargetType)
	}

	for _, want := range []string{
		`<MsgId>DD-20260824-001</MsgId>`,
		`<NbOfTxs>2</NbOfTxs>`,
		`<PmtMtd>DD</PmtMtd>`,
		`<ReqdColltnDt>2026-09-01</ReqdColltnDt>`,
		`<Nm>ACME UTILITIES LIMITED</Nm>`,
		`<IBAN>GB29NWBK60161331926819</IBAN>`,
		`<InstdAmt Ccy="EUR">120.50</InstdAmt>`,
		`<InstdAmt Ccy="EUR">75.00</InstdAmt>`,
		`<MndtId>MANDATE-4471</MndtId>`,
		`<Nm>MUELLER GMBH</Nm>`,
		`<IBAN>DE89370400440532013000</IBAN>`,
		`<BICFI>BANKDEFFXXX</BICFI>`,
		`<ChrgBr>SHAR</ChrgBr>`,
		`<Ustrd>INVOICE 2026-0902</Ustrd>`,
	} {
		if !strings.Contains(c.XML, want) {
			t.Errorf("output is missing %s\n%s", want, c.XML)
		}
	}

	// Both collections become their own PmtInf, because MT104 lets a
	// transaction override the creditor and its bank.
	if n := strings.Count(c.XML, "<PmtInf>"); n != 2 {
		t.Errorf("PmtInf count = %d, want 2", n)
	}

	// The instructing party and the creditor share tag 50 and are told apart by
	// their option letter.
	if !strings.Contains(c.XML, "<AnyBIC>ACMEGB2LXXX</AnyBIC>") {
		t.Errorf("the option-C instructing party was not used\n%s", c.XML)
	}
}

func TestConvert104ReportsWhatItCannotCarry(t *testing.T) {
	c := convert(t, mt104)

	for _, tag := range []string{"26T", "77B", "21D"} {
		if got := fidelityOf(t, c, tag).Fidelity; got != FidelityUnmapped {
			t.Errorf(":%s: fidelity = %q, want unmapped", tag, got)
		}
	}
	// And the ones it can.
	for _, tag := range []string{"21C", "21E", "25A (txn 2)", "71A (txn 2)"} {
		if got := fidelityOf(t, c, tag).Fidelity; got == FidelityUnmapped {
			t.Errorf(":%s: was reported as dropped", tag)
		}
	}
}

func TestConvert107MatchesMT104(t *testing.T) {
	// MT107 has MT104's structure exactly, so the two must produce the same
	// document apart from the source type.
	raw107 := strings.Replace(mt104, "I104BANK", "I107BANK", 1)

	from104 := convert(t, mt104)
	from107 := convert(t, raw107)

	if from107.SourceType != "107" {
		t.Errorf("SourceType = %q", from107.SourceType)
	}
	if from107.TargetType != from104.TargetType {
		t.Errorf("targets differ: %s and %s", from104.TargetType, from107.TargetType)
	}
	if stripTimestamp(from107.XML) != stripTimestamp(from104.XML) {
		t.Error("MT104 and MT107 produced different documents")
	}
}

// stripTimestamp removes the generated creation time, which differs between two
// conversions made a moment apart.
func stripTimestamp(doc string) string {
	start := strings.Index(doc, "<CreDtTm>")
	end := strings.Index(doc, "</CreDtTm>")
	if start < 0 || end < 0 {
		return doc
	}
	return doc[:start] + doc[end:]
}

func TestConvert104SequenceADefaults(t *testing.T) {
	raw := `{1:F01ACMEGB2LAXXX0000000000}{2:I104BANKGB2LXXXXN}{4:
:20:DD-1
:30:260901
:50K:/GB29NWBK60161331926819
ACME UTILITIES LIMITED
:52A:BANKGB2LXXX
:21:COLL-1
:32B:EUR1,00
:59:MUELLER GMBH
-}`
	c := convert(t, raw)

	// The sequence A creditor becomes both the instructing party and the
	// creditor when no separate instructing party is named.
	if strings.Count(c.XML, "<Nm>ACME UTILITIES LIMITED</Nm>") != 2 {
		t.Errorf("the creditor was not used as the instructing party\n%s", c.XML)
	}
	// The debtor's bank falls back to the receiver.
	if !strings.Contains(c.XML, "<BICFI>BANKGB2LXXX</BICFI>") {
		t.Errorf("no debtor agent was derived\n%s", c.XML)
	}
}

func TestConvert104WithoutAnyParty(t *testing.T) {
	raw := "{1:F01ACMEGB2LAXXX0000000000}{2:I104BANKGB2LXXXXN}{4:\n" +
		":20:DD-1\n:30:260901\n:21:COLL-1\n:32B:EUR1,00\n-}"
	c := convert(t, raw)

	// Cdtr, CdtrAcct, Dbtr and DbtrAcct are all mandatory; placeholders keep
	// the document valid while the report says what was missing.
	if strings.Count(c.XML, "<Nm>NOT PROVIDED</Nm>") < 2 {
		t.Errorf("the mandatory parties have no placeholders\n%s", c.XML)
	}
	if strings.Count(c.XML, "<Id>NOTPROVIDED</Id>") < 2 {
		t.Errorf("the mandatory accounts have no placeholders\n%s", c.XML)
	}
	if !strings.Contains(c.XML, "<Nm>ACMEGB2LXXX</Nm>") {
		t.Errorf("the sender BIC was not used as the instructing party\n%s", c.XML)
	}
}

func TestConvert104OptionLInstructingParty(t *testing.T) {
	raw := `{1:F01ACMEGB2LAXXX0000000000}{2:I104BANKGB2LXXXXN}{4:
:20:DD-1
:30:260901
:50L:ACME TREASURY DEPARTMENT
:50K:ACME UTILITIES LIMITED
:21:COLL-1
:32B:EUR1,00
:59:MUELLER GMBH
-}`
	c := convert(t, raw)
	if !strings.Contains(c.XML, "<Nm>ACME TREASURY DEPARTMENT</Nm>") {
		t.Errorf("the option-L instructing party was not used\n%s", c.XML)
	}
}

func TestConvert104EmptyInstructingParty(t *testing.T) {
	// An option-C field with nothing in it is not an instructing party.
	raw := `{1:F01ACMEGB2LAXXX0000000000}{2:I104BANKGB2LXXXXN}{4:
:20:DD-1
:30:260901
:50C:/
:50K:ACME UTILITIES LIMITED
:21:COLL-1
:32B:EUR1,00
:59:MUELLER GMBH
-}`
	c := convert(t, raw)
	if strings.Count(c.XML, "<Nm>ACME UTILITIES LIMITED</Nm>") != 2 {
		t.Errorf("the creditor did not stand in for the empty instructing party\n%s", c.XML)
	}
}

func TestConvert104CreditorWithoutAnOptionLetter(t *testing.T) {
	raw := `{1:F01ACMEGB2LAXXX0000000000}{2:I104BANKGB2LXXXXN}{4:
:20:DD-1
:30:260901
:50:ACME UTILITIES LIMITED
:21:COLL-1
:32B:EUR1,00
:59:MUELLER GMBH
-}`
	c := convert(t, raw)
	if !strings.Contains(c.XML, "<Nm>ACME UTILITIES LIMITED</Nm>") {
		t.Errorf("a creditor with no option letter was dropped\n%s", c.XML)
	}
}

func TestConvert104BadChargeBearer(t *testing.T) {
	raw := `{1:F01ACMEGB2LAXXX0000000000}{2:I104BANKGB2LXXXXN}{4:
:20:DD-1
:30:260901
:50K:ACME
:71A:XXX
:21:COLL-1
:32B:EUR1,00
:59:MUELLER
-}`
	c := convert(t, raw)
	if got := fidelityOf(t, c, "71A").Fidelity; got != FidelityUnmapped {
		t.Errorf(":71A: fidelity = %q, want unmapped", got)
	}
	// ChrgBr is optional in pain.008, so an unrecognised code is left out.
	if strings.Contains(c.XML, "<ChrgBr>") {
		t.Errorf("an unrecognised charge bearer was emitted\n%s", c.XML)
	}
}

func TestConvert104LongRemittanceAndMandate(t *testing.T) {
	long := strings.Repeat("B", 200)
	raw := `{1:F01ACMEGB2LAXXX0000000000}{2:I104BANKGB2LXXXXN}{4:
:20:DD-1
:30:260901
:50K:ACME
:21:COLL-1
:21C:` + strings.Repeat("M", 60) + `
:32B:EUR1,00
:59:MUELLER
:70:` + long + `
-}`
	c := convert(t, raw)
	if strings.Contains(c.XML, long) {
		t.Error("a 200-character remittance was emitted into a Max140Text element")
	}
	if got := fidelityOf(t, c, "70").Fidelity; got != FidelityTruncated {
		t.Errorf(":70: fidelity = %q, want truncated", got)
	}
	// A mandate identifier longer than Max35Text is trimmed rather than
	// producing an invalid document.
	if strings.Contains(c.XML, strings.Repeat("M", 40)) {
		t.Error("an over-long mandate identifier was emitted")
	}
}

func TestConvert104Rejects(t *testing.T) {
	base := "{1:F01ACMEGB2LAXXX0000000000}{2:I104BANKGB2LXXXXN}{4:\n:20:DD-1\n"
	for name, body := range map[string]string{
		"no transactions": ":30:260901\n:50K:ACME\n",
		"no date":         ":50K:ACME\n:21:C-1\n:32B:EUR1,00\n:59:M\n",
		"bad date":        ":30:269999\n:50K:ACME\n:21:C-1\n:32B:EUR1,00\n:59:M\n",
		"no amount":       ":30:260901\n:50K:ACME\n:21:C-1\n:59:M\n",
		"bad amount":      ":30:260901\n:50K:ACME\n:21:C-1\n:32B:NONSENSE\n:59:M\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Convert(mustParse(t, base+body+"-}")); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// MT204
// ---------------------------------------------------------------------------

const mt204 = `{1:F01CHASGB2LAXXX0000000000}{2:I204BANKGB2LXXXXN}{4:
:20:FMDD-20260824
:19:250000,00
:30:260826
:58A:BANKGB2LXXX
:20:TXN-001
:21:REL-001
:32B:EUR150000,00
:53A:DEUTDEFFXXX
:72:/ACC/MARGIN CALL
:20:TXN-002
:21:REL-002
:32B:EUR100000,00
:53A:BNPAFRPPXXX
-}`

func TestConvert204(t *testing.T) {
	c := convert(t, mt204)

	if c.SourceType != "204" || c.TargetType != "pacs.010.001.06" {
		t.Errorf("got %s -> %s", c.SourceType, c.TargetType)
	}

	for _, want := range []string{
		`<MsgId>FMDD-20260824</MsgId>`,
		`<NbOfTxs>2</NbOfTxs>`,
		`<CdtId>FMDD-20260824</CdtId>`,
		`<IntrBkSttlmDt>2026-08-26</IntrBkSttlmDt>`,
		`<BICFI>BANKGB2LXXX</BICFI>`,
		`<EndToEndId>REL-001</EndToEndId>`,
		`<IntrBkSttlmAmt Ccy="EUR">150000.00</IntrBkSttlmAmt>`,
		`<BICFI>DEUTDEFFXXX</BICFI>`,
		`<BICFI>BNPAFRPPXXX</BICFI>`,
	} {
		if !strings.Contains(c.XML, want) {
			t.Errorf("output is missing %s\n%s", want, c.XML)
		}
	}

	// Every party in pacs.010 is an institution, never a customer.
	if strings.Contains(c.XML, "<Nm>") {
		t.Errorf("a pacs.010 party was emitted with a name rather than a BICFI\n%s", c.XML)
	}
	if n := strings.Count(c.XML, "<DrctDbtTxInf>"); n != 2 {
		t.Errorf("transaction count = %d, want 2", n)
	}

	// The sum of amounts is derivable, so it is reported rather than carried.
	if got := fidelityOf(t, c, "19").Fidelity; got != FidelityUnmapped {
		t.Errorf(":19: fidelity = %q, want unmapped", got)
	}
}

func TestConvert204SplitsAfterTheFirstReference(t *testing.T) {
	// Sequence A carries a :20: of its own. Splitting on the first would put
	// the message reference into the first transaction.
	c := convert(t, mt204)
	if !strings.Contains(c.XML, "<MsgId>FMDD-20260824</MsgId>") {
		t.Errorf("the message reference was consumed by a transaction\n%s", c.XML)
	}
	if strings.Contains(c.XML, "<EndToEndId>FMDD-20260824</EndToEndId>") {
		t.Errorf("the message reference became a transaction reference\n%s", c.XML)
	}
}

func TestConvert204WithoutARelatedReference(t *testing.T) {
	raw := `{1:F01CHASGB2LAXXX0000000000}{2:I204BANKGB2LXXXXN}{4:
:20:FMDD-1
:58A:BANKGB2LXXX
:20:TXN-001
:32B:EUR1,00
:53A:DEUTDEFFXXX
-}`
	c := convert(t, raw)
	// Without :21: the transaction's own reference is the end-to-end
	// identification.
	if !strings.Contains(c.XML, "<EndToEndId>TXN-001</EndToEndId>") {
		t.Errorf("the transaction reference was not used\n%s", c.XML)
	}
	// And with no settlement date the element is left out rather than guessed.
	if strings.Contains(c.XML, "<IntrBkSttlmDt>") {
		t.Errorf("a settlement date was invented\n%s", c.XML)
	}
}

func TestConvert204FallsBackToTheAccountWithInstitution(t *testing.T) {
	raw := `{1:F01CHASGB2LAXXX0000000000}{2:I204BANKGB2LXXXXN}{4:
:20:FMDD-1
:57A:MIDLGB22XXX
:20:TXN-001
:32B:EUR1,00
:53A:DEUTDEFFXXX
-}`
	c := convert(t, raw)
	if !strings.Contains(c.XML, "<BICFI>MIDLGB22XXX</BICFI>") {
		t.Errorf(":57a: was not used when :58a: was absent\n%s", c.XML)
	}
}

func TestConvert204Rejects(t *testing.T) {
	for name, raw := range map[string]string{
		"no transactions": "{1:F01CHASGB2LAXXX0000000000}{2:I204BANKGB2LXXXXN}{4:\n:20:FMDD-1\n:58A:BANKGB2LXXX\n-}",
		"bad date":        "{1:F01CHASGB2LAXXX0000000000}{2:I204BANKGB2LXXXXN}{4:\n:20:F-1\n:30:269999\n:20:T-1\n:32B:EUR1,00\n-}",
		"no amount":       "{1:F01CHASGB2LAXXX0000000000}{2:I204BANKGB2LXXXXN}{4:\n:20:F-1\n:20:T-1\n:53A:DEUTDEFFXXX\n-}",
		"bad amount":      "{1:F01CHASGB2LAXXX0000000000}{2:I204BANKGB2LXXXXN}{4:\n:20:F-1\n:20:T-1\n:32B:NONSENSE\n-}",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Convert(mustParse(t, raw)); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestSplitAfter(t *testing.T) {
	m := mustParse(t, mt204)

	head, groups := m.SplitAfter("20", 1)
	if len(groups) != 2 {
		t.Fatalf("got %d groups, want 2", len(groups))
	}
	if _, ok := FieldsOf(head, "19"); !ok {
		t.Error("the sum of amounts is missing from sequence A")
	}
	if _, ok := FieldsOf(head, "20"); !ok {
		t.Error("the message reference is missing from sequence A")
	}

	// Skipping more occurrences than exist leaves everything in the head.
	h, g := m.SplitAfter("20", 99)
	if len(g) != 0 || len(h) != len(m.Fields) {
		t.Errorf("SplitAfter with a large skip = %d head, %d groups", len(h), len(g))
	}
}

func TestSupportedCoversTheDirectDebits(t *testing.T) {
	got := Supported()
	for _, want := range []string{"104", "107", "204"} {
		found := false
		for _, g := range got {
			if g == want {
				found = true
			}
		}
		if !found {
			t.Errorf("MT%s is missing from %v", want, got)
		}
	}
}

func TestConvert104TransactionOverrides(t *testing.T) {
	// A transaction may name its own creditor and its own bank, which then
	// apply to that collection only.
	raw := `{1:F01ACMEGB2LAXXX0000000000}{2:I104BANKGB2LXXXXN}{4:
:20:DD-1
:30:260901
:50K:ACME UTILITIES LIMITED
:52A:BANKGB2LXXX
:21:COLL-1
:32B:EUR1,00
:59:MUELLER
:21:COLL-2
:50K:ACME TELECOM LIMITED
:52A:MIDLGB22XXX
:32B:EUR2,00
:59:SCHMIDT
-}`
	c := convert(t, raw)

	if !strings.Contains(c.XML, "<Nm>ACME TELECOM LIMITED</Nm>") {
		t.Errorf("the transaction-level creditor was ignored\n%s", c.XML)
	}
	if !strings.Contains(c.XML, "<BICFI>MIDLGB22XXX</BICFI>") {
		t.Errorf("the transaction-level creditor agent was ignored\n%s", c.XML)
	}
	if !strings.Contains(c.XML, "<Nm>ACME UTILITIES LIMITED</Nm>") {
		t.Errorf("the sequence A creditor was not used for the first collection\n%s", c.XML)
	}
	if got := fidelityOf(t, c, "52A (txn 2)").Fidelity; got != FidelityMapped {
		t.Errorf(":52A (txn 2): fidelity = %q, want mapped", got)
	}
}

func TestConvert104ChargesAccountWithoutASlash(t *testing.T) {
	raw := `{1:F01ACMEGB2LAXXX0000000000}{2:I104BANKGB2LXXXXN}{4:
:20:DD-1
:30:260901
:50K:ACME
:21:COLL-1
:32B:EUR1,00
:25A:CHARGES-ACCOUNT
:59:MUELLER
-}`
	c := convert(t, raw)
	if !strings.Contains(c.XML, "<Id>CHARGES-ACCOUNT</Id>") {
		t.Errorf("a charges account without a leading slash was dropped\n%s", c.XML)
	}
}

func TestConvert204WithoutAHeaderBIC(t *testing.T) {
	// InstgAgt is optional. A truncated header leaves it out rather than
	// naming an institution that does not exist.
	raw := "{1:F01}{2:I204}{4:\n:20:FMDD-1\n:20:TXN-1\n:32B:EUR1,00\n-}"
	c := convert(t, raw)

	if strings.Contains(c.XML, "<InstgAgt>") {
		t.Errorf("an instructing agent was invented from an empty header\n%s", c.XML)
	}
	// Cdtr and Dbtr are mandatory, so those get placeholders.
	if strings.Count(c.XML, "<BICFI>NOTPROVIDED</BICFI>") < 2 {
		t.Errorf("the mandatory institutions have no placeholders\n%s", c.XML)
	}
}

func TestHeaderBIC(t *testing.T) {
	if got := headerBIC(""); got != "" {
		t.Errorf("headerBIC(\"\") = %q", got)
	}
	if got := headerBIC("  "); got != "" {
		t.Errorf("headerBIC(spaces) = %q", got)
	}
	if got := headerBIC("BANKGB2LXXX"); got != "BANKGB2LXXX" {
		t.Errorf("headerBIC = %q", got)
	}
}
