// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package linter_test

import (
	"strings"
	"testing"

	"github.com/sebastienrousseau/askiso/internal/linter"
)

// A failed checksum is the most common thing a payments team sees, and
// "mod 97 = 59, expected 1" tells them nothing they can act on. These tests pin
// the parts that make the finding actionable: which party, where, and what the
// check digits should have been.

func TestInspectIBANComputesTheRequiredCheckDigits(t *testing.T) {
	// GB89NWKP60161333333333 leaves 59, not 1. The check digits that satisfy
	// ISO 13616 for this account number are 31.
	r := linter.InspectIBAN("GB89NWKP60161333333333")
	if r.Valid {
		t.Fatal("an IBAN with a broken checksum was accepted")
	}
	if r.Remainder != 59 {
		t.Errorf("remainder is %d, want 59", r.Remainder)
	}
	if r.CheckDigits != "89" {
		t.Errorf("check digits read as %q, want 89", r.CheckDigits)
	}
	if r.WantCheckDigits != "31" {
		t.Errorf("required check digits computed as %q, want 31", r.WantCheckDigits)
	}
	if r.Corrected != "GB31NWKP60161333333333" {
		t.Errorf("corrected IBAN is %q", r.Corrected)
	}
	// The correction has to actually pass, or it is worse than saying nothing.
	if fixed := linter.InspectIBAN(r.Corrected); !fixed.Valid {
		t.Errorf("the IBAN this suggests is itself invalid: %s", fixed.Problem)
	}
}

// The derivation must hold for every country's length, not just the one that
// prompted the change.
func TestCorrectedIBANAlwaysValidates(t *testing.T) {
	valid := []string{
		"DE89370400440532013000",
		"FR7630006000011234567890189",
		"GB33BUKB20201555555555",
		"NL91ABNA0417164300",
		"IT60X0542811101000000123456",
		"NO9386011117947",
		"MT84MALT011000012345MTLCAST001S",
	}
	for _, iban := range valid {
		if r := linter.InspectIBAN(iban); !r.Valid {
			t.Errorf("%s is a valid IBAN but was rejected: %s", iban, r.Problem)
			continue
		}
		// Break the check digits deliberately, then see whether the report
		// leads back to the original.
		broken := iban[:2] + wrongDigits(iban[2:4]) + iban[4:]
		r := linter.InspectIBAN(broken)
		if r.Valid {
			t.Errorf("%s should have failed", broken)
			continue
		}
		if r.Corrected != iban {
			t.Errorf("from %s the report suggests %s, want %s", broken, r.Corrected, iban)
		}
	}
}

func wrongDigits(d string) string {
	if d == "00" {
		return "01"
	}
	return "00"
}

// Malformed input must not produce a confident "corrected" IBAN. Suggesting a
// fix for a string that is not an IBAN at all would be worse than the terse
// message this replaced.
func TestInspectIBANSuggestsNothingWhenItCannotKnow(t *testing.T) {
	cases := []struct{ in, contains string }{
		{"GB89", "between 14 and 34"},
		{"1289NWKP60161333333333", "country code"},
		{"GBXXNWKP60161333333333", "check digits"},
		{"GB89NWKP6016133333333!", "only letters and digits"},
	}
	for _, c := range cases {
		r := linter.InspectIBAN(c.in)
		if r.Valid {
			t.Errorf("%q was accepted", c.in)
		}
		if !strings.Contains(r.Problem, c.contains) {
			t.Errorf("%q reported %q, want it to mention %q", c.in, r.Problem, c.contains)
		}
		if r.Corrected != "" {
			t.Errorf("%q produced a suggested IBAN %q, which cannot be known", c.in, r.Corrected)
		}
	}
}

