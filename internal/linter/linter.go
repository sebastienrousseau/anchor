// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package linter

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// IssueSeverity represents the severity level of a lint issue.
type IssueSeverity string

const (
	SeverityError   IssueSeverity = "ERROR"
	SeverityWarning IssueSeverity = "WARNING"
	SeverityInfo    IssueSeverity = "INFO"
)

// Issue represents a single rule violation or warning.
//
// Path and Remediation exist because a finding that states only what is wrong
// leaves the reader to work out where it is and what to do about it. The scheme
// rule findings have carried both from the start; lint findings render in the
// same lists, and used to arrive with neither.
type Issue struct {
	Rule     string        `json:"rule"`
	Severity IssueSeverity `json:"severity"`
	Field    string        `json:"field"`
	Value    string        `json:"value"`
	Message  string        `json:"message"`

	// Path is the XPath of the element that failed. It is the other half of
	// the citation: the rule says what, the path says where.
	Path string `json:"path,omitempty"`

	// Expected and Actual are filled where the check has a definite right
	// answer to compare against, and left empty where it does not.
	Expected string `json:"expected,omitempty"`
	Actual   string `json:"actual,omitempty"`

	// Remediation says what to change. Where the fix is ambiguous it says so
	// rather than picking one and sounding certain.
	Remediation string `json:"remediation,omitempty"`
}

// Result holds the complete linting analysis for an XML instance.
type Result struct {
	FilePath string  `json:"file_path"`
	Issues   []Issue `json:"issues"`
	Passed   int     `json:"passed_count"`
	Errors   int     `json:"error_count"`
	Warnings int     `json:"warning_count"`
}

var (
	bicRegex  = regexp.MustCompile(`^[A-Z]{4}[A-Z]{2}[A-Z0-9]{2}([A-Z0-9]{3})?$`)
	uetrRegex = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-4[0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)

	// ISO 4217 known currency decimals mapping
	currencyDecimals = map[string]int{
		"EUR": 2, "USD": 2, "GBP": 2, "CHF": 2, "CAD": 2, "AUD": 2,
		"JPY": 0, "KRW": 0, "BHD": 3, "KWD": 3, "OMR": 3, "JOD": 3,
		"SEK": 2, "NOK": 2, "DKK": 2, "PLN": 2, "SGD": 2, "HKD": 2,
		"CNY": 2, "INR": 2, "BRL": 2, "ZAR": 2, "NZD": 2, "MXN": 2,
	}
)

// IBANReport is the outcome of inspecting one IBAN.
//
// It exists because "mod 97 = 59, expected 1" is a true statement that helps
// nobody. ISO 13616 derives the two check digits from the country code and the
// account number, which means that when the arithmetic fails there is exactly
// one correct pair of check digits for the account number as written -- and it
// can be computed, not guessed. Reporting it turns a failed sum into an
// actionable instruction.
type IBANReport struct {
	Valid bool

	// Problem is a short statement of what is wrong, in the reader's terms.
	Problem string

	// Country and CheckDigits are the parts as written.
	Country     string
	CheckDigits string

	// WantCheckDigits is the pair ISO 13616 requires for this account number.
	// Empty when the IBAN is malformed enough that the question is moot.
	WantCheckDigits string

	// Corrected is the whole IBAN with the required check digits substituted.
	Corrected string

	// Remainder is the mod-97 result. Kept because somebody reproducing the
	// check by hand wants to compare against it.
	Remainder int64
}

// InspectIBAN checks an IBAN under the ISO 13616 mod-97 algorithm and reports
// what it found.
func InspectIBAN(iban string) IBANReport {
	clean := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(iban), " ", ""))

	if len(clean) < 14 || len(clean) > 34 {
		return IBANReport{Problem: fmt.Sprintf(
			"it is %d characters long; an IBAN is between 14 and 34", len(clean))}
	}

	country := clean[:2]
	for _, ch := range country {
		if ch < 'A' || ch > 'Z' {
			return IBANReport{Problem: fmt.Sprintf(
				"it starts with %q; an IBAN starts with a two-letter country code", clean[:2])}
		}
	}

	check := clean[2:4]
	if _, err := strconv.Atoi(check); err != nil {
		return IBANReport{Country: country, Problem: fmt.Sprintf(
			"its third and fourth characters are %q; those two are the check digits and must be numeric", check)}
	}

	rem, err := mod97(clean)
	if err != nil {
		return IBANReport{Country: country, CheckDigits: check, Problem: err.Error()}
	}
	if rem == 1 {
		return IBANReport{Valid: true, Country: country, CheckDigits: check, Remainder: 1}
	}

	// The check digits that would satisfy the algorithm for this account
	// number. Substituting "00" and taking 98 - (n mod 97) is the standard
	// derivation, and it is exact -- there is no second candidate.
	want := ""
	corrected := ""
	if zeroed, err := mod97(clean[:2] + "00" + clean[4:]); err == nil {
		want = fmt.Sprintf("%02d", 98-zeroed)
		corrected = clean[:2] + want + clean[4:]
	}

	return IBANReport{
		Country:         country,
		CheckDigits:     check,
		WantCheckDigits: want,
		Corrected:       corrected,
		Remainder:       rem,
		Problem: fmt.Sprintf(
			"its check digits are %s, but this account number requires %s", check, want),
	}
}

