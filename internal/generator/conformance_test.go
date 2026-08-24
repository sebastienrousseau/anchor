// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package generator_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebastienrousseau/anchor/internal/catalog"
	"github.com/sebastienrousseau/anchor/internal/converter"
	"github.com/sebastienrousseau/anchor/internal/generator"
	"github.com/sebastienrousseau/anchor/internal/linter"
)

// Anchor generates synthetic messages and claims they are compliant. This
// checks that against the real schemas, which means it needs a catalogue and
// xmllint. Both are absent on a clean CI runner, so it skips there and runs for
// anyone who has installed a catalogue.
func requireCatalogue(t *testing.T) *catalog.Index {
	t.Helper()
	if _, err := exec.LookPath("xmllint"); err != nil {
		t.Skip("xmllint not installed; skipping schema conformance")
	}
	idx, err := catalog.LoadResolved("")
	if err != nil {
		t.Skip("no ISO 20022 catalogue installed; skipping schema conformance")
	}
	return idx
}

// schemaFor returns the XSD path for the highest catalogue version of msgID.
func schemaFor(t *testing.T, idx *catalog.Index, msgID string) string {
	t.Helper()
	m, ok := idx.MessageMap[msgID]
	if !ok {
		t.Skipf("%s is not in the installed catalogue", msgID)
	}
	return m.XSDPath
}

func validateAgainst(t *testing.T, xsdPath, xmlPath string) (bool, string) {
	t.Helper()
	out, err := exec.Command("xmllint", "--noout", "--nonet", "--schema", xsdPath, xmlPath).CombinedOutput()
	return err == nil, string(out)
}

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// generatorTargets maps each supported type to the concrete schema version its
// generated payload declares in xmlns. Every one must validate.
var generatorTargets = []struct {
	msgType  string
	schemaID string
}{
	{msgType: "pacs.008", schemaID: "pacs.008.001.10"},
	{msgType: "pacs.009", schemaID: "pacs.009.001.10"},
	{msgType: "pain.001", schemaID: "pain.001.001.11"},
	{msgType: "camt.053", schemaID: "camt.053.001.11"},
}

func TestGeneratedMessagesMatchTheirSchema(t *testing.T) {
	idx := requireCatalogue(t)

	for _, tc := range generatorTargets {
		t.Run(tc.msgType, func(t *testing.T) {
			xml, err := generator.Generate(generator.DefaultOptions(tc.msgType))
			if err != nil {
				t.Fatalf("Generate(%s): %v", tc.msgType, err)
			}

			xsd := schemaFor(t, idx, tc.schemaID)
			if ok, out := validateAgainst(t, xsd, writeTemp(t, tc.msgType+".xml", xml)); !ok {
				t.Errorf("generated %s does not validate against %s:\n%s", tc.msgType, tc.schemaID, out)
			}
		})
	}
}

// Anchor must not emit payloads its own linter rejects. The FedNow preset does
// exactly that today: the US has no IBAN scheme, and the placeholder values fail
// the mod-97 check (F-04b).
func TestGeneratedMessagesPassOwnLinter(t *testing.T) {
	for _, preset := range []string{"sepa", "target2", "chaps", "fednow", "standard"} {
		t.Run(preset, func(t *testing.T) {
			opts := generator.DefaultOptions("pacs.008")
			opts.Preset = preset

			xml, err := generator.Generate(opts)
			if err != nil {
				t.Fatalf("Generate(preset=%s): %v", preset, err)
			}

			res, err := linter.Lint([]byte(xml), preset+".xml")
			if err != nil {
				t.Fatalf("Lint: %v", err)
			}
			if res.Errors > 0 {
				var msgs []string
				for _, iss := range res.Issues {
					msgs = append(msgs, iss.Rule+": "+iss.Message)
				}
				t.Errorf("preset %s generated %d lint error(s):\n  %s",
					preset, res.Errors, strings.Join(msgs, "\n  "))
			}
		})
	}
}

// The Business Application Header must carry the full message definition
// identifier, and both halves of the envelope must validate against their own
// schema.
func TestBAHEnvelope(t *testing.T) {
	idx := requireCatalogue(t)

	opts := generator.DefaultOptions("pacs.008")
	opts.WithBAH = true
	doc, err := generator.Generate(opts)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if !strings.Contains(doc, "<MsgDefIdr>pacs.008.001.10</MsgDefIdr>") {
		t.Errorf("MsgDefIdr must be the full message definition identifier:\n%s", doc)
	}

	header := extractElement(t, doc, "AppHdr")
	body := extractElement(t, doc, "Document")

	if ok, out := validateAgainst(t, schemaFor(t, idx, "head.001.001.02"),
		writeTemp(t, "apphdr.xml", header)); !ok {
		t.Errorf("the header does not validate against head.001.001.02:\n%s", out)
	}
	if ok, out := validateAgainst(t, schemaFor(t, idx, "pacs.008.001.10"),
		writeTemp(t, "body.xml", body)); !ok {
		t.Errorf("the document does not validate against its schema:\n%s", out)
	}
}

// extractElement pulls one top-level element out of the envelope, so each half
// can be checked against its own schema.
func extractElement(t *testing.T, doc, name string) string {
	t.Helper()
	open := "<" + name
	start := strings.Index(doc, open)
	end := strings.Index(doc, "</"+name+">")
	if start < 0 || end < 0 {
		t.Fatalf("<%s> not found in the envelope:\n%s", name, doc)
	}
	return `<?xml version="1.0" encoding="UTF-8"?>` + "\n" + doc[start:end+len(name)+3]
}

// A round trip through JSON must preserve document order: ISO 20022 complex
// types are xs:sequence, so reordering produces schema-invalid output.
func TestJSONRoundTripPreservesSchemaValidity(t *testing.T) {
	idx := requireCatalogue(t)

	xml, err := generator.Generate(generator.DefaultOptions("pacs.008"))
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	xsd := schemaFor(t, idx, "pacs.008.001.10")

	if ok, out := validateAgainst(t, xsd, writeTemp(t, "orig.xml", xml)); !ok {
		t.Fatalf("precondition failed - generated pacs.008 is already invalid:\n%s", out)
	}

	// Repeat: Go map ordering used to make this fail intermittently, so a single
	// pass would not have proven the fix.
	for i := 0; i < 8; i++ {
		jsonBytes, err := converter.XMLToJSON([]byte(xml))
		if err != nil {
			t.Fatalf("XMLToJSON: %v", err)
		}
		back, err := converter.JSONToXML(jsonBytes)
		if err != nil {
			t.Fatalf("JSONToXML: %v", err)
		}
		if ok, out := validateAgainst(t, xsd, writeTemp(t, "roundtrip.xml", string(back))); !ok {
			t.Fatalf("attempt %d: the round trip produced an invalid document:\n%s", i+1, out)
		}
	}
}
