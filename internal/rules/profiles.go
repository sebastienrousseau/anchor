// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package rules

import (
	"fmt"
	"sort"
	"strings"
)

// Profiles are the named rule sets a message can be checked against.
var profiles = map[string]Profile{
	"base": {
		Name:        "base",
		Description: "Structural sanity checks that apply to any ISO 20022 message.",
		Rules:       nil,
	},
	"cbpr-2026": {
		Name: "cbpr-2026",
		Description: "CBPR+ requirements effective 14 November 2026: postal addresses " +
			"must be hybrid or fully structured.",
		Rules: AddressRules,
	},
	"cbpr-plus": {
		Name:        "cbpr-plus",
		Description: "CBPR+ requirements in force today.",
		Rules:       []Rule{CountryCodeFormat},
	},
	"cbpr-2027": {
		Name: "cbpr-2027",
		Description: "Enhanced data for November 2027: purpose codes, structured " +
			"remittance information, legal entity identifiers and end-to-end " +
			"tracking references. Includes the 2026 address requirements.",
		Rules: append(append([]Rule{}, AddressRules...), EnhancedRules...),
	},
	"investigations": {
		Name: "investigations",
		Description: "camt.110 and camt.111, which replace the MT n92/n95/n96 " +
			"investigation flow: every investigation must identify its payment, and " +
			"every response must quote its request.",
		Rules: InvestigationRules,
	},
	"verification-of-payee": {
		Name: "verification-of-payee",
		Description: "acmt.023 and acmt.024: a verification request must say what to " +
			"check, and a report that could not verify a payee must say why.",
		Rules: VerificationRules,
	},
	"all": {
		Name: "all",
		Description: "Every rule Anchor knows. Findings from dates that have not " +
			"arrived yet are warnings, so this is usable as a readiness report.",
		Rules: allRules(),
	},
}

// allRules gathers every pack, skipping the duplicates that arise because a
// profile may include another's rules.
func allRules() []Rule {
	var out []Rule
	seen := map[string]bool{}
	for _, pack := range [][]Rule{
		AddressRules,
		{CountryCodeFormat},
		EnhancedRules,
		InvestigationRules,
		VerificationRules,
	} {
		for _, r := range pack {
			if seen[r.ID] {
				continue
			}
			seen[r.ID] = true
			out = append(out, r)
		}
	}
	return out
}

// Profile returns a named rule set.
func Get(name string) (Profile, error) {
	p, ok := profiles[strings.ToLower(strings.TrimSpace(name))]
	if !ok {
		return Profile{}, fmt.Errorf("unknown profile %q (available: %s)",
			name, strings.Join(Names(), ", "))
	}
	return p, nil
}

// Names lists the available profiles.
func Names() []string {
	out := make([]string, 0, len(profiles))
	for n := range profiles {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Describe returns a profile's description.
func Describe(name string) string {
	if p, err := Get(name); err == nil {
		return p.Description
	}
	return ""
}
