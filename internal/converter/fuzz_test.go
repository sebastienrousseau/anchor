// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package converter_test

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"strings"
	"testing"

	"github.com/sebastienrousseau/askiso/internal/converter"
)

// The converter's whole claim is that it preserves element order in both
// directions. A round trip is the property that claim reduces to, and it is
// cheap to check on arbitrary input.
//
//	go test ./internal/converter/ -fuzz FuzzRoundTrip -fuzztime 60s

func FuzzRoundTrip(f *testing.F) {
	f.Add(`<?xml version="1.0"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">
  <GrpHdr><MsgId>MSG-1</MsgId><CreDtTm>2026-08-24T09:00:00Z</CreDtTm></GrpHdr>
  <Tx><Amt Ccy="EUR">25000.00</Amt></Tx>
  <Tx><Amt Ccy="EUR">1.00</Amt></Tx>
</Document>`)

	f.Add(`<A><B>1</B><C/><B>2</B></A>`) // non-adjacent repeats: must be refused
	f.Add(`<A a="1" b="2">text</A>`)
	f.Add(`<A><B>&amp;&lt;&gt;</B></A>`)
	f.Add(`<A></A>`)
	f.Add(`<A>0.10</A>`)
	f.Add("")
	f.Add("<unclosed>")

	f.Fuzz(func(t *testing.T, data string) {
		if len(data) > 1<<20 {
			return
		}
		jsonBytes, err := converter.XMLToJSON([]byte(data))
		if err != nil {
			return
		}
		if !json.Valid(jsonBytes) {
			t.Fatalf("XMLToJSON produced invalid JSON:\n%s", jsonBytes)
		}

		xmlBytes, err := converter.JSONToXML(jsonBytes)
		if err != nil {
			t.Fatalf("JSONToXML rejected what XMLToJSON produced: %v\n%s", err, jsonBytes)
		}

		// Round-tripping again has to reach the same JSON. If it does not, the
		// conversion is losing or reordering something.
		again, err := converter.XMLToJSON(xmlBytes)
		if err != nil {
			t.Fatalf("XMLToJSON rejected its own output: %v\n%s", err, xmlBytes)
		}
		if string(again) != string(jsonBytes) {
			t.Fatalf("the round trip is not stable\nfirst:\n%s\nsecond:\n%s", jsonBytes, again)
		}
	})
}

// FuzzStructuredRoundTrip generates a bounded semantic document rather than
// waiting for byte mutations to rediscover XML grammar. Adjacent repeated
// transactions, attributes, optional elements, Unicode text and decimal
// lexical forms all remain in the representable subset and therefore must be
// bijective under XML -> JSON -> XML -> JSON.
func FuzzStructuredRoundTrip(f *testing.F) {
	f.Add("PAYMENT-1", uint8(1), uint64(2500000), true)
	f.Add("A&B <priority>", uint8(8), ^uint64(0), false)
	f.Add("東京-€-😀", uint8(255), uint64(1), true)

	f.Fuzz(func(t *testing.T, rawID string, count uint8, cents uint64, optional bool) {
		if len(rawID) > 4096 {
			rawID = rawID[:4096]
		}
		var escaped bytes.Buffer
		if err := xml.EscapeText(&escaped, []byte(rawID)); err != nil {
			t.Fatal(err)
		}
		id := escaped.String()
		txCount := int(count%16) + 1
		amount := fmt.Sprintf("%d.%02d", (cents%1_000_000_000_000)/100, cents%100)

		var tx strings.Builder
		for i := range txCount {
			fmt.Fprintf(&tx, `<Tx index="%d"><Amt Ccy="EUR">%s</Amt><Ref>%s</Ref></Tx>`, i, amount, id)
		}
		extra := ""
		if optional {
			extra = "<SupplementaryData><PlaceAndName>audit</PlaceAndName></SupplementaryData>"
		}
		doc := `<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10"><GrpHdr><MsgId>` + id +
			`</MsgId></GrpHdr>` + tx.String() + extra + `</Document>`

		first, err := converter.XMLToJSON([]byte(doc))
		if err != nil {
			t.Fatalf("generated semantic XML was rejected: %v\n%s", err, doc)
		}
		back, err := converter.JSONToXML(first)
		if err != nil {
			t.Fatalf("JSONToXML rejected converter output: %v\n%s", err, first)
		}
		second, err := converter.XMLToJSON(back)
		if err != nil {
			t.Fatalf("XMLToJSON rejected round-trip XML: %v\n%s", err, back)
		}
		if !bytes.Equal(first, second) {
			t.Fatalf("non-bijective conversion\nfirst: %s\nsecond: %s", first, second)
		}
	})
}