// mod97 applies the ISO 13616 transformation: move the first four characters to
// the end, map letters to numbers (A=10 ... Z=35), and take the remainder
// modulo 97. A valid IBAN leaves 1.
func mod97(clean string) (int64, error) {
	var b strings.Builder
	for _, ch := range clean[4:] + clean[:4] {
		switch {
		case ch >= '0' && ch <= '9':
			b.WriteRune(ch)
		case ch >= 'A' && ch <= 'Z':
			b.WriteString(strconv.Itoa(int(ch - 'A' + 10)))
		default:
			return 0, fmt.Errorf("it contains %q, and an IBAN holds only letters and digits", string(ch))
		}
	}
	n, ok := new(big.Int).SetString(b.String(), 10)
	if !ok {
		return 0, fmt.Errorf("its characters could not be read as a number")
	}
	return new(big.Int).Mod(n, big.NewInt(97)).Int64(), nil
}

// ValidateIBAN verifies an IBAN string using the ISO 13616 Modulo-97 algorithm.
//
// It answers the old two-value shape that the public API and the browser build
// use. InspectIBAN carries the detail the linter needs.
func ValidateIBAN(iban string) (bool, string) {
	r := InspectIBAN(iban)
	if r.Valid {
		return true, ""
	}
	return false, "this IBAN is not valid: " + r.Problem
}

// ValidateBIC verifies an ISO 9362 Business Identifier Code.
func ValidateBIC(bic string) (bool, string) {
	clean := strings.ToUpper(strings.TrimSpace(bic))
	if !bicRegex.MatchString(clean) {
		return false, fmt.Sprintf("invalid BIC format '%s' (must be 8 or 11 characters: 4 bank + 2 country + 2 location + optional 3 branch)", bic)
	}
	return true, ""
}

// ValidateUETR verifies an RFC 4122 Version 4 UUID.
func ValidateUETR(uetr string) (bool, string) {
	clean := strings.TrimSpace(uetr)
	if !uetrRegex.MatchString(clean) {
		return false, fmt.Sprintf("invalid UETR '%s' (must be RFC 4122 Version 4 UUID format xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx)", uetr)
	}
	return true, ""
}

// ValidateCurrencyAmount verifies ISO 4217 currency code and decimal fraction count.
func ValidateCurrencyAmount(ccy, amtStr string) (bool, string) {
	cleanCCY := strings.ToUpper(strings.TrimSpace(ccy))
	if len(cleanCCY) != 3 {
		return false, fmt.Sprintf("invalid ISO 4217 currency code '%s' (must be 3 letters)", ccy)
	}

	expectedDecimals, known := currencyDecimals[cleanCCY]
	if !known {
		expectedDecimals = 2 // Default expectation
	}

	if amtStr != "" {
		parts := strings.Split(amtStr, ".")
		var actualDecimals int
		if len(parts) == 2 {
			actualDecimals = len(parts[1])
		} else {
			actualDecimals = 0
		}

		if actualDecimals > expectedDecimals {
			return false, fmt.Sprintf("currency %s permits maximum %d decimal places (got %d in '%s')", cleanCCY, expectedDecimals, actualDecimals, amtStr)
		}
	}

	return true, ""
}

// elementStackItem is one open element in the walk: its name, and the Ccy
// attribute if it carried one.
type elementStackItem struct {
	name string
	ccy  string
}

// partyOf names the party an element belongs to, in the words somebody
// reconciling a payment would use. An IBAN finding that says "the creditor's"
// is found in seconds; one that says only "IBAN" has to be hunted for in a
// message that may carry four of them.
var partyOf = map[string]string{
	"CdtrAcct":       "the creditor's",
	"DbtrAcct":       "the debtor's",
	"IntrmyAgt1Acct": "the first intermediary agent's",
	"IntrmyAgt2Acct": "the second intermediary agent's",
	"IntrmyAgt3Acct": "the third intermediary agent's",
	"CdtrAgtAcct":    "the creditor agent's",
	"DbtrAgtAcct":    "the debtor agent's",
	"Cdtr":           "the creditor's",
	"Dbtr":           "the debtor's",
	"CdtrAgt":        "the creditor agent's",
	"DbtrAgt":        "the debtor agent's",
	"InstgAgt":       "the instructing agent's",
	"InstdAgt":       "the instructed agent's",
}

