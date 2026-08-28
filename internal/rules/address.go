// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package rules

import (
	"fmt"
	"strings"

	"github.com/sebastienrousseau/askiso/internal/converter"
)

// CBPR+ rejects fully unstructured postal addresses outright, with no
// contingency. Any address that is present must be hybrid or fully structured:
// town name and country in their own elements, at minimum.
//
// Swift deferred the 14 November 2026 cutover on 27 August 2026 and will confirm
// replacement timing by December, so no date is asserted in what this prints.
// The requirement was agreed by the community in 2023 and stands; only when it
// bites has moved.
//
// The requirement applies to every agent and party in a payment message, with a
// short list of reporting and administration messages exempted.
//
// Reference: https://www.swift.com/standards/iso-20022/removal-unstructured-address

// AddressShape classifies a postal address.
type AddressShape string

const (
	// ShapeStructured uses the individual elements only.
	ShapeStructured AddressShape = "structured"
	// ShapeHybrid keeps town and country structured while retaining address
	// lines for the remainder. Permitted from November 2025 with no end date.
	ShapeHybrid AddressShape = "hybrid"
	// ShapeUnstructured carries address lines alone. Rejected by CBPR+.
	ShapeUnstructured AddressShape = "unstructured"
	// ShapeEmpty is an address element with nothing usable in it.
	ShapeEmpty AddressShape = "empty"
)

// structuredElements are the address components other than AdrLine.
var structuredElements = []string{
	"Dept", "SubDept", "StrtNm", "BldgNb", "BldgNm", "Flr", "PstBx", "Room",
	"PstCd", "TwnNm", "TwnLctnNm", "DstrctNm", "CtrySubDvsn", "Ctry",
}

// Classify reports the shape of one PstlAdr element.
func Classify(addr *converter.Node) AddressShape {
	lines := len(Children(addr, "AdrLine"))

	structured := 0
	for _, name := range structuredElements {
		if ChildText(addr, name) != "" {
			structured++
		}
	}

	switch {
	case structured == 0 && lines == 0:
		return ShapeEmpty
	case structured == 0:
		return ShapeUnstructured
	case lines == 0:
		return ShapeStructured
	default:
		return ShapeHybrid
	}
}

// addressExemptMessages are the identifiers the structured-address requirement
// does not reach: reporting and administration messages rather than payment
// instructions.
var addressExemptMessages = map[string]bool{
	"admi.024": true,
	"camt.025": true,
	"camt.052": true,
	"camt.053": true,
	"camt.054": true,
	"camt.060": true,
}

// exemptFromAddressRules matches on the base code, so every version of an
// exempted message is covered.
func exemptFromAddressRules(msgID string) bool {
	return addressExemptMessages[baseCode(msgID)]
}

const addressReference = "https://www.swift.com/standards/iso-20022/removal-unstructured-address"

// TownAndCountryRequired is the core of the November 2026 change.
var TownAndCountryRequired = Rule{
	ID:       "CBPR-ADDR-001",
	Name:     "Town and country required",
	Severity: SeverityError,
	Description: "Whenever a postal address is present, town name and country must " +
		"be carried in their own elements.",
	Remediation: "Populate <TwnNm> and <Ctry> inside <PstlAdr>.",
	Reference:   addressReference,
	Exempt:      exemptFromAddressRules,
	Check: func(ctx *Context) []Finding {
		var out []Finding
		for _, addr := range FindAll(ctx.Root, "PstlAdr") {
			if Classify(addr.Node) == ShapeEmpty {
				continue
			}
			if ChildText(addr.Node, "TwnNm") == "" {
				out = append(out, Finding{
					Path:     addr.Path + "/TwnNm",
					Message:  "the address has no town name",
					Expected: "a <TwnNm> element",
					Found:    "(absent)",
				})
			}
			if ChildText(addr.Node, "Ctry") == "" {
				out = append(out, Finding{
					Path:     addr.Path + "/Ctry",
					Message:  "the address has no country",
					Expected: "a <Ctry> element",
					Found:    "(absent)",
				})
			}
		}
		return out
	},
}

// NoUnstructuredAddress rejects addresses carried in AdrLine alone.
var NoUnstructuredAddress = Rule{
	ID:       "CBPR-ADDR-002",
	Name:     "Unstructured address not accepted",
	Severity: SeverityError,
	Description: "An address made only of address lines is rejected by CBPR+. " +
		"Use a hybrid or fully structured address.",
	Remediation: "Move the town into <TwnNm> and the country into <Ctry>; keep the " +
		"remainder in at most two <AdrLine> elements.",
	Reference: addressReference,
	Exempt:    exemptFromAddressRules,
	Check: func(ctx *Context) []Finding {
		var out []Finding
		for _, addr := range FindAll(ctx.Root, "PstlAdr") {
			if Classify(addr.Node) != ShapeUnstructured {
				continue
			}
			lines := Children(addr.Node, "AdrLine")
			out = append(out, Finding{
				Path:     addr.Path,
				Message:  fmt.Sprintf("the address is fully unstructured (%d address line(s), no structured element)", len(lines)),
				Expected: "hybrid or structured",
				Found:    string(ShapeUnstructured),
			})
		}
		return out
	},
}

