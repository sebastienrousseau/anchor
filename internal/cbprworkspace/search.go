// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package cbprworkspace

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sebastienrousseau/askiso/internal/codes"
	"github.com/sebastienrousseau/askiso/internal/rules"
)

// SearchResult contains local extractive evidence and non-fatal source
// warnings. No extracted text is persisted or sent to a model provider.
type SearchResult struct {
	Sources  int                 `json:"sources"`
	Hits     []rules.CBPRPackHit `json:"hits"`
	Warnings []string            `json:"warnings,omitempty"`
}

type searchableSource struct {
	abs  string
	rel  string
	kind string
}

var (
	searchPDFSource        = rules.SearchCBPRPack
	extractSpreadsheetText = codes.SpreadsheetText
)

// SearchLocalSources searches PDF, JSON, XML/XSD, and XLSX sources entirely on
// the local machine. ZIP and compiled-pack files contain no directly searchable
// source prose and are ignored.
func SearchLocalSources(source, query string, limit int) (*SearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("local CBPR+ search needs a question")
	}
	if limit < 1 || limit > 20 {
		return nil, errors.New("local CBPR+ search limit must be between 1 and 20")
	}
	files, err := searchableSources(source)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, errors.New("CBPR+ search source contains no PDF, JSON, XML, XSD, or XLSX files")
	}
	result := &SearchResult{Sources: len(files)}
	for _, file := range files {
		var hits []rules.CBPRPackHit
		switch file.kind {
		case "pdf":
			hits, err = searchPDFSource(file.abs, query, 20)
			for i := range hits {
				hits[i].Source = file.rel
				hits[i].Kind = "pdf"
			}
		case "json":
			var text string
			text, err = searchableJSON(file.abs)
			hits = rankLocalSource(file.rel, "json", text, query)
		case "xml":
			var data []byte
			data, err = readBounded(file.abs)
			hits = rankLocalSource(file.rel, "xml", string(data), query)
		case "xsd":
			var data []byte
			data, err = readBounded(file.abs)
			hits = rankLocalSource(file.rel, "xsd", string(data), query)
		case "xlsx":
			var text string
			text, err = extractSpreadsheetText(file.abs)
			hits = rankLocalSource(file.rel, "xlsx", text, query)
		case "xls":
			err = fmt.Errorf("legacy .xls search is unsupported; save it as .xlsx")
		}
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("%s: %v", file.rel, err))
			err = nil
			continue
		}
		result.Hits = append(result.Hits, hits...)
	}
	sort.SliceStable(result.Hits, func(i, j int) bool {
		if result.Hits[i].Score != result.Hits[j].Score {
			return result.Hits[i].Score > result.Hits[j].Score
		}
		if result.Hits[i].Source != result.Hits[j].Source {
			return result.Hits[i].Source < result.Hits[j].Source
		}
		return result.Hits[i].Page < result.Hits[j].Page
	})
	if len(result.Hits) > limit {
		result.Hits = result.Hits[:limit]
	}
	sort.Strings(result.Warnings)
	return result, nil
}

func searchableSources(source string) ([]searchableSource, error) {
	root := filepath.Clean(source)
	info, err := os.Lstat(root)
	if err != nil {
		return nil, fmt.Errorf("reading CBPR+ search source: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("CBPR+ search source must not be a symlink: %s", root)
	}
	if !info.IsDir() {
		if strings.HasSuffix(strings.ToLower(root), ".cbpr-pack.json") {
			return nil, errors.New("compiled CBPR+ packs contain no document prose; search the private source directory")
		}
		kind := searchableKind(root)
		if kind == "" {
			return nil, nil
		}
		if info.Size() > maxSourceSize {
			return nil, fmt.Errorf("%s is %d bytes; per-file safety limit is %d", filepath.Base(root), info.Size(), maxSourceSize)
		}
		return []searchableSource{{abs: root, rel: filepath.Base(root), kind: kind}}, nil
	}
	var files []searchableSource
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		kind := searchableKind(path)
		if kind == "" {
			return nil
		}
		if len(files) >= maxFiles {
			return fmt.Errorf("search source contains more than %d supported files", maxFiles)
		}
		stat, err := entry.Info()
		if err != nil {
			return err
		}
		if stat.Size() > maxSourceSize {
			return fmt.Errorf("%s is %d bytes; per-file safety limit is %d", entry.Name(), stat.Size(), maxSourceSize)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, searchableSource{abs: path, rel: filepath.ToSlash(rel), kind: kind})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scanning CBPR+ search source: %w", err)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].rel < files[j].rel })
	return files, nil
}

func searchableKind(path string) string {
	lower := strings.ToLower(path)
	if strings.HasSuffix(lower, ".cbpr-pack.json") {
		return ""
	}
	switch filepath.Ext(lower) {
	case ".pdf":
		return "pdf"
	case ".json":
		return "json"
	case ".xml":
		return "xml"
	case ".xsd":
		return "xsd"
	case ".xlsx":
		return "xlsx"
	case ".xls":
		return "xls"
	default:
		return ""
	}
}

func searchableJSON(path string) (string, error) {
	data, err := readBounded(path)
	if err != nil {
		return "", err
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return "", fmt.Errorf("decoding JSON: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return "", errors.New("decoding JSON: trailing value")
	}
	var text strings.Builder
	if err := appendJSONText(&text, value, "", 0); err != nil {
		return "", err
	}
	return text.String(), nil
}

func appendJSONText(out *strings.Builder, value any, key string, depth int) error {
	return appendJSONTextLimit(out, value, key, depth, maxSourceSize)
}

func appendJSONTextLimit(out *strings.Builder, value any, key string, depth, limit int) error {
	if depth > 128 {
		return errors.New("JSON nesting exceeds 128 levels")
	}
	if strings.EqualFold(key, "legalNotices") {
		return nil
	}
	appendText := func(value string) error {
		if out.Len()+len(value)+1 > limit {
			return fmt.Errorf("searchable JSON text exceeds %d bytes", limit)
		}
		out.WriteString(value)
		out.WriteByte('\n')
		return nil
	}
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for childKey := range typed {
			keys = append(keys, childKey)
		}
		sort.Strings(keys)
		for _, childKey := range keys {
			if err := appendText(childKey); err != nil {
				return err
			}
			if err := appendJSONTextLimit(out, typed[childKey], childKey, depth+1, limit); err != nil {
				return err
			}
		}
	case []any:
		for _, item := range typed {
			if err := appendJSONTextLimit(out, item, key, depth+1, limit); err != nil {
				return err
			}
		}
	case string:
		return appendText(typed)
	case json.Number:
		return appendText(typed.String())
	}
	return nil
}

func rankLocalSource(source, kind, text, query string) []rules.CBPRPackHit {
	hits := rules.RankCBPRText(source, text, query)
	for i := range hits {
		hits[i].Kind = kind
		hits[i].Page = 0
	}
	return hits
}
