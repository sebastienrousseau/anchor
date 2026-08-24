// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package swift

import (
	"encoding/xml"
	"regexp"
	"strings"
	"testing"
)

// convert parses and converts, failing the test on either error.
func convert(t *testing.T, raw string) *Conversion {
	t.Helper()
	c, err := Convert(mustParse(t, raw))
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	// Every conversion must at least be well-formed XML; schema conformance is
	// checked separately against the user's catalogue.
	if err := xml.Unmarshal([]byte(c.XML), new(struct{})); err != nil {
		t.Fatalf("generated XML is not well-formed: %v\n%s", err, c.XML)
	}
	return c
}

// fidelityOf finds the report entry for a tag.
func fidelityOf(t *testing.T, c *Conversion, tag string) FieldReport {
	t.Helper()
	for _, r := range c.Report {
		if r.Tag == tag {
			return r
		}
	}
	t.Fatalf("no report entry for %q; report is %+v", tag, c.Report)
	return FieldReport{}
}

func TestConvert103(t *testing.T) {
	c := convert(t, mt103)

	if c.SourceType != "103" || c.TargetType != "pacs.008.001.10" {
		t.Errorf("got %s -> %s", c.SourceType, c.TargetType)
	}

	for _, want := range []string{
		`<MsgId>REF20260824001</MsgId>`,
		`<UETR>f3a1b2c4-5d6e-4f70-8a91-2b3c4d5e6f70</UETR>`,
		`<IntrBkSttlmAmt Ccy="EUR">25000.00</IntrBkSttlmAmt>`,
		`<IntrBkSttlmDt>2026-08-24</IntrBkSttlmDt>`,
		`<ChrgBr>SHAR</ChrgBr>`,
		`<Nm>ACME TRADING LIMITED</Nm>`,
		`<IBAN>GB29NWBK60161331926819</IBAN>`,
		`<IBAN>DE89370400440532013000</IBAN>`,
		`<BICFI>BANKGB2LXXX</BICFI>`,
		`<BICFI>BANKDEFFXXX</BICFI>`,
		`<Ustrd>INVOICE 2026-0815 CONSULTING SERVICES</Ustrd>`,
	} {
		if !strings.Contains(c.XML, want) {
			t.Errorf("output is missing %s\n%s", want, c.XML)
		}
	}
}

func TestConvert103ReportCoversEveryField(t *testing.T) {
	m := mustParse(t, mt103)
	c, err := Convert(m)
	if err != nil {
		t.Fatal(err)
	}

	// Nothing may vanish: every source field has to appear in the report.
	for _, f := range m.Fields {
		found := false
		for _, r := range c.Report {
			if strings.HasPrefix(r.Tag, f.Name()) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("field :%s: is absent from the report", f.Name())
		}
	}

	// An MT103 carrying addresses can never be lossless, because those addresses
	// are unstructured and CBPR+ stops accepting them on 14 November 2026.
	if c.Lossless() {
		t.Error("Lossless() = true for a message with unstructured addresses")
	}
	if got := fidelityOf(t, c, "23B").Fidelity; got != FidelityUnmapped {
		t.Errorf(":23B: fidelity = %q, want unmapped", got)
	}
	if n := len(c.Unmapped()); n != 1 {
		t.Errorf("Unmapped() = %d entries, want 1: %+v", n, c.Unmapped())
	}
	counts := c.Counts()
	if counts[FidelityMapped] == 0 || counts[FidelityTruncated] != 2 {
		t.Errorf("counts = %v", counts)
	}
}

func TestConvert103AddressFlagsTheDeadline(t *testing.T) {
	c := convert(t, mt103)
	r := fidelityOf(t, c, "50K (address)")

	if r.Fidelity != FidelityTruncated {
		t.Errorf("fidelity = %q", r.Fidelity)
	}
	if !strings.Contains(r.Note, "14 November 2026") {
		t.Errorf("note does not name the deadline: %q", r.Note)
	}
	if !strings.Contains(c.XML, "<AdrLine>14 GRESHAM STREET</AdrLine>") {
		t.Error("the address lines were not carried")
	}
}

var uuidV4 = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestConvert103GeneratesUETRWhenAbsent(t *testing.T) {
	raw := strings.Replace(mt103, "{3:{108:MT103REF}{121:f3a1b2c4-5d6e-4f70-8a91-2b3c4d5e6f70}}", "", 1)
	c := convert(t, raw)

	r := fidelityOf(t, c, "121")
	if r.Fidelity != FidelityDerived {
		t.Errorf("fidelity = %q, want derived", r.Fidelity)
	}

	// The generated value has to be a real UUIDv4, because the linter checks it.
	start := strings.Index(c.XML, "<UETR>") + len("<UETR>")
	got := c.XML[start : start+36]
	if !uuidV4.MatchString(got) {
		t.Errorf("generated UETR %q is not an RFC 4122 v4 UUID", got)
	}
}

func TestGenerateUETRIsDistinct(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 64; i++ {
		u := generateUETR()
		if !uuidV4.MatchString(u) {
			t.Fatalf("generateUETR() = %q", u)
		}
		if seen[u] {
			t.Fatalf("generateUETR() repeated %q", u)
		}
		seen[u] = true
	}
}

