// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package iso20022

import (
	"strings"
	"testing"
)

func TestSDKAPI(t *testing.T) {
	// Generate
	opts := GeneratorOptions{MsgType: "pacs.008", Preset: "sepa"}
	xml, err := Generate(opts)
	if err != nil {
		t.Fatalf("SDK Generate failed: %v", err)
	}

	// Lint
	res, err := Lint([]byte(xml), "test.xml")
	if err != nil {
		t.Fatalf("SDK Lint failed: %v", err)
	}
	if res.Errors > 0 {
		t.Errorf("Expected 0 errors from SDK-generated payload, got %d", res.Errors)
	}

	// Code Lookup
	codes := LookupCode("AC04")
	if len(codes) == 0 {
		t.Errorf("Expected to find code AC04")
	}

	// SWIFT translation
	mapping, ok := TranslateSWIFT("MT103")
	if !ok || !strings.HasPrefix(mapping.MXCode, "pacs.008") {
		t.Errorf("Expected MT103 to map to pacs.008, got %s", mapping.MXCode)
	}
}

// ---------------------------------------------------------------------------
// SWIFT MT translation
// ---------------------------------------------------------------------------

const mtSample = `{1:F01BANKGB2LAXXX0000000000}{2:I103BANKDEFFXXXXN}{3:{121:f3a1b2c4-5d6e-4f70-8a91-2b3c4d5e6f70}}{4:
:20:REF20260824001
:32A:260824EUR25000,00
:50K:/GB29NWBK60161331926819
ACME TRADING LIMITED
LONDON EC2V 7NN
:59:/DE89370400440532013000
MUELLER GMBH
:71A:SHA
-}`

func TestParseMT(t *testing.T) {
	m, err := ParseMT([]byte(mtSample))
	if err != nil {
		t.Fatalf("ParseMT: %v", err)
	}
	if m.Type != "103" {
		t.Errorf("Type = %q, want 103", m.Type)
	}
	if len(m.Fields) != 5 {
		t.Errorf("got %d fields, want 5", len(m.Fields))
	}

	if _, err := ParseMT([]byte("not an MT message")); err == nil {
		t.Error("ParseMT accepted a non-MT input")
	}
}

func TestTranslateMT(t *testing.T) {
	conv, err := TranslateMT([]byte(mtSample))
	if err != nil {
		t.Fatalf("TranslateMT: %v", err)
	}
	if conv.TargetType != "pacs.008.001.10" {
		t.Errorf("TargetType = %q", conv.TargetType)
	}
	if !strings.Contains(conv.XML, "<IBAN>GB29NWBK60161331926819</IBAN>") {
		t.Errorf("the debtor account was not carried:\n%s", conv.XML)
	}

	// The result has to be lintable by the same engine, because that is the
	// workflow the conversion exists to serve.
	res, err := Lint([]byte(conv.XML), "converted.xml")
	if err != nil {
		t.Fatalf("linting the conversion: %v", err)
	}
	if res.Errors != 0 {
		t.Errorf("the converted message does not lint clean: %+v", res.Issues)
	}

	// And an MT address is exactly what CBPR+ stops accepting in November 2026.
	prof, err := CheckProfile([]byte(conv.XML), "cbpr-2026", "converted.xml")
	if err != nil {
		t.Fatalf("checking the profile: %v", err)
	}
	if prof.Errors == 0 {
		t.Error("an MT-derived unstructured address passed the cbpr-2026 profile")
	}
}

func TestTranslateMTRejects(t *testing.T) {
	if _, err := TranslateMT([]byte("garbage")); err == nil {
		t.Error("TranslateMT accepted a non-MT input")
	}
	unsupported := "{1:F01BANKGB2LAXXX0000000000}{2:I700BANKDEFFXXXXN}{4:\n:20:REF1\n-}"
	if _, err := TranslateMT([]byte(unsupported)); err == nil {
		t.Error("TranslateMT accepted an unsupported message type")
	}
}

