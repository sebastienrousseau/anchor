// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package cmd

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeSetZip builds an archive shaped like a Registration Authority download.
func writeSetZip(t *testing.T, path string) {
	t.Helper()

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	zw := zip.NewWriter(f)
	for name, body := range map[string]string{
		"Schemas/pacs.008.001.10.xsd":         fixtureSchema("pacs.008.001.10"),
		"Sample Messages/pacs.008.001.10.xml": fixtureInstance("pacs.008.001.10", "EUR"),
	} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestFetchFindsAMessageSet(t *testing.T) {
	// --no-open keeps the browser out of a test run; the point being checked is
	// that a message identifier resolves to the right download.
	watch := t.TempDir()
	_, err := run(t, "catalog", "fetch", "pacs.008",
		"--no-open", "--watch", watch, "--timeout", "1s")
	if err == nil {
		t.Fatal("expected a timeout with nothing to find")
	}
	if !strings.Contains(err.Error(), "catalog add") {
		t.Errorf("the timeout does not say what to do instead: %v", err)
	}
}

func TestFetchImportsAnArchiveThatArrives(t *testing.T) {
	watch := t.TempDir()
	dest := filepath.Join(t.TempDir(), "catalogue")

	// The archive lands after the watch begins, which is the case the command
	// exists for.
	go func() {
		time.Sleep(1500 * time.Millisecond)
		writeSetZip(t, filepath.Join(watch, "PaymentsClearingAndSettlement_v11.zip"))
	}()

	out, err := run(t, "catalog", "fetch", "pacs.008",
		"--no-open", "--watch", watch, "--timeout", "30s", "--to", dest)
	if err != nil {
		t.Fatalf("catalog fetch: %v\n%s", err, out)
	}

	wantContains(t, out, "FETCH", "iso20022.org/message-set", "INSTALLED", "schemas")

	// And the schema really is where Anchor said it put it.
	found := false
	err = filepath.Walk(dest, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(p, "pacs.008.001.10.xsd") {
			found = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Errorf("the schema was not installed under %s", dest)
	}
}

func TestFetchIgnoresArchivesThatWereAlreadyThere(t *testing.T) {
	// A downloads folder is full of zips. Only one that arrives after the wait
	// begins is the one the user just fetched.
	watch := t.TempDir()
	writeSetZip(t, filepath.Join(watch, "something-else.zip"))

	_, err := run(t, "catalog", "fetch", "pacs.008",
		"--no-open", "--watch", watch, "--timeout", "2s", "--to", t.TempDir())
	if err == nil {
		t.Error("a pre-existing archive was imported")
	}
}

func TestFetchRejectsAnUnknownQuery(t *testing.T) {
	_, err := run(t, "catalog", "fetch", "zzzz.999",
		"--no-open", "--watch", t.TempDir(), "--timeout", "1s")
	if err == nil {
		t.Fatal("an unknown message was accepted")
	}
	if !strings.Contains(err.Error(), "catalog status") {
		t.Errorf("the error does not say how to see what exists: %v", err)
	}
}

func TestFetchReportsAnAmbiguousQuery(t *testing.T) {
	// "payments" spans several published sets; naming one is the user's call.
	_, err := run(t, "catalog", "fetch", "Payments",
		"--no-open", "--watch", t.TempDir(), "--timeout", "1s")
	if err == nil {
		t.Fatal("an ambiguous query was accepted")
	}
	if !strings.Contains(err.Error(), "message sets") {
		t.Errorf("the error does not list the candidates: %v", err)
	}
}

func TestFetchRejectsABadWatchDirectory(t *testing.T) {
	_, err := run(t, "catalog", "fetch", "pacs.008",
		"--no-open", "--watch", filepath.Join(t.TempDir(), "absent"), "--timeout", "1s")
	if err == nil {
		t.Fatal("a missing watch directory was accepted")
	}
	if !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("error = %v", err)
	}
}

func TestFetchIgnoresAHalfWrittenDownload(t *testing.T) {
	// A browser writes a download incrementally. Importing it while it is still
	// growing would produce a corrupt catalogue.
	watch := t.TempDir()
	path := filepath.Join(watch, "growing.zip")

	go func() {
		time.Sleep(1200 * time.Millisecond)
		_ = os.WriteFile(path, []byte("PK\x03\x04 partial"), 0o644)
		time.Sleep(1500 * time.Millisecond)
		writeSetZip(t, path)
	}()

	out, err := run(t, "catalog", "fetch", "pacs.008",
		"--no-open", "--watch", watch, "--timeout", "30s", "--to", filepath.Join(t.TempDir(), "cat"))
	if err != nil {
		t.Fatalf("catalog fetch: %v\n%s", err, out)
	}
	wantContains(t, out, "INSTALLED")
}

func TestHumanBytes(t *testing.T) {
	cases := map[int64]string{
		0:       "0 B",
		512:     "512 B",
		1536:    "1.5 KB",
		5 << 20: "5.0 MB",
		3 << 30: "3.0 GB",
	}
	for in, want := range cases {
		if got := humanBytes(in); got != want {
			t.Errorf("humanBytes(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	if got := expandHome("~/Downloads"); got != filepath.Join(home, "Downloads") {
		t.Errorf("expandHome = %q", got)
	}
	if got := expandHome("/absolute"); got != "/absolute" {
		t.Errorf("expandHome = %q", got)
	}
}

func TestFetchDefaultsToTheDownloadsFolder(t *testing.T) {
	// With no --watch, the Downloads folder is where a browser puts things.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("LocalAppData", home)
	if err := os.MkdirAll(filepath.Join(home, "Downloads"), 0o755); err != nil {
		t.Fatal(err)
	}

	out, err := run(t, "catalog", "fetch", "pacs.008", "--no-open", "--timeout", "1s")
	if err == nil {
		t.Fatal("expected a timeout")
	}
	if !strings.Contains(err.Error(), filepath.Join(home, "Downloads")) {
		t.Errorf("the watch directory was not the Downloads folder: %v\n%s", err, out)
	}
}

func TestFetchWithoutADownloadsFolder(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LocalAppData", t.TempDir())

	_, err := run(t, "catalog", "fetch", "pacs.008", "--no-open", "--timeout", "1s")
	if err == nil {
		t.Fatal("expected an error with no Downloads folder")
	}
	if !strings.Contains(err.Error(), "--watch") {
		t.Errorf("the error does not say how to fix it: %v", err)
	}
}

func TestFetchExpandsATildeInTheWatchPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("LocalAppData", home)
	if err := os.MkdirAll(filepath.Join(home, "Inbox"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := run(t, "catalog", "fetch", "pacs.008", "--no-open",
		"--watch", "~/Inbox", "--timeout", "1s")
	if err == nil {
		t.Fatal("expected a timeout")
	}
	if !strings.Contains(err.Error(), filepath.Join(home, "Inbox")) {
		t.Errorf("the tilde was not expanded: %v", err)
	}
}

func TestFetchReportsAnUnusableArchive(t *testing.T) {
	// A zip that holds nothing Anchor recognises has to say so, and say what to
	// try instead.
	watch := t.TempDir()

	go func() {
		time.Sleep(1200 * time.Millisecond)
		f, err := os.Create(filepath.Join(watch, "empty.zip"))
		if err != nil {
			return
		}
		zw := zip.NewWriter(f)
		// A file type Anchor does not classify, so the archive holds nothing.
		w, err := zw.Create("installer.bin")
		if err == nil {
			_, _ = w.Write([]byte("nothing useful here"))
		}
		_ = zw.Close()
		_ = f.Close()
	}()

	_, err := run(t, "catalog", "fetch", "pacs.008", "--no-open",
		"--watch", watch, "--timeout", "30s", "--to", filepath.Join(t.TempDir(), "cat"))
	if err == nil {
		t.Fatal("an archive with no ISO 20022 content was accepted")
	}
	if !strings.Contains(err.Error(), "catalog add") {
		t.Errorf("the error does not suggest the manual route: %v", err)
	}
}

func TestFetchWatchOnAFileRatherThanADirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "catalog", "fetch", "pacs.008", "--no-open",
		"--watch", path, "--timeout", "1s"); err == nil {
		t.Error("a file was accepted as a watch directory")
	}
}

func TestZipsInReportsAMissingDirectory(t *testing.T) {
	if _, err := zipsIn(filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Error("a missing directory was accepted")
	}
}
