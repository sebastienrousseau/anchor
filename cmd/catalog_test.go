// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package cmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebastienrousseau/askiso/internal/catalog"
	"github.com/spf13/cobra"
)

func writeFixtureCatalogue(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "Payments Clearing and Settlement", "Version 11.0", "Schemas")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"pacs.008.001.10", "pacs.009.001.10"} {
		if err := os.WriteFile(filepath.Join(dir, id+".xsd"), []byte("<xs:schema/>"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// isolate removes every ambient way a catalogue could be discovered, so a test
// asserting the not-found path cannot accidentally find the developer's own
// installed copy.
func isolate(t *testing.T) {
	t.Helper()
	t.Setenv(catalog.EnvCatalog, "")
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LocalAppData", t.TempDir())
	t.Chdir(t.TempDir())

	prev := catalogPath
	catalogPath = ""
	t.Cleanup(func() { catalogPath = prev })
}

// Commands that read the catalogue must fail loudly when none is installed.
// Returning an empty result with exit status 0 is the regression this guards.
func TestLoadCatalogFailsWithoutCatalogue(t *testing.T) {
	isolate(t)

	idx, err := loadCatalog()
	if err == nil {
		t.Fatalf("expected an error, got an index with %d messages", len(idx.Messages))
	}
	msg := err.Error()
	for _, want := range []string{"no ISO 20022 catalogue found", "iso20022.org", "askiso catalog add"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error should mention %q:\n%s", want, msg)
		}
	}
}

func TestLoadCatalogUsesEnv(t *testing.T) {
	isolate(t)
	want := writeFixtureCatalogue(t)
	t.Setenv(catalog.EnvCatalog, want)

	idx, err := loadCatalog()
	if err != nil {
		t.Fatalf("loadCatalog: %v", err)
	}
	if idx.RootDir != want {
		t.Errorf("RootDir = %s, want %s", idx.RootDir, want)
	}
	if len(idx.Messages) != 2 {
		t.Errorf("got %d messages, want 2", len(idx.Messages))
	}
}

// The --catalog flag must beat $ASKISO_CATALOG.
func TestLoadCatalogFlagBeatsEnv(t *testing.T) {
	isolate(t)
	t.Setenv(catalog.EnvCatalog, writeFixtureCatalogue(t))

	want := writeFixtureCatalogue(t)
	catalogPath = want

	idx, err := loadCatalog()
	if err != nil {
		t.Fatalf("loadCatalog: %v", err)
	}
	if idx.RootDir != want {
		t.Errorf("--catalog should win; RootDir = %s, want %s", idx.RootDir, want)
	}
}

// Commands that need the actual XSD files must fail loudly when none are
// installed. Printing an empty result with exit status 0 is the regression this
// guards.
func TestSchemaCommandsErrorWithoutCatalogue(t *testing.T) {
	cases := map[string][]string{
		"schema": {"pacs.008.001.10"},
		"diff":   {"pacs.008.001.09", "pacs.008.001.10"},
		"doctor": nil,
		// sample and stats are deliberately absent: they degrade instead of
		// failing. See TestDiscoveryCommandsFallBackToRegistry.
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			isolate(t)
			cmd := findCommand(t, name)
			if err := cmd.RunE(cmd, args); err == nil {
				t.Errorf("%s returned nil error with no catalogue installed", name)
			}
		})
	}
}

// Light mode: with no catalogue AskISO still knows what exists, what it is
// called, and where the RA publishes it. These commands must answer usefully
// rather than fail, so a fresh install is a starting point, not a dead end.
func TestDiscoveryCommandsFallBackToRegistry(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want []string
	}{
		// search and info name the message and the official download.
		{"search", []string{"pacs.008"}, []string{"pacs.008", "iso20022.org"}},
		{"info", []string{"pacs.008.001.10"}, []string{"pacs.008", "iso20022.org"}},
		// The RA publishes sample instances for very few messages, so a
		// generated one is the best answer available and needs no catalogue.
		{"sample", []string{"pacs.008"}, []string{"<Document", "pacs.008"}},
		// stats reports the whole published standard from the embedded index.
		{"stats", nil, []string{"embedded registry"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			isolate(t)
			cmd := findCommand(t, tc.name)

			out := captureStdout(t, func() {
				if err := cmd.RunE(cmd, tc.args); err != nil {
					t.Errorf("%s should degrade to light mode, got: %v", tc.name, err)
				}
			})
			for _, want := range tc.want {
				if !strings.Contains(out, want) {
					t.Errorf("%s output should mention %q:\n%s", tc.name, want, out)
				}
			}
		})
	}
}

// An identifier that is in no ISO 20022 message set must still be an error.
func TestInfoRejectsUnknownMessage(t *testing.T) {
	isolate(t)
	cmd := findCommand(t, "info")
	if err := cmd.RunE(cmd, []string{"zzzz.999.999.99"}); err == nil {
		t.Error("an unknown message identifier should be an error")
	}
}

func findCommand(t *testing.T, name string) *cobra.Command {
	t.Helper()
	for _, c := range RootCmd.Commands() {
		if c.Name() == name {
			if c.RunE == nil {
				t.Fatalf("%s has no RunE", name)
			}
			return c
		}
	}
	t.Fatalf("command %q is not registered", name)
	return nil
}

// captureStdout collects what a command prints. The commands write with fmt
// directly rather than through cobra's output writer.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	fn()

	_ = w.Close()
	os.Stdout = orig
	return <-done
}

func TestRootCmdSilencesUsageOnRuntimeError(t *testing.T) {
	if !RootCmd.SilenceUsage {
		t.Error("SilenceUsage should be set so a runtime failure does not dump usage")
	}
	if !RootCmd.SilenceErrors {
		t.Error("SilenceErrors should be set so Execute prints the error exactly once")
	}
}

func TestCatalogFlagIsRegistered(t *testing.T) {
	f := RootCmd.PersistentFlags().Lookup("catalog")
	if f == nil {
		t.Fatal("--catalog should be a persistent flag")
	}
	if !strings.Contains(f.Usage, catalog.EnvCatalog) {
		t.Errorf("--catalog usage should mention %s, got %q", catalog.EnvCatalog, f.Usage)
	}
}