func TestTranslatableMT(t *testing.T) {
	got := TranslatableMT()
	if len(got) != 10 {
		t.Fatalf("TranslatableMT() = %v", got)
	}
	for _, want := range []string{"101", "103", "104", "107", "202", "204", "940"} {
		found := false
		for _, g := range got {
			if g == want {
				found = true
			}
		}
		if !found {
			t.Errorf("MT%s is missing from %v", want, got)
		}
	}
}

// ---------------------------------------------------------------------------
// Schema diff
// ---------------------------------------------------------------------------

func TestCatalogueDiff(t *testing.T) {
	cat, err := OpenCatalogue("")
	if err != nil {
		t.Skip("no ISO 20022 catalogue installed")
	}

	rep, err := cat.Diff("pacs.008.001.09", "pacs.008.001.10")
	if err != nil {
		t.Skipf("both schemas must be installed: %v", err)
	}
	if rep.From != "pacs.008.001.09" || rep.To != "pacs.008.001.10" {
		t.Errorf("got %s -> %s", rep.From, rep.To)
	}
	if rep.Common == 0 {
		t.Error("the two versions share no paths, which cannot be right")
	}

	// CashAccount38 became CashAccount40, which made Id optional. That relaxes
	// a rule, so nothing in this step breaks.
	if breaking, _ := rep.Counts(); breaking != 0 {
		t.Errorf("pacs.008.001.09 -> .10 reported %d breaking change(s): %+v",
			breaking, rep.Breaking())
	}
	var sawIDRelaxed bool
	for _, c := range rep.Changes {
		if strings.HasSuffix(c.Path, "/DbtrAcct/Id") && c.Kind == "cardinality" {
			sawIDRelaxed = true
		}
	}
	if !sawIDRelaxed {
		t.Error("the account identifier becoming optional was not reported")
	}
}

func TestCatalogueDiffMissingSchema(t *testing.T) {
	cat, err := OpenCatalogue("")
	if err != nil {
		t.Skip("no ISO 20022 catalogue installed")
	}

	if _, err := cat.Diff("zzzz.999.999.99", "pacs.008.001.10"); err == nil {
		t.Error("an unknown first identifier should be an error")
	}
	if _, err := cat.Diff("pacs.008.001.10", "zzzz.999.999.99"); err == nil {
		t.Error("an unknown second identifier should be an error")
	}
}

func TestTranslateMX(t *testing.T) {
	doc := `<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">
  <FIToFICstmrCdtTrf>
    <GrpHdr><MsgId>MSG-0001</MsgId><CreDtTm>2026-08-24T09:00:00Z</CreDtTm><NbOfTxs>1</NbOfTxs></GrpHdr>
    <CdtTrfTxInf>
      <PmtId><EndToEndId>E2E-1</EndToEndId></PmtId>
      <IntrBkSttlmAmt Ccy="EUR">25000.00</IntrBkSttlmAmt>
      <IntrBkSttlmDt>2026-08-24</IntrBkSttlmDt>
      <ChrgBr>SHAR</ChrgBr>
      <Dbtr><Nm>ACME TRADING LIMITED</Nm>
        <PstlAdr><TwnNm>LONDON</TwnNm><Ctry>GB</Ctry></PstlAdr></Dbtr>
      <DbtrAgt><FinInstnId><BICFI>BANKGB2LXXX</BICFI></FinInstnId></DbtrAgt>
      <CdtrAgt><FinInstnId><BICFI>BANKDEFFXXX</BICFI></FinInstnId></CdtrAgt>
      <Cdtr><Nm>MUELLER GMBH</Nm></Cdtr>
      <Purp><Cd>SUPP</Cd></Purp>
    </CdtTrfTxInf>
  </FIToFICstmrCdtTrf>
</Document>`

	conv, err := TranslateMX([]byte(doc))
	if err != nil {
		t.Fatalf("TranslateMX: %v", err)
	}
	if conv.SourceType != "pacs.008.001.10" || conv.TargetType != "MT103" {
		t.Errorf("got %s -> %s", conv.SourceType, conv.TargetType)
	}
	if !strings.Contains(conv.XML, ":32A:260824EUR25000,00") {
		t.Errorf("the amount was not converted:\n%s", conv.XML)
	}

	// MT has no purpose code and no structured address, and the report has to
	// say both rather than leaving a message that looks complete.
	if conv.Lossless() {
		t.Error("a conversion that lost a purpose code reported itself as lossless")
	}
	if len(conv.Unmapped()) == 0 {
		t.Errorf("nothing was reported as lost: %+v", conv.Report)
	}

	if _, err := TranslateMX([]byte("<unclosed>")); err == nil {
		t.Error("TranslateMX accepted malformed XML")
	}
	if got := TranslatableMX(); len(got) != 6 {
		t.Errorf("TranslatableMX() = %v", got)
	}
}

