// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package swift_test

import (
	"encoding/xml"
	"strings"
	"testing"

	"github.com/sebastienrousseau/anchor/internal/swift"
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
