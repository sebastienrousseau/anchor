// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package linter_test

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/sebastienrousseau/askiso/internal/linter"
)

// The contract every lint finding has to keep.
//
// AskISO is meant to be self-service: somebody who is not an ISO 20022 expert
// should be able to act on a finding without asking one. That only holds if
// every finding, from every check, carries the same four things -- what is
// wrong, where, what was expected, and what to change. A single check that
// forgets one of them is the one that sends somebody to a forum.
//
// This is a table over every error class the linter can produce. A new check
// added without a case here fails TestEveryRuleIsCovered below.

func doc(body string) []byte {
	return []byte(`<?xml version="1.0" encoding="UTF-8"?>` +
		`<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">` +
		`<FIToFICstmrCdtTrf><CdtTrfTxInf>` + body +
		`</CdtTrfTxInf></FIToFICstmrCdtTrf></Document>`)
}

type lintCase struct {
	name string
	// rule is the identifier the finding must cite.
	rule string
	body string
	// wantPath is the exact XPath the finding must point at.
	wantPath string
	// wantExpected and wantActual are checked when the rule has a definite
	// right answer; empty means the case does not assert them.
	wantExpected string
	wantActual   string
	// mustSay are fragments the message or remediation has to contain, in the
	// reader's language rather than the implementation's.
	mustSay []string
	// mustNotSay catches regressions into jargon or false certainty.
	mustNotSay []string
}

var lintCases = []lintCase{
	{
		name:         "IBAN with wrong check digits names the party and the correction",
		rule:         "ISO 13616 IBAN Checksum",
		body:         `<CdtrAcct><Id><IBAN>GB89NWKP60161333333333</IBAN></Id></CdtrAcct>`,
		wantPath:     "/Document/FIToFICstmrCdtTrf/CdtTrfTxInf/CdtrAcct/Id/IBAN",
		wantExpected: "31",
		wantActual:   "89",
		mustSay:      []string{"creditor", "GB31NWKP60161333333333", "typo"},
		mustNotSay:   []string{"mod 97", "validation failed"},
	},
	{
		name:       "IBAN too short is reported without inventing a correction",
		rule:       "ISO 13616 IBAN Checksum",
		body:       `<DbtrAcct><Id><IBAN>GB89NW</IBAN></Id></DbtrAcct>`,
		wantPath:   "/Document/FIToFICstmrCdtTrf/CdtTrfTxInf/DbtrAcct/Id/IBAN",
		mustSay:    []string{"debtor", "14 and 34"},
		mustNotSay: []string{"the IBAN is GB"},
	},
	{
		name:       "IBAN with a non-alphabetic country code",
		rule:       "ISO 13616 IBAN Checksum",
		body:       `<CdtrAcct><Id><IBAN>1289NWKP60161333333333</IBAN></Id></CdtrAcct>`,
		wantPath:   "/Document/FIToFICstmrCdtTrf/CdtTrfTxInf/CdtrAcct/Id/IBAN",
		mustSay:    []string{"country code"},
		mustNotSay: []string{"the IBAN is"},
	},
	{
		name:       "IBAN containing a symbol",
		rule:       "ISO 13616 IBAN Checksum",
		body:       `<CdtrAcct><Id><IBAN>GB89NWKP6016133333333!</IBAN></Id></CdtrAcct>`,
		wantPath:   "/Document/FIToFICstmrCdtTrf/CdtTrfTxInf/CdtrAcct/Id/IBAN",
		mustSay:    []string{"letters and digits"},
		mustNotSay: []string{"the IBAN is"},
	},
	{
		name:         "BIC of the wrong length says what a BIC is",
		rule:         "ISO 9362 BIC Format",
		body:         `<CdtrAgt><FinInstnId><BICFI>DEUTDE</BICFI></FinInstnId></CdtrAgt>`,
		wantPath:     "/Document/FIToFICstmrCdtTrf/CdtTrfTxInf/CdtrAgt/FinInstnId/BICFI",
		wantExpected: "8 or 11 characters",
		wantActual:   "6 characters",
		mustSay:      []string{"creditor agent", "institution", "country", "branch"},
		mustNotSay:   []string{"regex", "bicRegex"},
	},
	{
		name:       "UETR that is not a version 4 UUID explains the shape",
		rule:       "RFC 4122 UUIDv4 UETR",
		body:       `<PmtId><UETR>e1b2c3d4-5678-1abc-8def-1234567890ab</UETR></PmtId>`,
		wantPath:   "/Document/FIToFICstmrCdtTrf/CdtTrfTxInf/PmtId/UETR",
		mustSay:    []string{"version 4", "8-4-4-4-12", "unchanged"},
		mustNotSay: []string{"regex"},
	},
	{
		name:         "too many decimals for the currency",
		rule:         "ISO 4217 Currency Precision",
		body:         `<IntrBkSttlmAmt Ccy="EUR">1000.123</IntrBkSttlmAmt>`,
		wantPath:     "/Document/FIToFICstmrCdtTrf/CdtTrfTxInf/IntrBkSttlmAmt",
		wantExpected: "at most 2 decimal place(s)",
		mustSay:      []string{"minor unit", "ledger"},
	},
	{
		name:         "a currency with no minor unit at all",
		rule:         "ISO 4217 Currency Precision",
		body:         `<IntrBkSttlmAmt Ccy="JPY">1000.50</IntrBkSttlmAmt>`,
		wantPath:     "/Document/FIToFICstmrCdtTrf/CdtTrfTxInf/IntrBkSttlmAmt",
		wantExpected: "at most 0 decimal place(s)",
		mustSay:      []string{"no minor unit", "whole number", "1000"},
	},
}