func TestConvert103OptionalFields(t *testing.T) {
	raw := `{1:F01BANKGB2LAXXX0000000000}{2:I103BANKDEFFXXXXN}{4:
:20:REF1
:23E:PHON/CALL TREASURY
:32A:260824EUR24950,00
:33B:GBP21000,00
:36:1,1880
:50K:/GB29NWBK60161331926819
ACME TRADING LIMITED
:59:/DE89370400440532013000
MUELLER GMBH
:71A:OUR
:71F:EUR25,00
:71G:EUR25,00
-}`
	c := convert(t, raw)

	for _, want := range []string{
		`<InstdAmt Ccy="GBP">21000.00</InstdAmt>`,
		`<XchgRate>1.1880</XchgRate>`,
		`<ChrgBr>DEBT</ChrgBr>`,
		`<Amt Ccy="EUR">25.00</Amt>`,
		`<Cd>PHOA</Cd>`,
		`<InstrInf>CALL TREASURY</InstrInf>`,
	} {
		if !strings.Contains(c.XML, want) {
			t.Errorf("output is missing %s\n%s", want, c.XML)
		}
	}

	// Both charge fields become their own ChrgsInf, each naming the agent that
	// took the charge.
	if n := strings.Count(c.XML, "<ChrgsInf>"); n != 2 {
		t.Errorf("ChrgsInf count = %d, want 2", n)
	}
	// With no :52A:/:57A: the agents fall back to the header BICs.
	if !strings.Contains(c.XML, "<BICFI>BANKGB2LXXX</BICFI>") {
		t.Error("the debtor agent did not fall back to the sender BIC")
	}
	if got := fidelityOf(t, c, "52").Fidelity; got != FidelityDerived {
		t.Errorf("missing :52A: fidelity = %q, want derived", got)
	}
}

func TestConvert103BadOptionalFields(t *testing.T) {
	raw := `{1:F01BANKGB2LAXXX0000000000}{2:I103BANKDEFFXXXXN}{4:
:20:REF1
:23E:SDVA
:32A:260824EUR1,00
:33B:NOT AN AMOUNT
:50K:ACME
:59:MUELLER
:71A:XXX
-}`
	c := convert(t, raw)

	if got := fidelityOf(t, c, "33B").Fidelity; got != FidelityUnmapped {
		t.Errorf(":33B: fidelity = %q, want unmapped", got)
	}
	if got := fidelityOf(t, c, "71A").Fidelity; got != FidelityUnmapped {
		t.Errorf(":71A: fidelity = %q, want unmapped for an unknown code", got)
	}
	// An unrecognised charge bearer must not stop the conversion; SHAR is the
	// documented default.
	if !strings.Contains(c.XML, "<ChrgBr>SHAR</ChrgBr>") {
		t.Error("the charge bearer did not fall back to SHAR")
	}
	// SDVA has no Instruction4Code equivalent, so it survives as free text only.
	r := fidelityOf(t, c, "23E")
	if r.Fidelity != FidelityTruncated || strings.Contains(c.XML, "<Cd>") {
		t.Errorf(":23E: = %+v; XML should carry no Cd", r)
	}
	if !strings.Contains(c.XML, "<InstrInf>SDVA</InstrInf>") {
		t.Error("the instruction narrative was dropped")
	}
}

func TestConvert103LongInstruction(t *testing.T) {
	long := strings.Repeat("A", 200)
	raw := `{1:F01BANKGB2LAXXX0000000000}{2:I103BANKDEFFXXXXN}{4:
:20:REF1
:23E:` + long + `
:32A:260824EUR1,00
:50K:ACME
:59:MUELLER
-}`
	c := convert(t, raw)
	r := fidelityOf(t, c, "23E")
	if !strings.Contains(r.Note, "140 characters") {
		t.Errorf("note = %q", r.Note)
	}
	if strings.Contains(c.XML, long) {
		t.Error("a 200-character instruction was emitted into a Max140Text element")
	}
}

func TestConvert103TruncatesLongText(t *testing.T) {
	longName := strings.Repeat("A", 200)
	longRemit := strings.Repeat("B", 200)
	raw := `{1:F01BANKGB2LAXXX0000000000}{2:I103BANKDEFFXXXXN}{4:
:20:REF1
:32A:260824EUR1,00
:50K:` + longName + `
:59:MUELLER
:70:` + longRemit + `
-}`
	c := convert(t, raw)

	if strings.Contains(c.XML, longName) {
		t.Error("a 200-character name was emitted into a Max140Text element")
	}
	if r := fidelityOf(t, c, "50K"); r.Fidelity != FidelityTruncated {
		t.Errorf(":50K: fidelity = %q, want truncated", r.Fidelity)
	}
	if r := fidelityOf(t, c, "70"); r.Fidelity != FidelityTruncated {
		t.Errorf(":70: fidelity = %q, want truncated", r.Fidelity)
	}
}

func TestConvert103PartyOptionA(t *testing.T) {
	raw := `{1:F01BANKGB2LAXXX0000000000}{2:I103BANKDEFFXXXXN}{4:
:20:REF1
:32A:260824EUR1,00
:50A:/GB29NWBK60161331926819
ACMEGB2LXXX
:59:MUELLER
-}`
	c := convert(t, raw)

	// An option-A party identifies by BIC, which belongs in Id/OrgId/AnyBIC.
	if !strings.Contains(c.XML, "<AnyBIC>ACMEGB2LXXX</AnyBIC>") {
		t.Errorf("the option-A BIC was dropped\n%s", c.XML)
	}
}

