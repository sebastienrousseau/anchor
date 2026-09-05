// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package iso20022_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebastienrousseau/askiso/pkg/iso20022"
)

// fixtureCatalogue builds a tiny catalogue so tests never depend on the
// developer having downloaded one.
func fixtureCatalogue(t *testing.T, ids ...string) *iso20022.Catalogue {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "Payments Clearing and Settlement", "Version 11.0", "Schemas")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		if err := os.WriteFile(filepath.Join(dir, id+".xsd"), []byte("<xs:schema/>"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	c, err := iso20022.OpenCatalogue(root)
	if err != nil {
		t.Fatalf("OpenCatalogue: %v", err)
	}
	return c
}

// A nil *Catalogue is light mode, and every read path must accept it. This is
// the shape the browser build uses, so a nil-pointer panic here would take the
// website down.
func TestNilCatalogueIsLightMode(t *testing.T) {
	var c *iso20022.Catalogue

	if got := c.Root(); got != "" {
		t.Errorf("Root() on nil = %q, want empty", got)
	}
	if got := c.Installed(); got != 0 {
		t.Errorf("Installed() on nil = %d, want 0", got)
	}

	info, err := c.Lookup("pacs.008.001.10")
	if err != nil {
		t.Fatalf("Lookup on nil catalogue: %v", err)
	}
	if info.Installed {
		t.Error("nothing can be installed in light mode")
	}
	if len(info.Sets) == 0 {
		t.Error("light mode must name the message sets that publish the message")
	}
	if info.DomainName != "Payments Clearing and Settlement" {
		t.Errorf("DomainName = %q", info.DomainName)
	}

	hits, err := c.Search("camt.053")
	if err != nil {
		t.Fatalf("Search on nil catalogue: %v", err)
	}
	if len(hits) == 0 {
		t.Error("light-mode search should find camt.053 versions")
	}

	counts, err := c.DomainCounts()
	if err != nil {
		t.Fatalf("DomainCounts on nil catalogue: %v", err)
	}
	if counts["pacs"] == 0 {
		t.Error("light-mode domain counts should include pacs")
	}
}

func TestLookupPrefersInstalledSchema(t *testing.T) {
	c := fixtureCatalogue(t, "pacs.008.001.10")

	info, err := c.Lookup("pacs.008.001.10")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if !info.Installed {
		t.Fatal("an installed message must report Installed=true")
	}
	if info.SchemaPath == "" {
		t.Error("an installed message must carry a schema path")
	}
	if info.Category != "Payments Clearing and Settlement" || info.Version != "Version 11.0" {
		t.Errorf("category/version wrong: %+v", info)
	}

	// A real message that is not installed still resolves, via the registry.
	other, err := c.Lookup("camt.053.001.11")
	if err != nil {
		t.Fatalf("Lookup of an uninstalled message: %v", err)
	}
	if other.Installed {
		t.Error("camt.053.001.11 is not in this fixture")
	}
	if len(other.Sets) == 0 {
		t.Error("an uninstalled message must name its message sets")
	}
}

func TestLookupRejectsUnknown(t *testing.T) {
	var c *iso20022.Catalogue
	if _, err := c.Lookup("zzzz.999.999.99"); err == nil {
		t.Error("an identifier outside the standard must be an error")
	}
}

func TestSchemaPathReportsWhatToDownload(t *testing.T) {
	var c *iso20022.Catalogue

	_, err := c.SchemaPath("pacs.008.001.10")
	if err == nil {
		t.Fatal("expected an error with no catalogue")
	}
	if !errors.Is(err, iso20022.ErrSchemaNotInstalled) {
		t.Errorf("error should match ErrSchemaNotInstalled, got %T", err)
	}

	var missing *iso20022.SchemaMissingError
	if !errors.As(err, &missing) {
		t.Fatalf("error should be a *SchemaMissingError, got %T", err)
	}
	if len(missing.Sets) == 0 {
		t.Error("the error must name at least one message set")
	}
	if !strings.Contains(err.Error(), "iso20022.org") {
		t.Errorf("the message must point at the Registration Authority:\n%s", err)
	}

	// With the schema installed it resolves normally.
	full := fixtureCatalogue(t, "pacs.008.001.10")
	p, err := full.SchemaPath("pacs.008.001.10")
	if err != nil {
		t.Fatalf("SchemaPath: %v", err)
	}
	if !strings.HasSuffix(p, "pacs.008.001.10.xsd") {
		t.Errorf("unexpected path %q", p)
	}
}

func TestCatalogueCounts(t *testing.T) {
	c := fixtureCatalogue(t, "pacs.008.001.10", "pacs.009.001.10")
	if got := c.Installed(); got != 2 {
		t.Errorf("Installed() = %d, want 2", got)
	}
	if c.Root() == "" {
		t.Error("Root() should report where the catalogue came from")
	}

	counts, err := c.DomainCounts()
	if err != nil {
		t.Fatal(err)
	}
	if counts["pacs"] != 2 {
		t.Errorf("pacs count = %d, want 2", counts["pacs"])
	}
}

func TestOpenCatalogueFailsWithoutOne(t *testing.T) {
	t.Setenv("ASKISO_CATALOG", "")
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LocalAppData", t.TempDir())
	t.Chdir(t.TempDir())

	if _, err := iso20022.OpenCatalogue(""); err == nil {
		t.Error("expected an error when no catalogue exists")
	}
}

func TestMessageSetsAreComplete(t *testing.T) {
	sets, err := iso20022.MessageSets()
	if err != nil {
		t.Fatal(err)
	}
	if len(sets) < 250 {
		t.Errorf("only %d message sets; the RA publishes far more", len(sets))
	}
	for _, s := range sets {
		if s.ID == "" || s.Name == "" {
			t.Fatalf("incomplete set record: %+v", s)
		}
		if !strings.HasPrefix(s.DownloadURL(), "https://www.iso20022.org/") {
			t.Fatalf("bad download URL: %s", s.DownloadURL())
		}
	}
	// Sorted by name so the output is stable for callers that render it.
	for i := 1; i < len(sets); i++ {
		if sets[i-1].Name > sets[i].Name {
			t.Fatalf("not sorted: %q before %q", sets[i-1].Name, sets[i].Name)
		}
	}
}

// The website serialises these straight to JSON, so field names must be stable
// and lowercase. A rename here silently breaks the page.
func TestJSONShapeIsStable(t *testing.T) {
	var c *iso20022.Catalogue
	info, err := c.Lookup("pacs.008.001.10")
	if err != nil {
		t.Fatal(err)
	}

	b, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	var generic map[string]any
	if err := json.Unmarshal(b, &generic); err != nil {
		t.Fatal(err)
	}

	for _, key := range []string{"id", "base_code", "domain", "domain_name", "installed", "message_sets"} {
		if _, ok := generic[key]; !ok {
			t.Errorf("MessageInfo JSON is missing %q: %s", key, b)
		}
	}

	sets, _ := generic["message_sets"].([]any)
	if len(sets) == 0 {
		t.Fatal("expected message sets in the JSON")
	}
	first, _ := sets[0].(map[string]any)
	for _, key := range []string{"id", "name", "version", "url"} {
		if _, ok := first[key]; !ok {
			t.Errorf("MessageSet JSON is missing %q: %v", key, first)
		}
	}
	if url, _ := first["url"].(string); !strings.Contains(url, "/message-set/") {
		t.Errorf("serialised url is wrong: %v", first["url"])
	}
}

func TestDomainName(t *testing.T) {
	cases := map[string]string{
		"pacs": "Payments Clearing and Settlement",
		"PACS": "Payments Clearing and Settlement",
		"camt": "Cash Management",
		"zzzz": "ZZZZ",
	}
	for in, want := range cases {
		if got := iso20022.DomainName(in); got != want {
			t.Errorf("DomainName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCatalogueLocations(t *testing.T) {
	locs := iso20022.CatalogueLocations()
	if len(locs) == 0 {
		t.Fatal("there should be at least one conventional location")
	}
	for _, l := range locs {
		if !strings.Contains(l, "askiso") {
			t.Errorf("location %q should be namespaced under askiso", l)
		}
	}
}

func TestSequenceDiagram(t *testing.T) {
	if d := iso20022.SequenceDiagram("pacs.008", "sepa", "mermaid"); !strings.Contains(d, "sequenceDiagram") {
		t.Errorf("mermaid output missing its header:\n%s", d)
	}
	if d := iso20022.SequenceDiagram("pacs.008", "sepa", "ascii"); strings.Contains(d, "sequenceDiagram") {
		t.Error("ascii format should not emit mermaid")
	}
}

func TestFieldValidators(t *testing.T) {
	if ok, _ := iso20022.ValidateIBAN("DE89370400440532013000"); !ok {
		t.Error("a valid IBAN was rejected")
	}
	if ok, reason := iso20022.ValidateIBAN("DE00370400440532013000"); ok || reason == "" {
		t.Error("a bad checksum should be rejected with a reason")
	}
	if ok, _ := iso20022.ValidateBIC("DEUTDEDDXXX"); !ok {
		t.Error("a valid BIC was rejected")
	}
	if ok, _ := iso20022.ValidateBIC("NOPE"); ok {
		t.Error("a malformed BIC was accepted")
	}
	if ok, _ := iso20022.ValidateUETR("e1b2c3d4-5678-4abc-8def-1234567890ab"); !ok {
		t.Error("a valid UETR was rejected")
	}
	if ok, _ := iso20022.ValidateUETR("e1b2c3d4-5678-1abc-8def-1234567890ab"); ok {
		t.Error("a non-v4 UUID was accepted as a UETR")
	}
}

func TestGenerateAndLintAgree(t *testing.T) {
	for _, preset := range []string{"sepa", "target2", "chaps"} {
		t.Run(preset, func(t *testing.T) {
			opts := iso20022.DefaultGeneratorOptions("pacs.008")
			opts.Preset = preset

			xml, err := iso20022.Generate(opts)
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			res, err := iso20022.Lint([]byte(xml), "generated.xml")
			if err != nil {
				t.Fatalf("Lint: %v", err)
			}
			if res.Errors != 0 {
				t.Errorf("preset %s produced output its own linter rejects: %+v", preset, res.Issues)
			}
		})
	}
}

func TestStandardIsEmbedded(t *testing.T) {
	reg, err := iso20022.Standard()
	if err != nil {
		t.Fatal(err)
	}
	if len(reg.Messages) < 2000 {
		t.Errorf("only %d messages in the embedded standard", len(reg.Messages))
	}
}

func TestAllCodesAndMappings(t *testing.T) {
	if len(iso20022.AllCodes()) == 0 {
		t.Error("AllCodes returned nothing")
	}
	if len(iso20022.AllMappings()) == 0 {
		t.Error("AllMappings returned nothing")
	}
	if got := iso20022.LookupCode("AC04"); len(got) == 0 || got[0].Code != "AC04" {
		t.Errorf("LookupCode(AC04) = %v", got)
	}
	if _, ok := iso20022.TranslateSWIFT("MT103"); !ok {
		t.Error("MT103 should have a mapping")
	}
	if _, ok := iso20022.TranslateSWIFT("MT999999"); ok {
		t.Error("an unknown MT code should not resolve")
	}
}

func TestSearchWithInstalledCatalogue(t *testing.T) {
	c := fixtureCatalogue(t, "pacs.008.001.10", "pacs.009.001.10")

	hits, err := c.Search("pacs.008")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("expected hits from the installed catalogue")
	}
	for _, h := range hits {
		if !h.Installed {
			t.Errorf("%s should report Installed=true", h.ID)
		}
		if h.SchemaPath == "" {
			t.Errorf("%s should carry a schema path", h.ID)
		}
	}
}

func TestValidateThroughCatalogue(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "Payments Clearing and Settlement", "Version 11.0", "Schemas")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	schema := `<?xml version="1.0"?>
<xs:schema xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10"
           xmlns:xs="http://www.w3.org/2001/XMLSchema" elementFormDefault="qualified"
           targetNamespace="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">
  <xs:element name="Document" type="Doc"/>
  <xs:complexType name="Doc"><xs:sequence>
    <xs:element name="Ccy" type="Cur"/>
  </xs:sequence></xs:complexType>
  <xs:simpleType name="Cur"><xs:restriction base="xs:string">
    <xs:pattern value="[A-Z]{3,3}"/></xs:restriction></xs:simpleType>
</xs:schema>`
	if err := os.WriteFile(filepath.Join(dir, "pacs.008.001.10.xsd"), []byte(schema), 0o644); err != nil {
		t.Fatal(err)
	}

	cat, err := iso20022.OpenCatalogue(root)
	if err != nil {
		t.Fatal(err)
	}

	good := []byte(`<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10"><Ccy>EUR</Ccy></Document>`)
	res, err := cat.Validate(good)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if !res.Valid {
		t.Errorf("the document should be valid: %v", res.Errors)
	}

	bad := []byte(`<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10"><Ccy>EURO</Ccy></Document>`)
	if res, _ := cat.Validate(bad); res != nil && res.Valid {
		t.Error("the invalid document should be rejected")
	}

	// A document with no ISO namespace cannot be resolved.
	if _, err := cat.Validate([]byte(`<root/>`)); err == nil {
		t.Error("a document with no ISO namespace should be an error")
	}

	// A message whose schema is not installed.
	notThere := []byte(`<Document xmlns="urn:iso:std:iso:20022:tech:xsd:camt.053.001.11"/>`)
	if _, err := cat.Validate(notThere); err == nil {
		t.Error("an uninstalled schema should be reported")
	}
}

func TestValidateFileAndAgainstErrors(t *testing.T) {
	dir := t.TempDir()
	junk := filepath.Join(dir, "junk.xsd")
	if err := os.WriteFile(junk, []byte("not a schema"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := iso20022.ValidateFile([]byte(`<a/>`), junk); err == nil {
		t.Error("an unparseable schema file should be an error")
	}
	if _, err := iso20022.ValidateFile([]byte(`<a/>`), filepath.Join(dir, "missing.xsd")); err == nil {
		t.Error("a missing schema file should be an error")
	}
	if _, err := iso20022.ValidateAgainst([]byte(`<a/>`), []byte("not a schema")); err == nil {
		t.Error("an unparseable schema document should be an error")
	}
}

func TestMessageIDFromInstance(t *testing.T) {
	id, err := iso20022.MessageIDFromInstance(
		[]byte(`<Document xmlns="urn:iso:std:iso:20022:tech:xsd:camt.053.001.11"/>`))
	if err != nil {
		t.Fatal(err)
	}
	if id != "camt.053.001.11" {
		t.Errorf("id = %q", id)
	}

	if _, err := iso20022.MessageIDFromInstance([]byte(`<root/>`)); err == nil {
		t.Error("a document with no ISO namespace should be an error")
	}

	// Namespace detection parses XML rather than scanning a fixed prefix.
	big := append([]byte(strings.Repeat(" ", 9000)),
		[]byte(`<Document xmlns="urn:iso:std:iso:20022:tech:xsd:camt.053.001.11"/>`)...)
	if id, err := iso20022.MessageIDFromInstance(big); err != nil || id != "camt.053.001.11" {
		t.Errorf("large prefix: id=%q err=%v", id, err)
	}

	spoofed := []byte(`<!-- urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10 --><Document xmlns="urn:iso:std:iso:20022:tech:xsd:camt.053.001.11"/>`)
	if id, err := iso20022.MessageIDFromInstance(spoofed); err != nil || id != "camt.053.001.11" {
		t.Errorf("comment spoof selected id=%q err=%v", id, err)
	}

	envelope := []byte(`<Envelope><AppHdr xmlns="urn:iso:std:iso:20022:tech:xsd:head.001.001.02"/><Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10"/></Envelope>`)
	if id, err := iso20022.MessageIDFromInstance(envelope); err != nil || id != "pacs.008.001.10" {
		t.Errorf("envelope selected id=%q err=%v", id, err)
	}
}

func TestJSONToXMLReExport(t *testing.T) {
	out, err := iso20022.JSONToXML([]byte(`{"Doc":{"A":"1"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "<A>1</A>") {
		t.Errorf("unexpected output: %s", out)
	}
}

func TestDefaultGeneratorOptionsReExport(t *testing.T) {
	if o := iso20022.DefaultGeneratorOptions("camt.053"); o.MsgType != "camt.053" {
		t.Errorf("MsgType = %q", o.MsgType)
	}
}

func TestSeverityConstants(t *testing.T) {
	if iso20022.SeverityError == "" || iso20022.SeverityWarning == "" || iso20022.SeverityInfo == "" {
		t.Error("severity constants should be populated")
	}
}

// ---------------------------------------------------------------------------
// Scheme rule profiles
// ---------------------------------------------------------------------------

const unstructuredMessage = `<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">
  <Dbtr><PstlAdr><AdrLine>12 High Street</AdrLine><AdrLine>London</AdrLine></PstlAdr></Dbtr>
</Document>`

const structuredMessage = `<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">
  <Dbtr><PstlAdr><StrtNm>High St</StrtNm><TwnNm>London</TwnNm><Ctry>GB</Ctry></PstlAdr></Dbtr>
</Document>`

func TestRuleProfilesAreListed(t *testing.T) {
	names := iso20022.RuleProfiles()
	if len(names) == 0 {
		t.Fatal("no profiles registered")
	}
	var found bool
	for _, n := range names {
		if n == "cbpr-2026" {
			found = true
		}
		if iso20022.DescribeProfile(n) == "" {
			t.Errorf("profile %q has no description", n)
		}
	}
	if !found {
		t.Errorf("the November 2026 profile should be available: %v", names)
	}
	if iso20022.DescribeProfile("no-such-profile") != "" {
		t.Error("an unknown profile has no description")
	}
}

func TestCheckProfileFindsAddressFaults(t *testing.T) {
	res, err := iso20022.CheckProfile([]byte(unstructuredMessage), "cbpr-2026", "m.xml")
	if err != nil {
		t.Fatalf("CheckProfile: %v", err)
	}
	if res.Valid() {
		t.Fatal("an unstructured address should be reported")
	}
	if res.Errors == 0 || len(res.Findings) == 0 {
		t.Errorf("expected findings: %+v", res)
	}
	for _, f := range res.Findings {
		if f.RuleID == "" || f.Path == "" || f.Remediation == "" {
			t.Errorf("incomplete finding: %+v", f)
		}
	}
}

func TestCheckProfileAcceptsAStructuredAddress(t *testing.T) {
	res, err := iso20022.CheckProfile([]byte(structuredMessage), "cbpr-2026", "m.xml")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Valid() {
		t.Errorf("a structured address should pass: %+v", res.Findings)
	}
}

func TestCheckProfileHonoursExemptions(t *testing.T) {
	exempt := strings.ReplaceAll(unstructuredMessage, "pacs.008.001.10", "camt.053.001.11")
	res, err := iso20022.CheckProfile([]byte(exempt), "cbpr-2026", "m.xml")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Valid() || res.Skipped == 0 {
		t.Errorf("camt.053 is out of scope: %+v", res)
	}
}

func TestCheckProfileErrors(t *testing.T) {
	if _, err := iso20022.CheckProfile([]byte(structuredMessage), "no-such-profile", "m.xml"); err == nil {
		t.Error("an unknown profile should be an error")
	}
	if _, err := iso20022.CheckProfile([]byte("<not-closed>"), "cbpr-2026", "m.xml"); err == nil {
		t.Error("malformed XML should be an error")
	}
}

func TestClassifyAddresses(t *testing.T) {
	shapes, err := iso20022.ClassifyAddresses([]byte(unstructuredMessage))
	if err != nil {
		t.Fatal(err)
	}
	if len(shapes) != 1 {
		t.Fatalf("expected one address, got %d", len(shapes))
	}
	for path, shape := range shapes {
		if shape != "unstructured" {
			t.Errorf("%s classified as %q", path, shape)
		}
		if !strings.Contains(path, "PstlAdr") {
			t.Errorf("path should name the address element: %q", path)
		}
	}

	hybrid := `<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">
	  <Dbtr><PstlAdr><AdrLine>a</AdrLine><TwnNm>London</TwnNm><Ctry>GB</Ctry></PstlAdr></Dbtr>
	</Document>`
	shapes, err = iso20022.ClassifyAddresses([]byte(hybrid))
	if err != nil {
		t.Fatal(err)
	}
	for _, shape := range shapes {
		if shape != "hybrid" {
			t.Errorf("shape = %q, want hybrid", shape)
		}
	}

	if _, err := iso20022.ClassifyAddresses([]byte("<not-closed>")); err == nil {
		t.Error("malformed XML should be an error")
	}

	// A message with no addresses yields an empty map, not an error.
	none, err := iso20022.ClassifyAddresses([]byte(`<Document xmlns="urn:t"><A/></Document>`))
	if err != nil {
		t.Fatal(err)
	}
	if len(none) != 0 {
		t.Errorf("expected no addresses, got %v", none)
	}
}

// ExampleCheckProfile shows the November 2026 readiness check.
func ExampleCheckProfile() {
	msg := []byte(`<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">
  <Dbtr><PstlAdr><AdrLine>12 High Street</AdrLine></PstlAdr></Dbtr>
</Document>`)

	res, err := iso20022.CheckProfile(msg, "cbpr-2026", "payment.xml")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(res.Valid())
	// Output: false
}
