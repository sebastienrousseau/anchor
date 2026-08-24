// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package linter

import (
	"strings"
	"testing"
)

func TestValidateIBANRejections(t *testing.T) {
	cases := map[string]string{
		"too short":            "DE89",
		"too long":             strings.Repeat("A", 40),
		"digits for country":   "1289370400440532013000",
		"non-numeric checksum": "DEXX370400440532013000",
		"bad character":        "DE89-70400440532013000",
		"wrong checksum":       "DE00370400440532013000",
	}
	for name, iban := range cases {
		t.Run(name, func(t *testing.T) {
			ok, reason := ValidateIBAN(iban)
			if ok {
				t.Errorf("%q should be rejected", iban)
			}
			if reason == "" {
				t.Error("a rejection should explain itself")
			}
		})
	}

	// Spaces are normalised away.
	if ok, _ := ValidateIBAN("DE89 3704 0044 0532 0130 00"); !ok {
		t.Error("a grouped IBAN should be accepted")
	}
	// Input is upper-cased before checking, so case does not matter.
	if ok, reason := ValidateIBAN("de89370400440532013000"); !ok {
		t.Errorf("a lower-case IBAN should be accepted: %s", reason)
	}
}

func TestValidateCurrencyAmountRejections(t *testing.T) {
	if ok, reason := ValidateCurrencyAmount("EU", "1.00"); ok || reason == "" {
		t.Error("a two-letter currency code should be rejected")
	}
	if ok, _ := ValidateCurrencyAmount("JPY", "100.50"); ok {
		t.Error("JPY permits no decimal places")
	}
	if ok, _ := ValidateCurrencyAmount("BHD", "1.234"); !ok {
		t.Error("BHD permits three decimal places")
	}
	if ok, _ := ValidateCurrencyAmount("BHD", "1.2345"); ok {
		t.Error("BHD permits at most three decimal places")
	}
	// An unknown code falls back to two decimals.
	if ok, _ := ValidateCurrencyAmount("XYZ", "1.00"); !ok {
		t.Error("an unknown currency should default to two decimals")
	}
	if ok, _ := ValidateCurrencyAmount("XYZ", "1.000"); ok {
		t.Error("an unknown currency should reject three decimals")
	}
	// An empty amount only checks the code.
	if ok, _ := ValidateCurrencyAmount("EUR", ""); !ok {
		t.Error("an empty amount should pass the code check")
	}
	// An integer amount has no fraction to check.
	if ok, _ := ValidateCurrencyAmount("JPY", "100"); !ok {
		t.Error("an integer JPY amount is valid")
	}
}

func TestLintReportsEveryRule(t *testing.T) {
	doc := `<?xml version="1.0"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">
  <CreDtTm>2026-08-24T10:00:00Z</CreDtTm>
  <IntrBkSttlmDt>2020-01-01</IntrBkSttlmDt>
  <DbtrAcct><Id><IBAN>DE00370400440532013000</IBAN></Id></DbtrAcct>
  <DbtrAgt><FinInstnId><BICFI>NOTABIC</BICFI></FinInstnId></DbtrAgt>
  <CdtrAgt><FinInstnId><BIC>ALSOBAD</BIC></FinInstnId></CdtrAgt>
  <Cdtr><AnyBIC>XX</AnyBIC></Cdtr>
  <PmtId><UETR>not-a-uuid</UETR></PmtId>
  <IntrBkSttlmAmt Ccy="EUR">1.234</IntrBkSttlmAmt>
  <InstdAmt Ccy="JPY">1.5</InstdAmt>
  <Amt Ccy="EU">1</Amt>
  <TtlIntrBkSttlmAmt Ccy="EUR">2.00</TtlIntrBkSttlmAmt>
</Document>`

	res, err := Lint([]byte(doc), "all-rules.xml")
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}

	rules := map[string]bool{}
	for _, i := range res.Issues {
		rules[i.Rule] = true
	}
	for _, want := range []string{
		"ISO 13616 IBAN Checksum",
		"ISO 9362 BIC Format",
		"RFC 4122 UUIDv4 UETR",
		"ISO 4217 Currency Precision",
	} {
		if !rules[want] {
			t.Errorf("rule %q did not fire; got %v", want, rules)
		}
	}
	if res.Warnings == 0 {
		t.Error("the settlement date precedes creation, which should warn")
	}
	if res.Errors == 0 {
		t.Error("expected errors")
	}
}

