// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package cbprworkspace

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/sebastienrousseau/askiso/internal/rules"
)

func TestSearchLocalSourcesAcrossFormats(t *testing.T) {
	root := t.TempDir()
	writeWorkspaceFile(t, filepath.Join(root, "guide.pdf"), "%PDF fixture")
	writeWorkspaceFile(t, filepath.Join(root, "guide.json"),
		`{"description":"mandatory routing evidence","legalNotices":"excluded secret boilerplate"}`)
	writeWorkspaceFile(t, filepath.Join(root, "guide.xml"), `<Rule>mandatory routing evidence</Rule>`)
	writeWorkspaceFile(t, filepath.Join(root, "guide.xsd"), `<xs:annotation>mandatory routing evidence</xs:annotation>`)
	writeWorkspaceFile(t, filepath.Join(root, "guide.xlsx"), "fixture")
	writeWorkspaceFile(t, filepath.Join(root, "ignored.zip"), "not searched")

	originalPDF, originalSpreadsheet := searchPDFSource, extractSpreadsheetText
	searchPDFSource = func(path, query string, limit int) ([]rules.CBPRPackHit, error) {
		return []rules.CBPRPackHit{{Source: filepath.Base(path), Page: 4, Score: 80, Snippet: "mandatory routing evidence"}}, nil
	}
	extractSpreadsheetText = func(string) (string, error) { return "mandatory routing evidence", nil }
	t.Cleanup(func() {
		searchPDFSource, extractSpreadsheetText = originalPDF, originalSpreadsheet
	})

	result, err := SearchLocalSources(root, "mandatory routing", 20)
	if err != nil {
		t.Fatal(err)
	}
	if result.Sources != 5 || len(result.Hits) != 5 || len(result.Warnings) != 0 {
		t.Fatalf("multi-format result = %+v", result)
	}
	kinds := map[string]bool{}
	for _, hit := range result.Hits {
		kinds[hit.Kind] = true
		if filepath.IsAbs(hit.Source) {
			t.Fatalf("search result disclosed absolute path: %+v", hit)
		}
	}
	for _, kind := range []string{"pdf", "json", "xml", "xsd", "xlsx"} {
		if !kinds[kind] {
			t.Fatalf("missing %s hit: %+v", kind, result.Hits)
		}
	}

	secret, err := SearchLocalSources(filepath.Join(root, "guide.json"), "excluded secret", 5)
	if err != nil || len(secret.Hits) != 0 {
		t.Fatalf("legal notice was searchable: %+v, %v", secret, err)
	}
}

func TestSearchLocalSourcesOrdersTruncatesAndWarns(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a.pdf", "b.pdf", "guide.xlsx"} {
		writeWorkspaceFile(t, filepath.Join(root, name), "fixture")
	}
	originalPDF, originalSpreadsheet := searchPDFSource, extractSpreadsheetText
	searchPDFSource = func(path, _ string, _ int) ([]rules.CBPRPackHit, error) {
		if filepath.Base(path) == "b.pdf" {
			return nil, errors.New("unreadable PDF")
		}
		return []rules.CBPRPackHit{
			{Page: 2, Score: 50, Snippet: "later"},
			{Page: 1, Score: 50, Snippet: "first"},
		}, nil
	}
	extractSpreadsheetText = func(string) (string, error) {
		return "", errors.New("unreadable workbook")
	}
	t.Cleanup(func() {
		searchPDFSource, extractSpreadsheetText = originalPDF, originalSpreadsheet
	})

	result, err := SearchLocalSources(root, "evidence", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Hits) != 1 || result.Hits[0].Source != "a.pdf" || result.Hits[0].Page != 1 {
		t.Fatalf("ordered and limited hits = %+v", result.Hits)
	}
	if len(result.Warnings) != 2 || !strings.Contains(result.Warnings[0], "b.pdf") ||
		!strings.Contains(result.Warnings[1], "guide.xlsx") {
		t.Fatalf("sorted extraction warnings = %v", result.Warnings)
	}
}