func TestEveryFindingIsActionable(t *testing.T) {
	for _, c := range lintCases {
		t.Run(c.name, func(t *testing.T) {
			res, err := linter.Lint(doc(c.body), "m.xml")
			if err != nil {
				t.Fatalf("linting: %v", err)
			}
			noteExercised(res.Issues)

			var got *linter.Issue
			for i := range res.Issues {
				if res.Issues[i].Rule == c.rule {
					got = &res.Issues[i]
					break
				}
			}
			if got == nil {
				t.Fatalf("no %s finding; got %+v", c.rule, res.Issues)
			}

			// Where. Without it the reader is searching a document by hand.
			if got.Path != c.wantPath {
				t.Errorf("path is %q, want %q", got.Path, c.wantPath)
			}
			// What to do. This is the whole point of the contract.
			if strings.TrimSpace(got.Remediation) == "" {
				t.Error("the finding carries no remediation")
			}
			// The value that failed, so it can be found in the source system.
			if got.Value == "" {
				t.Error("the finding does not report the value that failed")
			}
			if got.Severity == "" {
				t.Error("the finding has no severity")
			}

			if c.wantExpected != "" && got.Expected != c.wantExpected {
				t.Errorf("expected is %q, want %q", got.Expected, c.wantExpected)
			}
			if c.wantActual != "" && got.Actual != c.wantActual {
				t.Errorf("actual is %q, want %q", got.Actual, c.wantActual)
			}

			text := got.Message + " " + got.Remediation
			for _, want := range c.mustSay {
				if !strings.Contains(text, want) {
					t.Errorf("the finding never mentions %q:\n  %s\n  %s",
						want, got.Message, got.Remediation)
				}
			}
			for _, unwanted := range c.mustNotSay {
				if strings.Contains(text, unwanted) {
					t.Errorf("the finding still says %q:\n  %s\n  %s",
						unwanted, got.Message, got.Remediation)
				}
			}
		})
	}
}

// A message with several different defects must report all of them. Stopping at
// the first turns one round of fixing into four.
func TestAllDefectsAreReportedTogether(t *testing.T) {
	body := `<PmtId><UETR>not-a-uuid</UETR></PmtId>` +
		`<IntrBkSttlmAmt Ccy="EUR">1.234</IntrBkSttlmAmt>` +
		`<DbtrAcct><Id><IBAN>GB89NWKP60161333333333</IBAN></Id></DbtrAcct>` +
		`<CdtrAgt><FinInstnId><BICFI>NOPE</BICFI></FinInstnId></CdtrAgt>`

	res, err := linter.Lint(doc(body), "m.xml")
	if err != nil {
		t.Fatalf("linting: %v", err)
	}

	want := map[string]bool{
		"ISO 13616 IBAN Checksum":     false,
		"ISO 9362 BIC Format":         false,
		"RFC 4122 UUIDv4 UETR":        false,
		"ISO 4217 Currency Precision": false,
	}
	for _, issue := range res.Issues {
		if _, tracked := want[issue.Rule]; tracked {
			want[issue.Rule] = true
		}
	}
	for rule, seen := range want {
		if !seen {
			t.Errorf("%s was not reported alongside the others", rule)
		}
	}

	// Every finding in a multi-defect message still has to carry a distinct
	// path, or two findings become indistinguishable in a list.
	paths := map[string]int{}
	for _, issue := range res.Issues {
		paths[issue.Path]++
	}
	for path, n := range paths {
		if path == "" {
			t.Error("a finding in a multi-defect message has no path")
		}
		if n > 1 {
			t.Errorf("%d findings share the path %s", n, path)
		}
	}
}

// Valid input must stay silent. A checker that cries wolf is one people learn
// to ignore, and then it catches nothing.
func TestValidValuesProduceNoFindings(t *testing.T) {
	body := `<PmtId><UETR>e997314e-deb1-45d2-9d07-e3405d31772f</UETR></PmtId>` +
		`<IntrBkSttlmAmt Ccy="EUR">5000.00</IntrBkSttlmAmt>` +
		`<DbtrAcct><Id><IBAN>DE89370400440532013000</IBAN></Id></DbtrAcct>` +
		`<DbtrAgt><FinInstnId><BICFI>DEUTDEDDXXX</BICFI></FinInstnId></DbtrAgt>` +
		`<CdtrAgt><FinInstnId><BICFI>BNPAFRPP</BICFI></FinInstnId></CdtrAgt>` +
		`<CdtrAcct><Id><IBAN>FR7630006000011234567890189</IBAN></Id></CdtrAcct>`

	res, err := linter.Lint(doc(body), "m.xml")
	if err != nil {
		t.Fatalf("linting: %v", err)
	}
	if res.Errors != 0 {
		t.Errorf("a clean message produced %d error(s): %+v", res.Errors, res.Issues)
	}
	if res.Passed == 0 {
		t.Error("no checks were recorded as passing, so nothing was actually inspected")
	}
}

