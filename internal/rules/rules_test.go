// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package rules_test

import (
	"strings"
	"testing"

	"github.com/sebastienrousseau/askiso/internal/converter"
	"github.com/sebastienrousseau/askiso/internal/rules"
)

// message wraps address fragments in a minimal document of the given type.
func message(t *testing.T, msgID, body string) *converter.Node {
	t.Helper()
	doc := `<Document xmlns="urn:iso:std:iso:20022:tech:xsd:` + msgID + `">` + body + `</Document>`
	root, err := converter.Parse([]byte(doc))
	if err != nil {
		t.Fatalf("parsing fixture: %v", err)
	}
	return root
}

func check(t *testing.T, msgID, body string) *rules.Result {
	t.Helper()
	p, err := rules.Get("cbpr-2026")
	if err != nil {
		t.Fatal(err)
	}
	return rules.Run(p, message(t, msgID, body), msgID, "test.xml")
}

func ruleIDs(res *rules.Result) []string {
	var out []string
	for _, f := range res.Findings {
		out = append(out, f.RuleID)
	}
	return out
}

func hasRule(res *rules.Result, id string) bool {
	for _, f := range res.Findings {
		if f.RuleID == id {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Classification
// ---------------------------------------------------------------------------

func TestClassify(t *testing.T) {
	cases := []struct {
		name string
		body string
		want rules.AddressShape
	}{
		{"structured", `<PstlAdr><StrtNm>High St</StrtNm><TwnNm>London</TwnNm><Ctry>GB</Ctry></PstlAdr>`, rules.ShapeStructured},
		{"hybrid", `<PstlAdr><AdrLine>High St 1</AdrLine><TwnNm>London</TwnNm><Ctry>GB</Ctry></PstlAdr>`, rules.ShapeHybrid},
		{"unstructured", `<PstlAdr><AdrLine>High St 1</AdrLine><AdrLine>London GB</AdrLine></PstlAdr>`, rules.ShapeUnstructured},
		{"empty", `<PstlAdr></PstlAdr>`, rules.ShapeEmpty},
		{"town only is structured", `<PstlAdr><TwnNm>London</TwnNm></PstlAdr>`, rules.ShapeStructured},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := message(t, "pacs.008.001.10", tc.body)
			addrs := rules.FindAll(root, "PstlAdr")
			if len(addrs) != 1 {
				t.Fatalf("expected one address, got %d", len(addrs))
			}
			if got := rules.Classify(addrs[0].Node); got != tc.want {
				t.Errorf("Classify = %q, want %q", got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// The November 2026 rules
// ---------------------------------------------------------------------------

func TestFullyUnstructuredIsRejected(t *testing.T) {
	res := check(t, "pacs.008.001.10",
		`<Dbtr><PstlAdr><AdrLine>12 High Street</AdrLine><AdrLine>London</AdrLine></PstlAdr></Dbtr>`)

	if res.Valid() {
		t.Fatal("a fully unstructured address must be rejected")
	}
	for _, want := range []string{"CBPR-ADDR-001", "CBPR-ADDR-002"} {
		if !hasRule(res, want) {
			t.Errorf("%s should have fired; got %v", want, ruleIDs(res))
		}
	}
}

func TestStructuredAddressPasses(t *testing.T) {
	res := check(t, "pacs.008.001.10",
		`<Dbtr><PstlAdr><StrtNm>High Street</StrtNm><BldgNb>12</BldgNb>
		 <PstCd>EC1A 1BB</PstCd><TwnNm>London</TwnNm><Ctry>GB</Ctry></PstlAdr></Dbtr>`)

	if !res.Valid() {
		t.Errorf("a fully structured address should pass: %v", res.Findings)
	}
	// And it should not be flagged as hybrid.
	if hasRule(res, "CBPR-ADDR-005") {
		t.Error("a structured address is not hybrid")
	}
}

func TestHybridAddressPassesWithAdvisory(t *testing.T) {
	res := check(t, "pacs.008.001.10",
		`<Dbtr><PstlAdr><AdrLine>12 High Street</AdrLine><TwnNm>London</TwnNm><Ctry>GB</Ctry></PstlAdr></Dbtr>`)

	if !res.Valid() {
		t.Errorf("hybrid is permitted with no announced end date: %v", res.Findings)
	}
	if !hasRule(res, "CBPR-ADDR-005") {
		t.Error("the hybrid advisory should be reported")
	}
	for _, f := range res.Findings {
		if f.RuleID == "CBPR-ADDR-005" && f.Severity != rules.SeverityInfo {
			t.Errorf("the hybrid advisory should be informational, got %s", f.Severity)
		}
	}
}

func TestMissingTownOrCountry(t *testing.T) {
	town := check(t, "pacs.008.001.10",
		`<Dbtr><PstlAdr><StrtNm>High St</StrtNm><Ctry>GB</Ctry></PstlAdr></Dbtr>`)
	if town.Valid() || !hasRule(town, "CBPR-ADDR-001") {
		t.Errorf("a missing town should be reported: %v", ruleIDs(town))
	}

	country := check(t, "pacs.008.001.10",
		`<Dbtr><PstlAdr><StrtNm>High St</StrtNm><TwnNm>London</TwnNm></PstlAdr></Dbtr>`)
	if country.Valid() || !hasRule(country, "CBPR-ADDR-001") {
		t.Errorf("a missing country should be reported: %v", ruleIDs(country))
	}
}

func TestEmptyAddressIsNotFlagged(t *testing.T) {
	res := check(t, "pacs.008.001.10", `<Dbtr><PstlAdr></PstlAdr></Dbtr>`)
	if !res.Valid() {
		t.Errorf("an empty address element carries nothing to check: %v", res.Findings)
	}
}

func TestSpelledOutCountrySuggestsTheCode(t *testing.T) {
	for name, want := range map[string]string{
		"France":         "FR",
		"United Kingdom": "GB",
		"Great Britain":  "GB",
		"Germany":        "DE",
		"united states":  "US",
		"Holland":        "NL",
	} {
		t.Run(name, func(t *testing.T) {
			res := check(t, "pacs.008.001.10",
				`<Dbtr><PstlAdr><TwnNm>X</TwnNm><Ctry>`+name+`</Ctry></PstlAdr></Dbtr>`)
			if res.Valid() {
				t.Fatalf("%q is not a country code", name)
			}
			var found bool
			for _, f := range res.Findings {
				if f.RuleID == "CBPR-ADDR-003" {
					found = true
					if !strings.Contains(f.Expected, want) {
						t.Errorf("expected a suggestion of %q, got %q", want, f.Expected)
					}
				}
			}
			if !found {
				t.Errorf("CBPR-ADDR-003 should have fired: %v", ruleIDs(res))
			}
		})
	}
}

func TestTwoLetterNonCodeIsRejected(t *testing.T) {
	// The schema permits any two upper-case letters, so this is exactly the
	// case XSD validation cannot catch.
	res := check(t, "pacs.008.001.10",
		`<Dbtr><PstlAdr><TwnNm>X</TwnNm><Ctry>XX</Ctry></PstlAdr></Dbtr>`)
	if res.Valid() || !hasRule(res, "CBPR-ADDR-003") {
		t.Errorf("XX is not an assigned country code: %v", ruleIDs(res))
	}
}

func TestHybridLineLimits(t *testing.T) {
	tooMany := check(t, "pacs.008.001.10",
		`<Dbtr><PstlAdr><AdrLine>a</AdrLine><AdrLine>b</AdrLine><AdrLine>c</AdrLine>
		 <TwnNm>London</TwnNm><Ctry>GB</Ctry></PstlAdr></Dbtr>`)
	if tooMany.Valid() || !hasRule(tooMany, "CBPR-ADDR-004") {
		t.Errorf("three address lines exceed the CBPR+ limit: %v", ruleIDs(tooMany))
	}

	tooLong := check(t, "pacs.008.001.10",
		`<Dbtr><PstlAdr><AdrLine>`+strings.Repeat("x", 71)+`</AdrLine>
		 <TwnNm>London</TwnNm><Ctry>GB</Ctry></PstlAdr></Dbtr>`)
	if tooLong.Valid() || !hasRule(tooLong, "CBPR-ADDR-004") {
		t.Errorf("a 71-character address line exceeds the limit: %v", ruleIDs(tooLong))
	}

	atLimit := check(t, "pacs.008.001.10",
		`<Dbtr><PstlAdr><AdrLine>`+strings.Repeat("x", 70)+`</AdrLine>
		 <TwnNm>London</TwnNm><Ctry>GB</Ctry></PstlAdr></Dbtr>`)
	if hasRule(atLimit, "CBPR-ADDR-004") {
		t.Error("exactly 70 characters is within the limit")
	}
}

// The requirement does not reach reporting and administration messages.
func TestExemptMessages(t *testing.T) {
	body := `<Dbtr><PstlAdr><AdrLine>12 High Street</AdrLine><AdrLine>London</AdrLine></PstlAdr></Dbtr>`

	for _, msgID := range []string{
		"camt.052.001.11", "camt.053.001.11", "camt.054.001.11",
		"camt.060.001.07", "camt.025.001.08", "admi.024.001.01",
	} {
		t.Run(msgID, func(t *testing.T) {
			res := check(t, msgID, body)
			if !res.Valid() {
				t.Errorf("%s is exempt but reported %v", msgID, ruleIDs(res))
			}
			if res.Skipped == 0 {
				t.Errorf("%s should have skipped rules", msgID)
			}
		})
	}

	// A payment message is not exempt.
	if res := check(t, "pacs.008.001.10", body); res.Valid() {
		t.Error("pacs.008 is in scope and should be reported")
	}
}

func TestEveryAddressInTheMessageIsChecked(t *testing.T) {
	res := check(t, "pacs.008.001.10", `
	  <Dbtr><PstlAdr><AdrLine>a</AdrLine></PstlAdr></Dbtr>
	  <Cdtr><PstlAdr><AdrLine>b</AdrLine></PstlAdr></Cdtr>
	  <DbtrAgt><FinInstnId><PstlAdr><AdrLine>c</AdrLine></PstlAdr></FinInstnId></DbtrAgt>`)

	paths := map[string]bool{}
	for _, f := range res.Findings {
		paths[f.Path] = true
	}
	for _, want := range []string{
		"/Document/Dbtr/PstlAdr",
		"/Document/Cdtr/PstlAdr",
		"/Document/DbtrAgt/FinInstnId/PstlAdr",
	} {
		var seen bool
		for p := range paths {
			if strings.HasPrefix(p, want) {
				seen = true
			}
		}
		if !seen {
			t.Errorf("no finding for %s; got %v", want, paths)
		}
	}
}

// ---------------------------------------------------------------------------
// Engine
// ---------------------------------------------------------------------------

func TestProfileRegistry(t *testing.T) {
	names := rules.Names()
	if len(names) == 0 {
		t.Fatal("no profiles registered")
	}
	for _, n := range names {
		p, err := rules.Get(n)
		if err != nil {
			t.Errorf("Get(%q): %v", n, err)
		}
		if p.Name != n {
			t.Errorf("profile %q reports name %q", n, p.Name)
		}
		if rules.Describe(n) == "" {
			t.Errorf("profile %q has no description", n)
		}
	}

	if _, err := rules.Get("no-such-profile"); err == nil {
		t.Error("an unknown profile should be an error")
	}
	if rules.Describe("no-such-profile") != "" {
		t.Error("an unknown profile has no description")
	}
	// Case and padding are tolerated.
	if _, err := rules.Get("  CBPR-2026 "); err != nil {
		t.Errorf("profile lookup should be forgiving: %v", err)
	}
}

func TestFindingsCarryContext(t *testing.T) {
	res := check(t, "pacs.008.001.10",
		`<Dbtr><PstlAdr><AdrLine>a</AdrLine></PstlAdr></Dbtr>`)

	if len(res.Findings) == 0 {
		t.Fatal("expected findings")
	}
	for _, f := range res.Findings {
		if f.RuleID == "" || f.Rule == "" || f.Path == "" || f.Message == "" {
			t.Errorf("incomplete finding: %+v", f)
		}
		if f.Severity == "" {
			t.Errorf("finding has no severity: %+v", f)
		}
		if f.Remediation == "" || f.Reference == "" {
			t.Errorf("a finding should say how to fix it and cite the rule: %+v", f)
		}
	}
	if res.Checked == 0 {
		t.Error("the result should report how many rules ran")
	}
}

func TestFindingsAreOrderedByPath(t *testing.T) {
	res := check(t, "pacs.008.001.10", `
	  <Zed><PstlAdr><AdrLine>a</AdrLine></PstlAdr></Zed>
	  <Alpha><PstlAdr><AdrLine>b</AdrLine></PstlAdr></Alpha>`)

	for i := 1; i < len(res.Findings); i++ {
		if res.Findings[i-1].Path > res.Findings[i].Path {
			t.Errorf("findings should be ordered by path: %q before %q",
				res.Findings[i-1].Path, res.Findings[i].Path)
		}
	}
}

func TestRepeatedSiblingsGetIndexedPaths(t *testing.T) {
	res := check(t, "pacs.008.001.10", `
	  <CdtTrfTxInf><Dbtr><PstlAdr><AdrLine>a</AdrLine></PstlAdr></Dbtr></CdtTrfTxInf>
	  <CdtTrfTxInf><Dbtr><PstlAdr><AdrLine>b</AdrLine></PstlAdr></Dbtr></CdtTrfTxInf>`)

	var indexed int
	for _, f := range res.Findings {
		if strings.Contains(f.Path, "CdtTrfTxInf[") {
			indexed++
		}
	}
	if indexed == 0 {
		t.Errorf("repeated siblings should get indexed paths; got %v", ruleIDs(res))
	}
}

func TestNodeHelpers(t *testing.T) {
	root := message(t, "pacs.008.001.10",
		`<A><B>one</B><B>two</B><C> spaced </C></A>`)

	a, ok := rules.Child(root, "A")
	if !ok {
		t.Fatal("Child should find A")
	}
	if _, ok := rules.Child(root, "Nope"); ok {
		t.Error("Child should not invent an element")
	}
	if got := rules.Children(a, "B"); len(got) != 2 {
		t.Errorf("Children(B) = %d, want 2", len(got))
	}
	if got := rules.ChildText(a, "C"); got != "spaced" {
		t.Errorf("ChildText should trim: %q", got)
	}
	if got := rules.ChildText(a, "Nope"); got != "" {
		t.Errorf("missing child should yield empty text, got %q", got)
	}
	if got := rules.FindAll(root, "B"); len(got) != 2 {
		t.Errorf("FindAll(B) = %d, want 2", len(got))
	}
	if got := rules.FindAll(root, "Nope"); len(got) != 0 {
		t.Errorf("FindAll of a missing name should be empty, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// Country codes
// ---------------------------------------------------------------------------

func TestCountryCodes(t *testing.T) {
	for _, code := range []string{"GB", "US", "FR", "DE", "JP", "AU", "ZW", "AD"} {
		if !rules.IsCountryCode(code) {
			t.Errorf("%s should be an assigned code", code)
		}
		if _, ok := rules.CountryName(code); !ok {
			t.Errorf("%s should have a name", code)
		}
	}
	for _, code := range []string{"XX", "ZZ", "", "GBR", "g"} {
		if rules.IsCountryCode(code) {
			t.Errorf("%q should not be an assigned code", code)
		}
	}
	// Case and padding are tolerated on lookup.
	if !rules.IsCountryCode(" gb ") {
		t.Error("lookup should trim and upper-case")
	}
	if _, ok := rules.CountryCodeFor("not a country at all"); ok {
		t.Error("a nonsense name should not resolve")
	}
	if _, ok := rules.CountryCodeFor(""); ok {
		t.Error("an empty name should not resolve")
	}
}

func TestRunOnMessageWithNoAddresses(t *testing.T) {
	res := check(t, "pacs.008.001.10", `<Dbtr><Nm>No address here</Nm></Dbtr>`)
	if !res.Valid() {
		t.Errorf("a message with no address has nothing to fail: %v", res.Findings)
	}
	if res.Checked == 0 {
		t.Error("rules should still have run")
	}
}

func TestProfilesWithNoRules(t *testing.T) {
	p, err := rules.Get("base")
	if err != nil {
		t.Fatal(err)
	}
	res := rules.Run(p, message(t, "pacs.008.001.10", `<A/>`), "pacs.008.001.10", "x.xml")
	if !res.Valid() || len(res.Findings) != 0 {
		t.Errorf("an empty profile should report nothing: %+v", res)
	}
}

func TestCBPRPlusProfile(t *testing.T) {
	p, err := rules.Get("cbpr-plus")
	if err != nil {
		t.Fatal(err)
	}
	root := message(t, "pacs.008.001.10",
		`<Dbtr><PstlAdr><TwnNm>X</TwnNm><Ctry>France</Ctry></PstlAdr></Dbtr>`)
	res := rules.Run(p, root, "pacs.008.001.10", "x.xml")
	if res.Valid() {
		t.Error("the country-code rule applies today, not only from 2026")
	}
	// The unstructured-address rule is not in this profile.
	if hasRule(res, "CBPR-ADDR-002") {
		t.Error("cbpr-plus should not enforce the 2026 address shape")
	}
}
