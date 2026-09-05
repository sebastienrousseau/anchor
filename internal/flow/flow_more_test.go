// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package flow

import (
	"encoding/xml"
	"io"
	"regexp"
	"strings"
	"testing"

	"github.com/sebastienrousseau/askiso/internal/generator"
)

var uuidV4 = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestGenerateUUIDv4Shape(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		u := generateUUIDv4()
		if !uuidV4.MatchString(u) {
			t.Fatalf("not an RFC 4122 v4 UUID: %q", u)
		}
		if seen[u] {
			t.Fatalf("duplicate UUID after %d draws: %s", i, u)
		}
		seen[u] = true
	}
}

func TestGenerateLifecycleEscapesCallerValues(t *testing.T) {
	opt := generator.DefaultOptions("pacs.008")
	opt.Debtor = `A&B <Holdings>`
	opt.Creditor = `"Quoted" & Co`
	opt.EndToEndID = `E2E<&>`
	chain, err := GenerateLifecycle(opt)
	if err != nil {
		t.Fatal(err)
	}
	if chain.Debtor != opt.Debtor || chain.Creditor != opt.Creditor {
		t.Fatal("chain metadata should preserve caller values")
	}
	for _, step := range chain.Steps {
		dec := xml.NewDecoder(strings.NewReader(step.XMLPayload))
		for {
			if _, err := dec.Token(); err != nil {
				if err == io.EOF {
					break
				}
				t.Fatalf("%s generated malformed XML: %v\n%s", step.MsgType, err, step.XMLPayload)
			}
		}
	}
}

func TestFedwireNamesItsClearingSystem(t *testing.T) {
	chain, err := GenerateLifecycle(generator.Options{MsgType: "pacs.008", Preset: "fedwire"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(chain.Steps[1].XMLPayload, "<Prtry>FEDWIRE</Prtry>") {
		t.Errorf("fedwire lifecycle used the wrong clearing system:\n%s", chain.Steps[1].XMLPayload)
	}
}

// Empty options must be filled in, and the four stages must share one UETR and
// one end-to-end identifier -- that linkage is the point of the lifecycle.
func TestGenerateLifecycleFillsDefaultsAndLinks(t *testing.T) {
	chain, err := GenerateLifecycle(generator.Options{MsgType: "pacs.008"})
	if err != nil {
		t.Fatalf("GenerateLifecycle: %v", err)
	}

	if !uuidV4.MatchString(chain.UETR) {
		t.Errorf("UETR was not generated: %q", chain.UETR)
	}
	for _, f := range []struct{ name, val string }{
		{"EndToEndID", chain.EndToEndID},
		{"Amount", chain.Amount},
		{"Currency", chain.Currency},
		{"Debtor", chain.Debtor},
		{"Creditor", chain.Creditor},
		{"DebtorIBAN", chain.DebtorIBAN},
		{"CreditorIBAN", chain.CreditorIBAN},
	} {
		if f.val == "" {
			t.Errorf("%s was left empty", f.name)
		}
	}

	if len(chain.Steps) < 4 {
		t.Fatalf("got %d steps, want at least 4", len(chain.Steps))
	}
	for i, s := range chain.Steps {
		if s.Index != i+1 {
			t.Errorf("step %d has index %d", i, s.Index)
		}
		if s.MsgType == "" || s.Title == "" || s.FileName == "" || s.XMLPayload == "" {
			t.Errorf("step %d is incomplete: %+v", i, s)
		}
	}

	// The interbank leg and the statement must carry the shared references.
	joined := ""
	for _, s := range chain.Steps {
		joined += s.XMLPayload
	}
	if !strings.Contains(joined, chain.UETR) {
		t.Error("the shared UETR does not appear in the generated payloads")
	}
	if !strings.Contains(joined, chain.EndToEndID) {
		t.Error("the shared end-to-end identifier does not appear in the payloads")
	}
}

func TestGenerateLifecycleHonoursPresets(t *testing.T) {
	for _, preset := range []string{"sepa", "fednow", "target2", "chaps", "standard", ""} {
		t.Run(preset, func(t *testing.T) {
			chain, err := GenerateLifecycle(generator.Options{MsgType: "pacs.008", Preset: preset})
			if err != nil {
				t.Fatalf("preset %q: %v", preset, err)
			}
			if len(chain.Steps) < 4 {
				t.Errorf("preset %q produced %d steps", preset, len(chain.Steps))
			}
		})
	}
}

func TestGenerateLifecycleRejectsUnknownPreset(t *testing.T) {
	if _, err := GenerateLifecycle(generator.Options{MsgType: "pacs.008", Preset: "not-a-rail"}); err == nil || !strings.Contains(err.Error(), "unknown preset") {
		t.Fatalf("unknown preset error = %v", err)
	}
}

func TestGenerateLifecycleKeepsSuppliedValues(t *testing.T) {
	opts := generator.Options{
		MsgType:    "pacs.008",
		UETR:       "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee",
		EndToEndID: "E2E-FIXED",
		Amount:     "77.77",
		Currency:   "GBP",
	}
	chain, err := GenerateLifecycle(opts)
	if err != nil {
		t.Fatal(err)
	}
	if chain.UETR != opts.UETR || chain.EndToEndID != opts.EndToEndID {
		t.Errorf("supplied identifiers were overwritten: %+v", chain)
	}
	if chain.Amount != "77.77" || chain.Currency != "GBP" {
		t.Errorf("supplied amount/currency were overwritten: %+v", chain)
	}
}
