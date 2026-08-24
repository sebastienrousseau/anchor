// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package converter_test

import (
	"encoding/json"
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
