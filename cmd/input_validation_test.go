// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package cmd

import (
	"strings"
	"testing"
)

func TestCommandsRejectUnknownEnumeratedFlags(t *testing.T) {
	tests := [][]string{
		{"batch", "missing", "--format", "yaml"},
		{"lint", "missing.xml", "--format", "yaml"},
		{"validate", "missing.xml", "--engine", "magic"},
		{"translate", "--format", "yaml"},
		{"graph", "--format", "dot"},
		{"graph", "--preset", "unknown"},
		{"flow", "--preset", "unknown"},
		{"generate", "pacs.008", "--preset", "unknown"},
		{"batch", "missing", "--workers", "-1"},
	}

	for _, args := range tests {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			if _, err := run(t, args...); err == nil {
				t.Fatalf("%v should reject an unsupported flag value", args)
			}
		})
	}
}

func TestCommandsRejectSurplusArguments(t *testing.T) {
	for _, args := range [][]string{
		{"graph", "pacs.008", "extra"},
		{"flow", "pacs.008", "extra"},
		{"translate", "MT103", "extra"},
		{"list", "extra"},
		{"stats", "extra"},
		{"doctor", "extra"},
		{"version", "extra"},
		{"mock", "extra"},
		{"catalog", "where", "extra"},
		{"catalog", "status", "extra"},
	} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			if _, err := run(t, args...); err == nil {
				t.Fatalf("%v should reject surplus arguments", args)
			}
		})
	}
}

func TestGenerateExplicitCurrencyOverridesPresetDefault(t *testing.T) {
	out, err := run(t, "generate", "pacs.008", "--preset", "fednow", "--currency", "CAD")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `Ccy="CAD"`) {
		t.Errorf("generated payment did not preserve --currency CAD:\n%s", out)
	}
	if !strings.Contains(out, "<Cd>USABA</Cd>") {
		t.Error("explicit currency override should not discard FedNow rail defaults")
	}
}
