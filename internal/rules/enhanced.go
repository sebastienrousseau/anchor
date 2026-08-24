// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package rules

import (
	"fmt"
	"strings"

	"github.com/sebastienrousseau/askiso/internal/converter"
)

// 14 November 2026 ends the unstructured address. November 2027 is the next
// step: the point at which the data a payment carries has to be usable by a
// machine rather than merely present. CHAPS mandates purpose codes and legal
// entity identifiers for the payment types it names, and structured remittance
// information replaces free text.
//
// These rules are warnings today and become errors on the dates they name. A
// tool that reported them as errors now would be wrong; one that stayed silent
// would leave the work until it is urgent.
//
// References:
//   https://www.bankofengland.co.uk/payment-and-settlement/rtgs-renewal-programme
//   https://www.swift.com/standards/iso-20022

// PurposeCodePresent asks for a purpose code on a customer credit transfer.
var PurposeCodePresent = Rule{
	ID:       "ENH-PURP-001",
	Name:     "Purpose code present",
	Severity: SeverityWarning,
	Description: "A customer credit transfer should carry a purpose code. CHAPS " +
		"mandates one for the payment types it names from November 2027, and a " +
		"payment without one cannot be routed or reported on by purpose.",
	Remediation: "Populate <Purp><Cd> with an ExternalPurpose1Code, for example SALA " +
		"for salary or SUPP for a supplier payment.",
	Reference: "CHAPS enhanced data, November 2027",
	Exempt:    exemptFromEnhancedRules,
	Check: func(ctx *Context) []Finding {
		var out []Finding
		for _, tx := range transactions(ctx.Root) {
			if _, ok := Child(tx.Node, "Purp"); ok {
				continue
			}
			out = append(out, Finding{
				Path:     tx.Path + "/Purp",
				Message:  "the transaction carries no purpose code",
				Expected: "a <Purp><Cd> element",
			})
		}
		return out
	},
}

// StructuredRemittance prefers structured remittance information to free text.
var StructuredRemittance = Rule{
	ID:       "ENH-RMT-001",
	Name:     "Structured remittance information",
	Severity: SeverityWarning,
	Description: "Remittance information carried as free text cannot be reconciled " +
		"automatically. Structured remittance names the invoice, the amount and " +
		"the reference in their own elements.",
	Remediation: "Move the invoice details into <RmtInf><Strd>, keeping <Ustrd> only " +
		"for text that has no structured equivalent.",
	Reference: "CBPR+ enhanced data",
	Exempt:    exemptFromEnhancedRules,
	Check: func(ctx *Context) []Finding {
		var out []Finding
		for _, rmt := range FindAll(ctx.Root, "RmtInf") {
			if _, ok := Child(rmt.Node, "Strd"); ok {
				continue
			}
			unstructured := Children(rmt.Node, "Ustrd")
			if len(unstructured) == 0 {
				continue
			}
			out = append(out, Finding{
				Path:     rmt.Path + "/Ustrd",
				Message:  fmt.Sprintf("remittance information is %d line(s) of free text", len(unstructured)),
				Found:    strings.TrimSpace(unstructured[0].Text),
				Expected: "a <Strd> element",
			})
		}
		return out
	},
}

// LEIFormat checks any legal entity identifier present.
//
// This one is an error today rather than a warning: an LEI that fails its own
// checksum is not a partially populated field, it is a wrong one, and no date
// makes it acceptable.
var LEIFormat = Rule{
	ID:       "ENH-LEI-001",
	Name:     "Legal Entity Identifier format",
	Severity: SeverityError,
	Description: "An LEI is 20 characters -- 18 alphanumerics and two check digits -- " +
		"verified by the ISO 7064 MOD 97-10 algorithm, the same scheme an IBAN uses.",
	Remediation: "Check the identifier against the GLEIF register at " +
		"https://search.gleif.org/. A transposed character fails the checksum.",
	Reference: "ISO 17442",
	Check: func(ctx *Context) []Finding {
		var out []Finding
		for _, lei := range FindAll(ctx.Root, "LEI") {
			value := strings.TrimSpace(lei.Node.Text)
			if value == "" {
				continue
			}
			if ok, reason := ValidateLEI(value); !ok {
				out = append(out, Finding{
					Path:     lei.Path,
					Message:  reason,
					Found:    value,
					Expected: "20 characters passing the ISO 7064 MOD 97-10 check",
				})
			}
		}
		return out
	},
}

