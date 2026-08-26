// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package codes

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// An imported publication lives beside the catalogue it belongs to, in a form
// AskISO can read quickly. Tab-separated text is enough: a few thousand rows,
// no escaping to get wrong, and a file anyone can open and check.
//
// It sits under the catalogue root rather than in the repository, because it is
// the user's copy of a publication AskISO does not redistribute.

// externalCodesFile is where an imported publication is stored, relative to the
// catalogue root.
const externalCodesFile = ".askiso/external-codes.tsv"

// ExternalCodesPath returns where an imported publication lives for a
// catalogue root.
func ExternalCodesPath(root string) string {
	return filepath.Join(root, filepath.FromSlash(externalCodesFile))
}

// SaveExternalSets writes an imported publication into a catalogue.
func SaveExternalSets(root string, sets *ExternalSets) (string, error) {
	if sets == nil || len(sets.Codes) == 0 {
		return "", fmt.Errorf("there is nothing to save")
	}

	path := ExternalCodesPath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("creating %s: %w", filepath.Dir(path), err)
	}

	f, err := os.Create(path)
	if err != nil {
		return "", fmt.Errorf("writing %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	w := bufio.NewWriter(f)
	_, _ = fmt.Fprintf(w, "# ISO 20022 external code sets, imported by AskISO from %s\n",
		filepath.Base(sets.Source))
	_, _ = fmt.Fprintln(w, "# set\tcode\tname\tdefinition")

	for _, c := range sets.Codes {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			clean(c.Set), clean(c.Code), clean(c.Name), clean(c.Definition))
	}
	if err := w.Flush(); err != nil {
		return "", fmt.Errorf("writing %s: %w", path, err)
	}
	return path, nil
}

// clean removes the characters the format cannot carry. A definition with a
// newline in it would otherwise become two rows.
func clean(s string) string {
	replacer := strings.NewReplacer("\t", " ", "\n", " ", "\r", " ")
	return strings.TrimSpace(replacer.Replace(s))
}

// LoadExternalSets reads a publication previously imported into a catalogue.
// A catalogue with none returns nil, which every caller treats as "not
// imported" rather than as a failure.
func LoadExternalSets(root string) (*ExternalSets, error) {
	if root == "" {
		return nil, nil
	}
	path := ExternalCodesPath(root)

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	out := &ExternalSets{Source: path}

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 2 {
			continue
		}
		code := ExternalCode{Set: parts[0], Code: parts[1]}
		if len(parts) > 2 {
			code.Name = parts[2]
		}
		if len(parts) > 3 {
			code.Definition = parts[3]
		}
		out.Codes = append(out.Codes, code)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	if len(out.Codes) == 0 {
		return nil, nil
	}
	out.index()
	return out, nil
}

var (
	externalOnce  sync.Mutex
	externalCache = map[string]*ExternalSets{}
)

// ExternalSetsFor loads, or returns a cached, publication for a catalogue root.
func ExternalSetsFor(root string) *ExternalSets {
	if root == "" {
		return nil
	}

	externalOnce.Lock()
	defer externalOnce.Unlock()

	key := filepath.Clean(root)
	if cached, ok := externalCache[key]; ok {
		return cached
	}
	loaded, err := LoadExternalSets(root)
	if err != nil {
		// A publication that will not read is the same as none, for a lookup.
		loaded = nil
	}
	externalCache[key] = loaded
	return loaded
}

// ForgetExternalSets drops the cached publication for a root, so a fresh import
// is visible without restarting.
func ForgetExternalSets(root string) {
	externalOnce.Lock()
	defer externalOnce.Unlock()
	delete(externalCache, filepath.Clean(root))
}