func TestLintCountsPasses(t *testing.T) {
	doc := `<?xml version="1.0"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">
  <CreDtTm>2026-08-24T10:00:00Z</CreDtTm>
  <IntrBkSttlmDt>2026-08-25</IntrBkSttlmDt>
  <DbtrAcct><Id><IBAN>DE89370400440532013000</IBAN></Id></DbtrAcct>
  <DbtrAgt><FinInstnId><BICFI>DEUTDEDDXXX</BICFI></FinInstnId></DbtrAgt>
  <PmtId><UETR>e1b2c3d4-5678-4abc-8def-1234567890ab</UETR></PmtId>
  <IntrBkSttlmAmt Ccy="EUR">1.00</IntrBkSttlmAmt>
</Document>`

	res, err := Lint([]byte(doc), "clean.xml")
	if err != nil {
		t.Fatal(err)
	}
	if res.Errors != 0 || res.Warnings != 0 {
		t.Errorf("a clean document should report nothing: %+v", res.Issues)
	}
	if res.Passed < 5 {
		t.Errorf("passed = %d, want at least 5", res.Passed)
	}
	if res.FilePath != "clean.xml" {
		t.Errorf("FilePath = %q", res.FilePath)
	}
}

func TestLintRejectsMalformedXML(t *testing.T) {
	if _, err := Lint([]byte("<not-closed>"), "bad.xml"); err == nil {
		t.Error("malformed XML should be an error")
	}
}

func TestLintIgnoresAmountsWithoutCurrency(t *testing.T) {
	doc := `<?xml version="1.0"?><Document><Amt>1.23456</Amt></Document>`
	res, err := Lint([]byte(doc), "x.xml")
	if err != nil {
		t.Fatal(err)
	}
	if res.Errors != 0 {
		t.Errorf("an amount with no Ccy attribute cannot be checked: %+v", res.Issues)
	}
}

func TestLintHandlesUnparseableDates(t *testing.T) {
	doc := `<?xml version="1.0"?>
<Document>
  <CreDtTm>not-a-date</CreDtTm>
  <IntrBkSttlmDt>also-not-a-date</IntrBkSttlmDt>
</Document>`
	if _, err := Lint([]byte(doc), "x.xml"); err != nil {
		t.Errorf("unparseable dates should be skipped, not fatal: %v", err)
	}
}

func TestValidateBICForms(t *testing.T) {
	for _, good := range []string{"DEUTDEFF", "DEUTDEFF500", "NWBKGB2LXXX"} {
		if ok, reason := ValidateBIC(good); !ok {
			t.Errorf("%q should be valid: %s", good, reason)
		}
	}
	for _, bad := range []string{"", "SHORT", "TOOLONGBICVALUE", "1234DEFF", "deutdeff!"} {
		if ok, _ := ValidateBIC(bad); ok {
			t.Errorf("%q should be invalid", bad)
		}
	}
	// Surrounding whitespace and case are normalised.
	if ok, _ := ValidateBIC("  deutdeff  "); !ok {
		t.Error("a padded lower-case BIC should be accepted")
	}
}

func TestValidateUETRForms(t *testing.T) {
	if ok, _ := ValidateUETR(" e1b2c3d4-5678-4abc-8def-1234567890ab "); !ok {
		t.Error("a padded UETR should be accepted")
	}
	for _, bad := range []string{
		"",
		"e1b2c3d4-5678-1abc-8def-1234567890ab", // version 1
		"e1b2c3d4-5678-4abc-cdef-1234567890ab", // bad variant nibble
		"e1b2c3d4567842bc8def1234567890ab",     // no dashes
	} {
		if ok, reason := ValidateUETR(bad); ok || reason == "" {
			t.Errorf("%q should be rejected with a reason", bad)
		}
	}
}
