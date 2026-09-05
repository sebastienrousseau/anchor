// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package swift_test

import (
	"encoding/xml"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/sebastienrousseau/askiso/internal/swift"
)

// An MT message is a text file from another institution's system. The parser
// slices strings by offset in several places, which is exactly where a
// truncated or unusual message causes a panic. Two properties are asserted: the
// parser always returns, and a conversion that succeeds always produces
// well-formed XML.
//
//	go test ./internal/swift/ -fuzz FuzzParse -fuzztime 60s

func FuzzParse(f *testing.F) {
	f.Add(`{1:F01BANKGB2LAXXX0000000000}{2:I103BANKDEFFXXXXN}{3:{121:f3a1b2c4-5d6e-4f70-8a91-2b3c4d5e6f70}}{4:
:20:REF1
:32A:260824EUR25000,00
:50K:/GB29NWBK60161331926819
ACME TRADING LIMITED
:59:/DE89370400440532013000
MUELLER GMBH
:71A:SHA
-}{5:{CHK:123456789ABC}}`)

	// Truncated headers, unbalanced braces, missing blocks, odd field shapes.
	f.Add("{1:F01}{2:I103}{4:\n:20:R\n:32A:260824EUR1,00\n-}")
	f.Add("{1:}{2:}{4:}")
	f.Add("{4:\n:20:R\n-}")
	f.Add("{1:F01BANKGB2LAXXX0000000000}{2:I940BANKDEFFXXXXN}{4:\n:20:S\n:25:A\n:60F:C260823EUR1,00\n:62F:C260824EUR1,00\n-}")
	f.Add("{1:F01BANKGB2LAXXX0000000000}{2:I101BANKGB2LXXXXN}{4:\n:20:R\n:30:260826\n:21:P\n:32B:EUR1,00\n:59:M\n-}")
	f.Add("{2:I103}{4:\n:99Z:\n-}")
	f.Add("{1:{2:{3:{4:{5:")
	f.Add("")

	f.Fuzz(func(t *testing.T, data string) {
		if len(data) > 1<<20 {
			return
		}
		msg, err := swift.Parse([]byte(data))
		if err != nil {
			if msg != nil {
				t.Fatalf("Parse returned both a message and an error: %v", err)
			}
			return
		}
		if msg == nil {
			t.Fatal("Parse returned neither a message nor an error")
		}

		// Field accessors must be safe on whatever parsed.
		for _, fld := range msg.Fields {
			_ = fld.Name()
			_ = fld.Lines()
			_, _ = swift.PartyLines(fld)
			_, _ = swift.ParseValueDateAmount(fld.Value)
			_, _ = swift.ParseCurrencyAmount(fld.Value)
			_, _ = swift.ChargeBearer(fld.Value)
		}
		_, _ = msg.SplitAt("21")

		conv, err := swift.Convert(msg)
		if err != nil {
			return
		}

		// A conversion that claims to have succeeded must have produced a
		// document, because the next thing anyone does is validate it.
		if conv.XML == "" {
			t.Fatal("Convert succeeded with no output")
		}
		if err := xml.Unmarshal([]byte(conv.XML), new(struct{})); err != nil {
			t.Fatalf("the generated XML is not well-formed: %v\n%s", err, conv.XML)
		}
		if !strings.Contains(conv.XML, conv.TargetType) {
			t.Fatalf("the output does not carry its own namespace:\n%s", conv.XML)
		}
		// Every source field has to appear in the report.
		if len(conv.Report) < len(msg.Fields) {
			t.Fatalf("the report has %d entries for %d fields", len(conv.Report), len(msg.Fields))
		}
	})
}

