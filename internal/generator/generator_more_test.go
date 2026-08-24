// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package generator

import (
	"strings"
	"testing"
)

// Every blank field must be filled, so a bare Options still yields a complete
// message rather than one with holes in it.
func TestGenerateFillsEveryBlank(t *testing.T) {
	xml, err := Generate(Options{MsgType: "pacs.008"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	for _, want := range []string{"EUR", "Acme", "Global", "DE89", "FR76", "DEUTDEDD", "BNPAFRPP", "E2E-"} {
		if !strings.Contains(xml, want) {
			t.Errorf("default %q missing from the output", want)
		}
	}
}

func TestGenerateHonoursEveryOption(t *testing.T) {
	opt := Options{
		MsgType:      "pacs.008",
		Amount:       "1.23",
		Currency:     "CHF",
		Debtor:       "Debtor Name",
		Creditor:     "Creditor Name",
		DebtorIBAN:   "CH9300762011623852957",
		CreditorIBAN: "CH5604835012345678009",
		DebtorBIC:    "UBSWCHZH80A",
		CreditorBIC:  "CRESCHZZ80A",
		EndToEndID:   "E2E-CUSTOM",
		UETR:         "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee",
		Preset:       "standard",
	}
	xml, err := Generate(opt)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"1.23", "CHF", "Debtor Name", "Creditor Name",
		"CH9300762011623852957", "UBSWCHZH80A", "E2E-CUSTOM", opt.UETR} {
		if !strings.Contains(xml, want) {
			t.Errorf("supplied value %q was not used", want)
		}
	}
}

func TestPresetsSetTheirOwnDefaults(t *testing.T) {
	cases := map[string]string{
		"sepa":    "EUR",
		"target2": "EUR",
		"chaps":   "GBP",
		"fednow":  "USD",
	}
	for preset, ccy := range cases {
		t.Run(preset, func(t *testing.T) {
			xml, err := Generate(Options{MsgType: "pacs.008", Preset: preset})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(xml, ccy) {
				t.Errorf("preset %s should use %s", preset, ccy)
			}
		})
	}

	// An unrecognised preset leaves the defaults alone.
	if _, err := Generate(Options{MsgType: "pacs.008", Preset: "nonexistent"}); err != nil {
		t.Errorf("an unknown preset should fall back to defaults: %v", err)
	}
}

func TestClearingSystemFollowsPreset(t *testing.T) {
	for preset, want := range map[string]string{
		"sepa": "SEPA", "fednow": "FDNW", "chaps": "CHAPS", "target2": "TARGET2", "": "TARGET2",
	} {
		xml, err := Generate(Options{MsgType: "pacs.008", Preset: preset})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(xml, want) {
			t.Errorf("preset %q should name clearing system %q", preset, want)
		}
	}
}

func TestUnsupportedMessageType(t *testing.T) {
	_, err := Generate(Options{MsgType: "zzzz.999"})
	if err == nil {
		t.Fatal("an unsupported type should be an error")
	}
	if !strings.Contains(err.Error(), "pacs.008") {
		t.Errorf("the error should list what is supported: %v", err)
	}
}

func TestDefaultOptionsAreComplete(t *testing.T) {
	o := DefaultOptions("pacs.008")
	if o.MsgType != "pacs.008" || o.Amount == "" || o.Currency == "" ||
		o.Debtor == "" || o.Creditor == "" || o.DebtorIBAN == "" ||
		o.CreditorIBAN == "" || o.DebtorBIC == "" || o.CreditorBIC == "" ||
		o.EndToEndID == "" || o.UETR == "" {
		t.Errorf("DefaultOptions left a field empty: %+v", o)
	}
}
