// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package swift

import (
	"strings"
	"testing"
)

const mt103 = `{1:F01BANKGB2LAXXX0000000000}{2:I103BANKDEFFXXXXN}{3:{108:MT103REF}{121:f3a1b2c4-5d6e-4f70-8a91-2b3c4d5e6f70}}{4:
:20:REF20260824001
:23B:CRED
:32A:260824EUR25000,00
:50K:/GB29NWBK60161331926819
ACME TRADING LIMITED
14 GRESHAM STREET
LONDON EC2V 7NN
:52A:BANKGB2LXXX
:57A:BANKDEFFXXX
:59:/DE89370400440532013000
MUELLER GMBH
HAUPTSTRASSE 12
60311 FRANKFURT AM MAIN
:70:INVOICE 2026-0815 CONSULTING SERVICES
:71A:SHA
-}{5:{CHK:123456789ABC}}`

func mustParse(t *testing.T, raw string) *Message {
	t.Helper()
	m, err := Parse([]byte(raw))
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	return m
}

func TestParseHeaders(t *testing.T) {
	m := mustParse(t, mt103)

	if m.Type != "103" {
		t.Errorf("Type = %q, want 103", m.Type)
	}
	// The headers carry twelve-character logical terminal addresses; the ninth
	// character identifies the terminal and is not part of the BIC.
	if m.Sender != "BANKGB2LXXX" {
		t.Errorf("Sender = %q, want BANKGB2LXXX", m.Sender)
	}
	if m.Receiver != "BANKDEFFXXX" {
		t.Errorf("Receiver = %q, want BANKDEFFXXX", m.Receiver)
	}
	if m.UETR != "f3a1b2c4-5d6e-4f70-8a91-2b3c4d5e6f70" {
		t.Errorf("UETR = %q", m.UETR)
	}
	// Block 5 must be captured even though nothing reads it, because a message
	// that loses its trailer is not the message that arrived.
	if _, ok := m.Blocks["5"]; !ok {
		t.Error("block 5 was not captured")
	}
}

func TestParseCarriageReturns(t *testing.T) {
	// Real MT arrives with CRLF line endings.
	m := mustParse(t, strings.ReplaceAll(mt103, "\n", "\r\n"))
	f, ok := m.GetExact("32A")
	if !ok {
		t.Fatal("32A missing")
	}
	if f.Value != "260824EUR25000,00" {
		t.Errorf("32A = %q", f.Value)
	}
}

func TestParseFieldsInOrder(t *testing.T) {
	m := mustParse(t, mt103)

	var got []string
	for _, f := range m.Fields {
		got = append(got, f.Name())
	}
	want := []string{"20", "23B", "32A", "50K", "52A", "57A", "59", "70", "71A"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("fields = %v, want %v", got, want)
	}
}

func TestFieldAccessors(t *testing.T) {
	m := mustParse(t, mt103)

	// Get ignores the option letter; GetExact does not.
	if f, ok := m.Get("50"); !ok || f.Option != "K" {
		t.Errorf("Get(50) = %+v, %v", f, ok)
	}
	if _, ok := m.GetExact("50A"); ok {
		t.Error("GetExact(50A) matched a 50K field")
	}
	if _, ok := m.Get("99"); ok {
		t.Error("Get(99) matched something")
	}
	if _, ok := m.GetExact("99Z"); ok {
		t.Error("GetExact(99Z) matched something")
	}
	if n := len(m.All("71")); n != 1 {
		t.Errorf("All(71) returned %d fields, want 1", n)
	}
	if n := len(m.All("99")); n != 0 {
		t.Errorf("All(99) returned %d fields, want 0", n)
	}
}

func TestFieldLines(t *testing.T) {
	m := mustParse(t, mt103)
	f, _ := m.Get("50")

	lines := f.Lines()
	if len(lines) != 4 {
		t.Fatalf("Lines() = %d, want 4: %q", len(lines), lines)
	}
	if lines[0] != "/GB29NWBK60161331926819" {
		t.Errorf("first line = %q", lines[0])
	}
	if got := (Field{}).Lines(); got != nil {
		t.Errorf("empty field Lines() = %v, want nil", got)
	}
}