// The whole point of the change: the finding names the party, cites the path,
// and says what to do.
func TestLintIBANFindingIsActionable(t *testing.T) {
	const msg = `<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">
  <FIToFICstmrCdtTrf><CdtTrfTxInf>
    <DbtrAcct><Id><IBAN>GB33BUKB20201555555555</IBAN></Id></DbtrAcct>
    <CdtrAcct><Id><IBAN>GB89NWKP60161333333333</IBAN></Id></CdtrAcct>
  </CdtTrfTxInf></FIToFICstmrCdtTrf></Document>`

	res, err := linter.Lint([]byte(msg), "m.xml")
	if err != nil {
		t.Fatalf("linting: %v", err)
	}
	if len(res.Issues) != 1 {
		t.Fatalf("got %d issues, want 1: %+v", len(res.Issues), res.Issues)
	}
	got := res.Issues[0]

	if !strings.Contains(got.Message, "creditor") {
		t.Errorf("the finding does not say which party: %q", got.Message)
	}
	if got.Path != "/Document/FIToFICstmrCdtTrf/CdtTrfTxInf/CdtrAcct/Id/IBAN" {
		t.Errorf("path is %q", got.Path)
	}
	if got.Expected != "31" || got.Actual != "89" {
		t.Errorf("expected/actual are %q/%q, want 31/89", got.Expected, got.Actual)
	}
	if !strings.Contains(got.Remediation, "GB31NWKP60161333333333") {
		t.Errorf("the fix does not give the corrected IBAN: %q", got.Remediation)
	}
	// It must not claim the check digits are definitely the error, because the
	// arithmetic cannot distinguish that from a mistyped account number.
	if !strings.Contains(got.Remediation, "typo") {
		t.Errorf("the fix does not admit the account number could be wrong: %q", got.Remediation)
	}
	// And the valid debtor IBAN must not be reported.
	if strings.Contains(got.Value, "BUKB") {
		t.Errorf("the valid debtor IBAN was reported: %q", got.Value)
	}
}

// The debtor is named too, so the label is read from the message rather than
// hardcoded to the creditor.
func TestLintNamesTheDebtorWhenTheDebtorIsWrong(t *testing.T) {
	const msg = `<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">
  <FIToFICstmrCdtTrf><CdtTrfTxInf>
    <DbtrAcct><Id><IBAN>GB89NWKP60161333333333</IBAN></Id></DbtrAcct>
  </CdtTrfTxInf></FIToFICstmrCdtTrf></Document>`

	res, err := linter.Lint([]byte(msg), "m.xml")
	if err != nil {
		t.Fatalf("linting: %v", err)
	}
	if len(res.Issues) != 1 {
		t.Fatalf("got %d issues, want 1", len(res.Issues))
	}
	if !strings.Contains(res.Issues[0].Message, "debtor") {
		t.Errorf("the finding does not name the debtor: %q", res.Issues[0].Message)
	}
}

// The message the user actually pasted passes. Keeping it as a case guards
// against a "helpful" message that starts firing on valid input.
func TestTheReportedMessageIsClean(t *testing.T) {
	const msg = `<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">
  <FIToFICstmrCdtTrf><CdtTrfTxInf>
    <PmtId><UETR>e997314e-deb1-45d2-9d07-e3405d31772f</UETR></PmtId>
    <IntrBkSttlmAmt Ccy="EUR">5000.00</IntrBkSttlmAmt>
    <DbtrAcct><Id><IBAN>DE89370400440532013000</IBAN></Id></DbtrAcct>
    <DbtrAgt><FinInstnId><BICFI>DEUTDEDDXXX</BICFI></FinInstnId></DbtrAgt>
    <CdtrAgt><FinInstnId><BICFI>BNPAFRPPXXX</BICFI></FinInstnId></CdtrAgt>
    <CdtrAcct><Id><IBAN>FR7630006000011234567890189</IBAN></Id></CdtrAcct>
  </CdtTrfTxInf></FIToFICstmrCdtTrf></Document>`

	res, err := linter.Lint([]byte(msg), "m.xml")
	if err != nil {
		t.Fatalf("linting: %v", err)
	}
	if res.Errors != 0 {
		t.Errorf("a clean message produced %d error(s): %+v", res.Errors, res.Issues)
	}
}
