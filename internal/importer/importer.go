// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

// Package importer turns ISO 20022 downloads into a catalogue AskISO can read.
//
// The Registration Authority ships each message set as a zip, often with more
// zips nested inside. This package explodes those archives, works out what each
// file is, and files it under the layout the catalogue scanner expects:
//
//	<root>/<Category>/Version <N.0>/{Schemas,Sample Messages,Message Definition Reports,...}
//
// Archives are untrusted input, so extraction is bounded on path, entry count,
// total size, and compression ratio.
package importer

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/sebastienrousseau/askiso/internal/registry"
)

// Limits bound what a single import may extract.
type Limits struct {
	MaxFiles     int   // entries across all nested archives
	MaxTotalSize int64 // bytes written to disk
	MaxFileSize  int64 // bytes for any single entry
	MaxDepth     int   // nested archive depth
	MaxRatio     int64 // decompressed:compressed ratio for one entry
}

// DefaultLimits comfortably fits the largest published message set while still
// bounding a hostile archive.
func DefaultLimits() Limits {
	return Limits{
		MaxFiles:     200_000,
		MaxTotalSize: 12 << 30, // 12 GiB
		MaxFileSize:  2 << 30,  // 2 GiB
		MaxDepth:     8,
		MaxRatio:     2000,
	}
}

// Result reports what an import produced.
type Result struct {
	Source       string
	Categories   map[string]bool
	Schemas      int
	Samples      int
	Reports      int
	Guidelines   int
	Docs         int
	Skipped      int
	BytesWritten int64
}

// Classification is the catalogue subfolder a file belongs in.
type Classification string

const (
	ClassSchema    Classification = "Schemas"
	ClassSample    Classification = "Sample Messages"
	ClassReport    Classification = "Message Definition Reports"
	ClassGuideline Classification = "Message Usage Guidelines"
	ClassDoc       Classification = "Documentation"
	ClassSkip      Classification = ""
)

var (
	// pacs.008.001.10.xsd, and RA duplicates such as pacs.008.001.10_1.xsd.
	messageFileRe = regexp.MustCompile(`^([a-z]{4}\.\d{3}\.\d{3}\.\d{2})(_\d+)?\.(xsd|xml)$`)
	mugRe         = regexp.MustCompile(`(?i)(^|[_\-\s])mug([_\-\s]|\.|$)|usage.?guideline`)
	mdrRe         = regexp.MustCompile(`(?i)mdr|messagedefinitionreport|ISO20022_MDR`)
)

// Classify decides where a file belongs from its name alone.
func Classify(name string) Classification {
	base := filepath.Base(name)
	lower := strings.ToLower(base)
	ext := strings.ToLower(filepath.Ext(base))

	if m := messageFileRe.FindStringSubmatch(lower); m != nil {
		if m[3] == "xsd" {
			return ClassSchema
		}
		return ClassSample
	}

	switch ext {
	case ".xsd":
		return ClassSchema
	case ".pdf", ".docx", ".doc", ".xlsx", ".xls":
		if mugRe.MatchString(lower) {
			return ClassGuideline
		}
		if mdrRe.MatchString(lower) {
			return ClassReport
		}
		return ClassReport
	case ".png", ".jpg", ".jpeg", ".gif", ".wmf", ".emf", ".svg",
		".htm", ".html", ".txt", ".cnt", ".rtf":
		return ClassDoc
	case ".xml":
		return ClassSample
	}
	return ClassSkip
}

// Target is where a file should land in the catalogue.
type Target struct {
	Category string // "Payments Clearing and Settlement"
	Version  string // "Version 11.0"
	Class    Classification
	Name     string
}

// Path renders the catalogue-relative destination.
func (t Target) Path() string {
	return filepath.Join(t.Category, t.Version, string(t.Class), t.Name)
}

// ErrNoContent means an archive held nothing AskISO recognises.
var ErrNoContent = errors.New("archive contained no ISO 20022 content")

// Options configures an import.
type Options struct {
	Root     string // catalogue root to write into
	Category string // display name; derived from the archive when empty
	Version  string // "Version 11.0"; derived when empty
	Limits   Limits
	DryRun   bool
	OnFile   func(t Target) // optional progress callback
}

type counter struct {
	files int
	bytes int64
}

// ImportArchive explodes one zip into the catalogue.
func ImportArchive(archivePath string, opt Options) (*Result, error) {
	if opt.Limits == (Limits{}) {
		opt.Limits = DefaultLimits()
	}
	if opt.Root == "" {
		return nil, errors.New("importer: Root is required")
	}

	res := &Result{Source: archivePath, Categories: map[string]bool{}}
	cnt := &counter{}

	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", archivePath, err)
	}
	defer func() { _ = zr.Close() }()

	category := opt.Category
	if category == "" {
		category = categoryFromArchiveName(archivePath)
	}
	version := opt.Version
	if version == "" {
		version = versionFromArchiveName(archivePath)
	}

	if err := walkZip(&zr.Reader, category, version, opt, res, cnt, 0); err != nil {
		return res, err
	}
	if res.Schemas+res.Samples+res.Reports+res.Guidelines+res.Docs == 0 {
		return res, fmt.Errorf("%s: %w", filepath.Base(archivePath), ErrNoContent)
	}
	return res, nil
}

