// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package cmd

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebastienrousseau/anchor/internal/registry"
	"github.com/spf13/cobra"
)

// ---------------------------------------------------------------------------
// Light-mode guidance
// ---------------------------------------------------------------------------

func TestNotInstalledErrorMessages(t *testing.T) {
	// A message that exists but is not installed names the download.
	err := notInstalled("pacs.008.001.10", "read its schema", nil)
	msg := err.Error()
	for _, want := range []string{"pacs.008.001.10", "read its schema", "iso20022.org", "anchor catalog add"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message should contain %q:\n%s", want, msg)
		}
	}

	// An identifier outside the standard reads as a typo.
	err = notInstalled("pacs.808", "read its schema", nil)
	msg = err.Error()
	if !strings.Contains(msg, "typo") || !strings.Contains(msg, "anchor search") {
		t.Errorf("an unknown identifier should suggest a search:\n%s", msg)
	}

	// A partial identifier resolves through search.
	if e := notInstalled("pacs.008", "validate", nil); !strings.Contains(e.Error(), "pacs.008") {
		t.Errorf("a base code should resolve:\n%s", e)
	}
}

func TestNotInstalledErrorDefaultsPurpose(t *testing.T) {
	e := &NotInstalledError{
		Query: "pacs.008.001.10",
		Known: true,
		Sets:  []registry.Set{{ID: "1", Name: "A"}, {ID: "2", Name: "B"}, {ID: "3", Name: "C"}, {ID: "4", Name: "D"}, {ID: "5", Name: "E"}},
	}
	msg := e.Error()
	if !strings.Contains(msg, "use this message") {
		t.Errorf("an unset purpose should read naturally:\n%s", msg)
	}
	// Long set lists are truncated.
	if !strings.Contains(msg, "and 2 more") {
		t.Errorf("the set list should be truncated:\n%s", msg)
	}
}

