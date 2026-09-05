// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package cmd

import (
	"bytes"
	"testing"
)

func TestRootCommands(t *testing.T) {
	buf := new(bytes.Buffer)
	RootCmd.SetOut(buf)
	RootCmd.SetErr(buf)

	if RootCmd.Use != "askiso" {
		t.Errorf("expected command name 'askiso', got '%s'", RootCmd.Use)
	}

	subCmds := []string{"search", "ask", "info", "list", "validate", "version", "generate", "translate", "diff", "sample", "schema", "lint", "completion", "flow", "doctor", "format", "code", "convert", "graph", "mock", "stats", "cbpr-pack"}
	for _, expected := range subCmds {
		found := false
		for _, c := range RootCmd.Commands() {
			if c.Name() == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected subcommand '%s' to be registered", expected)
		}
	}
}
