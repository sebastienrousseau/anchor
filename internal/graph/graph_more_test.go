// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package graph

import (
	"strings"
	"testing"
)

func TestDiagramsCoverEveryPreset(t *testing.T) {
	presets := map[string]string{
		"sepa":    "SEPA",
		"fednow":  "FEDNOW",
		"target2": "TARGET2",
		"chaps":   "CHAPS",
		"":        "",
		"unknown": "UNKNOWN",
	}
	for preset, marker := range presets {
		t.Run("mermaid/"+preset, func(t *testing.T) {
			out := GenerateMermaid("pacs.008", preset)
			if !strings.Contains(out, "sequenceDiagram") {
				t.Errorf("not a mermaid diagram:\n%s", out)
			}
			if marker != "" && !strings.Contains(strings.ToUpper(out), marker) {
				t.Errorf("preset %q should appear in the diagram", preset)
			}
		})
		t.Run("ascii/"+preset, func(t *testing.T) {
			out := GenerateASCII("pacs.008", preset)
			if strings.TrimSpace(out) == "" {
				t.Error("ascii diagram was empty")
			}
			if strings.Contains(out, "sequenceDiagram") {
				t.Error("ascii output should not be mermaid")
			}
		})
	}
}

func TestDiagramsCoverMessageTypes(t *testing.T) {
	for _, msg := range []string{"pacs.008", "pain.001", "camt.053", "unknown.999"} {
		if strings.TrimSpace(GenerateMermaid(msg, "sepa")) == "" {
			t.Errorf("mermaid for %s was empty", msg)
		}
		if strings.TrimSpace(GenerateASCII(msg, "sepa")) == "" {
			t.Errorf("ascii for %s was empty", msg)
		}
	}
}
