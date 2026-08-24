// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package importer

import (
	"archive/zip"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type entry struct {
	name string
	body []byte
}

func buildZip(t *testing.T, entries ...entry) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		w, err := zw.Create(e.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(e.body); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func writeZip(t *testing.T, dir, name string, entries ...entry) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, buildZip(t, entries...), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestClassify(t *testing.T) {
	cases := map[string]Classification{
		"pacs.008.001.10.xsd":                        ClassSchema,
		"pacs.008.001.10_1.xsd":                      ClassSchema,
		"camt.053.001.11.xml":                        ClassSample,
		"head.001.001.02.xsd":                        ClassSchema,
		"ISO20022_MDRPart1_PaymentsClearing_v11.pdf": ClassReport,
		"ISO20022_MDRPart3_Payments_v1.xlsx":         ClassReport,
		"PaymentsClearing_MUG_2024.pdf":              ClassGuideline,
		"SEPA usage guidelines.docx":                 ClassGuideline,
		"SequenceDiagram.wmf":                        ClassDoc,
		"overview.htm":                               ClassDoc,
		"notes.txt":                                  ClassDoc,
		"diagram.png":                                ClassDoc,
		"binary.bin":                                 ClassSkip,
		"archive.tar.gz":                             ClassSkip,
		"":                                           ClassSkip,
	}
	for name, want := range cases {
		if got := Classify(name); got != want {
			t.Errorf("Classify(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestImportArchiveClassifiesAndFiles(t *testing.T) {
	src := t.TempDir()
	dest := t.TempDir()

	archive := writeZip(t, src, "PaymentsClearingAndSettlement_v11.zip",
		entry{"pacs.008.001.10.xsd", []byte("<xs:schema/>")},
		entry{"pacs.009.001.10.xsd", []byte("<xs:schema/>")},
		entry{"pacs.008.001.10.xml", []byte("<Document/>")},
		entry{"ISO20022_MDRPart1_Payments_v11.pdf", []byte("%PDF")},
		entry{"scratch.bin", []byte("junk")},
	)

	res, err := ImportArchive(archive, Options{Root: dest})
	if err != nil {
		t.Fatalf("ImportArchive: %v", err)
	}
	if res.Schemas != 2 || res.Samples != 1 || res.Reports != 1 || res.Skipped != 1 {
		t.Errorf("got %+v", res)
	}

	// The archive name is matched against the published message sets, so the
	// canonical directory name is used rather than the run-together filename.
	want := filepath.Join(dest, "Payments Clearing and Settlement", "Version 11.0")
	if _, err := os.Stat(filepath.Join(want, "Schemas", "pacs.008.001.10.xsd")); err != nil {
		t.Errorf("schema not filed at the canonical path: %v", err)
	}
	if _, err := os.Stat(filepath.Join(want, "Sample Messages", "pacs.008.001.10.xml")); err != nil {
		t.Errorf("sample not filed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(want, "Message Definition Reports", "ISO20022_MDRPart1_Payments_v11.pdf")); err != nil {
		t.Errorf("report not filed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(want, "Schemas", "scratch.bin")); err == nil {
		t.Error("unrecognised file should not have been written")
	}
}

// The RA ships zips inside zips; the importer must recurse.
func TestImportArchiveRecursesIntoNestedZips(t *testing.T) {
	src := t.TempDir()
	dest := t.TempDir()

	inner := buildZip(t,
		entry{"pacs.008.001.10.xsd", []byte("<xs:schema/>")},
		entry{"pain.001.001.11.xsd", []byte("<xs:schema/>")},
	)
	outer := writeZip(t, src, "Payments Initiation v13.zip",
		entry{"schemas.zip", inner},
		entry{"readme.txt", []byte("notes")},
	)

	res, err := ImportArchive(outer, Options{Root: dest})
	if err != nil {
		t.Fatalf("ImportArchive: %v", err)
	}
	if res.Schemas != 2 {
		t.Errorf("got %d schemas from the nested archive, want 2", res.Schemas)
	}
	if _, err := os.Stat(filepath.Join(dest, "Payments Initiation", "Version 13.0", "Schemas", "pacs.008.001.10.xsd")); err != nil {
		t.Errorf("nested schema not extracted: %v", err)
	}
}

func TestDryRunWritesNothing(t *testing.T) {
	src := t.TempDir()
	dest := t.TempDir()

	archive := writeZip(t, src, "set_v01.zip",
		entry{"pacs.008.001.10.xsd", []byte("<xs:schema/>")})

	res, err := ImportArchive(archive, Options{Root: dest, DryRun: true})
	if err != nil {
		t.Fatalf("ImportArchive: %v", err)
	}
	if res.Schemas != 1 {
		t.Errorf("dry run should still report 1 schema, got %d", res.Schemas)
	}
	if res.BytesWritten != 0 {
		t.Errorf("dry run wrote %d bytes", res.BytesWritten)
	}
	entries, _ := os.ReadDir(dest)
	if len(entries) != 0 {
		t.Errorf("dry run created %d entries in the destination", len(entries))
	}
}

// Zip slip. Entry names must never be able to place a file outside the
// catalogue root, whether by relative traversal, an absolute path, or a Windows
// separator. None of these begin with a dot, so the hidden-file filter cannot
// mask a regression here.
func TestZipSlipCannotEscapeRoot(t *testing.T) {
	src := t.TempDir()
	sandbox := t.TempDir()
	root := filepath.Join(sandbox, "catalog")

	archive := writeZip(t, src, "evil_v01.zip",
		// Deep enough to clear the Category/Version/Class prefix and escape.
		entry{"a/../../../../../../../../../../escaped.xsd", []byte("<xs:schema/>")},
		entry{"/etc/pacs.008.001.10.xsd", []byte("<xs:schema/>")},
		entry{`dir\..\..\windows.xsd`, []byte("<xs:schema/>")},
		entry{"pacs.009.001.10.xsd", []byte("<xs:schema/>")},
	)

	if _, err := ImportArchive(archive, Options{Root: root}); err != nil {
		t.Fatalf("ImportArchive: %v", err)
	}

	var escaped []string
	_ = filepath.Walk(sandbox, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !strings.HasPrefix(p, root+string(filepath.Separator)) {
			escaped = append(escaped, p)
		}
		return nil
	})
	if len(escaped) > 0 {
		t.Errorf("files written outside the catalogue root: %v", escaped)
	}

	// The legitimate entry must still have been imported.
	if _, err := os.Stat(filepath.Join(root, "Imported", "Version 1.0", "Schemas", "pacs.009.001.10.xsd")); err != nil {
		if _, err2 := os.Stat(root); err2 != nil {
			t.Fatalf("nothing was imported at all: %v", err)
		}
	}
}

// withinRoot is the last line of defence, so test it directly.
func TestWithinRoot(t *testing.T) {
	root := t.TempDir()
	if err := withinRoot(root, filepath.Join(root, "Cat", "Version 1.0", "Schemas", "a.xsd")); err != nil {
		t.Errorf("a path inside the root should be allowed: %v", err)
	}
	for _, bad := range []string{
		filepath.Join(root, "..", "escaped.xsd"),
		filepath.Join(root, "Cat", "..", "..", "escaped.xsd"),
	} {
		if err := withinRoot(root, bad); err == nil {
			t.Errorf("withinRoot should reject %s", bad)
		}
	}
}

// A highly compressible entry must be rejected on ratio rather than filling the
// disk.
func TestCompressionBombRejected(t *testing.T) {
	src := t.TempDir()
	dest := t.TempDir()

	archive := writeZip(t, src, "bomb_v01.zip",
		entry{"pacs.008.001.10.xsd", bytes.Repeat([]byte("A"), 8<<20)})

	lim := DefaultLimits()
	lim.MaxRatio = 10

	_, err := ImportArchive(archive, Options{Root: dest, Limits: lim})
	if err == nil {
		t.Fatal("a high compression ratio should be rejected")
	}
	if !strings.Contains(err.Error(), "compression ratio") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestFileCountLimit(t *testing.T) {
	src := t.TempDir()
	dest := t.TempDir()

	var entries []entry
	for i := 0; i < 20; i++ {
		entries = append(entries, entry{
			name: filepath.Join("s", string(rune('a'+i))+".xsd"),
			body: []byte("<xs:schema/>"),
		})
	}
	archive := writeZip(t, src, "many_v01.zip", entries...)

	lim := DefaultLimits()
	lim.MaxFiles = 5

	if _, err := ImportArchive(archive, Options{Root: dest, Limits: lim}); err == nil {
		t.Fatal("exceeding MaxFiles should be an error")
	}
}

func TestSingleFileSizeLimit(t *testing.T) {
	src := t.TempDir()
	dest := t.TempDir()

	archive := writeZip(t, src, "big_v01.zip",
		entry{"pacs.008.001.10.xsd", bytes.Repeat([]byte("<a/>"), 4096)})

	lim := DefaultLimits()
	lim.MaxFileSize = 100
	lim.MaxRatio = 1 << 30

	if _, err := ImportArchive(archive, Options{Root: dest, Limits: lim}); err == nil {
		t.Fatal("an oversized entry should be rejected")
	}
}

func TestNestingDepthLimit(t *testing.T) {
	src := t.TempDir()
	dest := t.TempDir()

	blob := buildZip(t, entry{"pacs.008.001.10.xsd", []byte("<xs:schema/>")})
	for i := 0; i < 6; i++ {
		blob = buildZip(t, entry{"inner.zip", blob})
	}
	archive := filepath.Join(src, "deep_v01.zip")
	if err := os.WriteFile(archive, blob, 0o644); err != nil {
		t.Fatal(err)
	}

	lim := DefaultLimits()
	lim.MaxDepth = 2

	if _, err := ImportArchive(archive, Options{Root: dest, Limits: lim}); err == nil {
		t.Fatal("exceeding MaxDepth should be an error")
	}
}

func TestArchiveWithNoISOContent(t *testing.T) {
	src := t.TempDir()
	dest := t.TempDir()

	archive := writeZip(t, src, "holiday_photos.zip",
		entry{"a.bin", []byte("x")},
		entry{"b.dat", []byte("y")})

	_, err := ImportArchive(archive, Options{Root: dest})
	if !errors.Is(err, ErrNoContent) {
		t.Errorf("want ErrNoContent, got %v", err)
	}
}

func TestRootIsRequired(t *testing.T) {
	src := t.TempDir()
	archive := writeZip(t, src, "s_v01.zip", entry{"pacs.008.001.10.xsd", []byte("<xs:schema/>")})
	if _, err := ImportArchive(archive, Options{}); err == nil {
		t.Error("Root must be required")
	}
}

func TestCategoryAndVersionOverride(t *testing.T) {
	src := t.TempDir()
	dest := t.TempDir()

	archive := writeZip(t, src, "mystery.zip",
		entry{"pacs.008.001.10.xsd", []byte("<xs:schema/>")})

	if _, err := ImportArchive(archive, Options{
		Root:     dest,
		Category: "My Category",
		Version:  "Version 42.0",
	}); err != nil {
		t.Fatalf("ImportArchive: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "My Category", "Version 42.0", "Schemas", "pacs.008.001.10.xsd")); err != nil {
		t.Errorf("overrides not honoured: %v", err)
	}
}

// Downloads get named inconsistently; they should still land in one directory.
func TestCategoryNameCanonicalisation(t *testing.T) {
	for _, name := range []string{
		"PaymentsClearingAndSettlement_v11.zip",
		"payments-clearing-and-settlement.zip",
		"Payments Clearing and Settlement v11.zip",
		"payments_clearing_and_settlement_V11BAH.zip",
	} {
		if got := categoryFromArchiveName(name); got != "Payments Clearing and Settlement" {
			t.Errorf("categoryFromArchiveName(%q) = %q", name, got)
		}
	}
}

func TestVersionFromArchiveName(t *testing.T) {
	cases := map[string]string{
		"PaymentsClearing_v11.zip": "Version 11.0",
		"AccountSwitching_v03.zip": "Version 3.0",
		"Something V07BAH.zip":     "Version 7.0",
		"no-version-here.zip":      "",
	}
	for in, want := range cases {
		if got := versionFromArchiveName(in); got != want {
			t.Errorf("versionFromArchiveName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestImportDir(t *testing.T) {
	src := t.TempDir()
	dest := t.TempDir()

	writeZip(t, src, "AccountSwitching_v05.zip", entry{"acmt.027.001.05.xsd", []byte("<xs:schema/>")})
	writeZip(t, src, "PaymentsInitiation_v13.zip", entry{"pain.001.001.11.xsd", []byte("<xs:schema/>")})
	if err := os.WriteFile(filepath.Join(src, "notes.txt"), []byte("ignore me"), 0o644); err != nil {
		t.Fatal(err)
	}

	results, err := ImportDir(src, Options{Root: dest})
	if err != nil {
		t.Fatalf("ImportDir: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}

	if _, err := ImportDir(t.TempDir(), Options{Root: dest}); err == nil {
		t.Error("a directory with no archives should be an error")
	}
}

func TestTargetPath(t *testing.T) {
	tg := Target{
		Category: "Payments Clearing and Settlement",
		Version:  "Version 11.0",
		Class:    ClassSchema,
		Name:     "pacs.008.001.10.xsd",
	}
	want := filepath.Join("Payments Clearing and Settlement", "Version 11.0", "Schemas", "pacs.008.001.10.xsd")
	if got := tg.Path(); got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}
}
