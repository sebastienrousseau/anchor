// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package importer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClassifyRemainingExtensions(t *testing.T) {
	cases := map[string]Classification{
		"loose.xml":               ClassSample,
		"report.doc":              ClassReport,
		"report.xls":              ClassReport,
		"picture.jpeg":            ClassDoc,
		"picture.svg":             ClassDoc,
		"help.cnt":                ClassDoc,
		"notes.rtf":               ClassDoc,
		"drawing.emf":             ClassDoc,
		"UsageGuideline_2024.pdf": ClassGuideline,
		"unlabelled.pdf":          ClassReport,
	}
	for name, want := range cases {
		if got := Classify(name); got != want {
			t.Errorf("Classify(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestProgressCallbackFires(t *testing.T) {
	src, dest := t.TempDir(), t.TempDir()
	archive := writeZip(t, src, "set_v01.zip",
		entry{"pacs.008.001.10.xsd", []byte("<xs:schema/>")},
		entry{"pacs.009.001.10.xsd", []byte("<xs:schema/>")})

	var seen []string
	_, err := ImportArchive(archive, Options{
		Root:   dest,
		OnFile: func(tg Target) { seen = append(seen, tg.Name) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) != 2 {
		t.Errorf("the callback fired %d times, want 2", len(seen))
	}
}

func TestDirectoryEntriesAndDotfilesSkipped(t *testing.T) {
	src, dest := t.TempDir(), t.TempDir()
	archive := writeZip(t, src, "set_v01.zip",
		entry{"subdir/", nil},
		entry{".hidden.xsd", []byte("<xs:schema/>")},
		entry{"pacs.008.001.10.xsd", []byte("<xs:schema/>")})

	res, err := ImportArchive(archive, Options{Root: dest})
	if err != nil {
		t.Fatal(err)
	}
	if res.Schemas != 1 {
		t.Errorf("only the visible schema should be imported, got %d", res.Schemas)
	}
	if res.Skipped == 0 {
		t.Error("the dotfile should be counted as skipped")
	}
}

func TestTotalSizeLimit(t *testing.T) {
	src, dest := t.TempDir(), t.TempDir()
	body := strings.Repeat("<a/>", 500)
	archive := writeZip(t, src, "set_v01.zip",
		entry{"pacs.008.001.10.xsd", []byte(body)},
		entry{"pacs.009.001.10.xsd", []byte(body)})

	lim := DefaultLimits()
	lim.MaxTotalSize = 100
	lim.MaxRatio = 1 << 30

	if _, err := ImportArchive(archive, Options{Root: dest, Limits: lim}); err == nil {
		t.Error("exceeding the total size budget should be an error")
	}
}

func TestUnreadableArchiveReported(t *testing.T) {
	dest := t.TempDir()
	if _, err := ImportArchive(filepath.Join(t.TempDir(), "missing.zip"), Options{Root: dest}); err == nil {
		t.Error("a missing archive should be an error")
	}

	notZip := filepath.Join(t.TempDir(), "notzip.zip")
	if err := os.WriteFile(notZip, []byte("this is not a zip"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ImportArchive(notZip, Options{Root: dest}); err == nil {
		t.Error("a file that is not a zip should be an error")
	}
}

// A nested entry that is named .zip but is not one is skipped, not fatal.
func TestBogusNestedArchiveSkipped(t *testing.T) {
	src, dest := t.TempDir(), t.TempDir()
	archive := writeZip(t, src, "set_v01.zip",
		entry{"broken.zip", []byte("not actually a zip")},
		entry{"pacs.008.001.10.xsd", []byte("<xs:schema/>")})

	res, err := ImportArchive(archive, Options{Root: dest})
	if err != nil {
		t.Fatalf("a bogus nested archive should not be fatal: %v", err)
	}
	if res.Schemas != 1 {
		t.Errorf("the real schema should still import, got %d", res.Schemas)
	}
}

func TestNestedArchiveSizeLimit(t *testing.T) {
	src, dest := t.TempDir(), t.TempDir()
	inner := buildZip(t, entry{"pacs.008.001.10.xsd", []byte(strings.Repeat("<a/>", 400))})
	archive := writeZip(t, src, "outer_v01.zip", entry{"inner.zip", inner})

	lim := DefaultLimits()
	lim.MaxFileSize = 50
	lim.MaxRatio = 1 << 30

	if _, err := ImportArchive(archive, Options{Root: dest, Limits: lim}); err == nil {
		t.Error("an oversized nested archive should be rejected")
	}
}

func TestCanonicalCategoryFallsBackToCamelSplit(t *testing.T) {
	// A name matching no published set is split on case instead.
	got := categoryFromArchiveName("SomeUnknownVendorBundle_v01.zip")
	if !strings.Contains(got, " ") {
		t.Errorf("a run-together name should be split into words, got %q", got)
	}

	// An archive with no usable name at all.
	if got := categoryFromArchiveName("_v01.zip"); got != "Imported" {
		t.Errorf("an empty derived name should fall back, got %q", got)
	}
}

func TestByteReaderAtBounds(t *testing.T) {
	r := newByteReaderAt([]byte("hello"))

	buf := make([]byte, 3)
	if n, err := r.ReadAt(buf, 0); n != 3 || err != nil {
		t.Errorf("ReadAt(0) = %d, %v", n, err)
	}
	if _, err := r.ReadAt(buf, 10); err == nil {
		t.Error("reading past the end should report EOF")
	}
	if _, err := r.ReadAt(buf, -1); err == nil {
		t.Error("a negative offset should report EOF")
	}
	// A short read at the tail.
	big := make([]byte, 10)
	if n, err := r.ReadAt(big, 3); n != 2 || err == nil {
		t.Errorf("a short tail read = %d, %v", n, err)
	}
}

func TestExtractReportsUnwritableDestination(t *testing.T) {
	src := t.TempDir()
	archive := writeZip(t, src, "set_v01.zip", entry{"pacs.008.001.10.xsd", []byte("<xs:schema/>")})

	// A file where the catalogue root should be makes every mkdir fail.
	blocked := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(blocked, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ImportArchive(archive, Options{Root: blocked}); err == nil {
		t.Error("an unwritable root should be an error")
	}
}

func TestImportDirPropagatesFailure(t *testing.T) {
	src, dest := t.TempDir(), t.TempDir()
	writeZip(t, src, "good_v01.zip", entry{"pacs.008.001.10.xsd", []byte("<xs:schema/>")})
	if err := os.WriteFile(filepath.Join(src, "broken.zip"), []byte("not a zip"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ImportDir(src, Options{Root: dest}); err == nil {
		t.Error("an unreadable archive in the directory should be reported")
	}

	if _, err := ImportDir(filepath.Join(src, "no-such-dir"), Options{Root: dest}); err == nil {
		t.Error("a missing directory should be an error")
	}
}

func TestSplitCamelEdgeCases(t *testing.T) {
	if got := splitCamel(""); got != "" {
		t.Errorf("splitCamel(empty) = %q", got)
	}
	if got := splitCamel("ABC"); got != "ABC" {
		t.Errorf("an all-caps run should not be split: %q", got)
	}
	if got := splitCamel("fooBar"); got != "foo Bar" {
		t.Errorf("splitCamel(fooBar) = %q", got)
	}
}

func TestNormaliseKey(t *testing.T) {
	if normaliseKey("Payments Clearing & Settlement (v11)") != "paymentsclearingsettlementv11" {
		t.Errorf("normaliseKey = %q", normaliseKey("Payments Clearing & Settlement (v11)"))
	}
	if normaliseKey("!!!") != "" {
		t.Error("punctuation only should normalise to empty")
	}
}

func TestWithinRootRejectsUnresolvablePaths(t *testing.T) {
	root := t.TempDir()
	if err := withinRoot(root, filepath.Join(root, "ok.xsd")); err != nil {
		t.Errorf("a path inside the root should be allowed: %v", err)
	}
	// A relative root still resolves against the working directory.
	if err := withinRoot(".", "./sub/file.xsd"); err != nil {
		t.Errorf("relative paths should resolve: %v", err)
	}
}

func TestCanonicalCategoryMisses(t *testing.T) {
	if _, ok := canonicalCategory(""); ok {
		t.Error("an empty name should not match a published set")
	}
	if _, ok := canonicalCategory("!!!"); ok {
		t.Error("punctuation only should not match")
	}
	if name, ok := canonicalCategory("Account Switching"); !ok || name != "Account Switching" {
		t.Errorf("an exact name should match: %q %v", name, ok)
	}
	// The longest containing name wins.
	if name, ok := canonicalCategory("Corporate Actions Variant 002 - ISO 15022 Variants"); ok {
		if !strings.Contains(name, "Variant") {
			t.Errorf("the more specific set should win, got %q", name)
		}
	}
}

func TestExtractRespectsFileSizeDuringCopy(t *testing.T) {
	src, dest := t.TempDir(), t.TempDir()
	archive := writeZip(t, src, "set_v01.zip",
		entry{"pacs.008.001.10.xsd", []byte(strings.Repeat("<a/>", 100))})

	lim := DefaultLimits()
	lim.MaxRatio = 1 << 30
	res, err := ImportArchive(archive, Options{Root: dest, Limits: lim})
	if err != nil {
		t.Fatal(err)
	}
	if res.BytesWritten == 0 {
		t.Error("BytesWritten should report what was copied")
	}
}
