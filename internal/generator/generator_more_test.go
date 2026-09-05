// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package generator

import (
	"strings"
	"testing"
)

func TestMessageDefinitionIDFallsBackWithoutNamespace(t *testing.T) {
	if got := messageDefinitionID("pacs.008", `<Document/>`); got != "pacs.008" {
		t.Fatalf("message definition fallback = %q", got)
	}
}

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

	if _, err := Generate(Options{MsgType: "pacs.008", Preset: "nonexistent"}); err == nil {
		t.Error("an unknown preset should be rejected")
	}
}

func TestExplicitOverrideAfterPresetIsPreserved(t *testing.T) {
	opt := DefaultOptions("pacs.008")
	opt.Preset = "fednow"
	opt.ApplyPreset()
	opt.Currency = "CAD"

	xml, err := Generate(opt)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(xml, `Ccy="CAD"`) {
		t.Error("an explicit option set after ApplyPreset should override the preset default")
	}
}

func TestValidateOptionsRejectsEveryInvalidTypedField(t *testing.T) {
	valid := DefaultOptions("pacs.008")
	tests := map[string]func(*Options){
		"XML control character": func(o *Options) { o.Debtor = "bad\x00name" },
		"amount":                func(o *Options) { o.Amount = "1.234" },
		"debtor BIC":            func(o *Options) { o.DebtorBIC = "BAD" },
		"creditor BIC":          func(o *Options) { o.CreditorBIC = "BAD" },
		"UETR":                  func(o *Options) { o.UETR = "not-a-uetr" },
		"debtor IBAN":           func(o *Options) { o.DebtorIBAN = "DE00BAD" },
		"creditor IBAN":         func(o *Options) { o.CreditorIBAN = "FR00BAD" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			opt := valid
			mutate(&opt)
			if err := ValidateOptions(opt); err == nil {
				t.Fatal("invalid option should be rejected")
			}
		})
	}

	domestic := valid
	domestic.AccountScheme = SchemeOther
	domestic.DebtorIBAN, domestic.CreditorIBAN = "", ""
	if err := ValidateOptions(domestic); err != nil {
		t.Fatalf("non-IBAN account schemes should skip IBAN validation: %v", err)
	}
}

func TestAccountBlockFallsBackToSyntheticDomesticID(t *testing.T) {
	got := AccountBlock(Options{AccountScheme: SchemeOther}, "", "", "  ")
	if !strings.Contains(got, "000000000") {
		t.Errorf("empty domestic ID should get a safe placeholder: %s", got)
	}
}

func TestClearingSystemFollowsPreset(t *testing.T) {
	for preset, want := range map[string]string{
		"sepa": "SEPA", "fednow": "FDNW", "fedwire": "FEDWIRE", "chaps": "CHAPS", "target2": "TARGET2", "": "TARGET2",
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