func walkZip(zr *zip.Reader, category, version string, opt Options, res *Result, cnt *counter, depth int) error {
	if depth > opt.Limits.MaxDepth {
		return fmt.Errorf("archive nesting deeper than %d levels", opt.Limits.MaxDepth)
	}

	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		cnt.files++
		if cnt.files > opt.Limits.MaxFiles {
			return fmt.Errorf("archive holds more than %d files", opt.Limits.MaxFiles)
		}

		name := filepath.Base(filepath.FromSlash(f.Name))
		if name == "" || name == "." || name == ".." || strings.HasPrefix(name, ".") {
			res.Skipped++
			continue
		}

		if strings.EqualFold(filepath.Ext(name), ".zip") {
			if err := walkNested(f, category, version, opt, res, cnt, depth); err != nil {
				return err
			}
			continue
		}

		class := Classify(name)
		if class == ClassSkip {
			res.Skipped++
			continue
		}

		if err := checkEntry(f, opt.Limits); err != nil {
			return fmt.Errorf("%s: %w", f.Name, err)
		}

		t := Target{Category: category, Version: version, Class: class, Name: name}
		if opt.OnFile != nil {
			opt.OnFile(t)
		}
		res.Categories[category] = true

		if !opt.DryRun {
			n, err := extract(f, opt.Root, filepath.Join(opt.Root, t.Path()), opt.Limits)
			if err != nil {
				return err
			}
			cnt.bytes += n
			res.BytesWritten += n
			if cnt.bytes > opt.Limits.MaxTotalSize {
				return fmt.Errorf("import exceeded %d bytes", opt.Limits.MaxTotalSize)
			}
		}

		switch class {
		case ClassSchema:
			res.Schemas++
		case ClassSample:
			res.Samples++
		case ClassReport:
			res.Reports++
		case ClassGuideline:
			res.Guidelines++
		case ClassDoc:
			res.Docs++
		}
	}
	return nil
}

// walkNested reads an inner archive into memory. The RA nests zips inside zips,
// and archive/zip needs a ReaderAt, so this is bounded by MaxFileSize.
func walkNested(f *zip.File, category, version string, opt Options, res *Result, cnt *counter, depth int) error {
	if err := checkEntry(f, opt.Limits); err != nil {
		return fmt.Errorf("%s: %w", f.Name, err)
	}

	rc, err := f.Open()
	if err != nil {
		return fmt.Errorf("opening nested archive %s: %w", f.Name, err)
	}
	defer func() { _ = rc.Close() }()

	buf, err := io.ReadAll(io.LimitReader(rc, opt.Limits.MaxFileSize+1))
	if err != nil {
		return fmt.Errorf("reading nested archive %s: %w", f.Name, err)
	}
	if int64(len(buf)) > opt.Limits.MaxFileSize {
		return fmt.Errorf("nested archive %s exceeds %d bytes", f.Name, opt.Limits.MaxFileSize)
	}

	inner, err := zip.NewReader(newByteReaderAt(buf), int64(len(buf)))
	if err != nil {
		// A .zip that will not parse is not fatal; record and move on.
		res.Skipped++
		return nil
	}

	nestedVersion := version
	if v := versionFromArchiveName(f.Name); v != "" {
		nestedVersion = v
	}
	return walkZip(inner, category, nestedVersion, opt, res, cnt, depth+1)
}

// withinRoot rejects any destination that resolves outside the catalogue root.
// Entry names are already reduced to their base name, so this is a second line
// of defence against path traversal rather than the only one.
func withinRoot(root, dest string) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolving catalogue root: %w", err)
	}
	absDest, err := filepath.Abs(dest)
	if err != nil {
		return fmt.Errorf("resolving destination: %w", err)
	}
	rel, err := filepath.Rel(absRoot, absDest)
	if err != nil {
		return fmt.Errorf("%s escapes the catalogue root", dest)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("refusing to write %s outside the catalogue root", dest)
	}
	return nil
}

func checkEntry(f *zip.File, lim Limits) error {
	if int64(f.UncompressedSize64) > lim.MaxFileSize {
		return fmt.Errorf("entry is %d bytes, over the %d limit", f.UncompressedSize64, lim.MaxFileSize)
	}
	if f.CompressedSize64 > 0 {
		ratio := int64(f.UncompressedSize64 / f.CompressedSize64)
		if ratio > lim.MaxRatio {
			return fmt.Errorf("compression ratio %d:1 exceeds %d:1", ratio, lim.MaxRatio)
		}
	}
	return nil
}

