// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package codes

import (
	"archive/zip"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/sebastienrousseau/askiso/internal/catalog"
)

func TestCuratedAndSchemaSortingFallbacks(t *testing.T) {
	if got := Lookup("   "); len(got) != len(GetAllCodes()) {
		t.Fatalf("empty curated lookup returned %d of %d codes", len(got), len(GetAllCodes()))
	}
	items := []SchemaCode{{Code: "Z", Set: "Same"}, {Code: "A", Set: "Same"}}
	sortCodes(items)
	if items[0].Code != "A" {
		t.Fatalf("same-set codes were not sorted by code: %+v", items)
	}
	sameCode := []SchemaCode{{Code: "A", Set: "Z"}, {Code: "A", Set: "A"}}
	sortCodes(sameCode)
	if sameCode[0].Set != "A" {
		t.Fatalf("equal codes were not sorted by set: %+v", sameCode)
	}
	lookup := &SchemaIndex{Sets: map[string]map[string]*SchemaCode{
		"Z": {"A": {Code: "A", Set: "Z"}},
		"A": {"A": {Code: "A", Set: "A"}},
	}}
	if got := lookup.Lookup("a"); len(got) != 2 || got[0].Set != "A" {
		t.Fatalf("multi-set lookup was not sorted: %+v", got)
	}
}

func TestBuildSchemaIndexWithoutSchemaJobs(t *testing.T) {
	idx, err := BuildIndex(&catalog.Index{RootDir: t.TempDir()})
	if err != nil || idx.Total() != 0 {
		t.Fatalf("empty catalogue index = %+v, %v", idx, err)
	}
}

func TestMergeSchemaIndexFillsDescriptionAndCapsMessages(t *testing.T) {
	existing := &SchemaCode{Code: "AB", Set: "Codes", Messages: []string{"m0", "m1", "m2", "m3", "m4", "m5", "m6"}}
	dst := &SchemaIndex{Sets: map[string]map[string]*SchemaCode{"Codes": {"AB": existing}}}
	src := &SchemaIndex{Sets: map[string]map[string]*SchemaCode{"Codes": {"AB": {
		Code: "AB", Set: "Codes", Description: "description", Messages: []string{"m6", "m7", "m8"},
	}}}}
	mergeInto(dst, src)
	if existing.Description != "description" || len(existing.Messages) != 8 || existing.Messages[7] != "m7" {
		t.Fatalf("merged code = %+v", existing)
	}
}

