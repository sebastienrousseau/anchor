// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package cmd

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sebastienrousseau/anchor/internal/ai"
	"github.com/sebastienrousseau/anchor/internal/catalog"
)

// A catalogue inside iCloud is called out, because macOS can evict those files
// and break validation silently.
func TestDoctorWarnsAboutICloudCatalogue(t *testing.T) {
	root := t.TempDir()
	icloud := filepath.Join(root, "Mobile Documents", "com~apple~CloudDocs", "catalog")
	schemas := filepath.Join(icloud, "Payments Clearing and Settlement", "Version 11.0", "Schemas")
	if err := os.MkdirAll(schemas, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(schemas, "pacs.008.001.10.xsd"),
		[]byte(fixtureSchema("pacs.008.001.10")), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ANCHOR_CATALOG", icloud)
	prev := catalogPath
	catalogPath = ""
	t.Cleanup(func() { catalogPath = prev })

	out, err := run(t, "doctor")
	if err == nil {
		t.Error("an iCloud catalogue should be reported as a problem")
	}
	wantContains(t, out, "iCloud")
}

func TestDoctorReportsReachableOllama(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"models":[]}`))
	}))
	defer srv.Close()

	withCatalogue(t)
	t.Setenv("OLLAMA_HOST", srv.URL)
	t.Setenv("OPENAI_API_KEY", "present")

	out, err := run(t, "doctor")
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, out)
	}
	wantContains(t, out, "Ollama", "OPENAI_API_KEY")
}

func TestDoctorHandlesUnreachableOllama(t *testing.T) {
	withCatalogue(t)
	t.Setenv("OLLAMA_HOST", "http://127.0.0.1:1")
	t.Setenv("OPENAI_API_KEY", "")

	if _, err := run(t, "doctor"); err != nil {
		t.Errorf("an unreachable assistant is informational, not fatal: %v", err)
	}
}

// The mock server binds a real port, so the command is driven directly and shut
// down rather than left listening.
func TestMockCommandServes(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	prev := mockPort
	mockPort = port
	t.Cleanup(func() { mockPort = prev })

	done := make(chan error, 1)
	go func() {
		cmd := findCommand(t, "mock")
		done <- cmd.RunE(cmd, nil)
	}()

	base := "http://127.0.0.1:" + strconv.Itoa(port)
	var resp *http.Response
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, err = http.Get(base + "/v1/health")
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("mock server never came up: %v", err)
	}
	_ = resp.Body.Close()

	// The command blocks; the test ends here and the process tears it down.
	select {
	case err := <-done:
		t.Logf("mock returned early: %v", err)
	default:
	}
}

func TestFormatCopyAndPrologue(t *testing.T) {
	dir := t.TempDir()

	// A document with no XML declaration gets one added.
	noDecl := filepath.Join(dir, "nodecl.xml")
	if err := os.WriteFile(noDecl, []byte(`<Document xmlns="urn:t"><A>1</A></Document>`), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := run(t, "format", noDecl)
	if err != nil {
		t.Fatalf("format: %v", err)
	}
	if !strings.Contains(out, "<?xml") {
		t.Errorf("a declaration should be added:\n%s", out)
	}

	// --copy exercises the clipboard branch; it may be unavailable in CI.
	if _, err := run(t, "format", noDecl, "--copy"); err != nil {
		t.Errorf("--copy should not fail the command: %v", err)
	}
}

func TestCatalogAddFallsBackToFlagAndDefault(t *testing.T) {
	src := t.TempDir()
	archive := filepath.Join(src, "AccountSwitching_v05.zip")
	writeTestZip(t, archive, map[string]string{
		"acmt.027.001.05.xsd": fixtureSchema("acmt.027.001.05"),
	})

	// --catalog is used when --to and the environment are unset.
	dest := t.TempDir()
	t.Setenv("ANCHOR_CATALOG", "")
	prev := catalogPath
	catalogPath = dest
	t.Cleanup(func() { catalogPath = prev })

	if _, err := run(t, "catalog", "add", archive); err != nil {
		t.Fatalf("catalog add with --catalog: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "Account Switching", "Version 5.0")); err != nil {
		t.Errorf("--catalog should have been used: %v", err)
	}
}

func TestCatalogStatusWithoutCatalogue(t *testing.T) {
	isolate(t)
	out, err := run(t, "catalog", "status")
	if err != nil {
		t.Fatalf("catalog status should work in light mode: %v", err)
	}
	wantContains(t, out, "no catalogue installed", "published sets")
}

func TestRenderPlainTextNarrowWidth(t *testing.T) {
	// A width below the minimum falls back to a readable default.
	if out := renderPlainText("# T\n\nbody text", 5); strings.TrimSpace(out) == "" {
		t.Error("a very narrow width should still render")
	}
}

func TestRenderAnswerWithLongSuggestionList(t *testing.T) {
	idx := askFixtureIndex(t)
	ans := ai.MessageAnswer{
		Summary:     "S",
		Details:     strings.Repeat("Long body text. ", 100),
		Suggestions: []string{"a", "b", "c", "d", "e"},
		RelatedMsgs: idx.Messages,
	}
	if out := captureStdout(t, func() { renderAnswerWithContext(ans, true) }); strings.TrimSpace(out) == "" {
		t.Error("rendering produced nothing")
	}
}

func TestReplSlashCommandDirectly(t *testing.T) {
	idx := askFixtureIndex(t)

	for _, cmdStr := range []string{"/help", "/h", "/?", "/clear", "/cls",
		"/info pacs.008.001.10", "/info", "/xml pacs.008.001.10", "/xml",
		"/xsd pacs.008.001.10", "/xsd", "/unknown"} {
		if done := runReplSlashCommandFor(t, idx, cmdStr); done {
			t.Errorf("%q should not end the session", cmdStr)
		}
	}

	for _, cmdStr := range []string{"/exit", "/quit", "/q"} {
		if !runReplSlashCommandFor(t, idx, cmdStr) {
			t.Errorf("%q should end the session", cmdStr)
		}
	}
}

func runReplSlashCommandFor(t *testing.T, idx *catalog.Index, line string) bool {
	t.Helper()
	var done bool
	captureStdout(t, func() { done = runReplSlashCommand(idx, line) })
	return done
}

// Run is the whole program minus the process exit, so the top-level error
// handling can be driven directly.
func TestRunReturnsExitCodes(t *testing.T) {
	withCatalogue(t)

	var errOut strings.Builder
	RootCmd.SetArgs([]string{"version"})
	if code := Run(&errOut); code != 0 {
		t.Errorf("a successful command should exit 0, got %d", code)
	}
	if errOut.String() != "" {
		t.Errorf("nothing should be written on success: %q", errOut.String())
	}

	// A failing command prints a diagnostic and exits 1.
	errOut.Reset()
	RootCmd.SetArgs([]string{"generate", "zzzz.999"})
	if code := Run(&errOut); code != 1 {
		t.Errorf("a failing command should exit 1, got %d", code)
	}
	if !strings.Contains(errOut.String(), "Error:") {
		t.Errorf("the diagnostic should be written: %q", errOut.String())
	}

	// A command that already reported its own diagnostics stays quiet.
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.xml")
	if err := os.WriteFile(bad, []byte(fixtureInstance("pacs.008.001.10", "EURO")), 0o644); err != nil {
		t.Fatal(err)
	}
	errOut.Reset()
	RootCmd.SetArgs([]string{"validate", bad})
	captureStdout(t, func() {
		if code := Run(&errOut); code != 1 {
			t.Errorf("an invalid document should exit 1, got %d", code)
		}
	})
	if errOut.String() != "" {
		t.Errorf("a silent error should print nothing extra: %q", errOut.String())
	}

	RootCmd.SetArgs(nil)
	resetFlags(t)
}