func extract(f *zip.File, root, dest string, lim Limits) (int64, error) {
	if err := withinRoot(root, dest); err != nil {
		return 0, err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return 0, fmt.Errorf("creating %s: %w", filepath.Dir(dest), err)
	}

	rc, err := f.Open()
	if err != nil {
		return 0, fmt.Errorf("opening %s: %w", f.Name, err)
	}
	defer func() { _ = rc.Close() }()

	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return 0, fmt.Errorf("creating %s: %w", dest, err)
	}
	// The copy below is the operation whose failure matters; a close error on a
	// file that already wrote its bytes adds nothing.
	defer func() { _ = out.Close() }()

	n, err := io.Copy(out, io.LimitReader(rc, lim.MaxFileSize))
	if err != nil {
		return n, fmt.Errorf("writing %s: %w", dest, err)
	}
	return n, nil
}

// ImportDir imports every zip in a directory, or a directory that is already
// laid out as a catalogue tree.
func ImportDir(dir string, opt Options) ([]*Result, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var archives []string
	for _, e := range entries {
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".zip") {
			archives = append(archives, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(archives)

	if len(archives) == 0 {
		return nil, fmt.Errorf("no .zip archives found in %s", dir)
	}

	var out []*Result
	for _, a := range archives {
		r, err := ImportArchive(a, opt)
		if err != nil && !errors.Is(err, ErrNoContent) {
			return out, err
		}
		if r != nil {
			out = append(out, r)
		}
	}
	return out, nil
}

// categoryFromArchiveName derives the catalogue category for an archive.
//
// Downloads get named all sorts of ways, so the derived name is matched against
// the message set names the RA actually publishes. That way
// "PaymentsClearingAndSettlement_v11.zip", "payments-clearing-and-settlement.zip"
// and "Payments Clearing and Settlement v11.zip" all land in one canonical
// directory instead of three near-identical ones.
func categoryFromArchiveName(path string) string {
	base := filepath.Base(path)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	base = versionTokenRe.ReplaceAllString(base, "")

	if canonical, ok := canonicalCategory(base); ok {
		return canonical
	}

	base = strings.NewReplacer("_", " ", "-", " ", ".", " ").Replace(base)
	base = splitCamel(base)
	base = strings.Join(strings.Fields(base), " ")
	if base == "" {
		return "Imported"
	}
	return base
}

// canonicalCategory matches a derived name against the published message sets,
// comparing on letters and digits only so punctuation and casing do not matter.
func canonicalCategory(name string) (string, bool) {
	key := normaliseKey(name)
	if key == "" {
		return "", false
	}
	reg, err := registry.Load()
	if err != nil {
		return "", false
	}

	best, bestLen := "", 0
	for _, s := range reg.Sets {
		k := normaliseKey(s.Name)
		if k == "" {
			continue
		}
		if k == key {
			return s.Name, true
		}
		// Prefer the longest set name that the archive name contains, so
		// "Corporate Actions" does not win over "Corporate Actions (Variant 002)".
		if strings.Contains(key, k) && len(k) > bestLen {
			best, bestLen = s.Name, len(k)
		}
	}
	return best, best != ""
}

func normaliseKey(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// splitCamel inserts spaces at lower-to-upper boundaries so a run-together
// archive name still reads as words.
func splitCamel(s string) string {
	var b strings.Builder
	runes := []rune(s)
	for i, r := range runes {
		if i > 0 && unicode.IsUpper(r) && !unicode.IsUpper(runes[i-1]) && runes[i-1] != ' ' {
			b.WriteRune(' ')
		}
		b.WriteRune(r)
	}
	return b.String()
}

var versionTokenRe = regexp.MustCompile(`(?i)[_\-\s]*v\d{1,2}(bah)?$`)

var versionInNameRe = regexp.MustCompile(`(?i)(?:^|[_\-\s])v(\d{1,2})(?:bah)?(?:[_\-\s]|\.|$)`)

func versionFromArchiveName(path string) string {
	base := filepath.Base(path)
	if m := versionInNameRe.FindStringSubmatch(base); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil {
			return fmt.Sprintf("Version %d.0", n)
		}
	}
	return ""
}

// byteReaderAt adapts a byte slice to io.ReaderAt for nested archives.
type byteReaderAt struct{ b []byte }

func newByteReaderAt(b []byte) *byteReaderAt { return &byteReaderAt{b: b} }

func (r *byteReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 || off >= int64(len(r.b)) {
		return 0, io.EOF
	}
	n := copy(p, r.b[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}
