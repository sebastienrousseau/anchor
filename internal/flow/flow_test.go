// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package flow

import (
	"strings"
	"testing"

	"github.com/sebastienrousseau/askiso/internal/generator"
)

func TestGenerateLifecycle(t *testing.T) {
	opts := generator.DefaultOptions("pacs.008")
	opts.Preset = "sepa"
	opts.Amount = "25000.00"

	chain, err := GenerateLifecycle(opts)
	if err != nil {
		t.Fatalf("GenerateLifecycle failed: %v", err)
	}

	if len(chain.Steps) != 4 {
		t.Fatalf("Expected 4 steps in lifecycle, got %d", len(chain.Steps))
	}

	if chain.Steps[0].MsgType != "pain.001.001.11" {
		t.Errorf("Expected step 1 pain.001, got %s", chain.Steps[0].MsgType)
	}
	if chain.Steps[1].MsgType != "pacs.008.001.10" {
		t.Errorf("Expected step 2 pacs.008, got %s", chain.Steps[1].MsgType)
	}
	if chain.Steps[2].MsgType != "pacs.002.001.12" {
		t.Errorf("Expected step 3 pacs.002, got %s", chain.Steps[2].MsgType)
	}
	if chain.Steps[3].MsgType != "camt.053.001.11" {
		t.Errorf("Expected step 4 camt.053, got %s", chain.Steps[3].MsgType)
	}

	// Verify shared UETR across all steps
	for i, step := range chain.Steps {
		if !strings.Contains(step.XMLPayload, chain.UETR) {
			t.Errorf("Step %d missing shared UETR %s", i+1, chain.UETR)
		}
	}
}
