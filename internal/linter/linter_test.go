// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package linter

import (
	"testing"
)

func TestValidateIBAN(t *testing.T) {
	validIBANs := []string{
		"DE89370400440532013000",
		"FR7630006000011234567890189",
		"GB29NWBK60161331926819",
	}

	for _, iban := range validIBANs {
		if ok, msg := ValidateIBAN(iban); !ok {
			t.Errorf("Expected IBAN %s to be valid, got error: %s", iban, msg)
		}
	}

	invalidIBANs := []string{
		"DE89370400440532013009", // Corrupt check digits
		"12345678901234",         // No country code
		"DE89",                   // Too short
	}

	for _, iban := range invalidIBANs {
		if ok, _ := ValidateIBAN(iban); ok {
			t.Errorf("Expected IBAN %s to be invalid", iban)
		}
	}
}

func TestValidateBIC(t *testing.T) {
	validBICs := []string{
		"DEUTDEDDXXX",
		"BNPAFRPP",
		"CHASUS33XXX",
	}
	for _, bic := range validBICs {
		if ok, msg := ValidateBIC(bic); !ok {
			t.Errorf("Expected BIC %s to be valid, got: %s", bic, msg)
		}
	}

	invalidBICs := []string{
		"DEUT",
		"12345678",
		"DEUTDEDDXXXXX",
	}
	for _, bic := range invalidBICs {
		if ok, _ := ValidateBIC(bic); ok {
			t.Errorf("Expected BIC %s to be invalid", bic)
		}
	}
}

func TestValidateUETR(t *testing.T) {
	validUETR := "f81d4fae-7dec-11d0-a765-00a0c91e6bf6" // UUIDv1 or UUIDv4
	validV4 := "9b1deb4d-3b7d-4bad-9bdd-2b0d7b3dcb6d"

	if ok, _ := ValidateUETR(validV4); !ok {
		t.Errorf("Expected valid UUIDv4 %s", validV4)
	}

	invalidUETR := "not-a-uuid"
	if ok, _ := ValidateUETR(invalidUETR); ok {
		t.Errorf("Expected %s to fail", invalidUETR)
	}
	_ = validUETR
}

func TestValidateCurrencyAmount(t *testing.T) {
	if ok, msg := ValidateCurrencyAmount("EUR", "100.50"); !ok {
		t.Errorf("EUR 100.50 should be valid: %s", msg)
	}
	if ok, msg := ValidateCurrencyAmount("JPY", "100"); !ok {
		t.Errorf("JPY 100 should be valid: %s", msg)
	}
	if ok, _ := ValidateCurrencyAmount("JPY", "100.50"); ok {
		t.Errorf("JPY 100.50 should fail (0 decimals expected)")
	}
}

func TestLintXML(t *testing.T) {
	sampleXML := `<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">
  <FIToFICstmrCdtTrf>
    <GrpHdr>
      <MsgId>MSG-12345</MsgId>
      <CreDtTm>2026-08-23T12:00:00Z</CreDtTm>
    </GrpHdr>
    <CdtTrfTxInf>
      <PmtId>
        <UETR>9b1deb4d-3b7d-4bad-9bdd-2b0d7b3dcb6d</UETR>
      </PmtId>
      <IntrBkSttlmAmt Ccy="EUR">5000.00</IntrBkSttlmAmt>
      <IntrBkSttlmDt>2026-08-23</IntrBkSttlmDt>
      <DbtrAcct>
        <Id>
          <IBAN>DE89370400440532013000</IBAN>
        </Id>
      </DbtrAcct>
      <DbtrAgt>
        <FinInstnId>
          <BICFI>DEUTDEDDXXX</BICFI>
        </FinInstnId>
      </DbtrAgt>
    </CdtTrfTxInf>
  </FIToFICstmrCdtTrf>
</Document>`

	res, err := Lint([]byte(sampleXML), "test.xml")
	if err != nil {
		t.Fatalf("Lint failed: %v", err)
	}
	if res.Errors > 0 {
		t.Errorf("Expected 0 errors on valid message, got %d", res.Errors)
	}
	if res.Passed == 0 {
		t.Errorf("Expected passed checks")
	}
}
