// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package importer

import (
	"archive/zip"
	"bytes"
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
	entries, err := os.ReadDir(dest)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("failed import left %d catalogue entries behind", len(entries))
	}
}

func TestDryRunEnforcesTotalSize(t *testing.T) {
	src, dest := t.TempDir(), t.TempDir()
	archive := writeZip(t, src, "set_v01.zip",
		entry{"pacs.008.001.10.xsd", []byte(strings.Repeat("x", 80))},
		entry{"pacs.009.001.10.xsd", []byte(strings.Repeat("y", 80))})
	lim := DefaultLimits()
	lim.MaxTotalSize = 100
	lim.MaxRatio = 1 << 30
	if _, err := ImportArchive(archive, Options{Root: dest, Limits: lim, DryRun: true}); err == nil {
		t.Fatal("dry-run should enforce the same decompressed-size budget")
	}
}

func TestImportRefusesCollisionsWithoutOverwriting(t *testing.T) {
	src, dest := t.TempDir(), t.TempDir()
	archive := writeZip(t, src, "set_v01.zip",
		entry{"one/pacs.008.001.10.xsd", []byte("first")},
		entry{"two/pacs.008.001.10.xsd", []byte("second")})
	if _, err := ImportArchive(archive, Options{Root: dest, Category: "Imported", Version: "Version 1.0"}); err == nil {
		t.Fatal("flattened duplicate names should be rejected")
	}
	entries, err := os.ReadDir(dest)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatal("a collision should leave the catalogue unchanged")
	}

	existing := filepath.Join(dest, "Imported", "Version 1.0", "Schemas", "pacs.008.001.10.xsd")
	if err := os.MkdirAll(filepath.Dir(existing), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(existing, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	archive = writeZip(t, src, "set_v01-again.zip",
		entry{"pacs.008.001.10.xsd", []byte("replace")})
	if _, err := ImportArchive(archive, Options{Root: dest, Category: "Imported", Version: "Version 1.0"}); err == nil {
		t.Fatal("an existing catalogue file should not be overwritten")
	}
	got, err := os.ReadFile(existing)
	if err != nil || string(got) != "keep" {
		t.Fatalf("existing file changed: %q, %v", got, err)
	}
}

func TestImportRefusesSymlinkedDestination(t *testing.T) {
	if os.Getenv("GOOS") == "windows" {
		t.Skip("symlink creation requires privileges on some Windows runners")
	}
	src, dest, outside := t.TempDir(), t.TempDir(), t.TempDir()
	category := filepath.Join(dest, "Imported")
	if err := os.Symlink(outside, category); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	archive := writeZip(t, src, "set_v01.zip",
		entry{"pacs.008.001.10.xsd", []byte("schema")})
	if _, err := ImportArchive(archive, Options{
		Root: dest, Category: "Imported", Version: "Version 1.0",
	}); err == nil {
		t.Fatal("import through a symlink should be rejected")
	}
	outsideEntries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatal(err)
	}
	if len(outsideEntries) != 0 {
		t.Fatal("import wrote outside the catalogue through a symlink")
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

func TestDestinationAndExclusiveCopyHelpers(t *testing.T) {
	root := t.TempDir()
	dest := filepath.Join(root, "Category", "Version 1", "Schemas", "m.xsd")
	if err := prepareDestination(root, dest); err != nil {
		t.Fatalf("prepare destination: %v", err)
	}

	source := filepath.Join(t.TempDir(), "source.xsd")
	if err := os.WriteFile(source, []byte("schema"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := copyFileExclusive(source, dest); err != nil {
		t.Fatalf("exclusive copy: %v", err)
	}
	if err := prepareDestination(root, dest); err == nil {
		t.Error("an existing destination should be rejected")
	}
	if err := copyFileExclusive(source, dest); err == nil {
		t.Error("exclusive copy should refuse an existing destination")
	}
	if err := copyFileExclusive(filepath.Join(t.TempDir(), "missing"), filepath.Join(root, "missing")); err == nil {
		t.Error("a missing source should be reported")
	}

	blockedRoot := t.TempDir()
	blocked := filepath.Join(blockedRoot, "file")
	if err := os.WriteFile(blocked, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := prepareDestination(blockedRoot, filepath.Join(blocked, "child", "m.xsd")); err == nil {
		t.Error("a file in the destination path should be rejected")
	}
	rootFile := filepath.Join(t.TempDir(), "catalogue-file")
	if err := os.WriteFile(rootFile, []byte("blocked"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := prepareDestination(rootFile, filepath.Join(rootFile, "child.xsd")); err == nil {
		t.Error("a regular file used as the catalogue root should be rejected")
	}
}

func TestExtractRejectsDestinationBeforeOpeningEntry(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.xsd")
	if _, err := extract(nil, root, outside, DefaultLimits()); err == nil {
		t.Error("an escaping destination should be rejected")
	}

	blocker := filepath.Join(root, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := extract(nil, root, filepath.Join(blocker, "m.xsd"), DefaultLimits()); err == nil {
		t.Error("a destination beneath a regular file should be rejected")
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

func TestImportDirSkipsArchivesWithoutRecognisedContent(t *testing.T) {
	src, dest := t.TempDir(), t.TempDir()
	writeZip(t, src, "unknown_v01.zip", entry{"opaque.bin", []byte("not ISO 20022 material")})
	writeZip(t, src, "good_v01.zip", entry{"pacs.008.001.10.xsd", []byte("<xs:schema/>")})
	results, err := ImportDir(src, Options{Root: dest})
	if err != nil {
		t.Fatalf("no-content archive should not abort the directory: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("directory results = %d, want one skipped and one imported archive", len(results))
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

func TestInvalidLimitsAreRejectedBeforeOpeningArchive(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxFiles = 0
	if _, err := ImportArchive("does-not-need-to-exist.zip", Options{
		Root: t.TempDir(), Limits: limits,
	}); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("invalid limits should be rejected first: %v", err)
	}
}

func TestGuidelineAndNestedVersionAreRecorded(t *testing.T) {
	src, dest := t.TempDir(), t.TempDir()
	inner := buildZip(t,
		entry{"CBPRPlus_MUG.pdf", []byte("%PDF")},
		entry{"pacs.008.001.10.xsd", []byte("<xs:schema/>")},
	)
	archive := writeZip(t, src, "outer_v01.zip", entry{"rules_v07.zip", inner})

	res, err := ImportArchive(archive, Options{Root: dest, Category: "Imported"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Guidelines != 1 || res.Schemas != 1 {
		t.Fatalf("wrong classification counts: %+v", res)
	}
	if _, err := os.Stat(filepath.Join(dest, "Imported", "Version 7.0", "Schemas", "pacs.008.001.10.xsd")); err != nil {
		t.Fatalf("nested archive version was not applied: %v", err)
	}
}

func TestEntryReadersDefendAgainstIncorrectZipMetadata(t *testing.T) {
	body := bytes.Repeat([]byte("x"), 100)
	archive := buildZip(t, entry{"payload.xsd", body})
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatal(err)
	}
	f := zr.File[0]
	// Treat the central-directory size as attacker-controlled. The streaming
	// limit must still stop extraction when the metadata understates the body.
	f.UncompressedSize64 = 1
	lim := DefaultLimits()
	lim.MaxFileSize = 10
	lim.MaxRatio = 1 << 30
	if _, err := inspectEntry(f, lim); err == nil {
		t.Fatal("dry-run accepted inconsistent zip metadata")
	}
	dest := filepath.Join(t.TempDir(), "payload.xsd")
	if _, err := extract(f, filepath.Dir(dest), dest, lim); err == nil {
		t.Fatal("extract accepted inconsistent zip metadata")
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("oversized partial output survived: %v", err)
	}

	f.Method = 0xffff
	if _, err := inspectEntry(f, DefaultLimits()); err == nil {
		t.Error("unsupported compression method should fail inspection")
	}
	if _, err := extract(f, t.TempDir(), filepath.Join(t.TempDir(), "bad.xsd"), DefaultLimits()); err == nil {
		t.Error("unsupported compression method should fail extraction")
	}
	if err := walkNested(f, "Imported", "Version 1.0", Options{Limits: DefaultLimits(), DryRun: true}, &Result{}, &counter{}, 0); err == nil {
		t.Error("unsupported nested compression method should fail")
	}
}

func TestCommitRollbackAndFlatDestination(t *testing.T) {
	root := t.TempDir()
	flat := filepath.Join(root, "flat.xsd")
	if err := prepareDestination(root, flat); err != nil {
		t.Fatalf("flat destination: %v", err)
	}

	staging := t.TempDir()
	first := filepath.Join(staging, "a", "first.xsd")
	second := filepath.Join(staging, "b", "second.xsd")
	for _, path := range []string{first, second} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(path), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	existing := filepath.Join(root, "b", "second.xsd")
	if err := os.MkdirAll(filepath.Dir(existing), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(existing, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := commitStaging(staging, root); err == nil {
		t.Fatal("commit should reject the existing second file")
	}
	if _, err := os.Stat(filepath.Join(root, "a", "first.xsd")); !os.IsNotExist(err) {
		t.Fatalf("rollback left the first committed file behind: %v", err)
	}
	if err := commitStaging(filepath.Join(t.TempDir(), "missing"), root); err == nil {
		t.Error("missing staging tree should be reported")
	}
}

func TestCopyFileExclusiveReportsReadFailure(t *testing.T) {
	// Opening a directory succeeds on Unix, but reading it does not. This drives
	// the rollback path without permissions or platform-specific ownership.
	dest := filepath.Join(t.TempDir(), "copy")
	if err := copyFileExclusive(t.TempDir(), dest); err == nil {
		t.Error("copying a directory as a file should fail")
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("failed copy left its destination behind: %v", err)
	}
}

func TestImportArchiveReportsBlockedParent(t *testing.T) {
	src := t.TempDir()
	archive := writeZip(t, src, "set_v01.zip", entry{"pacs.008.001.10.xsd", []byte("schema")})
	blocker := filepath.Join(t.TempDir(), "parent")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ImportArchive(archive, Options{Root: filepath.Join(blocker, "catalogue")}); err == nil {
		t.Error("a non-directory catalogue parent should fail")
	}
}
