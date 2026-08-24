// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package schemagen

import (
	"strings"

	"github.com/sebastienrousseau/anchor/internal/xsd"
)

// A value that satisfies a pattern is not the same as a value that is right.
// "AB01AAAAAAAAAAAAAA" matches the IBAN pattern and fails its checksum;
// "AAAAAAAA" matches the BIC pattern and names no bank. A generated message
// full of those validates against the schema and fails the linter, which makes
// it useless as an example of a correct message.
//
// So elements Anchor recognises get values that are correct as well as valid:
// an IBAN whose mod-97 works, a BIC with a real country code, a UUIDv4 that
// passes the version and variant checks. Everything else falls through to the
// facets.

// knownValues maps an element name to a value that is both schema-valid and
// semantically sound. The names are the ones the linter checks.
var knownValues = map[string]string{
	// Identifiers the linter verifies.
	"IBAN":     "GB29NWBK60161331926819",
	"BICFI":    "BANKGB2LXXX",
	"AnyBIC":   "BANKGB2LXXX",
	"BIC":      "BANKGB2LXXX",
	"BICOrBEI": "BANKGB2LXXX",
	"UETR":     "f3a1b2c4-5d6e-4f70-8a91-2b3c4d5e6f70",
	"LEI":      "7LTWFZYICNSX8D621K86",

	// Codes with a currency or country shape.
	"Ccy":         "EUR",
	"Ctry":        "GB",
	"CtryOfBirth": "GB",
	"CtryOfRes":   "GB",
	"CityOfBirth": "LONDON",
	"TwnNm":       "LONDON",
	"PstCd":       "EC2V 7NN",
	"StrtNm":      "GRESHAM STREET",
	"BldgNb":      "14",
	"CtrySubDvsn": "ENGLAND",

	// Securities and corporate actions, where a plausible identifier is the
	// difference between a sample someone can read and a sample they cannot.
	"ISIN":                "GB0002634946",
	"OtherIdentification": "GB0002634946",
	"CorpActnEvtId":       "CA-ANCHOR-0001",
	"OfficlCorpActnEvtId": "CA-OFFICIAL-0001",
	"SfkpgAcct":           "SAFE-0001",
	"AcctId":              "ACCT-ANCHOR-0001",
	"Id":                  "ANCHOR-0001",
	"Desc":                "Anchor sample value",
	"ShrtNm":              "ACME",
	"ClssfctnTp":          "ESVUFR",
	"PlcOfListg":          "XLON",
	"MktIdrCd":            "XLON",
	"Issr":                "ANCHOR",
	"SchmeNm":             "ANCHOR",
	"Prtry":               "ANCHOR",

	// Names and references that read as data rather than as filler.
	"Nm":         "ACME TRADING LIMITED",
	"MsgId":      "ANCHOR-SAMPLE-0001",
	"EndToEndId": "E2E-ANCHOR-0001",
	"InstrId":    "INSTR-ANCHOR-0001",
	"TxId":       "TX-ANCHOR-0001",
	"NbOfTxs":    "1",
	"Email":      "payments@example.com",
	"EmailAdr":   "payments@example.com",
	"PhneNb":     "+44-2071234567",
	"MobNb":      "+44-7700900000",
	"URL":        "https://www.iso20022.org/",
}

// semanticValue returns a value for a recognised element name, when that value
// also satisfies the type's own constraints. A schema whose IBAN element is
// somehow eight characters long gets the generated value, not this one: the
// facets are the authority.
func semanticValue(elementName, base string, f xsd.Facets) (string, bool) {
	value, ok := knownValues[elementName]
	if !ok {
		return "", false
	}

	// A recognised name on a numeric or temporal type is a coincidence of
	// naming, not a match.
	switch normaliseBase(base) {
	case "string", "normalizedString", "token", "NMTOKEN", "":
	default:
		return "", false
	}

	if !fitsFacets(value, f) {
		return "", false
	}
	return value, true
}

// fitsFacets reports whether a value satisfies the length and pattern
// constraints of a type.
func fitsFacets(value string, f xsd.Facets) bool {
	n := len([]rune(value))
	switch {
	case f.Length != nil && n != *f.Length:
		return false
	case f.MinLength != nil && n < *f.MinLength:
		return false
	case f.MaxLength != nil && n > *f.MaxLength:
		return false
	}

	for _, pattern := range f.Pattern {
		if !patternAccepts(pattern, value) {
			return false
		}
	}
	return true
}

// patternAccepts checks a value against an XSD pattern using the same parsed
// form the sampler uses, so the two never disagree about what a pattern means.
//
// Matching is a backtracking walk over the parsed pattern. The subset has no
// backreferences and no lookaround, and the values are short, so the cost is
// not worth avoiding.
func patternAccepts(pattern, value string) bool {
	node, err := parsePattern(pattern)
	if err != nil {
		// A pattern that cannot be parsed cannot be checked. Refusing the
		// candidate sends the caller to the sampler, which reports the same
		// failure with a path attached.
		return false
	}
	return matchNode(node, []rune(value), func(rest []rune) bool { return len(rest) == 0 })
}

// matchNode attempts to match a node at the start of input, calling cont with
// whatever is left over. Continuation passing is what makes the alternation and
// repeat cases straightforward.
func matchNode(n patternNode, input []rune, cont func([]rune) bool) bool {
	switch t := n.(type) {
	case *literal:
		if len(input) > 0 && input[0] == t.r {
			return cont(input[1:])
		}
		return false

	case *charClass:
		if len(input) > 0 && t.contains(input[0]) {
			return cont(input[1:])
		}
		return false

	case *sequence:
		return matchSequence(t.items, input, cont)

	case *alternation:
		for _, branch := range t.branches {
			if matchNode(branch, input, cont) {
				return true
			}
		}
		return false

	case *repeat:
		return matchRepeat(t, input, 0, cont)
	}
	return false
}

func matchSequence(items []patternNode, input []rune, cont func([]rune) bool) bool {
	if len(items) == 0 {
		return cont(input)
	}
	return matchNode(items[0], input, func(rest []rune) bool {
		return matchSequence(items[1:], rest, cont)
	})
}

// matchRepeat is greedy within the declared bounds, trying the longest match
// first and falling back.
func matchRepeat(r *repeat, input []rune, taken int, cont func([]rune) bool) bool {
	if r.max == unbounded || taken < r.max {
		// A repeat of something that matches nothing would loop; requiring the
		// input to shrink stops that.
		if matchNode(r.item, input, func(rest []rune) bool {
			if len(rest) == len(input) {
				return false
			}
			return matchRepeat(r, rest, taken+1, cont)
		}) {
			return true
		}
	}
	if taken >= r.min {
		return cont(input)
	}
	return false
}

// MessageIDFromNamespace reads the message identifier out of a target
// namespace, so a generated document can be labelled.
func MessageIDFromNamespace(ns string) string {
	const prefix = "urn:iso:std:iso:20022:tech:xsd:"
	if strings.HasPrefix(ns, prefix) {
		return strings.TrimPrefix(ns, prefix)
	}
	return ""
}