func TestTemplateTypes(t *testing.T) {
	got := TemplateTypes()
	if len(got) != 4 {
		t.Fatalf("TemplateTypes() = %v", got)
	}
	for _, msgType := range got {
		if !HasTemplate(msgType) {
			t.Errorf("%s is listed but has no template", msgType)
		}
		// A template type generates without any catalogue at all.
		if _, err := Generate(DefaultGeneratorOptions(msgType)); err != nil {
			t.Errorf("generating %s: %v", msgType, err)
		}
	}
	if HasTemplate("seev.031.001.09") {
		t.Error("a message with no template was reported as having one")
	}
}

func TestGenerateFromSchemaThroughTheSDK(t *testing.T) {
	cat, err := OpenCatalogue("")
	if err != nil {
		t.Skip("no ISO 20022 catalogue installed")
	}

	res, err := cat.GenerateFromSchema("pacs.008.001.10", DefaultSchemaGenOptions())
	if err != nil {
		t.Skipf("pacs.008.001.10 is not installed: %v", err)
	}
	if res.Root != "Document" || res.Elements == 0 {
		t.Errorf("got %+v", res)
	}

	// The claim the feature rests on: what comes out validates and lints clean.
	verdict, err := cat.Validate([]byte(res.XML))
	if err != nil {
		t.Fatalf("validating: %v", err)
	}
	if !verdict.Valid {
		t.Errorf("the generated message does not validate: %+v", verdict.Errors)
	}
	lint, err := Lint([]byte(res.XML), "")
	if err != nil {
		t.Fatalf("linting: %v", err)
	}
	if lint.Errors != 0 {
		t.Errorf("the generated message does not lint clean: %+v", lint.Issues)
	}

	if _, err := cat.GenerateFromSchema("zzzz.999.999.99", DefaultSchemaGenOptions()); err == nil {
		t.Error("an unknown message was generated")
	}
}

func TestExternalCodesThroughTheSDK(t *testing.T) {
	// The publication is the user's own download, so a catalogue with none
	// reports none rather than failing.
	cat, err := OpenCatalogue("")
	if err != nil {
		t.Skip("no ISO 20022 catalogue installed")
	}

	// Whatever this developer has imported, the accessors have to be safe.
	_ = cat.ExternalCodes()
	_ = cat.LookupExternalCode("SALA")

	// A nil catalogue is light mode, and behaves as empty.
	var none *Catalogue
	if none.ExternalCodes() != nil || none.LookupExternalCode("SALA") != nil {
		t.Error("a nil catalogue returned external codes")
	}
	if _, err := none.ImportExternalCodes("anything"); err == nil {
		t.Error("importing into no catalogue was accepted")
	}

	if _, err := cat.ImportExternalCodes("/no/such/publication.xlsx"); err == nil {
		t.Error("importing a missing file was accepted")
	}
}
