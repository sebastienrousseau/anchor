// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package cbprworkspace

import (
	"strings"
	"testing"
)

func TestTransportEnvelopes(t *testing.T) {
	header := []byte(`<?xml version="1.0"?><AppHdr xmlns="urn:iso:std:iso:20022:tech:xsd:head.001.001.02"/>`)
	payload := []byte(`<?xml version="1.0"?><Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.08"/>`)
	for _, test := range []struct {
		options SampleExportOptions
		want    string
	}{
		{options: SampleExportOptions{}, want: "<Envelope>"},
		{options: SampleExportOptions{Profile: TransportRequestPayload}, want: "<RequestPayload>"},
		{options: SampleExportOptions{Profile: TransportSwiftDataPDU, SenderDN: "o=a&b", ReceiverDN: "o=receiver"}, want: "<Saa:DataPDU"},
	} {
		data, err := transportEnvelope(header, payload, "pacs.008.001.08", "reference", test.options)
		if err != nil || !strings.Contains(string(data), test.want) || !strings.Contains(string(data), "<Document") {
			t.Fatalf("transport %q = %s, %v", test.options.Profile, data, err)
		}
		if test.options.Profile == TransportSwiftDataPDU && (!strings.Contains(string(data), "o=a&amp;b") || !strings.Contains(string(data), "swift.finplus")) {
			t.Fatalf("escaped/default DataPDU = %s", data)
		}
	}
}

func TestTransportOptionsRejectUnsafeProfiles(t *testing.T) {
	if _, err := normaliseTransportOptions(SampleExportOptions{Profile: "unknown"}); err == nil {
		t.Fatal("unknown transport profile succeeded")
	}
	if _, err := normaliseTransportOptions(SampleExportOptions{Profile: TransportSwiftDataPDU}); err == nil {
		t.Fatal("DataPDU without DNs succeeded")
	}
	if _, err := transportEnvelope(nil, nil, "", "", SampleExportOptions{Profile: "unknown"}); err == nil {
		t.Fatal("unknown transport envelope succeeded")
	}
}

func TestLegacySyntheticEnvelope(t *testing.T) {
	data := syntheticEnvelope([]byte(`<AppHdr/>`), []byte(`<Document/>`))
	if !strings.Contains(string(data), "<Envelope>") || !strings.Contains(string(data), "<Document/>") {
		t.Fatalf("legacy envelope = %s", data)
	}
}
