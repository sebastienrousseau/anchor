// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package generator

import (
	"encoding/xml"
	"io"
	"strings"
	"testing"
)

func TestGenerateMessages(t *testing.T) {
	types := []string{"pacs.008", "pacs.009", "pain.001", "camt.053"}

	for _, ty := range types {
		opts := DefaultOptions(ty)
		opts.Amount = "1234.56"
		opts.Currency = "EUR"
		opts.Debtor = "Test Debtor Corp"

		xml, err := Generate(opts)
		if err != nil {
			t.Fatalf("Generate failed for %s: %v", ty, err)
		}
		if !strings.Contains(xml, "<?xml") {
			t.Errorf("Generated XML for %s missing XML header", ty)
		}
		if !strings.Contains(xml, "1234.56") {
			t.Errorf("Generated XML for %s missing amount", ty)
		}
	}
}

func TestGenerateEscapesCallerValues(t *testing.T) {
	for _, ty := range TemplateTypes() {
		opt := DefaultOptions(ty)
		switch ty {
		case "pacs.008", "pain.001":
			opt.Debtor = `A&B <Holdings>`
		case "pacs.009":
			opt.EndToEndID = `E2E&<ID>`
		case "camt.053":
			opt.Currency = `E&<`
		}
		doc, err := Generate(opt)
		if ty == "camt.053" {
			if err == nil {
				t.Fatal("markup in a typed currency field should be rejected")
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s: %v", ty, err)
		}
		dec := xml.NewDecoder(strings.NewReader(doc))
		for {
			if _, err := dec.Token(); err != nil {
				if err == io.EOF {
					break
				}
				t.Fatalf("%s generated malformed XML: %v\n%s", ty, err, doc)
			}
		}
		if !strings.Contains(doc, "&amp;") || !strings.Contains(doc, "&lt;") {
			t.Errorf("%s did not escape text content", ty)
		}
	}
}

func TestGenerateUnsupported(t *testing.T) {
	opts := DefaultOptions("unsupported.999")
	_, err := Generate(opts)
	if err == nil {
		t.Errorf("Expected error for unsupported message type")
	}
}

func TestGeneratePresets(t *testing.T) {
	presets := []string{"sepa", "fednow", "target2", "chaps"}
	for _, p := range presets {
		opts := DefaultOptions("pacs.008")
		opts.Preset = p
		xml, err := Generate(opts)
		if err != nil {
			t.Fatalf("Generate with preset %s failed: %v", p, err)
		}
		if p == "fednow" && !strings.Contains(xml, "USD") {
			t.Errorf("Expected USD currency for fednow preset")
		}
		if p == "chaps" && !strings.Contains(xml, "GBP") {
			t.Errorf("Expected GBP currency for chaps preset")
		}
	}
}

func TestGenerateWithBAH(t *testing.T) {
	opts := DefaultOptions("pacs.008")
	opts.WithBAH = true
	xml, err := Generate(opts)
	if err != nil {
		t.Fatalf("Generate with BAH failed: %v", err)
	}

	if !strings.Contains(xml, "<AppHdr") {
		t.Error("the header should be present")
	}
	// head.001.001.02 declares AppHdr as its only global element, so the header
	// and the document are siblings in a network envelope, each carrying its own
	// namespace. A <BusMsg> root in the head namespace does not validate.
	if strings.Contains(xml, "BusMsg") {
		t.Error("BusMsg is not a global declaration in head.001.001.02")
	}
	if !strings.Contains(xml, `<AppHdr xmlns="urn:iso:std:iso:20022:tech:xsd:head.001.001.02">`) {
		t.Errorf("the header should declare its own namespace:\n%s", xml)
	}
	if !strings.Contains(xml, `<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">`) {
		t.Errorf("the document should declare its own namespace:\n%s", xml)
	}
	if !strings.Contains(xml, "<MsgDefIdr>pacs.008.001.10</MsgDefIdr>") {
		t.Errorf("MsgDefIdr must carry the full identifier, not the base code:\n%s", xml)
	}
}
