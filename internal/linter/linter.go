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
type Issue struct {
	Rule     string        `json:"rule"`
	Severity IssueSeverity `json:"severity"`
	Field    string        `json:"field"`
	Value    string        `json:"value"`
	Message  string        `json:"message"`
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

// ValidateIBAN verifies an IBAN string using the ISO 13616 Modulo-97 algorithm.
func ValidateIBAN(iban string) (bool, string) {
	clean := strings.ToUpper(strings.ReplaceAll(iban, " ", ""))
	if len(clean) < 14 || len(clean) > 34 {
		return false, fmt.Sprintf("invalid IBAN length (%d chars; must be between 14 and 34)", len(clean))
	}

	countryCode := clean[:2]
	for _, ch := range countryCode {
		if ch < 'A' || ch > 'Z' {
			return false, "IBAN must begin with 2-letter country code"
		}
	}

	checkDigits := clean[2:4]
	if _, err := strconv.Atoi(checkDigits); err != nil {
		return false, "IBAN check digits (chars 3-4) must be numeric"
	}

	// Rearrange: move first 4 chars to end
	rearranged := clean[4:] + clean[:4]

	// Convert letters to numbers (A=10, B=11, ..., Z=35)
	var numericStr strings.Builder
	for _, ch := range rearranged {
		if ch >= '0' && ch <= '9' {
			numericStr.WriteRune(ch)
		} else if ch >= 'A' && ch <= 'Z' {
			numericStr.WriteString(strconv.Itoa(int(ch - 'A' + 10)))
		} else {
			return false, fmt.Sprintf("invalid character '%c' in IBAN", ch)
		}
	}

	// Compute modulo 97
	n := new(big.Int)
	n.SetString(numericStr.String(), 10)
	rem := new(big.Int).Mod(n, big.NewInt(97))

	if rem.Int64() != 1 {
		return false, fmt.Sprintf("IBAN checksum validation failed (mod 97 = %d, expected 1)", rem.Int64())
	}

	return true, ""
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

// Lint inspects an ISO 20022 XML byte payload against semantic business rules.
func Lint(data []byte, filename string) (*Result, error) {
	res := &Result{
		FilePath: filename,
		Issues:   make([]Issue, 0),
	}

	decoder := xml.NewDecoder(bytes.NewReader(data))
	var creDtTmStr, sttlmDtStr string

	type elementStackItem struct {
		name string
		ccy  string
	}
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
				if ok, msg := ValidateIBAN(val); !ok {
					res.Issues = append(res.Issues, Issue{
						Rule:     "ISO 13616 IBAN Checksum",
						Severity: SeverityError,
						Field:    "IBAN",
						Value:    val,
						Message:  msg,
					})
					res.Errors++
				} else {
					res.Passed++
				}

			case "BICFI", "BIC", "AnyBIC":
				if ok, msg := ValidateBIC(val); !ok {
					res.Issues = append(res.Issues, Issue{
						Rule:     "ISO 9362 BIC Format",
						Severity: SeverityError,
						Field:    elemName,
						Value:    val,
						Message:  msg,
					})
					res.Errors++
				} else {
					res.Passed++
				}

			case "UETR":
				if ok, msg := ValidateUETR(val); !ok {
					res.Issues = append(res.Issues, Issue{
						Rule:     "RFC 4122 UUIDv4 UETR",
						Severity: SeverityError,
						Field:    "UETR",
						Value:    val,
						Message:  msg,
					})
					res.Errors++
				} else {
					res.Passed++
				}

			case "IntrBkSttlmAmt", "InstdAmt", "Amt", "TtlIntrBkSttlmAmt":
				if currentElem.ccy != "" {
					if ok, msg := ValidateCurrencyAmount(currentElem.ccy, val); !ok {
						res.Issues = append(res.Issues, Issue{
							Rule:     "ISO 4217 Currency Precision",
							Severity: SeverityError,
							Field:    elemName,
							Value:    fmt.Sprintf("%s %s", currentElem.ccy, val),
							Message:  msg,
						})
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
					Message:  fmt.Sprintf("Settlement date (%s) is prior to creation timestamp (%s)", sttlmDtStr, creDtTmStr),
				})
				res.Warnings++
			} else {
				res.Passed++
			}
		}
	}

	return res, nil
}
