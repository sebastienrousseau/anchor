// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package graph

import (
	"strings"
	"testing"
)

func TestGenerateMermaid(t *testing.T) {
	diagram := GenerateMermaid("pacs.008", "sepa")
	if !strings.Contains(diagram, "sequenceDiagram") {
		t.Errorf("Expected sequenceDiagram in Mermaid output")
	}
	if !strings.Contains(diagram, "pain.001") {
		t.Errorf("Expected pain.001 in Mermaid output")
	}
	if !strings.Contains(diagram, "pacs.008") {
		t.Errorf("Expected pacs.008 in Mermaid output")
	}
}

func TestGenerateASCII(t *testing.T) {
	diagram := GenerateASCII("pacs.008", "fednow")
	if !strings.Contains(diagram, "FEDNOW") {
		t.Errorf("Expected FEDNOW in ASCII diagram")
	}
	if !strings.Contains(diagram, "pacs.008") {
		t.Errorf("Expected pacs.008 in ASCII diagram")
	}
}