// CountryCodeFormat catches a spelled-out country name where a code belongs.
var CountryCodeFormat = Rule{
	ID:          "CBPR-ADDR-003",
	Name:        "Country must be an ISO 3166-1 alpha-2 code",
	Severity:    SeverityError,
	Description: "<Ctry> carries a two-letter country code such as US or GB, not a name.",
	Remediation: "Replace the country name with its ISO 3166-1 alpha-2 code.",
	Reference:   "https://www.iso.org/iso-3166-country-codes.html",
	Exempt:      exemptFromAddressRules,
	Check: func(ctx *Context) []Finding {
		var out []Finding
		for _, addr := range FindAll(ctx.Root, "PstlAdr") {
			code := ChildText(addr.Node, "Ctry")
			if code == "" {
				continue
			}
			if IsCountryCode(code) {
				continue
			}

			found := code
			expected := "a two-letter ISO 3166-1 code"
			if guess, ok := CountryCodeFor(code); ok {
				expected = fmt.Sprintf("%q (the code for %s)", guess, code)
			}
			out = append(out, Finding{
				Path:     addr.Path + "/Ctry",
				Message:  fmt.Sprintf("%q is not an ISO 3166-1 alpha-2 country code", code),
				Found:    found,
				Expected: expected,
			})
		}
		return out
	},
}

// HybridAddressLineLimit enforces the CBPR+ shape for hybrid addresses.
var HybridAddressLineLimit = Rule{
	ID:       "CBPR-ADDR-004",
	Name:     "Hybrid address line limit",
	Severity: SeverityError,
	Description: "A hybrid address keeps town and country structured and carries at " +
		"most two address lines of seventy characters each.",
	Remediation: "Reduce to two <AdrLine> elements, each no longer than 70 characters.",
	Reference:   addressReference,
	Exempt:      exemptFromAddressRules,
	Check: func(ctx *Context) []Finding {
		const maxLines, maxLen = 2, 70

		var out []Finding
		for _, addr := range FindAll(ctx.Root, "PstlAdr") {
			lines := Children(addr.Node, "AdrLine")
			if len(lines) > maxLines {
				out = append(out, Finding{
					Path:     addr.Path + "/AdrLine",
					Message:  fmt.Sprintf("%d address lines; CBPR+ permits at most %d", len(lines), maxLines),
					Found:    fmt.Sprintf("%d", len(lines)),
					Expected: fmt.Sprintf("at most %d", maxLines),
				})
			}
			for i, line := range lines {
				text := strings.TrimSpace(line.Text)
				if n := len([]rune(text)); n > maxLen {
					out = append(out, Finding{
						Path:     fmt.Sprintf("%s/AdrLine[%d]", addr.Path, i+1),
						Message:  fmt.Sprintf("address line is %d characters; the limit is %d", n, maxLen),
						Found:    fmt.Sprintf("%d characters", n),
						Expected: fmt.Sprintf("at most %d characters", maxLen),
					})
				}
			}
		}
		return out
	},
}

// HybridAddressAdvisory nudges hybrid addresses towards full structure. Hybrid
// remains acceptable with no announced end date, so this is informational.
var HybridAddressAdvisory = Rule{
	ID:          "CBPR-ADDR-005",
	Name:        "Hybrid address in use",
	Severity:    SeverityInfo,
	Description: "Hybrid addresses are accepted with no announced end date, but full structure is more durable.",
	Remediation: "Move the remaining address lines into <StrtNm>, <BldgNb> and <PstCd>.",
	Reference:   addressReference,
	Exempt:      exemptFromAddressRules,
	Check: func(ctx *Context) []Finding {
		var out []Finding
		for _, addr := range FindAll(ctx.Root, "PstlAdr") {
			if Classify(addr.Node) != ShapeHybrid {
				continue
			}
			out = append(out, Finding{
				Path:    addr.Path,
				Message: "address is hybrid; fully structured is more durable",
				Found:   string(ShapeHybrid),
			})
		}
		return out
	},
}

// AddressRules is the November 2026 pack.
var AddressRules = []Rule{
	TownAndCountryRequired,
	NoUnstructuredAddress,
	CountryCodeFormat,
	HybridAddressLineLimit,
	HybridAddressAdvisory,
}