func TestConvert103MissingParties(t *testing.T) {
	raw := `{1:F01BANKGB2LAXXX0000000000}{2:I103BANKDEFFXXXXN}{4:
:20:REF1
:32A:260824EUR1,00
:50K:/GB29NWBK60161331926819
-}`
	c := convert(t, raw)

	// Dbtr and Cdtr are mandatory in pacs.008; a placeholder keeps the document
	// schema-valid while the report says the name was never supplied.
	if strings.Count(c.XML, "<Nm>NOT PROVIDED</Nm>") != 2 {
		t.Errorf("expected two placeholder names\n%s", c.XML)
	}
	if got := fidelityOf(t, c, "59").Fidelity; got != FidelityDerived {
		t.Errorf(":59: fidelity = %q, want derived", got)
	}
	if got := fidelityOf(t, c, "50K").Fidelity; got != FidelityDerived {
		t.Errorf(":50K: fidelity = %q, want derived", got)
	}
	// A non-IBAN account must not be emitted as an IBAN.
	if !strings.Contains(c.XML, "<IBAN>GB29NWBK60161331926819</IBAN>") {
		t.Error("a valid IBAN was not recognised")
	}
}

func TestConvert103NonIBANAccount(t *testing.T) {
	raw := `{1:F01BANKGB2LAXXX0000000000}{2:I103BANKDEFFXXXXN}{4:
:20:REF1
:32A:260824USD1,00
:50K:/123456789
ACME
:59:/987654321
MUELLER
-}`
	c := convert(t, raw)
	if strings.Contains(c.XML, "<IBAN>") {
		t.Error("a domestic account number was emitted as an IBAN")
	}
	if !strings.Contains(c.XML, "<Othr>") {
		t.Error("a domestic account number was dropped")
	}
}

func TestConvert103EscapesText(t *testing.T) {
	raw := `{1:F01BANKGB2LAXXX0000000000}{2:I103BANKDEFFXXXXN}{4:
:20:REF1
:32A:260824EUR1,00
:50K:SMITH & SONS <LTD>
:59:MUELLER
-}`
	c := convert(t, raw)
	if strings.Contains(c.XML, "SMITH & SONS") {
		t.Error("an ampersand was emitted unescaped")
	}
	if !strings.Contains(c.XML, "SMITH &amp; SONS &lt;LTD&gt;") {
		t.Errorf("escaping is wrong\n%s", c.XML)
	}
}