func TestParseRejects(t *testing.T) {
	cases := []struct {
		name, raw, want string
	}{
		{"empty", "   \n ", "empty message"},
		{"no blocks", "just some text", "no SWIFT blocks"},
		{"unclosed", "{1:F01BANKGB2LAXXX0000000000}{4:\n:20:REF", "is not closed"},
		{"no text block", "{1:F01BANKGB2LAXXX0000000000}{2:I103BANKDEFFXXXXN}", "no text block"},
		{"no fields", "{1:F01BANKGB2LAXXX0000000000}{4:\nnothing here\n-}", "no fields"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.raw))
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestLogicalTerminalToBIC(t *testing.T) {
	if got := logicalTerminalToBIC("BANKGB2LAXXX"); got != "BANKGB2LXXX" {
		t.Errorf("got %q, want BANKGB2LXXX", got)
	}
	// Anything that is not a twelve-character address is left alone.
	for _, in := range []string{"BANKGB2L", "BANKGB2LXXX", ""} {
		if got := logicalTerminalToBIC(in); got != in {
			t.Errorf("logicalTerminalToBIC(%q) = %q", in, got)
		}
	}
}

func TestParseShortHeaders(t *testing.T) {
	// A truncated header must not panic; the fields simply stay empty.
	m := mustParse(t, "{1:F01}{2:I103}{4:\n:20:REF\n:32A:260824EUR1,00\n-}")
	if m.Sender != "" {
		t.Errorf("Sender = %q, want empty", m.Sender)
	}
	if m.Receiver != "" {
		t.Errorf("Receiver = %q, want empty", m.Receiver)
	}
	if m.Type != "103" {
		t.Errorf("Type = %q, want 103", m.Type)
	}
}

func TestParseValueDateAmount(t *testing.T) {
	got, err := ParseValueDateAmount("260824EUR25000,00")
	if err != nil {
		t.Fatal(err)
	}
	if got.Date != "2026-08-24" || got.Currency != "EUR" || got.Amount != "25000.00" {
		t.Errorf("got %+v", got)
	}

	// A year of 80 or more belongs to the twentieth century.
	old, err := ParseValueDateAmount("991231USD1,00")
	if err != nil {
		t.Fatal(err)
	}
	if old.Date != "1999-12-31" {
		t.Errorf("Date = %q, want 1999-12-31", old.Date)
	}

	// A whole amount is written with a trailing comma and no decimals.
	whole, err := ParseValueDateAmount("260824JPY5000,")
	if err != nil {
		t.Fatal(err)
	}
	if whole.Amount != "5000" {
		t.Errorf("Amount = %q, want 5000", whole.Amount)
	}
}

func TestParseValueDateAmountRejects(t *testing.T) {
	for _, raw := range []string{"", "not a field", "260824EU25000,00", "261324EUR1,00", "260832EUR1,00", "260824EUR,"} {
		if _, err := ParseValueDateAmount(raw); err == nil {
			t.Errorf("ParseValueDateAmount(%q) accepted an invalid field", raw)
		}
	}
}

func TestYYMMDDRejectsWrongLength(t *testing.T) {
	if _, err := yymmddToISO("2608"); err == nil {
		t.Error("a four-digit date was accepted")
	}
}

func TestParseCurrencyAmount(t *testing.T) {
	got, err := ParseCurrencyAmount("GBP21000,00")
	if err != nil {
		t.Fatal(err)
	}
	if got.Currency != "GBP" || got.Amount != "21000.00" {
		t.Errorf("got %+v", got)
	}
	for _, raw := range []string{"", "21000,00", "GBP", "GBP,"} {
		if _, err := ParseCurrencyAmount(raw); err == nil {
			t.Errorf("ParseCurrencyAmount(%q) accepted an invalid field", raw)
		}
	}
}

func TestPartyLines(t *testing.T) {
	account, rest := PartyLines(Field{Value: "/GB29NWBK60161331926819\nACME LIMITED\nLONDON"})
	if account != "GB29NWBK60161331926819" {
		t.Errorf("account = %q", account)
	}
	if len(rest) != 2 || rest[0] != "ACME LIMITED" {
		t.Errorf("rest = %v", rest)
	}

	// A double slash marks a national identifier, not an account number.
	nat, _ := PartyLines(Field{Value: "//CH1234567\nACME LIMITED"})
	if nat != "CH1234567" {
		t.Errorf("national id = %q", nat)
	}

	// Without a leading slash the first line is a name, not an account.
	none, lines := PartyLines(Field{Value: "ACME LIMITED\n\nLONDON"})
	if none != "" {
		t.Errorf("account = %q, want empty", none)
	}
	if len(lines) != 2 {
		t.Errorf("lines = %v, want the blank line dropped", lines)
	}
}

func TestChargeBearer(t *testing.T) {
	for mt, want := range map[string]string{"OUR": "DEBT", "BEN": "CRED", "SHA": "SHAR", " sha ": "SHAR"} {
		got, ok := ChargeBearer(mt)
		if !ok || got != want {
			t.Errorf("ChargeBearer(%q) = %q, %v; want %q", mt, got, ok, want)
		}
	}
	if _, ok := ChargeBearer("XXX"); ok {
		t.Error("ChargeBearer accepted an unknown code")
	}
}

func TestSupported(t *testing.T) {
	if got := Supported(); len(got) != 10 {
		t.Errorf("Supported() = %v", got)
	}
}

func TestParseTruncatedBasicHeader(t *testing.T) {
	// A basic header one character short of holding an address must not be
	// sliced. Found by fuzzing, which is where off-by-one slicing surfaces.
	for _, raw := range []string{
		"{1:00000000000000 }{4:\n:20:REF\n-}", // 14 characters after "{1:"
		"{1:F01BANKGB2LAXX}{4:\n:20:REF\n-}",
		"{1:F}{4:\n:20:REF\n-}",
	} {
		m, err := Parse([]byte(raw))
		if err != nil {
			t.Fatalf("Parse(%q): %v", raw, err)
		}
		if m.Sender != "" {
			t.Errorf("Parse(%q) read a sender from a short header: %q", raw, m.Sender)
		}
	}

	// One character longer and the address is there to read.
	m, err := Parse([]byte("{1:F01BANKGB2LAXXX}{4:\n:20:REF\n-}"))
	if err != nil {
		t.Fatal(err)
	}
	if m.Sender != "BANKGB2LXXX" {
		t.Errorf("Sender = %q, want BANKGB2LXXX", m.Sender)
	}
}
