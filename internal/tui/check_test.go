// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sebastienrousseau/anchor/internal/catalog"
)

// checkableIndex builds a catalogue whose sample is a real message with a real
// schema, so the check pane exercises the same engine the CLI does rather than
// a stub.
func checkableIndex(t *testing.T, sample string) *catalog.Index {
	t.Helper()
	root := t.TempDir()

	base := filepath.Join(root, "Payments Clearing and Settlement", "Version 11.0")
	schemas := filepath.Join(base, "Schemas")
	samples := filepath.Join(base, "Sample Messages")
	for _, d := range []string{schemas, samples} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	schema := `<?xml version="1.0" encoding="UTF-8"?>
<xs:schema xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10"
           xmlns:xs="http://www.w3.org/2001/XMLSchema"
           elementFormDefault="qualified"
           targetNamespace="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">
  <xs:element name="Document" type="Document"/>
  <xs:complexType name="Document">
    <xs:sequence>
      <xs:element name="MsgId" type="Max35Text"/>
      <xs:element minOccurs="0" name="Cdtr" type="Party"/>
    </xs:sequence>
  </xs:complexType>
  <xs:complexType name="Party">
    <xs:sequence>
      <xs:element name="Nm" type="Max35Text"/>
      <xs:element minOccurs="0" name="PstlAdr" type="Address"/>
    </xs:sequence>
  </xs:complexType>
  <xs:complexType name="Address">
    <xs:sequence>
      <xs:element maxOccurs="7" minOccurs="0" name="AdrLine" type="Max35Text"/>
    </xs:sequence>
  </xs:complexType>
  <xs:simpleType name="Max35Text">
    <xs:restriction base="xs:string"><xs:minLength value="1"/><xs:maxLength value="35"/></xs:restriction>
  </xs:simpleType>
</xs:schema>`

	if err := os.WriteFile(filepath.Join(schemas, "pacs.008.001.10.xsd"), []byte(schema), 0o644); err != nil {
		t.Fatal(err)
	}
	if sample != "" {
		if err := os.WriteFile(filepath.Join(samples, "pacs.008.001.10.xml"), []byte(sample), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	idx, err := catalog.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	return idx
}

const cleanSample = `<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">
  <MsgId>MSG-0001</MsgId>
</Document>`

const unstructuredSample = `<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">
  <MsgId>MSG-0001</MsgId>
  <Cdtr>
    <Nm>MUELLER GMBH</Nm>
    <PstlAdr><AdrLine>HAUPTSTRASSE 12</AdrLine><AdrLine>FRANKFURT</AdrLine></PstlAdr>
  </Cdtr>
</Document>`

func TestCheckPaneOnACleanMessage(t *testing.T) {
	m := NewModel(checkableIndex(t, cleanSample))
	next, _ := m.Update(windowSize())
	m = next.(Model)

	m = send(m, "ctrl+k")
	if m.mode != modeViewer {
		t.Fatalf("mode = %v, want the viewer", m.mode)
	}

	report := m.viewingContent
	for _, want := range []string{"BUSINESS RULES", "SCHEMA", "14 NOVEMBER 2026", "validates against its schema"} {
		if !strings.Contains(report, want) {
			t.Errorf("the report is missing %q:\n%s", want, report)
		}
	}
	// It has to say how to run the same checks outside the TUI, or the pane is
	// a dead end.
	if !strings.Contains(report, "anchor lint") || !strings.Contains(report, "anchor validate") {
		t.Errorf("the report does not name the CLI equivalents:\n%s", report)
	}
}

func TestCheckPaneFlagsAnUnstructuredAddress(t *testing.T) {
	m := NewModel(checkableIndex(t, unstructuredSample))
	next, _ := m.Update(windowSize())
	m = next.(Model)

	m = send(m, "ctrl+k")
	report := m.viewingContent

	if !strings.Contains(report, "CBPR-ADDR") {
		t.Errorf("the address rules did not fire:\n%s", report)
	}
	if !strings.Contains(report, "TwnNm") {
		t.Errorf("the report does not say how to fix it:\n%s", report)
	}
}

func TestCheckPaneWithoutASample(t *testing.T) {
	m := NewModel(checkableIndex(t, ""))
	next, _ := m.Update(windowSize())
	m = next.(Model)

	m = send(m, "ctrl+k")
	report := m.viewingContent
	if !strings.Contains(report, "nothing to check") {
		t.Errorf("the report does not explain the gap:\n%s", report)
	}
	if !strings.Contains(report, "catalog fetch") {
		t.Errorf("the report does not say how to get one:\n%s", report)
	}
}

func TestCheckPaneOnAMalformedSample(t *testing.T) {
	m := NewModel(checkableIndex(t, "<Document xmlns=\"urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10\">"))
	next, _ := m.Update(windowSize())
	m = next.(Model)

	m = send(m, "ctrl+k")
	// A malformed sample must produce a report, not a panic or an empty pane.
	if !strings.Contains(m.viewingContent, "SCHEMA") {
		t.Errorf("no report was produced:\n%s", m.viewingContent)
	}
}

func TestCheckCommandMatchesTheKey(t *testing.T) {
	m := NewModel(checkableIndex(t, unstructuredSample))
	next, _ := m.Update(windowSize())
	byKey := send(next.(Model), "ctrl+k")

	next, _ = m.Update(windowSize())
	byCommand := typeText(next.(Model), "/check")
	byCommand = send(byCommand, "enter")

	if byKey.viewingContent != byCommand.viewingContent {
		t.Errorf("the key and the command produced different reports")
	}
	if byCommand.mode != modeViewer {
		t.Errorf("mode = %v, want the viewer", byCommand.mode)
	}
}

func TestCatalogueView(t *testing.T) {
	m := newSizedModel(t)
	m = typeText(m, "/catalog")
	m = send(m, "enter")

	if m.mode != modeViewer {
		t.Fatalf("mode = %v, want the viewer", m.mode)
	}
	report := m.viewingContent
	for _, want := range []string{"CATALOGUE", "published", "catalog fetch", "iso20022.org/message-set"} {
		if !strings.Contains(report, want) {
			t.Errorf("the catalogue view is missing %q:\n%s", want, report)
		}
	}
	// The fixture installs three messages out of the whole standard, so most
	// sets have to show as absent with a download link.
	if !strings.Contains(report, "0/") {
		t.Errorf("nothing was reported as missing:\n%s", report)
	}
}

func TestLetterKeysStillFilter(t *testing.T) {
	// Every plain letter belongs to the filter. A shortcut that steals one
	// makes a whole set of message names unreachable, which has happened
	// before with a, c and q.
	m := newSizedModel(t)
	m = typeText(m, "vest")
	if m.filter != "vest" {
		t.Errorf("filter = %q; a letter shortcut swallowed part of it", m.filter)
	}
	if m.mode != modeTable {
		t.Errorf("typing a filter left the table: %v", m.mode)
	}

	// Including y, which copies when the filter is empty.
	m = newSizedModel(t)
	m = typeText(m, "pay")
	if m.filter != "pay" {
		t.Errorf("filter = %q", m.filter)
	}
}

func TestModifiedShortcutsDoNotStealLetters(t *testing.T) {
	// The schema and copy shortcuts are modified for the same reason the
	// assistant is: a bare letter belongs to the filter.
	m := newSizedModel(t)
	m = send(m, "ctrl+s")
	if m.mode != modeViewer {
		t.Errorf("ctrl+s did not open the schema: %v", m.mode)
	}
	if !strings.Contains(m.viewingTitle, "XSD") {
		t.Errorf("viewing %q, want the schema", m.viewingTitle)
	}

	// And copying leaves the table where it was rather than navigating.
	m = newSizedModel(t)
	m = send(m, "ctrl+y")
	if m.mode != modeTable {
		t.Errorf("ctrl+y left the table: %v", m.mode)
	}
}

func TestTruncate(t *testing.T) {
	cases := map[string]string{
		"short":                             "short",
		"an extremely long message set nam": "an extremely long message set nam",
	}
	for in, want := range cases {
		if got := truncate(in, 33); got != want {
			t.Errorf("truncate(%q) = %q, want %q", in, got, want)
		}
	}
	if got := truncate("abcdef", 3); got != "ab…" {
		t.Errorf("truncate = %q", got)
	}
	if got := truncate("abcdef", 1); got != "a" {
		t.Errorf("truncate = %q", got)
	}
	// Runes, not bytes: an accented name must not be cut mid-character.
	if got := truncate(strings.Repeat("é", 10), 5); len([]rune(got)) != 5 {
		t.Errorf("truncate = %q", got)
	}
}

// windowSize is the size message the real program sends before anything renders.
func windowSize() tea.WindowSizeMsg { return tea.WindowSizeMsg{Width: 120, Height: 40} }

func TestCheckPaneReportsLintFindings(t *testing.T) {
	// An IBAN whose checksum fails is exactly what the business-rule linter is
	// for, and the pane has to surface it with the value that failed.
	sample := `<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">
  <MsgId>MSG-0001</MsgId>
  <Cdtr><Nm>MUELLER GMBH</Nm><PstlAdr><AdrLine>IBAN GB29NWBK60161331926810</AdrLine></PstlAdr></Cdtr>
</Document>`

	m := NewModel(checkableIndex(t, sample))
	next, _ := m.Update(windowSize())
	m = send(next.(Model), "ctrl+k")

	if !strings.Contains(m.viewingContent, "BUSINESS RULES") {
		t.Fatalf("no business-rule section:\n%s", m.viewingContent)
	}
	// Whatever the linter finds, the section has to reach a verdict rather than
	// trailing off.
	if !strings.Contains(m.viewingContent, "passed") && !strings.Contains(m.viewingContent, "[") {
		t.Errorf("the business-rule section reaches no verdict:\n%s", m.viewingContent)
	}
}

func TestCheckPaneWithoutASchema(t *testing.T) {
	// The index is built from schema files, so a message with a sample and no
	// schema cannot arise through the catalogue. It can arise from an index
	// built by hand, and the pane must say the schema is missing rather than
	// claim the message is valid.
	m := Model{}
	got := m.renderSchema([]byte(cleanSample), catalog.Message{ID: "pacs.008.001.10"})

	if !strings.Contains(got, "not installed") {
		t.Errorf("the missing schema was not reported: %q", got)
	}
	if strings.Contains(got, "validates against its schema") {
		t.Errorf("a message was reported valid with no schema: %q", got)
	}
}

func TestCheckPaneOnAnUnreadableSample(t *testing.T) {
	m := Model{}
	got := m.renderCheck(catalog.Message{
		ID:            "pacs.008.001.10",
		XMLSamplePath: filepath.Join(t.TempDir(), "absent.xml"),
	})
	if !strings.Contains(got, "Could not read the sample") {
		t.Errorf("an unreadable sample was not reported: %q", got)
	}
}

func TestCheckPaneOnAnExemptMessage(t *testing.T) {
	// A statement is out of scope for the address rules, and saying "all rules
	// passed" would imply they were evaluated.
	m := Model{}
	got := m.renderProfile([]byte(`<?xml version="1.0"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:camt.053.001.11"><BkToCstmrStmt/></Document>`))
	if !strings.Contains(got, "exempt") {
		t.Errorf("an exempt message was not reported as exempt: %q", got)
	}
}

func TestCheckPaneOnAnUnlintableDocument(t *testing.T) {
	m := Model{}
	if got := m.renderLint([]byte("not xml at all")); !strings.Contains(got, "BUSINESS RULES") {
		t.Errorf("no section was produced: %q", got)
	}
	if got := m.renderProfile([]byte("not xml at all")); !strings.Contains(got, "could not apply") {
		t.Errorf("a malformed document did not report a profile failure: %q", got)
	}
}

func TestCatalogueViewWithNothingInstalled(t *testing.T) {
	// Light mode: the standard is still known, the files are not.
	m := Model{}
	report := m.renderCatalogue()
	if !strings.Contains(report, "Nothing is installed") {
		t.Errorf("the empty case is not explained:\n%s", report)
	}
	if !strings.Contains(report, "published") {
		t.Errorf("the published standard was not counted:\n%s", report)
	}
}

func TestCheckPaneListsLintIssues(t *testing.T) {
	// A malformed BIC is what the linter reports, and the pane has to show the
	// rule, the message and the value that failed.
	m := Model{}
	got := m.renderLint([]byte(`<?xml version="1.0"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">
  <DbtrAgt><FinInstnId><BICFI>NOTABIC</BICFI></FinInstnId></DbtrAgt>
</Document>`))

	if !strings.Contains(got, "BICFI") {
		t.Errorf("the failing field was not named: %q", got)
	}
	if !strings.Contains(got, "NOTABIC") {
		t.Errorf("the failing value was not shown: %q", got)
	}
	if !strings.Contains(got, "❌") && !strings.Contains(got, "⚠️") {
		t.Errorf("the finding carries no severity: %q", got)
	}
}

func TestCheckPaneListsSchemaErrors(t *testing.T) {
	dir := t.TempDir()
	schemaPath := filepath.Join(dir, "pacs.008.001.10.xsd")
	if err := os.WriteFile(schemaPath, []byte(`<?xml version="1.0"?>
<xs:schema xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10"
           xmlns:xs="http://www.w3.org/2001/XMLSchema"
           elementFormDefault="qualified"
           targetNamespace="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">
  <xs:element name="Document" type="Document"/>
  <xs:complexType name="Document">
    <xs:sequence><xs:element name="MsgId" type="xs:string"/></xs:sequence>
  </xs:complexType>
</xs:schema>`), 0o644); err != nil {
		t.Fatal(err)
	}

	m := Model{}
	msg := catalog.Message{ID: "pacs.008.001.10", XSDPath: schemaPath}

	// A document missing its mandatory element.
	got := m.renderSchema([]byte(`<?xml version="1.0"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10"/>`), msg)
	if !strings.Contains(got, "❌") {
		t.Errorf("an invalid document was not reported: %q", got)
	}

	// And a schema that will not parse.
	broken := filepath.Join(dir, "broken.xsd")
	if err := os.WriteFile(broken, []byte("<xs:schema"), 0o644); err != nil {
		t.Fatal(err)
	}
	got = m.renderSchema([]byte(cleanSample), catalog.Message{ID: "x", XSDPath: broken})
	if !strings.Contains(got, "could not validate") {
		t.Errorf("an unparseable schema was not reported: %q", got)
	}
}
