// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package catalog

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeCatalog builds the minimum tree that counts as a catalogue.
func fakeCatalog(t *testing.T, msgIDs ...string) string {
	t.Helper()
	root := t.TempDir()
	schemas := filepath.Join(root, "Payments Clearing and Settlement", "Version 11.0", "Schemas")
	if err := os.MkdirAll(schemas, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, id := range msgIDs {
		if err := os.WriteFile(filepath.Join(schemas, id+".xsd"), []byte("<xs:schema/>"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestIsCatalog(t *testing.T) {
	if got := IsCatalog(fakeCatalog(t, "pacs.008.001.10")); !got {
		t.Error("a tree with Category/Version N/Schemas should be a catalogue")
	}
	if IsCatalog(t.TempDir()) {
		t.Error("an empty directory must not be treated as a catalogue")
	}
	if IsCatalog("") {
		t.Error("the empty path must not be treated as a catalogue")
	}
	if IsCatalog(filepath.Join(t.TempDir(), "nope")) {
		t.Error("a missing directory must not be treated as a catalogue")
	}
}

// A source checkout has cmd/ and internal/ but no message sets. It must not be
// mistaken for a catalogue -- that is the bug that made `askiso search` return
// zero results with exit status 0.
func TestIsCatalogIgnoresSourceTree(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"cmd", "internal", "pkg"} {
		if err := os.MkdirAll(filepath.Join(root, d, "Version 1.0", "Schemas"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if IsCatalog(root) {
		t.Error("a source checkout must not resolve as a catalogue")
	}
}

func TestResolveOverrideWins(t *testing.T) {
	want := fakeCatalog(t, "pacs.008.001.10")
	t.Setenv(EnvCatalog, fakeCatalog(t, "camt.053.001.11"))

	got, err := Resolve(want)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != want {
		t.Errorf("override should beat %s; got %s, want %s", EnvCatalog, got, want)
	}
}

func TestResolveUsesEnv(t *testing.T) {
	want := fakeCatalog(t, "pacs.008.001.10")
	t.Setenv(EnvCatalog, want)

	got, err := Resolve("")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestResolveIgnoresBadEnvAndKeepsSearching(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LocalAppData", t.TempDir())
	t.Chdir(t.TempDir())
	t.Setenv(EnvCatalog, filepath.Join(t.TempDir(), "does-not-exist"))
	root := fakeCatalogAt(t)
	// DefaultDir appends "askiso/catalog", so XDG_DATA_HOME is two levels up.
	t.Setenv("XDG_DATA_HOME", filepath.Dir(filepath.Dir(root)))

	// AskISO is a source checkout with no catalogue, so the cwd walk-up cannot
	// mask a failure to consult XDG_DATA_HOME.
	got, err := Resolve("")
	if err != nil {
		t.Fatalf("a bad %s should fall through to the next candidate: %v", EnvCatalog, err)
	}
	if got != root {
		t.Errorf("got %s, want the XDG candidate %s", got, root)
	}
}

// fakeCatalogAt builds <tmp>/askiso/catalog so it is found via XDG_DATA_HOME.
func fakeCatalogAt(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "askiso", "catalog")
	schemas := filepath.Join(root, "Payments Initiation", "Version 13.0", "Schemas")
	if err := os.MkdirAll(schemas, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(schemas, "pain.001.001.11.xsd"), []byte("<xs:schema/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestResolveNotFoundIsActionable(t *testing.T) {
	t.Setenv(EnvCatalog, "")
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LocalAppData", t.TempDir())
	t.Chdir(t.TempDir())

	_, err := Resolve("")
	if err == nil {
		t.Fatal("expected an error when no catalogue exists")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("error should match ErrNotFound, got %T", err)
	}

	msg := err.Error()
	for _, want := range []string{"iso20022.org", "askiso catalog add", EnvCatalog, "Searched:"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message should tell the user about %q:\n%s", want, msg)
		}
	}
}

// Load must never hand back an empty index. Returning zero messages with a nil
// error is what let a broken install look healthy.
func TestLoadRejectsEmptyCatalogue(t *testing.T) {
	if _, err := Load(t.TempDir()); err == nil {
		t.Fatal("Load on a directory with no messages must return an error")
	}
}

func TestLoadCountsMessages(t *testing.T) {
	root := fakeCatalog(t, "pacs.008.001.10", "pacs.009.001.10")
	idx, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(idx.Messages) != 2 {
		t.Errorf("got %d messages, want 2", len(idx.Messages))
	}
	if idx.RootDir != root {
		t.Errorf("RootDir = %s, want %s", idx.RootDir, root)
	}
	if _, ok := idx.MessageMap["pacs.008.001.10"]; !ok {
		t.Error("MessageMap should be populated by Load")
	}
}

func TestDefaultDirHonoursXDG(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/xdg")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LocalAppData", t.TempDir())
	if got, want := DefaultDir(), filepath.Join("/xdg", "askiso", "catalog"); got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// Importing must extend a catalogue that already exists rather than starting a
// second one in a different conventional location.
func TestDefaultDirPrefersAnExistingCatalogue(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("LocalAppData", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "xdg"))

	dirs := DefaultDirs()
	if len(dirs) < 2 {
		t.Skip("platform has only one conventional location")
	}

	// Nothing installed yet: the first candidate wins.
	if got := DefaultDir(); got != dirs[0] {
		t.Errorf("with no catalogue installed, got %s, want %s", got, dirs[0])
	}

	// Put a catalogue in the *second* candidate; it must now be chosen.
	existing := dirs[1]
	schemas := filepath.Join(existing, "Payments Clearing and Settlement", "Version 11.0", "Schemas")
	if err := os.MkdirAll(schemas, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(schemas, "pacs.008.001.10.xsd"), []byte("<xs:schema/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := DefaultDir(); got != existing {
		t.Errorf("an existing catalogue should win: got %s, want %s", got, existing)
	}
}

func TestDefaultDirsAreDistinct(t *testing.T) {
	seen := map[string]bool{}
	for _, d := range DefaultDirs() {
		if d == "" {
			t.Error("DefaultDirs returned an empty path")
		}
		if seen[d] {
			t.Errorf("duplicate candidate %s", d)
		}
		seen[d] = true
	}
}

// An iCloud-evicted schema is a zero-length file shadowed by a hidden
// placeholder. Reading it would yield a stub, so it must be reported.
func TestCheckEvicted(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "pacs.008.001.10.xsd")
	if err := os.WriteFile(real, []byte("<xs:schema/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CheckEvicted(real); err != nil {
		t.Errorf("a present file must not be reported as evicted: %v", err)
	}

	gone := filepath.Join(dir, "camt.053.001.11.xsd")
	stub := filepath.Join(dir, ".camt.053.001.11.xsd.icloud")
	if err := os.WriteFile(stub, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	err := CheckEvicted(gone)
	if err == nil {
		t.Fatal("a file shadowed by an .icloud placeholder must be reported as evicted")
	}
	if !strings.Contains(err.Error(), "brctl download") {
		t.Errorf("message should name the fix command:\n%s", err)
	}
}

func TestCheckEvictedOnMissingFile(t *testing.T) {
	dir := t.TempDir()
	// A file that is simply absent, with no placeholder, is not an eviction.
	if err := CheckEvicted(filepath.Join(dir, "gone.xsd")); err != nil {
		t.Errorf("a plain missing file should not be reported as evicted: %v", err)
	}
	// A zero-length file with no placeholder is likewise not an eviction.
	empty := filepath.Join(dir, "empty.xsd")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CheckEvicted(empty); err != nil {
		t.Errorf("an empty file with no placeholder is not evicted: %v", err)
	}
}

func TestEvictedErrorNamesTheDirectory(t *testing.T) {
	e := &EvictedError{Path: "/some/dir/pacs.008.001.10.xsd"}
	msg := e.Error()
	if !strings.Contains(msg, "brctl download") || !strings.Contains(msg, "/some/dir") {
		t.Errorf("the message should name the fix and the directory:\n%s", msg)
	}
}

func TestExtractBaseCodeEdgeCases(t *testing.T) {
	cases := map[string]string{
		"pacs.008.001.10": "pacs.008",
		"pacs":            "pacs",
		"":                "",
		"a.b":             "a.b",
	}
	for in, want := range cases {
		if got := extractBaseCode(in); got != want {
			t.Errorf("extractBaseCode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLoadReportsUnreadableRoot(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "no-such-dir")); err == nil {
		t.Error("an unreadable root should be an error")
	}
}

func TestDefaultDirWithoutHome(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("HOME", "")
	t.Setenv("LocalAppData", "")
	// With no home and no XDG override there may be no candidate at all; the
	// call must still return rather than panic.
	_ = DefaultDir()
	_ = DefaultDirs()
}