func TestSearchLocalSourcesRejectsEmptyAndOversizedSources(t *testing.T) {
	if _, err := SearchLocalSources(t.TempDir(), "question", 5); err == nil || !strings.Contains(err.Error(), "contains no") {
		t.Fatalf("empty source error = %v", err)
	}

	large := filepath.Join(t.TempDir(), "large.xsd")
	writeWorkspaceFile(t, large, "x")
	if err := os.Truncate(large, maxSourceSize+1); err != nil {
		t.Fatal(err)
	}
	if _, err := SearchLocalSources(large, "question", 5); err == nil || !strings.Contains(err.Error(), "safety limit") {
		t.Fatalf("oversized source error = %v", err)
	}
	if _, err := searchableJSON(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("searchableJSON accepted a missing file")
	}

	if runtime.GOOS != "windows" {
		dir := t.TempDir()
		target := filepath.Join(t.TempDir(), "target.pdf")
		writeWorkspaceFile(t, target, "fixture")
		if err := os.Symlink(target, filepath.Join(dir, "linked.pdf")); err != nil {
			t.Fatal(err)
		}
		if files, err := searchableSources(dir); err != nil || len(files) != 0 {
			t.Fatalf("child symlink should be ignored: %+v, %v", files, err)
		}
	}

	for path, want := range map[string]string{
		"GUIDE.PDF":            "pdf",
		"guide.json":           "json",
		"guide.xml":            "xml",
		"guide.xsd":            "xsd",
		"guide.xlsx":           "xlsx",
		"guide.xls":            "xls",
		"guide.txt":            "",
		"guide.cbpr-pack.json": "",
	} {
		if got := searchableKind(path); got != want {
			t.Fatalf("searchableKind(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestSearchLocalSourcesWarningsAndValidation(t *testing.T) {
	root := t.TempDir()
	writeWorkspaceFile(t, filepath.Join(root, "broken.json"), `{`)
	writeWorkspaceFile(t, filepath.Join(root, "legacy.xls"), "binary")
	result, err := SearchLocalSources(root, "anything", 5)
	if err != nil || len(result.Warnings) != 2 || len(result.Hits) != 0 {
		t.Fatalf("warning result = %+v, %v", result, err)
	}
	if !strings.Contains(strings.Join(result.Warnings, "\n"), "legacy .xls") {
		t.Fatalf("legacy warning = %v", result.Warnings)
	}

	if _, err := SearchLocalSources(root, "", 5); err == nil {
		t.Fatal("empty query was accepted")
	}
	if _, err := SearchLocalSources(root, "query", 0); err == nil {
		t.Fatal("invalid limit was accepted")
	}
	if _, err := SearchLocalSources(filepath.Join(root, "missing"), "query", 5); err == nil {
		t.Fatal("missing source was accepted")
	}
	plain := filepath.Join(root, "notes.txt")
	writeWorkspaceFile(t, plain, "nothing")
	if _, err := SearchLocalSources(plain, "query", 5); err == nil || !strings.Contains(err.Error(), "contains no") {
		t.Fatalf("unsupported source error = %v", err)
	}
	pack := filepath.Join(root, "private.cbpr-pack.json")
	writeWorkspaceFile(t, pack, `{}`)
	if _, err := SearchLocalSources(pack, "query", 5); err == nil || !strings.Contains(err.Error(), "no document prose") {
		t.Fatalf("compiled pack error = %v", err)
	}
	if runtime.GOOS != "windows" {
		link := filepath.Join(t.TempDir(), "source-link")
		if err := os.Symlink(root, link); err != nil {
			t.Fatal(err)
		}
		if _, err := SearchLocalSources(link, "query", 5); err == nil || !strings.Contains(err.Error(), "symlink") {
			t.Fatalf("symlink search error = %v", err)
		}
	}
}

func TestSearchJSONNumbersTrailingDataAndDepth(t *testing.T) {
	root := t.TempDir()
	numbered := filepath.Join(root, "numbered.json")
	writeWorkspaceFile(t, numbered, `{"maximum":12345}`)
	result, err := SearchLocalSources(numbered, "12345", 5)
	if err != nil || len(result.Hits) != 1 {
		t.Fatalf("number search = %+v, %v", result, err)
	}

	trailing := filepath.Join(root, "trailing.json")
	writeWorkspaceFile(t, trailing, `{} {}`)
	result, err = SearchLocalSources(trailing, "anything", 5)
	if err != nil || len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "trailing") {
		t.Fatalf("trailing JSON result = %+v, %v", result, err)
	}

	var nested any = "bottom"
	for range 130 {
		nested = []any{nested}
	}
	if err := appendJSONText(&strings.Builder{}, nested, "", 0); err == nil || !strings.Contains(err.Error(), "nesting") {
		t.Fatalf("deep JSON error = %v", err)
	}
}

func TestAppendJSONTextHonoursBoundAcrossContainers(t *testing.T) {
	for name, value := range map[string]any{
		"scalar":     "too long",
		"object":     map[string]any{"key": "too long"},
		"object key": map[string]any{"long-key": "x"},
		"array":      []any{"too long"},
	} {
		t.Run(name, func(t *testing.T) {
			var out strings.Builder
			if err := appendJSONTextLimit(&out, value, "", 0, 4); err == nil || !strings.Contains(err.Error(), "exceeds 4 bytes") {
				t.Fatalf("bounded JSON extraction error = %v", err)
			}
		})
	}

	var ignored strings.Builder
	if err := appendJSONTextLimit(&ignored, map[string]any{
		"legalNotices": "private boilerplate",
		"boolean":      true,
		"nothing":      nil,
	}, "", 0, 100); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(ignored.String(), "private boilerplate") {
		t.Fatalf("legal notice leaked into searchable text: %q", ignored.String())
	}
}