func TestSchemaExtractionMalformedAndMissingAttributes(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.xsd")
	if err := os.WriteFile(bad, []byte(`<schema><simpleType name="Code"><restriction>`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := extractInto(&SchemaIndex{Sets: map[string]map[string]*SchemaCode{}}, bad, "pacs.008.001.08"); err == nil {
		t.Fatal("malformed schema should report its XML error")
	}
	good := filepath.Join(dir, "attributes.xsd")
	if err := os.WriteFile(good, []byte(`<schema><simpleType><restriction><enumeration/></restriction></simpleType></schema>`), 0o600); err != nil {
		t.Fatal(err)
	}
	idx := &SchemaIndex{Sets: map[string]map[string]*SchemaCode{}}
	if err := extractInto(idx, good, "pacs.008.001.08"); err != nil || idx.Total() != 0 {
		t.Fatalf("missing names should be ignored: %+v, %v", idx, err)
	}
}

func TestExternalImportRejectsEmptyRecognisedDocuments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.json")
	if err := os.WriteFile(path, []byte(`[]`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ImportExternalSets(path); err == nil {
		t.Fatal("empty external-code publication was accepted")
	}
}

func TestSchemaHelperFallbacks(t *testing.T) {
	if got := baseMessageCode("admi"); got != "admi" {
		t.Fatalf("short message base = %q", got)
	}
	if got := at([]string{"A"}, -1); got != "" {
		t.Fatalf("negative spreadsheet column = %q", got)
	}
	if got := columnIndex("12"); got != -1 {
		t.Fatalf("numeric cell reference = %d", got)
	}
}

func TestSchemaIndexSkipsDuplicateJobsAndRejectsBadInputs(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.xsd")
	idx, err := BuildIndex(&catalog.Index{Messages: []catalog.Message{
		{ID: "one", XSDPath: ""},
		{ID: "two", XSDPath: missing},
		{ID: "three", XSDPath: missing},
	}})
	if err != nil || idx.Total() != 0 {
		t.Fatalf("defensive schema build = %+v, %v", idx, err)
	}
	if err := extractInto(&SchemaIndex{Sets: map[string]map[string]*SchemaCode{}}, missing, "one"); err == nil {
		t.Fatal("extractInto accepted a missing schema")
	}
	empty := &SchemaIndex{Sets: map[string]map[string]*SchemaCode{}}
	addCode(empty, "", "A", "", "one")
	addCode(empty, "Codes", "", "", "one")
	if empty.Total() != 0 {
		t.Fatalf("empty code components were added: %+v", empty)
	}
}

func TestExternalStorageAndRecognisedEmptyJSONFailures(t *testing.T) {
	dir := t.TempDir()
	publication := filepath.Join(dir, "empty-record.json")
	if err := os.WriteFile(publication, []byte(`[{"set":"ExternalPurposeCode"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ImportExternalSets(publication); err == nil || !strings.Contains(err.Error(), "no external code sets") {
		t.Fatalf("empty recognised publication error = %v", err)
	}
	if _, err := importExternalJSON(filepath.Join(dir, "missing.json")); err == nil {
		t.Fatal("direct JSON import accepted a missing file")
	}

	root := t.TempDir()
	path := ExternalCodesPath(root)
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
	sets := &ExternalSets{Source: publication, Codes: []ExternalCode{{Set: "Codes", Code: "A"}}}
	if _, err := SaveExternalSets(root, sets); err == nil {
		t.Fatal("saving over a directory succeeded")
	}
}

func TestExternalJSONSkipsBlankEnumsAndStoreRejectsUnsafeRoots(t *testing.T) {
	publication := filepath.Join(t.TempDir(), "1Q2026_externalcodesets_v1.json")
	if err := os.WriteFile(publication, []byte(`{"definitions":{"ExternalPurpose1Code":{"type":"string","enum":["", "SALA"]}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	sets, err := ImportExternalSets(publication)
	if err != nil || sets.Total() != 1 || sets.Codes[0].Code != "SALA" {
		t.Fatalf("blank enum import = %+v, %v", sets, err)
	}

	blocked := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocked, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveExternalSets(blocked, sets); err == nil || !strings.Contains(err.Error(), "creating") {
		t.Fatalf("blocked external-code root error = %v", err)
	}

	if runtime.GOOS != "windows" {
		root := t.TempDir()
		store := ExternalCodesPath(root)
		if err := os.MkdirAll(filepath.Dir(store), 0o700); err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(root, "target")
		if err := os.WriteFile(target, []byte("keep"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, store); err != nil {
			t.Fatal(err)
		}
		if _, err := SaveExternalSets(root, sets); err == nil || !strings.Contains(err.Error(), "symlinked") {
			t.Fatalf("symlinked external-code store error = %v", err)
		}
	}
}

func writePartsZip(t *testing.T, parts map[string]string) *zip.Reader {
	t.Helper()
	path := filepath.Join(t.TempDir(), "parts.xlsx")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, body := range parts {
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
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = zr.Close() })
	return &zr.Reader
}

func TestSpreadsheetReadersRejectMalformedXMLAndKeepUnreferencedCells(t *testing.T) {
	sharedZip := writePartsZip(t, map[string]string{"xl/sharedStrings.xml": `<sst><si><t>broken`})
	if _, err := readSharedStrings(sharedZip); err == nil || !strings.Contains(err.Error(), "shared string") {
		t.Fatalf("malformed shared strings error = %v", err)
	}

	badSheet := writePartsZip(t, map[string]string{"xl/worksheets/sheet1.xml": `<worksheet><row>`})
	if _, err := readFirstSheet(badSheet, nil); err == nil || !strings.Contains(err.Error(), "worksheet") {
		t.Fatalf("malformed worksheet error = %v", err)
	}

	oddCell := writePartsZip(t, map[string]string{"xl/worksheets/sheet1.xml": `<worksheet><row><c r="12"><v>value</v></c></row></worksheet>`})
	rows, err := readFirstSheet(oddCell, nil)
	if err != nil || len(rows) != 1 || len(rows[0]) != 1 || rows[0][0] != "value" {
		t.Fatalf("unreferenced cell = %v, %v", rows, err)
	}
}

func writeSpreadsheetPartsFile(t *testing.T, parts map[string]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "parts.xlsx")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, body := range parts {
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
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSpreadsheetTextRejectsUnsafeAndMalformedInputs(t *testing.T) {
	if _, err := SpreadsheetText(filepath.Join(t.TempDir(), "missing.xlsx")); err == nil || !strings.Contains(err.Error(), "reading") {
		t.Fatalf("missing workbook error = %v", err)
	}
	legacy := filepath.Join(t.TempDir(), "legacy.xls")
	if err := os.WriteFile(legacy, []byte("legacy"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := SpreadsheetText(legacy); err == nil || !strings.Contains(err.Error(), "not an XLSX") {
		t.Fatalf("legacy workbook error = %v", err)
	}
	large := filepath.Join(t.TempDir(), "large.xlsx")
	if err := os.WriteFile(large, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(large, maxImportSize+1); err != nil {
		t.Fatal(err)
	}
	if _, err := SpreadsheetText(large); err == nil || !strings.Contains(err.Error(), "above") {
		t.Fatalf("oversized workbook error = %v", err)
	}
	broken := filepath.Join(t.TempDir(), "broken.xlsx")
	if err := os.WriteFile(broken, []byte("not a zip"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := SpreadsheetText(broken); err == nil || !strings.Contains(err.Error(), "opening") {
		t.Fatalf("invalid ZIP workbook error = %v", err)
	}

	noSheet := writeSpreadsheetPartsFile(t, map[string]string{"readme.txt": "none"})
	if _, err := SpreadsheetText(noSheet); err == nil || !strings.Contains(err.Error(), "no worksheet") {
		t.Fatalf("workbook without sheet error = %v", err)
	}
	badShared := writeSpreadsheetPartsFile(t, map[string]string{
		"xl/sharedStrings.xml":     `<sst><si><t>broken`,
		"xl/worksheets/sheet1.xml": `<worksheet/>`,
	})
	if _, err := SpreadsheetText(badShared); err == nil || !strings.Contains(err.Error(), "shared string") {
		t.Fatalf("malformed shared strings error = %v", err)
	}
	badSheet := writeSpreadsheetPartsFile(t, map[string]string{
		"xl/worksheets/sheet1.xml": `<worksheet><row>`,
	})
	if _, err := SpreadsheetText(badSheet); err == nil || !strings.Contains(err.Error(), "worksheet") {
		t.Fatalf("malformed worksheet error = %v", err)
	}
}
