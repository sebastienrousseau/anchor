// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Four message types come from templates. The other 2,841 come from their
// schema, and the point of this command is that the second group exists at all.

func TestGenerateFromSchemaForAMessageWithNoTemplate(t *testing.T) {
	withCatalogue(t)

	out, err := run(t, "generate", "pacs.009.001.10")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	// The fixture schema declares MsgId and Ccy; both are mandatory, so both
	// have to be there.
	wantContains(t, out,
		"urn:iso:std:iso:20022:tech:xsd:pacs.009.001.10",
		"<MsgId>", "<Ccy>")
}

func TestGenerateFromSchemaValidatesAgainstItsOwnSchema(t *testing.T) {
	root := withCatalogue(t)
	dest := filepath.Join(t.TempDir(), "generated.xml")

	if _, err := run(t, "generate", "pacs.009.001.10", "-o", dest); err != nil {
		t.Fatalf("generate: %v", err)
	}

	// The claim the whole feature rests on: what comes out validates.
	if _, err := run(t, "validate", dest); err != nil {
		data, _ := os.ReadFile(dest)
		t.Fatalf("the generated message does not validate: %v\n%s", err, data)
	}
	_ = root
}

func TestGenerateFromSchemaReportsItsDecisions(t *testing.T) {
	withCatalogue(t)
	dest := filepath.Join(t.TempDir(), "generated.xml")

	out, err := run(t, "generate", "pacs.009.001.10", "-o", dest)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	wantContains(t, out, "GENERATED", "elements")
}

func TestGenerateFromSchemaFlagForcesATemplateType(t *testing.T) {
	withCatalogue(t)

	// pacs.008 has a template; --from-schema asks for the schema walk instead,
	// which is how the two can be compared.
	fromTemplate, err := run(t, "generate", "pacs.008")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	fromSchema, err := run(t, "generate", "pacs.008.001.10", "--from-schema")
	if err != nil {
		t.Fatalf("generate --from-schema: %v", err)
	}
	if fromTemplate == fromSchema {
		t.Error("the template and the schema walk produced identical output")
	}
	// The template carries parties and an amount; the schema walk carries only
	// what the schema declares.
	if !strings.Contains(fromTemplate, "FIToFICstmrCdtTrf") {
		t.Errorf("the template output is not a payment:\n%s", fromTemplate)
	}
}

func TestGenerateOptionalIncludesMore(t *testing.T) {
	withCatalogue(t)

	minimal, err := run(t, "generate", "pacs.009.001.10")
	if err != nil {
		t.Fatal(err)
	}
	full, err := run(t, "generate", "pacs.009.001.10", "--optional")
	if err != nil {
		t.Fatal(err)
	}
	if len(full) < len(minimal) {
		t.Errorf("--optional produced less than the minimal message:\n%s\n---\n%s", minimal, full)
	}
}

func TestGenerateFromSchemaWithoutACatalogue(t *testing.T) {
	isolate(t)

	_, err := run(t, "generate", "seev.031.001.09")
	if err == nil {
		t.Fatal("a message with no template and no catalogue was generated")
	}
	if !strings.Contains(err.Error(), "iso20022.org") {
		t.Errorf("the error does not say where to get the schema: %v", err)
	}

	// The four template types still work with nothing installed, which is the
	// whole point of having them.
	if _, err := run(t, "generate", "pacs.008"); err != nil {
		t.Errorf("a template type failed in light mode: %v", err)
	}
}

func TestGenerateFromSchemaForAnUnknownMessage(t *testing.T) {
	withCatalogue(t)

	if _, err := run(t, "generate", "zzzz.999.999.99"); err == nil {
		t.Error("an unknown message identifier was generated")
	}
}

func TestGenerateFromSchemaHonoursValueOverrides(t *testing.T) {
	withCatalogue(t)

	out, err := run(t, "generate", "pacs.009.001.10", "--currency", "GBP")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(out, "<Ccy>GBP</Ccy>") {
		t.Errorf("the currency override was ignored:\n%s", out)
	}
}

func TestGenerateFromSchemaPrintsItsDecisions(t *testing.T) {
	// A choice the generator had to make on the reader's behalf is reported.
	// With the message on stdout the notes go to stderr, so piping stays clean;
	// with --output they go to stdout alongside the summary.
	dir := t.TempDir()
	root := filepath.Join(dir, "catalogue")
	schemas := filepath.Join(root, "Payments Clearing and Settlement", "Version 11.0", "Schemas")
	if err := os.MkdirAll(schemas, 0o755); err != nil {
		t.Fatal(err)
	}

	// A schema with a choice, so the walk has a decision to report.
	schema := `<?xml version="1.0" encoding="UTF-8"?>
<xs:schema xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10"
           xmlns:xs="http://www.w3.org/2001/XMLSchema"
           elementFormDefault="qualified"
           targetNamespace="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">
  <xs:element name="Document" type="Document"/>
  <xs:complexType name="Document">
    <xs:sequence><xs:element name="Acct" type="AccountChoice"/></xs:sequence>
  </xs:complexType>
  <xs:complexType name="AccountChoice">
    <xs:choice>
      <xs:element name="IBAN" type="Max35Text"/>
      <xs:element name="Othr" type="Max35Text"/>
    </xs:choice>
  </xs:complexType>
  <xs:simpleType name="Max35Text">
    <xs:restriction base="xs:string"><xs:minLength value="1"/><xs:maxLength value="35"/></xs:restriction>
  </xs:simpleType>
</xs:schema>`
	if err := os.WriteFile(filepath.Join(schemas, "pacs.008.001.10.xsd"), []byte(schema), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ASKISO_CATALOG", root)

	dest := filepath.Join(dir, "generated.xml")
	out, err := run(t, "generate", "pacs.008.001.10", "--from-schema", "-o", dest)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	wantContains(t, out, "decision(s)", "choice")

	written, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(written), "<IBAN>") {
		t.Errorf("the first branch was not taken:\n%s", written)
	}
}