// owner walks outwards from the current element to the nearest ancestor that
// names a party, and returns a possessive for it. Empty when nothing on the
// stack identifies one, which is better than inventing a label.
func owner(stack []elementStackItem) string {
	for i := len(stack) - 1; i >= 0; i-- {
		if who, found := partyOf[stack[i].name]; found {
			return who
		}
	}
	return ""
}

// pathOf renders the element stack as an XPath, matching the form the scheme
// rule findings use so both kinds of finding cite a location the same way.
func pathOf(stack []elementStackItem) string {
	var b strings.Builder
	for _, item := range stack {
		b.WriteByte('/')
		b.WriteString(item.name)
	}
	return b.String()
}

// RuleNames lists every rule the linter can report, which is what makes it
// possible to prove the test table covers all of them. A check added without a
// case here shows up as a failure rather than as silence.
func RuleNames() []string {
	return []string{
		"ISO 13616 IBAN Checksum",
		"ISO 9362 BIC Format",
		"RFC 4122 UUIDv4 UETR",
		"ISO 4217 Currency Precision",
		"Temporal Sequence Sanity",
	}
}

// Lint inspects an ISO 20022 XML byte payload against semantic business rules.
func Lint(data []byte, filename string) (*Result, error) {
	res := &Result{
		FilePath: filename,
		Issues:   make([]Issue, 0),
	}

	decoder := xml.NewDecoder(bytes.NewReader(data))
	var creDtTmStr, sttlmDtStr string

	// An empty file and a JSON payload both walk to EOF without producing a
	// single element, and used to come back as "no findings" -- indistinguishable
	// from a clean message. That is the most dangerous answer a checker can give,
	// because a pipeline reads it as a pass.
	var sawElement bool

	var stack []elementStackItem

	for {
		tok, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("xml parse error: %w", err)
		}

		switch elem := tok.(type) {
		case xml.StartElement:
			var ccyAttr string
			for _, attr := range elem.Attr {
				if attr.Name.Local == "Ccy" || attr.Name.Local == "ccy" {
					ccyAttr = attr.Value
				}
			}
			sawElement = true
			stack = append(stack, elementStackItem{name: elem.Name.Local, ccy: ccyAttr})

		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}

		case xml.CharData:
			val := strings.TrimSpace(string(elem))
			if val == "" || len(stack) == 0 {
				continue
			}

			currentElem := stack[len(stack)-1]
			elemName := currentElem.name

			switch elemName {
			case "IBAN":
				if r := InspectIBAN(val); !r.Valid {
					who := owner(stack)
					if who == "" {
						who = "an"
					}
					issue := Issue{
						Rule:     "ISO 13616 IBAN Checksum",
						Severity: SeverityError,
						Field:    "IBAN",
						Value:    val,
						Path:     pathOf(stack),
						Message:  fmt.Sprintf("%s IBAN is not valid: %s", who, r.Problem),
					}

					// When the check digits are the only thing wrong there is a
					// single correct answer, and saying it is the whole point.
					// But the arithmetic cannot tell a mistyped check digit from
					// a mistyped account number -- so the fix names both, rather
					// than sounding certain about the one it happens to compute.
					if r.WantCheckDigits != "" {
						issue.Expected = r.WantCheckDigits
						issue.Actual = r.CheckDigits
						issue.Remediation = fmt.Sprintf(
							"If the account number is right, the IBAN is %s. "+
								"If the check digits are right, the account number has a typo instead. "+
								"ISO 13616 derives the check digits from the country code and account "+
								"number, so exactly one of the two is wrong — confirm the account "+
								"with the beneficiary before changing it.",
							r.Corrected)
					} else {
						issue.Remediation = "Correct the IBAN to the ISO 13616 form: " +
							"two-letter country code, two check digits, then the national account number."
					}

					res.Issues = append(res.Issues, issue)
					res.Errors++
				} else {
					res.Passed++
				}

			case "BICFI", "BIC", "AnyBIC":
				if ok, _ := ValidateBIC(val); !ok {
					who := owner(stack)
					if who == "" {
						who = "a"
					}
					res.Issues = append(res.Issues, Issue{
						Rule:     "ISO 9362 BIC Format",
						Severity: SeverityError,
						Field:    elemName,
						Value:    val,
						Path:     pathOf(stack),
						Expected: "8 or 11 characters",
						Actual:   fmt.Sprintf("%d characters", len(strings.TrimSpace(val))),
						Message: fmt.Sprintf(
							"%s BIC %q is not an ISO 9362 code", who, strings.TrimSpace(val)),
						Remediation: "A BIC is 4 letters for the institution, 2 for the country, " +
							"2 for the location, and optionally 3 more for the branch — 8 or 11 " +
							"characters, no spaces. Use XXX as the branch to name the head office.",
					})
					res.Errors++
				} else {
					res.Passed++
				}

			case "UETR":
				if ok, _ := ValidateUETR(val); !ok {
					res.Issues = append(res.Issues, Issue{
						Rule:     "RFC 4122 UUIDv4 UETR",
						Severity: SeverityError,
						Field:    "UETR",
						Value:    val,
						Path:     pathOf(stack),
						Expected: "an RFC 4122 version 4 UUID",
						Message: fmt.Sprintf(
							"the UETR %q is not a version 4 UUID", strings.TrimSpace(val)),
						Remediation: "A UETR is 32 hexadecimal digits in 8-4-4-4-12 form. The first " +
							"digit of the third group must be 4, and the first of the fourth group " +
							"must be 8, 9, a or b. Generate it once at origination and carry it " +
							"unchanged through every message in the payment.",
					})
					res.Errors++
				} else {
					res.Passed++
				}

			case "IntrBkSttlmAmt", "InstdAmt", "Amt", "TtlIntrBkSttlmAmt":
				if currentElem.ccy != "" {
					if ok, msg := ValidateCurrencyAmount(currentElem.ccy, val); !ok {
						ccy := strings.ToUpper(strings.TrimSpace(currentElem.ccy))
						places, known := currencyDecimals[ccy]
						issue := Issue{
							Rule:     "ISO 4217 Currency Precision",
							Severity: SeverityError,
							Field:    elemName,
							Value:    fmt.Sprintf("%s %s", currentElem.ccy, val),
							Path:     pathOf(stack),
							Message:  msg,
						}
						if known {
							issue.Expected = fmt.Sprintf("at most %d decimal place(s)", places)
							issue.Actual = val
							issue.Message = fmt.Sprintf(
								"the amount %s %s has more decimal places than %s has minor units",
								ccy, val, ccy)
							switch places {
							case 0:
								issue.Remediation = fmt.Sprintf(
									"%s has no minor unit, so the amount is a whole number: write %s.",
									ccy, strings.SplitN(val, ".", 2)[0])
							default:
								issue.Remediation = fmt.Sprintf(
									"%s is quoted to %d decimal place(s). Round or truncate at the "+
										"source that produced the amount rather than here — a value "+
										"trimmed in the message no longer matches the ledger it came from.",
									ccy, places)
							}
						}
						res.Issues = append(res.Issues, issue)
						res.Errors++
					} else {
						res.Passed++
					}
				}

			case "CreDtTm":
				creDtTmStr = val
			case "IntrBkSttlmDt":
				sttlmDtStr = val
			}
		}
	}

	if !sawElement {
		return nil, fmt.Errorf("no XML elements found in %s: this is not an ISO 20022 message", filename)
	}

	// Temporal check
	if creDtTmStr != "" && sttlmDtStr != "" {
		creDt, err1 := time.Parse(time.RFC3339, creDtTmStr)
		if err1 != nil {
			creDt, _ = time.Parse("2006-01-02T15:04:05", creDtTmStr)
		}
		sttlmDt, err2 := time.Parse("2006-01-02", sttlmDtStr)
		if err1 == nil && err2 == nil {
			// Settlement date shouldn't be strictly earlier than creation date (ignoring timezone)
			if sttlmDt.Before(creDt.AddDate(0, 0, -1)) {
				res.Issues = append(res.Issues, Issue{
					Rule:     "Temporal Sequence Sanity",
					Severity: SeverityWarning,
					Field:    "IntrBkSttlmDt",
					Value:    sttlmDtStr,
					Expected: "on or after the creation timestamp",
					Actual:   sttlmDtStr,
					Message: fmt.Sprintf(
						"the settlement date %s is before the message was created (%s)",
						sttlmDtStr, creDtTmStr),
					Remediation: "A payment cannot settle before it was instructed. Either the " +
						"settlement date is wrong, or <CreDtTm> carries the wrong timestamp — " +
						"a stale clock on the originating system is the usual cause.",
				})
				res.Warnings++
			} else {
				res.Passed++
			}
		}
	}

	return res, nil
}