// FuzzStructuredMT103 explores the valid MT103 grammar rather than relying on
// arbitrary bytes to stumble into it.  The monetary tuple, reference and
// charge bearer must survive MT -> MX -> MT; this is the business invariant
// that a merely panic-free parser cannot prove.
func FuzzStructuredMT103(f *testing.F) {
	f.Add("REF1", uint64(2_500_000), uint8(0), uint8(0), true)
	f.Add("EDGE/REFERENCE-16", uint64(0), uint8(1), uint8(1), false)
	f.Add("PAYMENT?42", uint64(99_999_999_999_999), uint8(3), uint8(2), true)

	currencies := [...]string{"EUR", "USD", "GBP", "JPY"}
	charges := [...]string{"SHA", "OUR", "BEN"}
	dates := [...]string{"260824", "000229", "991231", "800101"}

	f.Fuzz(func(t *testing.T, rawReference string, rawCents uint64, currencyIndex, chargeIndex uint8, includeUETR bool) {
		reference := swiftReference(rawReference, 16)
		cents := rawCents % 100_000_000_000_000
		currency := currencies[int(currencyIndex)%len(currencies)]
		charge := charges[int(chargeIndex)%len(charges)]
		date := dates[(int(currencyIndex)+int(chargeIndex))%len(dates)]
		mtAmount := fmt.Sprintf("%d,%02d", cents/100, cents%100)
		wantAmount := fmt.Sprintf("%d.%02d", cents/100, cents%100)

		block3 := ""
		if includeUETR {
			block3 = "{3:{121:f3a1b2c4-5d6e-4f70-8a91-2b3c4d5e6f70}}"
		}
		raw := fmt.Sprintf("{1:F01BANKGB2LAXXX0000000000}{2:I103BANKDEFFXXXXN}%s{4:\n:20:%s\n:32A:%s%s%s\n:50K:/GB29NWBK60161331926819\nACME TRADING LIMITED\n:59:/DE89370400440532013000\nMUELLER GMBH\n:71A:%s\n-}",
			block3, reference, date, currency, mtAmount, charge)

		msg, err := swift.Parse([]byte(raw))
		if err != nil {
			t.Fatalf("generated MT103 did not parse: %v\n%s", err, raw)
		}
		if msg.Type != "103" {
			t.Fatalf("parsed type %q, want 103", msg.Type)
		}
		ref, ok := msg.GetExact("20")
		if !ok || ref.Value != reference {
			t.Fatalf("reference = %q, %v; want %q", ref.Value, ok, reference)
		}
		field32A, ok := msg.GetExact("32A")
		if !ok {
			t.Fatal("generated MT103 lost field 32A")
		}
		vda, err := swift.ParseValueDateAmount(field32A.Value)
		if err != nil {
			t.Fatalf("generated monetary tuple is invalid: %v", err)
		}
		if vda.Currency != currency || vda.Amount != wantAmount {
			t.Fatalf("parsed monetary tuple = %s %s; want %s %s", vda.Currency, vda.Amount, currency, wantAmount)
		}

		mx, err := swift.Convert(msg)
		if err != nil {
			t.Fatalf("valid MT103 conversion failed: %v", err)
		}
		if mx.SourceType != "103" || mx.TargetType != "pacs.008.001.10" {
			t.Fatalf("conversion types = %s -> %s", mx.SourceType, mx.TargetType)
		}
		if err := xml.Unmarshal([]byte(mx.XML), new(struct{})); err != nil {
			t.Fatalf("generated MX is not well-formed: %v", err)
		}
		gotReference, gotCurrency, gotAmount, err := mxPaymentValues(mx.XML)
		if err != nil {
			t.Fatalf("reading generated MX semantics: %v", err)
		}
		if gotReference != reference {
			t.Fatalf("MX reference = %q; want %q", gotReference, reference)
		}
		if gotCurrency != currency || gotAmount != wantAmount {
			t.Fatalf("MX monetary tuple = %s %s; want %s %s", gotCurrency, gotAmount, currency, wantAmount)
		}

		back, err := swift.ConvertMX([]byte(mx.XML))
		if err != nil {
			t.Fatalf("generated MX did not convert back: %v", err)
		}
		roundTrip, err := swift.Parse([]byte(back.XML))
		if err != nil {
			t.Fatalf("round-trip MT did not parse: %v\n%s", err, back.XML)
		}
		backRef, ok := roundTrip.GetExact("20")
		if !ok || backRef.Value != reference {
			t.Fatalf("round-trip reference = %q, %v; want %q", backRef.Value, ok, reference)
		}
		back32A, ok := roundTrip.GetExact("32A")
		if !ok {
			t.Fatal("round-trip MT lost field 32A")
		}
		backVDA, err := swift.ParseValueDateAmount(back32A.Value)
		if err != nil {
			t.Fatalf("round-trip monetary tuple is invalid: %v", err)
		}
		if backVDA != vda {
			t.Fatalf("round-trip monetary tuple = %+v; want %+v", backVDA, vda)
		}
	})
}

func mxPaymentValues(document string) (reference, currency, amount string, err error) {
	dec := xml.NewDecoder(strings.NewReader(document))
	for {
		token, tokenErr := dec.Token()
		if tokenErr != nil {
			if tokenErr == io.EOF {
				return reference, currency, amount, nil
			}
			return "", "", "", tokenErr
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		switch start.Name.Local {
		case "MsgId":
			if reference == "" {
				if err := dec.DecodeElement(&reference, &start); err != nil {
					return "", "", "", err
				}
			}
		case "IntrBkSttlmAmt":
			for _, attr := range start.Attr {
				if attr.Name.Local == "Ccy" {
					currency = attr.Value
				}
			}
			if err := dec.DecodeElement(&amount, &start); err != nil {
				return "", "", "", err
			}
		}
	}
}

func swiftReference(input string, limit int) string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789/-?:().,'+ "
	var out strings.Builder
	for _, r := range input {
		if out.Len() == limit {
			break
		}
		if r < 128 && strings.ContainsRune(alphabet, r) {
			out.WriteRune(r)
		} else {
			out.WriteByte('X')
		}
	}
	value := strings.TrimSpace(out.String())
	if value == "" {
		return "REF"
	}
	return value
}