// Every party label the linter knows must actually be reachable, or the map is
// a promise the walk cannot keep.
func TestPartyLabelsAreReachable(t *testing.T) {
	cases := []struct{ wrapper, want string }{
		{"CdtrAcct", "creditor"},
		{"DbtrAcct", "debtor"},
		{"IntrmyAgt1Acct", "first intermediary agent"},
		{"IntrmyAgt2Acct", "second intermediary agent"},
		{"IntrmyAgt3Acct", "third intermediary agent"},
		{"CdtrAgtAcct", "creditor agent"},
		{"DbtrAgtAcct", "debtor agent"},
	}
	for _, c := range cases {
		t.Run(c.wrapper, func(t *testing.T) {
			body := "<" + c.wrapper + "><Id><IBAN>GB89NWKP60161333333333</IBAN></Id></" + c.wrapper + ">"
			res, err := linter.Lint(doc(body), "m.xml")
			if err != nil {
				t.Fatalf("linting: %v", err)
			}
			if len(res.Issues) == 0 {
				t.Fatal("no finding was produced")
			}
			if !strings.Contains(res.Issues[0].Message, c.want) {
				t.Errorf("under <%s> the finding says %q, want it to name the %s",
					c.wrapper, res.Issues[0].Message, c.want)
			}
		})
	}
}

// An IBAN outside any recognised party still reports, rather than being skipped
// because the walk could not label it.
func TestAnUnlabelledPartyStillReports(t *testing.T) {
	res, err := linter.Lint(doc(`<SomeOtherAcct><Id><IBAN>GB89NWKP60161333333333</IBAN></Id></SomeOtherAcct>`), "m.xml")
	if err != nil {
		t.Fatalf("linting: %v", err)
	}
	if len(res.Issues) != 1 {
		t.Fatalf("got %d findings, want 1", len(res.Issues))
	}
	if res.Issues[0].Remediation == "" {
		t.Error("an unlabelled party lost its remediation")
	}
}

// Malformed XML is an error, not a silent pass. A tool that reports "no
// findings" for a document it could not read is actively dangerous.
func TestUnparseableInputIsAnError(t *testing.T) {
	for _, bad := range []string{"", "<not xml", "{\"json\": true}", "<a><b></a>"} {
		if _, err := linter.Lint([]byte(bad), "m.xml"); err == nil {
			t.Errorf("%q was accepted as a document", bad)
		}
	}
}

// exercised records every rule identifier that any test in this package saw the
// linter actually produce. It is checked once, after the whole suite has run,
// so the guarantee does not depend on the order tests happen to execute in --
// which the earlier version of this check did, and got wrong.
var exercised = map[string]bool{}

func noteExercised(issues []linter.Issue) {
	for _, i := range issues {
		exercised[i.Rule] = true
	}
}

// TestMain proves that every rule the linter can report was genuinely produced
// by at least one test. A new check added without a case fails here rather than
// shipping with no evidence that its finding is actionable.
func TestMain(m *testing.M) {
	code := m.Run()
	if code == 0 {
		for _, rule := range linter.RuleNames() {
			if !exercised[rule] {
				fmt.Fprintf(os.Stderr,
					"regression gap: %q can be reported but no test ever produced it\n", rule)
				code = 1
			}
		}
	}
	os.Exit(code)
}

func TestTemporalWarningExplainsTheCause(t *testing.T) {
	msg := []byte(`<?xml version="1.0" encoding="UTF-8"?>` +
		`<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">` +
		`<FIToFICstmrCdtTrf><GrpHdr><CreDtTm>2026-08-26T09:00:00Z</CreDtTm></GrpHdr>` +
		`<CdtTrfTxInf><IntrBkSttlmDt>2026-08-01</IntrBkSttlmDt></CdtTrfTxInf>` +
		`</FIToFICstmrCdtTrf></Document>`)

	res, err := linter.Lint(msg, "m.xml")
	if err != nil {
		t.Fatalf("linting: %v", err)
	}
	noteExercised(res.Issues)

	var found *linter.Issue
	for i := range res.Issues {
		if res.Issues[i].Rule == "Temporal Sequence Sanity" {
			found = &res.Issues[i]
		}
	}
	if found == nil {
		t.Fatalf("a settlement date before the creation timestamp was not reported: %+v", res.Issues)
	}
	if found.Severity != linter.SeverityWarning {
		t.Errorf("severity is %q, want a warning: a date order this tool cannot verify "+
			"against the scheme should not fail a build", found.Severity)
	}
	if found.Remediation == "" {
		t.Error("the warning carries no remediation")
	}
	if !strings.Contains(found.Remediation, "clock") {
		t.Errorf("the remediation does not name the usual cause: %q", found.Remediation)
	}
}