// UETRPresent asks for an end-to-end tracking reference.
var UETRPresent = Rule{
	ID:       "ENH-UETR-001",
	Name:     "UETR present",
	Severity: SeverityWarning,
	Description: "Without a UETR a payment cannot be tracked end to end, and an " +
		"investigation about it has nothing to reference.",
	Remediation: "Populate <PmtId><UETR> with an RFC 4122 version 4 UUID, generated " +
		"once by the instructing party and carried unchanged by every agent.",
	Reference: "CBPR+ / SWIFT gpi",
	Exempt:    exemptFromEnhancedRules,
	Check: func(ctx *Context) []Finding {
		var out []Finding
		for _, tx := range transactions(ctx.Root) {
			pmtID, ok := Child(tx.Node, "PmtId")
			if !ok {
				continue
			}
			if ChildText(pmtID, "UETR") != "" {
				continue
			}
			out = append(out, Finding{
				Path:     tx.Path + "/PmtId/UETR",
				Message:  "the transaction carries no end-to-end tracking reference",
				Expected: "a <UETR> element",
			})
		}
		return out
	},
}

// EnhancedRules is the November 2027 enhanced-data pack.
var EnhancedRules = []Rule{
	PurposeCodePresent,
	StructuredRemittance,
	LEIFormat,
	UETRPresent,
}

// transactions finds the transaction-level elements a payment message repeats.
// Their names differ by message, so the rules look for any of them rather than
// hard-coding one message's shape.
func transactions(root *converter.Node) []Located {
	var out []Located
	for _, name := range []string{"CdtTrfTxInf", "DrctDbtTxInf", "TxInf", "PmtInf"} {
		out = append(out, FindAll(root, name)...)
	}
	return out
}

// enhancedExemptMessages are the reporting and administration messages the
// enhanced-data rules do not reach: they carry no payment instruction to
// enrich.
var enhancedExemptMessages = map[string]bool{
	"admi.024": true,
	"camt.025": true,
	"camt.029": true,
	"camt.052": true,
	"camt.053": true,
	"camt.054": true,
	"camt.056": true,
	"camt.060": true,
	"camt.110": true,
	"camt.111": true,
	"pacs.002": true,
	"pacs.004": true,
}

func exemptFromEnhancedRules(msgID string) bool {
	return enhancedExemptMessages[baseCode(msgID)]
}

// ValidateLEI checks an ISO 17442 legal entity identifier.
//
// The format is 18 alphanumeric characters followed by two check digits, and
// the whole 20 characters must satisfy ISO 7064 MOD 97-10 -- the same scheme an
// IBAN uses, which is why a transposed character is caught rather than accepted.
func ValidateLEI(lei string) (bool, string) {
	v := strings.ToUpper(strings.TrimSpace(lei))
	if len(v) != 20 {
		return false, fmt.Sprintf("an LEI is 20 characters, this one is %d", len(v))
	}

	for i, r := range v {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'A' && r <= 'Z':
			if i >= 18 {
				return false, "the last two characters of an LEI are check digits, and these are not digits"
			}
		default:
			return false, fmt.Sprintf("%q is not permitted in an LEI", string(r))
		}
	}

	if mod97(v) != 1 {
		return false, "the check digits do not match the identifier (ISO 7064 MOD 97-10)"
	}
	return true, ""
}

// mod97 computes the ISO 7064 MOD 97-10 remainder, expanding letters to their
// two-digit values. It works a piece at a time so no big integer is needed.
func mod97(s string) int {
	remainder := 0
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			remainder = remainder*10 + int(r-'0')
		case r >= 'A' && r <= 'Z':
			remainder = remainder*100 + int(r-'A') + 10
		default:
			return -1
		}
		remainder %= 97
	}
	return remainder
}

// baseCode reduces "pacs.008.001.10" to "pacs.008".
func baseCode(msgID string) string {
	parts := strings.Split(msgID, ".")
	if len(parts) >= 2 {
		return parts[0] + "." + parts[1]
	}
	return msgID
}