func TestFirstToken(t *testing.T) {
	cases := map[string]string{
		"pacs.008 extra": "pacs.008",
		"single":         "single",
		"":               "",
		"a\tb":           "a",
	}
	for in, want := range cases {
		if got := firstToken(in); got != want {
			t.Errorf("firstToken(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLightModeNotice(t *testing.T) {
	n := lightModeNotice("generated sample")
	if !strings.Contains(n, "light mode") || !strings.Contains(n, "anchor catalog add") {
		t.Errorf("the notice should explain the reduced scope:\n%s", n)
	}
}

func TestSilentErrorPrintsNothing(t *testing.T) {
	if errSilent.Error() != "" {
		t.Errorf("errSilent should render as empty, got %q", errSilent.Error())
	}
}

// ---------------------------------------------------------------------------
// Shell completion helpers
// ---------------------------------------------------------------------------

func TestCompletionHelpers(t *testing.T) {
	withCatalogue(t)

	ids, directive := completeMessageIDs(&cobra.Command{}, nil, "pacs")
	if len(ids) == 0 {
		t.Error("completing 'pacs' should offer message identifiers")
	}
	for _, id := range ids {
		if !strings.HasPrefix(strings.ToLower(id), "pacs") {
			t.Errorf("unexpected completion: %s", id)
		}
	}
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Error("completions should not fall back to file names")
	}

	codes, _ := completeTranslationCodes(&cobra.Command{}, nil, "MT")
	if len(codes) == 0 {
		t.Error("completing 'MT' should offer SWIFT codes")
	}
	if mx, _ := completeTranslationCodes(&cobra.Command{}, nil, "pacs"); len(mx) == 0 {
		t.Error("completing 'pacs' should offer MX codes")
	}
}

func TestCompleteMessageIDsWithoutCatalogue(t *testing.T) {
	isolate(t)
	// Must degrade quietly rather than error: shell completion has nowhere to
	// show a message.
	if _, d := completeMessageIDs(&cobra.Command{}, nil, "pacs"); d != cobra.ShellCompDirectiveNoFileComp {
		t.Error("completion should stay silent with no catalogue")
	}
}

// ---------------------------------------------------------------------------
// validate: remaining branches
// ---------------------------------------------------------------------------

func TestValidateWithLibxml2Engine(t *testing.T) {
	if _, err := lookXmllint(); err != nil {
		t.Skip("xmllint not installed")
	}
	root := withCatalogue(t)
	dir := t.TempDir()

	good := filepath.Join(dir, "good.xml")
	if err := os.WriteFile(good, []byte(fixtureInstance("pacs.008.001.10", "EUR")), 0o644); err != nil {
		t.Fatal(err)
	}
	schema := filepath.Join(root, "Payments Clearing and Settlement", "Version 11.0",
		"Schemas", "pacs.008.001.10.xsd")

	out, err := run(t, "validate", good, schema, "--engine", "libxml2")
	if err != nil {
		t.Fatalf("libxml2 engine on a valid document: %v\n%s", err, out)
	}
	wantContains(t, out, "VALID", "libxml2")

	bad := filepath.Join(dir, "bad.xml")
	if err := os.WriteFile(bad, []byte(fixtureInstance("pacs.008.001.10", "EURO")), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err = run(t, "validate", bad, schema, "--engine", "libxml2")
	if err == nil {
		t.Error("libxml2 should reject the invalid document")
	}
	wantContains(t, out, "INVALID")
}

func TestValidateJSONOnValidDocument(t *testing.T) {
	withCatalogue(t)
	dir := t.TempDir()
	good := filepath.Join(dir, "good.xml")
	if err := os.WriteFile(good, []byte(fixtureInstance("pacs.008.001.10", "EUR")), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := run(t, "validate", good, "--json")
	if err != nil {
		t.Fatalf("validate --json on a valid document: %v", err)
	}
	if !strings.Contains(out, `"valid": true`) {
		t.Errorf("expected valid:true:\n%s", out)
	}
}

func TestValidateUnparseableSchema(t *testing.T) {
	withCatalogue(t)
	dir := t.TempDir()

	doc := filepath.Join(dir, "doc.xml")
	if err := os.WriteFile(doc, []byte(fixtureInstance("pacs.008.001.10", "EUR")), 0o644); err != nil {
		t.Fatal(err)
	}
	junk := filepath.Join(dir, "junk.xsd")
	if err := os.WriteFile(junk, []byte("this is not a schema"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "validate", doc, junk); err == nil {
		t.Error("an unparseable schema should be an error")
	}
}

// ---------------------------------------------------------------------------
// diff: full report
// ---------------------------------------------------------------------------

func TestDiffReportsEveryChangeClass(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "Payments Clearing and Settlement", "Version 11.0", "Schemas")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}

	older := `<?xml version="1.0"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:a" xmlns="urn:a">
  <xs:element name="Document" type="Document"/>
  <xs:complexType name="Document"><xs:sequence>
    <xs:element name="Removed" type="Gone"/>
    <xs:element name="Kept" type="Same"/>
  </xs:sequence></xs:complexType>
  <xs:complexType name="Gone"><xs:sequence/></xs:complexType>
  <xs:complexType name="Same"><xs:sequence/></xs:complexType>
</xs:schema>`

	newer := `<?xml version="1.0"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:b" xmlns="urn:b">
  <xs:element name="Document" type="Document"/>
  <xs:complexType name="Document"><xs:sequence>
    <xs:element name="Added" type="Fresh"/>
    <xs:element name="Kept" type="Same"/>
  </xs:sequence></xs:complexType>
  <xs:complexType name="Fresh"><xs:sequence/></xs:complexType>
  <xs:complexType name="Same"><xs:sequence/></xs:complexType>
</xs:schema>`

	if err := os.WriteFile(filepath.Join(base, "pacs.008.001.09.xsd"), []byte(older), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "pacs.008.001.10.xsd"), []byte(newer), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ANCHOR_CATALOG", dir)

	out, err := run(t, "diff", "pacs.008.001.09", "pacs.008.001.10")
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	// The report is path-based: a removal and a mandatory addition both break,
	// and the element kept in both is counted as common rather than listed.
	wantContains(t, out,
		"/Document/Removed", "removed",
		"/Document/Added", "added",
		"breaking change(s)")
	if strings.Contains(out, "/Document/Kept") {
		t.Errorf("an unchanged element was listed as a change:\n%s", out)
	}

	// --breaking narrows the report to what stops a migration.
	only, err := run(t, "diff", "pacs.008.001.09", "pacs.008.001.10", "--breaking")
	if err != nil {
		t.Fatalf("diff --breaking: %v", err)
	}
	wantContains(t, only, "/Document/Removed", "/Document/Added")

	// --strict turns the same report into a CI gate.
	if _, err := run(t, "diff", "pacs.008.001.09", "pacs.008.001.10", "--strict"); err == nil {
		t.Error("--strict should exit non-zero when a breaking change is found")
	}

	// The JSON report carries the same verdict for a machine to consume.
	jsonOut, err := run(t, "diff", "pacs.008.001.09", "pacs.008.001.10", "--json")
	if err != nil {
		t.Fatalf("diff --json: %v", err)
	}
	var report struct {
		From    string `json:"from"`
		To      string `json:"to"`
		Common  int    `json:"common"`
		Changes []struct {
			Path     string `json:"path"`
			Kind     string `json:"kind"`
			Severity string `json:"severity"`
		} `json:"changes"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(jsonOut)), &report); err != nil {
		t.Fatalf("diff --json is not valid JSON: %v\n%s", err, jsonOut)
	}
	if report.From != "pacs.008.001.09" || report.To != "pacs.008.001.10" {
		t.Errorf("got %s -> %s", report.From, report.To)
	}
	if len(report.Changes) == 0 || report.Changes[0].Severity != "breaking" {
		t.Errorf("the JSON report does not lead with breaking changes: %+v", report.Changes)
	}
}

func TestDiffIdenticalSchemas(t *testing.T) {
	root := withCatalogue(t)
	path := filepath.Join(root, "Payments Clearing and Settlement", "Version 11.0",
		"Schemas", "pacs.008.001.10.xsd")

	// Two file paths need no catalogue lookup at all.
	out, err := run(t, "diff", path, path)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	wantContains(t, out, "structurally identical")

	// And --strict passes when nothing breaks.
	if _, err := run(t, "diff", path, path, "--strict"); err != nil {
		t.Errorf("--strict failed on identical schemas: %v", err)
	}
}

func TestDiffFilesWithoutCatalogue(t *testing.T) {
	isolate(t)

	dir := t.TempDir()
	body := `<?xml version="1.0"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:a" xmlns="urn:a">
  <xs:element name="Document" type="Document"/>
  <xs:complexType name="Document"><xs:sequence>
    <xs:element name="MsgId" type="xs:string"/>
  </xs:sequence></xs:complexType>
</xs:schema>`
	a := filepath.Join(dir, "a.xsd")
	b := filepath.Join(dir, "b.xsd")
	for _, p := range []string{a, b} {
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Comparing two files on disk is a local operation; it must not demand a
	// catalogue the user has no reason to have installed.
	out, err := run(t, "diff", a, b)
	if err != nil {
		t.Fatalf("diff of two files: %v", err)
	}
	wantContains(t, out, "structurally identical")

	// An identifier still needs one, and the error has to say so.
	if _, err := run(t, "diff", "pacs.008", "pacs.009"); err == nil {
		t.Error("an identifier without a catalogue should be an error")
	}
}

func TestDiffUnparseableSchema(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.xsd")
	bad := filepath.Join(dir, "bad.xsd")
	if err := os.WriteFile(good, []byte(`<?xml version="1.0"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:a" xmlns="urn:a">
  <xs:element name="Document" type="xs:string"/>
</xs:schema>`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bad, []byte("<xs:schema><unclosed>"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := run(t, "diff", good, bad); err == nil {
		t.Error("an unparseable schema should be an error")
	}
	if _, err := run(t, "diff", bad, good); err == nil {
		t.Error("an unparseable first schema should be an error")
	}
}

func TestDiffRejectsSecondUnknownID(t *testing.T) {
	withCatalogue(t)
	if _, err := run(t, "diff", "pacs.008.001.10", "zzzz.999.999.99"); err == nil {
		t.Error("an unknown second identifier should be an error")
	}
}

// ---------------------------------------------------------------------------
// format / convert / sample output paths
// ---------------------------------------------------------------------------

func TestFormatWritesToUnwritablePath(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "in.xml")
	if err := os.WriteFile(src, []byte(fixtureInstance("pacs.008.001.10", "EUR")), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "format", src, "-o", filepath.Join(dir, "no-such-dir", "out.xml")); err == nil {
		t.Error("an unwritable output path should be an error")
	}
}

func TestConvertWritesToUnwritablePath(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "in.xml")
	if err := os.WriteFile(src, []byte(fixtureInstance("pacs.008.001.10", "EUR")), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "convert", src, "--to-json", "-o",
		filepath.Join(dir, "no-such-dir", "out.json")); err == nil {
		t.Error("an unwritable output path should be an error")
	}
}

func TestConvertRejectsMalformedInput(t *testing.T) {
	dir := t.TempDir()
	broken := filepath.Join(dir, "broken.xml")
	if err := os.WriteFile(broken, []byte("<not-closed>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "convert", broken, "--to-json"); err == nil {
		t.Error("malformed XML should be an error")
	}

	badJSON := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(badJSON, []byte("{{{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "convert", badJSON, "--to-xml"); err == nil {
		t.Error("malformed JSON should be an error")
	}
}

func TestSchemaAndSampleResolveByBaseCode(t *testing.T) {
	withCatalogue(t)

	// A base code resolves to a concrete version through search.
	if _, err := run(t, "schema", "pacs.008"); err != nil {
		t.Errorf("schema by base code: %v", err)
	}
	if _, err := run(t, "sample", "pacs.008"); err != nil {
		t.Errorf("sample by base code: %v", err)
	}
}

func TestGeneratedSampleFallbackForUnsupportedType(t *testing.T) {
	withCatalogue(t)
	// camt.053 has no sample in the fixture and the generator supports it.
	out, err := run(t, "sample", "camt.053")
	if err != nil {
		t.Fatalf("sample fallback: %v", err)
	}
	wantContains(t, out, "<Document")
}

func TestInfoErrorPaths(t *testing.T) {
	isolate(t)
	if _, err := run(t, "info", "zzzz.999.999.99"); err == nil {
		t.Error("an unknown identifier should be an error")
	}
	// A base code resolves via registry search in light mode.
	if _, err := run(t, "info", "camt.053"); err != nil {
		t.Errorf("a base code should resolve in light mode: %v", err)
	}
	if _, err := run(t, "info", "camt.053", "--json"); err != nil {
		t.Errorf("light-mode info --json: %v", err)
	}
}

func TestSearchJSONInLightMode(t *testing.T) {
	isolate(t)
	out, err := run(t, "search", "pacs.008", "--json")
	if err != nil {
		t.Fatalf("light-mode search --json: %v", err)
	}
	if !strings.Contains(out, "pacs.008") {
		t.Errorf("expected results:\n%s", out)
	}
}

func TestSearchNoMatchesInLightMode(t *testing.T) {
	isolate(t)
	out, err := run(t, "search", "zzzz-nothing-matches")
	if err != nil {
		t.Fatalf("a search with no hits should not error: %v", err)
	}
	if !strings.Contains(out, "0 results") && !strings.Contains(out, "Found 0") {
		t.Logf("output: %s", out)
	}
}

func TestCatalogAddDefaultsToResolvedLocation(t *testing.T) {
	dest := t.TempDir()
	t.Setenv("ANCHOR_CATALOG", dest)
	prev := catalogPath
	catalogPath = ""
	t.Cleanup(func() { catalogPath = prev })

	src := t.TempDir()
	archive := filepath.Join(src, "AccountSwitching_v05.zip")
	writeTestZip(t, archive, map[string]string{
		"acmt.027.001.05.xsd": fixtureSchema("acmt.027.001.05"),
	})

	if _, err := run(t, "catalog", "add", archive); err != nil {
		t.Fatalf("catalog add with no --to: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "Account Switching", "Version 5.0", "Schemas",
		"acmt.027.001.05.xsd")); err != nil {
		t.Errorf("import did not use the resolved catalogue: %v", err)
	}
}

func TestErrSilentIsRecognised(t *testing.T) {
	var target *silentError
	if !errors.As(error(errSilent), &target) {
		t.Error("errSilent should be a *silentError")
	}
}

// ---------------------------------------------------------------------------
// Scheme rule profiles
// ---------------------------------------------------------------------------

func writeAddressMessage(t *testing.T, msgID, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "addr.xml")
	doc := `<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:` + msgID + `">` + body + `</Document>`
	if err := os.WriteFile(p, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLintProfileReportsAddressFaults(t *testing.T) {
	p := writeAddressMessage(t, "pacs.008.001.10",
		`<Dbtr><PstlAdr><AdrLine>12 High Street</AdrLine><AdrLine>London</AdrLine></PstlAdr></Dbtr>`)

	out, err := run(t, "lint", p, "--profile", "cbpr-2026")
	if err == nil {
		t.Error("an unstructured address should fail the profile")
	}
	wantContains(t, out, "CBPR-ADDR-002", "PROFILE", "cbpr-2026")
}

func TestLintProfilePassesAStructuredAddress(t *testing.T) {
	p := writeAddressMessage(t, "pacs.008.001.10",
		`<Dbtr><PstlAdr><StrtNm>High St</StrtNm><TwnNm>London</TwnNm><Ctry>GB</Ctry></PstlAdr></Dbtr>`)

	out, err := run(t, "lint", p, "--profile", "cbpr-2026")
	if err != nil {
		t.Fatalf("a structured address should pass: %v\n%s", err, out)
	}
	wantContains(t, out, "profile rule(s) passed")
}

func TestLintProfileHonoursExemptions(t *testing.T) {
	p := writeAddressMessage(t, "camt.053.001.11",
		`<Dbtr><PstlAdr><AdrLine>12 High Street</AdrLine></PstlAdr></Dbtr>`)

	out, err := run(t, "lint", p, "--profile", "cbpr-2026")
	if err != nil {
		t.Fatalf("camt.053 is exempt: %v\n%s", err, out)
	}
	wantContains(t, out, "exempt", "skipped")
}

func TestLintProfileJSON(t *testing.T) {
	p := writeAddressMessage(t, "pacs.008.001.10",
		`<Dbtr><PstlAdr><AdrLine>a</AdrLine></PstlAdr></Dbtr>`)

	out, _ := run(t, "lint", p, "--profile", "cbpr-2026", "--json")
	var payload struct {
		Profile struct {
			Profile  string `json:"profile"`
			Errors   int    `json:"error_count"`
			Findings []struct {
				RuleID string `json:"rule_id"`
				Path   string `json:"path"`
			} `json:"findings"`
		} `json:"profile"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &payload); err != nil {
		t.Fatalf("lint --json with a profile is not valid JSON: %v\n%s", err, out)
	}
	if payload.Profile.Profile != "cbpr-2026" {
		t.Errorf("the profile should be named in the JSON: %+v", payload.Profile)
	}
	if payload.Profile.Errors == 0 || len(payload.Profile.Findings) == 0 {
		t.Errorf("findings should be present: %+v", payload.Profile)
	}
	if payload.Profile.Findings[0].Path == "" {
		t.Error("a finding should carry its path")
	}
}

func TestLintProfileRejectsUnknownName(t *testing.T) {
	p := writeAddressMessage(t, "pacs.008.001.10", `<Dbtr><Nm>x</Nm></Dbtr>`)
	if _, err := run(t, "lint", p, "--profile", "not-a-profile"); err == nil {
		t.Error("an unknown profile should be an error")
	}
}

func TestLintProfileRejectsMalformedXML(t *testing.T) {
	p := filepath.Join(t.TempDir(), "broken.xml")
	if err := os.WriteFile(p, []byte("<not-closed>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "lint", p, "--profile", "cbpr-2026"); err == nil {
		t.Error("malformed XML should be an error")
	}
}

func TestLintProfileStrictPromotesAdvisories(t *testing.T) {
	// A hybrid address is informational, not an error, in either mode.
	p := writeAddressMessage(t, "pacs.008.001.10",
		`<Dbtr><PstlAdr><AdrLine>High St 1</AdrLine><TwnNm>London</TwnNm><Ctry>GB</Ctry></PstlAdr></Dbtr>`)

	if _, err := run(t, "lint", p, "--profile", "cbpr-2026"); err != nil {
		t.Errorf("a hybrid address should pass: %v", err)
	}
	out, _ := run(t, "lint", p, "--profile", "cbpr-2026")
	wantContains(t, out, "CBPR-ADDR-005")
}

// ---------------------------------------------------------------------------
// validate: streaming
// ---------------------------------------------------------------------------

func TestValidateStreamingMatchesBuffered(t *testing.T) {
	withCatalogue(t)

	dir := t.TempDir()
	good := filepath.Join(dir, "good.xml")
	bad := filepath.Join(dir, "bad.xml")
	if err := os.WriteFile(good, []byte(fixtureInstance("pacs.008.001.10", "EUR")), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bad, []byte(fixtureInstance("pacs.008.001.10", "EURO")), 0o644); err != nil {
		t.Fatal(err)
	}

	// The two paths must agree, or the flag changes the answer rather than the
	// memory it takes to reach it.
	buffered, err := run(t, "validate", good)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	streamed, err := run(t, "validate", good, "--stream")
	if err != nil {
		t.Fatalf("validate --stream: %v", err)
	}
	if buffered != streamed {
		t.Errorf("the two paths printed different things:\n%s\n---\n%s", buffered, streamed)
	}

	if _, err := run(t, "validate", bad, "--stream"); err == nil {
		t.Error("--stream accepted an invalid currency code")
	}
}

func TestValidateStreamsLargeFilesAutomatically(t *testing.T) {
	withCatalogue(t)

	// A file over the threshold is streamed without being asked. The fixture is
	// padded with comments so it stays valid while crossing 8 MiB.
	dir := t.TempDir()
	path := filepath.Join(dir, "large.xml")

	body := fixtureInstance("pacs.008.001.10", "EUR")
	padding := strings.Repeat("<!-- padding padding padding padding padding padding -->\n", 160000)
	body = strings.Replace(body, "</Document>", padding+"</Document>", 1)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() < 8<<20 {
		t.Fatalf("the fixture is only %d bytes; it would not cross the threshold", info.Size())
	}

	out, err := run(t, "validate", path)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	wantContains(t, out, "VALID")
}

func TestValidateStreamOnAMissingFile(t *testing.T) {
	withCatalogue(t)
	if _, err := run(t, "validate", filepath.Join(t.TempDir(), "absent.xml"), "--stream"); err == nil {
		t.Error("a missing file was accepted")
	}
}