func TestConvert103Rejects(t *testing.T) {
	cases := map[string]string{
		"no amount":  "{1:F01BANKGB2LAXXX0000000000}{2:I103BANKDEFFXXXXN}{4:\n:20:REF1\n-}",
		"bad amount": "{1:F01BANKGB2LAXXX0000000000}{2:I103BANKDEFFXXXXN}{4:\n:20:REF1\n:32A:NONSENSE\n-}",
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Convert(mustParse(t, raw)); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

const mt202 = `{1:F01BANKGB2LAXXX0000000000}{2:I202BANKDEFFXXXXN}{4:
:20:COVER20260824
:21:REF20260824001
:32A:260824EUR25000,00
:52A:BANKGB2LXXX
:53A:CHASGB2LXXX
:57A:BANKDEFFXXX
:58A:DEUTDEFFXXX
-}`

func TestConvert202(t *testing.T) {
	c := convert(t, mt202)

	if c.TargetType != "pacs.009.001.10" {
		t.Errorf("TargetType = %q", c.TargetType)
	}
	for _, want := range []string{
		`<MsgId>COVER20260824</MsgId>`,
		`<EndToEndId>REF20260824001</EndToEndId>`,
		`<BICFI>CHASGB2LXXX</BICFI>`,
		`<BICFI>DEUTDEFFXXX</BICFI>`,
	} {
		if !strings.Contains(c.XML, want) {
			t.Errorf("output is missing %s\n%s", want, c.XML)
		}
	}
	// In pacs.009 the debtor and creditor are institutions, never customers.
	if strings.Contains(c.XML, "<Nm>") {
		t.Error("a pacs.009 party was emitted with a name rather than a BICFI")
	}
	// A UETR is generated because MT202 without block 3 carries none.
	if got := fidelityOf(t, c, "121").Fidelity; got != FidelityDerived {
		t.Errorf(":121: fidelity = %q, want derived", got)
	}
}

func TestConvert202FallsBackToHeaders(t *testing.T) {
	raw := `{1:F01BANKGB2LAXXX0000000000}{2:I202BANKDEFFXXXXN}{4:
:20:COVER1
:32A:260824EUR1,00
-}`
	c := convert(t, raw)

	// With no agent fields at all the header BICs stand in, and the end-to-end
	// reference falls back to the transaction reference.
	if !strings.Contains(c.XML, "<EndToEndId>COVER1</EndToEndId>") {
		t.Error("EndToEndId did not fall back to :20:")
	}
	if strings.Count(c.XML, "<BICFI>BANKGB2LXXX</BICFI>") != 2 {
		t.Errorf("sender BIC was not used for both sending agents\n%s", c.XML)
	}
}

func TestConvert202OptionDAgent(t *testing.T) {
	raw := `{1:F01BANKGB2LAXXX0000000000}{2:I202BANKDEFFXXXXN}{4:
:20:COVER1
:32A:260824EUR1,00
:52D:SOME BANK PLC
1 HIGH STREET
-}`
	c := convert(t, raw)

	// Option D is a name and address, which cannot become a BICFI.
	r := fidelityOf(t, c, "52D")
	if r.Fidelity != FidelityTruncated {
		t.Errorf(":52D: fidelity = %q, want truncated", r.Fidelity)
	}
	if !strings.Contains(r.Note, "not a BIC") {
		t.Errorf("note = %q", r.Note)
	}
}

func TestConvert202EmptyAgentField(t *testing.T) {
	raw := `{1:F01BANKGB2LAXXX0000000000}{2:I202BANKDEFFXXXXN}{4:
:20:COVER1
:32A:260824EUR1,00
:52A:/
-}`
	c := convert(t, raw)
	if got := fidelityOf(t, c, "52A").Fidelity; got != FidelityDerived {
		t.Errorf(":52A: fidelity = %q, want derived", got)
	}
}

func TestConvert202Rejects(t *testing.T) {
	for name, raw := range map[string]string{
		"no amount":  "{1:F01BANKGB2LAXXX0000000000}{2:I202BANKDEFFXXXXN}{4:\n:20:REF1\n-}",
		"bad amount": "{1:F01BANKGB2LAXXX0000000000}{2:I202BANKDEFFXXXXN}{4:\n:20:REF1\n:32A:NONSENSE\n-}",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Convert(mustParse(t, raw)); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

const mt940 = `{1:F01BANKGB2LAXXX0000000000}{2:I940BANKDEFFXXXXN}{4:
:20:STMT20260824
:25:GB29NWBK60161331926819
:28C:00123/001
:60F:C260823EUR100000,00
:61:2608240824C25000,00NTRFREF20260824001//BANKREF
:86:INVOICE 2026-0815
:62F:D260824EUR125000,00
-}`

func TestConvert940(t *testing.T) {
	c := convert(t, mt940)

	if c.TargetType != "camt.053.001.11" {
		t.Errorf("TargetType = %q", c.TargetType)
	}
	for _, want := range []string{
		`<Cd>OPBD</Cd>`,
		`<Cd>CLBD</Cd>`,
		`<Amt Ccy="EUR">100000.00</Amt>`,
		`<CdtDbtInd>CRDT</CdtDbtInd>`,
		`<CdtDbtInd>DBIT</CdtDbtInd>`,
		`<ElctrncSeqNb>123</ElctrncSeqNb>`,
		`<IBAN>GB29NWBK60161331926819</IBAN>`,
	} {
		if !strings.Contains(c.XML, want) {
			t.Errorf("output is missing %s\n%s", want, c.XML)
		}
	}

	// The statement line becomes an entry, and the information that follows it
	// becomes that entry's.
	for _, want := range []string{
		"<Ntry>",
		"<NtryRef>REF20260824001</NtryRef>",
		`<Amt Ccy="EUR">25000.00</Amt>`,
		"<AcctSvcrRef>BANKREF</AcctSvcrRef>",
		"<Cd>NTRF</Cd>",
		"<AddtlNtryInf>INVOICE 2026-0815</AddtlNtryInf>",
	} {
		if !strings.Contains(c.XML, want) {
			t.Errorf("the entry is missing %s\n%s", want, c.XML)
		}
	}

	// The entries are reported once, not once per line.
	var entries int
	for _, r := range c.Report {
		if r.Tag == "61" {
			entries++
			if r.Fidelity != FidelityMapped {
				t.Errorf(":61: fidelity = %q, want mapped", r.Fidelity)
			}
		}
	}
	if entries != 1 {
		t.Errorf(":61: appears %d times in the report, want 1", entries)
	}
	// The sequence number after the slash has no equivalent.
	if got := fidelityOf(t, c, "28C").Fidelity; got != FidelityTruncated {
		t.Errorf(":28C: fidelity = %q, want truncated", got)
	}
}

func TestConvert940StatementNumbers(t *testing.T) {
	cases := []struct {
		name, value, want string
	}{
		{"plain", ":28C:42", "<ElctrncSeqNb>42</ElctrncSeqNb>"},
		{"non numeric", ":28C:ABC/001", ""},
		{"too long", ":28C:1234567890123456789", ""},
		{"all zeroes", ":28C:0000", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := `{1:F01BANKGB2LAXXX0000000000}{2:I940BANKDEFFXXXXN}{4:
:20:STMT1
:25:GB29NWBK60161331926819
` + tc.value + `
:60F:C260823EUR1,00
:62F:C260824EUR1,00
-}`
			c := convert(t, raw)
			if tc.want == "" {
				if strings.Contains(c.XML, "<ElctrncSeqNb>") {
					t.Errorf("%q was emitted as a sequence number", tc.value)
				}
				if got := fidelityOf(t, c, "28C").Fidelity; got != FidelityUnmapped {
					t.Errorf("fidelity = %q, want unmapped", got)
				}
				return
			}
			if !strings.Contains(c.XML, tc.want) {
				t.Errorf("output is missing %s", tc.want)
			}
			if got := fidelityOf(t, c, "28C").Fidelity; got != FidelityMapped {
				t.Errorf("fidelity = %q, want mapped", got)
			}
		})
	}
}

func TestConvert940Rejects(t *testing.T) {
	base := "{1:F01BANKGB2LAXXX0000000000}{2:I940BANKDEFFXXXXN}{4:\n:20:STMT1\n"
	for name, body := range map[string]string{
		"no account":       ":60F:C260823EUR1,00\n:62F:C260824EUR1,00\n",
		"no opening":       ":25:GB29NWBK60161331926819\n:62F:C260824EUR1,00\n",
		"no closing":       ":25:GB29NWBK60161331926819\n:60F:C260823EUR1,00\n",
		"short balance":    ":25:GB29NWBK60161331926819\n:60F:C26\n:62F:C260824EUR1,00\n",
		"unparsed balance": ":25:GB29NWBK60161331926819\n:60F:CNONSENSE1\n:62F:C260824EUR1,00\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Convert(mustParse(t, base+body+"-}"))
			if err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestConvertUnsupportedType(t *testing.T) {
	raw := "{1:F01BANKGB2LAXXX0000000000}{2:I700BANKDEFFXXXXN}{4:\n:20:REF1\n-}"
	_, err := Convert(mustParse(t, raw))
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "MT700") {
		t.Errorf("error = %q, want it to name the message type", err)
	}
}

func TestLosslessConversion(t *testing.T) {
	// A message with no addresses and no unmappable fields converts intact.
	raw := `{1:F01BANKGB2LAXXX0000000000}{2:I202BANKDEFFXXXXN}{3:{121:f3a1b2c4-5d6e-4f70-8a91-2b3c4d5e6f70}}{4:
:20:COVER1
:21:REF1
:32A:260824EUR1,00
:52A:BANKGB2LXXX
:53A:CHASGB2LXXX
:57A:BANKDEFFXXX
:58A:DEUTDEFFXXX
-}`
	c := convert(t, raw)
	if !c.Lossless() {
		t.Errorf("Lossless() = false; report is %+v", c.Report)
	}
	if c.Unmapped() != nil {
		t.Errorf("Unmapped() = %+v, want nil", c.Unmapped())
	}
}

func TestNormaliseBIC(t *testing.T) {
	for in, want := range map[string]string{
		"bankgb2l":           "BANKGB2L",
		" BANKGB2LXXX ":      "BANKGB2LXXX",
		"BANKGB2L SOME BANK": "BANKGB2L",
		"":                   "NOTPROVIDED",
	} {
		if got := normaliseBIC(in); got != want {
			t.Errorf("normaliseBIC(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTruncate(t *testing.T) {
	// Truncation counts runes, not bytes: a name of accented characters must not
	// be cut mid-character.
	s := strings.Repeat("é", 10)
	got, cut := truncate(s, 5)
	if !cut || len([]rune(got)) != 5 {
		t.Errorf("truncate = %q, %v", got, cut)
	}
	if got, cut := truncate("short", 10); cut || got != "short" {
		t.Errorf("truncate = %q, %v", got, cut)
	}
}

func TestLooksLikeIBAN(t *testing.T) {
	for in, want := range map[string]bool{
		"GB29NWBK60161331926819":      true,
		"gb29 nwbk 6016 1331 9268 19": true,
		"123456789":                   false,
		"GBXXNWBK60161331926819":      false,
		"GB29NWBK6016133192681!":      false,
		"GB29NWBK":                    false,
		strings.Repeat("A", 40):       false,
	} {
		if got := looksLikeIBAN(in); got != want {
			t.Errorf("looksLikeIBAN(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestFirstLine(t *testing.T) {
	if got := firstLine("one\ntwo"); got != "one" {
		t.Errorf("firstLine = %q", got)
	}
	if got := firstLine("only"); got != "only" {
		t.Errorf("firstLine = %q", got)
	}
}

func TestReportIsSorted(t *testing.T) {
	c := convert(t, mt103)
	for i := 1; i < len(c.Report); i++ {
		if c.Report[i-1].Tag > c.Report[i].Tag {
			t.Fatalf("report is unsorted at %d: %q then %q",
				i, c.Report[i-1].Tag, c.Report[i].Tag)
		}
	}
}

func TestConvert103EndToEndFromField21(t *testing.T) {
	raw := `{1:F01BANKGB2LAXXX0000000000}{2:I103BANKDEFFXXXXN}{4:
:20:REF1
:21:E2E-REFERENCE
:32A:260824EUR1,00
:50K:ACME
:59:MUELLER
-}`
	c := convert(t, raw)
	if !strings.Contains(c.XML, "<EndToEndId>E2E-REFERENCE</EndToEndId>") {
		t.Errorf(":21: was not used as the end-to-end identification\n%s", c.XML)
	}
	if got := fidelityOf(t, c, "21").Fidelity; got != FidelityMapped {
		t.Errorf(":21: fidelity = %q, want mapped", got)
	}
}

func TestConvertWithoutHeaderBICs(t *testing.T) {
	// A message whose headers are truncated has no BIC to fall back on, so the
	// agents become explicit placeholders rather than empty elements.
	raw := "{1:F01}{2:I103}{4:\n:20:REF1\n:32A:260824EUR1,00\n:50K:ACME\n:59:MUELLER\n-}"
	c := convert(t, raw)

	if strings.Count(c.XML, "<BICFI>NOTPROVIDED</BICFI>") != 2 {
		t.Errorf("expected placeholder agents\n%s", c.XML)
	}
	if r := fidelityOf(t, c, "52"); !strings.Contains(r.Note, "header") {
		t.Errorf(":52: note = %q", r.Note)
	}
}

func TestPartyAddressCapped(t *testing.T) {
	// PostalAddress permits seven address lines; a longer MT party must not
	// produce an invalid document.
	var lines []string
	for i := 0; i < 12; i++ {
		lines = append(lines, "LINE "+string(rune('A'+i)))
	}
	raw := `{1:F01BANKGB2LAXXX0000000000}{2:I103BANKDEFFXXXXN}{4:
:20:REF1
:32A:260824EUR1,00
:50K:ACME LIMITED
` + strings.Join(lines, "\n") + `
:59:MUELLER
-}`
	c := convert(t, raw)
	if n := strings.Count(c.XML, "<AdrLine>"); n != 7 {
		t.Errorf("AdrLine count = %d, want 7", n)
	}
}

func TestLooksLikeIBANRejectsDigitPrefix(t *testing.T) {
	if looksLikeIBAN("1B29NWBK60161331926819") {
		t.Error("an IBAN must start with two letters")
	}
}

func TestUnvisitedFieldsAreReported(t *testing.T) {
	// A field the converter never looks at must still appear in the report, so a
	// reader can see it was dropped rather than assume it was carried.
	raw := `{1:F01BANKGB2LAXXX0000000000}{2:I103BANKDEFFXXXXN}{4:
:20:REF1
:32A:260824EUR1,00
:50K:ACME
:59:MUELLER
:77B:/ORDERRES/GB//RESIDENT
-}`
	c := convert(t, raw)
	r := fidelityOf(t, c, "77B")
	if r.Fidelity != FidelityUnmapped {
		t.Errorf(":77B: fidelity = %q, want unmapped", r.Fidelity)
	}
	if r.Value != "/ORDERRES/GB//RESIDENT" {
		t.Errorf(":77B: value = %q", r.Value)
	}
	if !strings.Contains(r.Note, "no equivalent") {
		t.Errorf(":77B: note = %q", r.Note)
	}
}

// ---------------------------------------------------------------------------
// MT101 -> pain.001
// ---------------------------------------------------------------------------

const mt101 = `{1:F01ACMEGB2LAXXX0000000000}{2:I101BANKGB2LXXXXN}{4:
:20:REQ20260824001
:28D:1/1
:25:AUTH-CODE
:30:260826
:50H:/GB29NWBK60161331926819
ACME TRADING LIMITED
14 GRESHAM STREET
LONDON EC2V 7NN
:52A:BANKGB2LXXX
:21:PAY-001
:23E:URGP
:32B:EUR12500,00
:57A:BANKDEFFXXX
:59:/DE89370400440532013000
MUELLER GMBH
HAUPTSTRASSE 12
:70:INVOICE 2026-0815
:71A:SHA
:21:PAY-002
:21F:FX-4471
:32B:USD8000,00
:36:1,0840
:50H:/GB29NWBK60161331926819
ACME TREASURY LIMITED
:57A:CHASUS33XXX
:59:/US64SVBKUS6S3300958879
NORTHWIND INC
:70:INVOICE 2026-0816
:71A:OUR
:25A:/GB29NWBK60161331926819
-}`

func TestConvert101(t *testing.T) {
	c := convert(t, mt101)

	if c.SourceType != "101" || c.TargetType != "pain.001.001.09" {
		t.Errorf("got %s -> %s", c.SourceType, c.TargetType)
	}

	// Both transactions become their own PmtInf, because MT101 lets a
	// transaction override the ordering customer sequence A named.
	if n := strings.Count(c.XML, "<PmtInf>"); n != 2 {
		t.Errorf("PmtInf count = %d, want 2\n%s", n, c.XML)
	}
	for _, want := range []string{
		`<MsgId>REQ20260824001</MsgId>`,
		`<NbOfTxs>2</NbOfTxs>`,
		`<Dt>2026-08-26</Dt>`,
		`<PmtInfId>PAY-001</PmtInfId>`,
		`<PmtInfId>PAY-002</PmtInfId>`,
		`<InstrId>FX-4471</InstrId>`,
		`<InstdAmt Ccy="EUR">12500.00</InstdAmt>`,
		`<InstdAmt Ccy="USD">8000.00</InstdAmt>`,
		`<XchgRate>1.0840</XchgRate>`,
		`<ChrgBr>SHAR</ChrgBr>`,
		`<ChrgBr>DEBT</ChrgBr>`,
		`<Nm>ACME TREASURY LIMITED</Nm>`,
		`<Nm>NORTHWIND INC</Nm>`,
		`<InstrForDbtrAgt>URGP</InstrForDbtrAgt>`,
		`<Ustrd>INVOICE 2026-0816</Ustrd>`,
	} {
		if !strings.Contains(c.XML, want) {
			t.Errorf("output is missing %s\n%s", want, c.XML)
		}
	}
}

func TestConvert101ReportsBothTransactions(t *testing.T) {
	c := convert(t, mt101)

	// A field that appears in both sequences is reported once per occurrence,
	// with the later ones labelled, and never as dropped.
	for _, tag := range []string{"32B", "32B (txn 2)", "59", "59 (txn 2)", "25A (txn 2)"} {
		r := fidelityOf(t, c, tag)
		if r.Fidelity == FidelityUnmapped {
			t.Errorf("%s reported as unmapped: %+v", tag, r)
		}
	}

	// A field handled only in the second transaction must not also be reported
	// as having no equivalent.
	for _, r := range c.Report {
		if (r.Tag == "25A" || r.Tag == "36" || r.Tag == "21F") && r.Fidelity == FidelityUnmapped {
			t.Errorf("%s was reported as dropped although the second transaction used it", r.Tag)
		}
	}

	// MT chaining and the authorisation field genuinely have no equivalent.
	if got := fidelityOf(t, c, "28D").Fidelity; got != FidelityUnmapped {
		t.Errorf(":28D: fidelity = %q, want unmapped", got)
	}
	if got := fidelityOf(t, c, "25").Fidelity; got != FidelityUnmapped {
		t.Errorf(":25: fidelity = %q, want unmapped", got)
	}
}

func TestConvert101SequenceADefaults(t *testing.T) {
	// A transaction that names no ordering customer inherits the one from
	// sequence A.
	raw := `{1:F01ACMEGB2LAXXX0000000000}{2:I101BANKGB2LXXXXN}{4:
:20:REQ1
:30:260826
:50H:/GB29NWBK60161331926819
ACME TRADING LIMITED
:52A:BANKGB2LXXX
:21:PAY-001
:32B:EUR1,00
:59:MUELLER GMBH
-}`
	c := convert(t, raw)
	if strings.Count(c.XML, "<Nm>ACME TRADING LIMITED</Nm>") != 2 {
		t.Errorf("the sequence A debtor was not inherited (InitgPty and Dbtr)\n%s", c.XML)
	}
	if !strings.Contains(c.XML, "<BICFI>BANKGB2LXXX</BICFI>") {
		t.Error("the sequence A account servicing institution was not inherited")
	}
}

func TestConvert101WithoutInstructingParty(t *testing.T) {
	raw := `{1:F01ACMEGB2LAXXX0000000000}{2:I101BANKGB2LXXXXN}{4:
:20:REQ1
:30:260826
:21:PAY-001
:32B:EUR1,00
:59:MUELLER GMBH
-}`
	c := convert(t, raw)

	// InitgPty is mandatory; with nothing to name, the sender BIC stands in and
	// the report says so.
	if !strings.Contains(c.XML, "<Nm>ACMEGB2LXXX</Nm>") {
		t.Errorf("the sender BIC was not used as the instructing party\n%s", c.XML)
	}
	// DbtrAcct is mandatory too.
	if !strings.Contains(c.XML, "<Id>NOTPROVIDED</Id>") {
		t.Errorf("the mandatory debtor account has no placeholder\n%s", c.XML)
	}
	if got := fidelityOf(t, c, "50").Fidelity; got != FidelityDerived {
		t.Errorf(":50: fidelity = %q, want derived", got)
	}
}

func TestConvert101OptionAParty(t *testing.T) {
	raw := `{1:F01ACMEGB2LAXXX0000000000}{2:I101BANKGB2LXXXXN}{4:
:20:REQ1
:30:260826
:50A:/GB29NWBK60161331926819
ACMEGB2LXXX
:21:PAY-001
:32B:EUR1,00
:59:MUELLER GMBH
-}`
	c := convert(t, raw)
	if !strings.Contains(c.XML, "<AnyBIC>ACMEGB2LXXX</AnyBIC>") {
		t.Errorf("the option-A instructing party BIC was dropped\n%s", c.XML)
	}
}

func TestConvert101OptionDAgent(t *testing.T) {
	raw := `{1:F01ACMEGB2LAXXX0000000000}{2:I101BANKGB2LXXXXN}{4:
:20:REQ1
:30:260826
:50H:ACME
:52D:SOME BANK PLC
1 HIGH STREET
:21:PAY-001
:32B:EUR1,00
:59:MUELLER GMBH
-}`
	c := convert(t, raw)
	if got := fidelityOf(t, c, "52D").Fidelity; got != FidelityTruncated {
		t.Errorf(":52D: fidelity = %q, want truncated", got)
	}
}

func TestConvert101LongNameAndRemittance(t *testing.T) {
	longName := strings.Repeat("A", 200)
	longRemit := strings.Repeat("B", 200)
	raw := `{1:F01ACMEGB2LAXXX0000000000}{2:I101BANKGB2LXXXXN}{4:
:20:REQ1
:30:260826
:50H:` + longName + `
:21:PAY-001
:32B:EUR1,00
:59:MUELLER GMBH
:70:` + longRemit + `
-}`
	c := convert(t, raw)
	if strings.Contains(c.XML, longName) || strings.Contains(c.XML, longRemit) {
		t.Error("an over-long value was emitted into a Max140Text element")
	}
	if got := fidelityOf(t, c, "50H").Fidelity; got != FidelityTruncated {
		t.Errorf(":50H: fidelity = %q, want truncated", got)
	}
	if got := fidelityOf(t, c, "70").Fidelity; got != FidelityTruncated {
		t.Errorf(":70: fidelity = %q, want truncated", got)
	}
}

func TestConvert101BadChargeBearer(t *testing.T) {
	raw := `{1:F01ACMEGB2LAXXX0000000000}{2:I101BANKGB2LXXXXN}{4:
:20:REQ1
:30:260826
:50H:ACME
:21:PAY-001
:32B:EUR1,00
:59:MUELLER GMBH
:71A:XXX
:25A:CHARGES-ACCOUNT
-}`
	c := convert(t, raw)
	if got := fidelityOf(t, c, "71A").Fidelity; got != FidelityUnmapped {
		t.Errorf(":71A: fidelity = %q, want unmapped", got)
	}
	// ChrgBr is optional in pain.001, so an unrecognised code is left out
	// entirely rather than guessed at.
	if strings.Contains(c.XML, "<ChrgBr>") {
		t.Error("an unrecognised charge bearer was emitted anyway")
	}
	// A charges account without a leading slash is still an account.
	if !strings.Contains(c.XML, "<Id>CHARGES-ACCOUNT</Id>") {
		t.Errorf("the charges account was dropped\n%s", c.XML)
	}
}

func TestConvert101EmptyAgentField(t *testing.T) {
	raw := `{1:F01ACMEGB2LAXXX0000000000}{2:I101BANKGB2LXXXXN}{4:
:20:REQ1
:30:260826
:50H:ACME
:52A:/
:21:PAY-001
:32B:EUR1,00
:57A:/
:59:MUELLER GMBH
-}`
	c := convert(t, raw)
	if got := fidelityOf(t, c, "52A").Fidelity; got != FidelityDerived {
		t.Errorf(":52A: fidelity = %q, want derived", got)
	}
	// A creditor agent that cannot be identified is left out, because CdtrAgt is
	// optional and an empty BICFI would be invalid.
	if strings.Contains(c.XML, "<CdtrAgt>") {
		t.Errorf("an unidentifiable creditor agent was emitted\n%s", c.XML)
	}
}

func TestConvert101Rejects(t *testing.T) {
	base := "{1:F01ACMEGB2LAXXX0000000000}{2:I101BANKGB2LXXXXN}{4:\n:20:REQ1\n"
	for name, body := range map[string]string{
		"no transactions": ":30:260826\n:50H:ACME\n",
		"no date":         ":50H:ACME\n:21:PAY-1\n:32B:EUR1,00\n:59:M\n",
		"bad date":        ":30:269999\n:50H:ACME\n:21:PAY-1\n:32B:EUR1,00\n:59:M\n",
		"no amount":       ":30:260826\n:50H:ACME\n:21:PAY-1\n:59:M\n",
		"bad amount":      ":30:260826\n:50H:ACME\n:21:PAY-1\n:32B:NONSENSE\n:59:M\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Convert(mustParse(t, base+body+"-}")); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestSplitAt(t *testing.T) {
	m := mustParse(t, mt101)
	head, groups := m.SplitAt("21")

	// :21R: shares the tag but belongs to sequence A, so only a bare :21: splits.
	if len(groups) != 2 {
		t.Fatalf("got %d groups, want 2", len(groups))
	}
	if _, ok := FieldsOf(head, "30"); !ok {
		t.Error("the execution date is missing from sequence A")
	}
	if _, ok := ExactOf(groups[1], "21F"); !ok {
		t.Error("the second transaction lost its :21F:")
	}
	if _, ok := FieldsOf(groups[0], "99"); ok {
		t.Error("FieldsOf matched a tag that is not present")
	}
	if _, ok := ExactOf(groups[0], "99Z"); ok {
		t.Error("ExactOf matched a tag that is not present")
	}

	// A message with no occurrence of the tag is all head and no groups.
	m2 := mustParse(t, mt103)
	h2, g2 := m2.SplitAt("21")
	if len(g2) != 0 || len(h2) != len(m2.Fields) {
		t.Errorf("SplitAt on a message without :21: = %d head, %d groups", len(h2), len(g2))
	}
}

func TestDateToISO(t *testing.T) {
	got, err := DateToISO(" 260826 ")
	if err != nil {
		t.Fatal(err)
	}
	if got != "2026-08-26" {
		t.Errorf("DateToISO = %q", got)
	}
	if _, err := DateToISO("2608"); err == nil {
		t.Error("a short date was accepted")
	}
}

func TestConvert101TransactionOverridesAgent(t *testing.T) {
	// Sequence B may name its own account servicing institution, which then
	// applies to that transaction only.
	raw := `{1:F01ACMEGB2LAXXX0000000000}{2:I101BANKGB2LXXXXN}{4:
:20:REQ1
:30:260826
:50H:ACME
:52A:BANKGB2LXXX
:21:PAY-001
:32B:EUR1,00
:59:MUELLER GMBH
:21:PAY-002
:52A:MIDLGB22XXX
:32B:EUR2,00
:59:NORTHWIND INC
-}`
	c := convert(t, raw)

	if !strings.Contains(c.XML, "<BICFI>MIDLGB22XXX</BICFI>") {
		t.Errorf("the transaction-level agent was ignored\n%s", c.XML)
	}
	if !strings.Contains(c.XML, "<BICFI>BANKGB2LXXX</BICFI>") {
		t.Error("the sequence A agent was not used for the first transaction")
	}
	if got := fidelityOf(t, c, "52A (txn 2)").Fidelity; got != FidelityMapped {
		t.Errorf(":52A (txn 2): fidelity = %q, want mapped", got)
	}
}

func TestConvert101PartyWithoutName(t *testing.T) {
	// A beneficiary field carrying only an account has no name to map.
	raw := `{1:F01ACMEGB2LAXXX0000000000}{2:I101BANKGB2LXXXXN}{4:
:20:REQ1
:30:260826
:50H:ACME
:21:PAY-001
:32B:EUR1,00
:59:/DE89370400440532013000
-}`
	c := convert(t, raw)

	if got := fidelityOf(t, c, "59").Fidelity; got != FidelityDerived {
		t.Errorf(":59: fidelity = %q, want derived", got)
	}
	if !strings.Contains(c.XML, "<Nm>NOT PROVIDED</Nm>") {
		t.Errorf("the mandatory creditor name has no placeholder\n%s", c.XML)
	}
	if !strings.Contains(c.XML, "<IBAN>DE89370400440532013000</IBAN>") {
		t.Error("the account was dropped along with the missing name")
	}
}

func TestConvert101WithoutHeaderBIC(t *testing.T) {
	// With truncated headers there is no BIC to fall back on for DbtrAgt, which
	// pain.001 makes mandatory.
	raw := "{1:F01}{2:I101}{4:\n:20:REQ1\n:30:260826\n:50H:ACME\n:21:PAY-001\n:32B:EUR1,00\n:59:MUELLER\n-}"
	c := convert(t, raw)

	if !strings.Contains(c.XML, "<BICFI>NOTPROVIDED</BICFI>") {
		t.Errorf("the mandatory debtor agent has no placeholder\n%s", c.XML)
	}
	r := fidelityOf(t, c, "52")
	if r.Fidelity != FidelityDerived || !strings.Contains(r.Note, "no agent in the message or the header") {
		t.Errorf(":52: = %+v", r)
	}
}
