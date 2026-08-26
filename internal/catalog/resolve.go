// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package catalog

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// EnvCatalog is the environment variable that overrides catalogue discovery.
const EnvCatalog = "ASKISO_CATALOG"

// ErrNotFound is returned when no ISO 20022 catalogue could be located.
var ErrNotFound = errors.New("no ISO 20022 catalogue found")

// NotFoundError reports which locations were searched, so the message can tell
// the user exactly what to do next rather than silently yielding zero results.
type NotFoundError struct {
	Searched []string
}

func (e *NotFoundError) Error() string {
	var sb strings.Builder
	sb.WriteString("no ISO 20022 catalogue found\n\nSearched:\n")
	for _, p := range e.Searched {
		sb.WriteString("  " + p + "\n")
	}
	sb.WriteString("\nAskISO does not redistribute ISO 20022 specifications. Download them\n")
	sb.WriteString("from https://www.iso20022.org/ and import them:\n\n")
	sb.WriteString("  askiso catalog add ~/Downloads/<message-set>.zip\n\n")
	sb.WriteString("Or point AskISO at an existing copy:\n\n")
	sb.WriteString("  export " + EnvCatalog + "=/path/to/catalogue\n")
	return sb.String()
}

func (e *NotFoundError) Is(target error) bool { return target == ErrNotFound }

// DefaultDir is where AskISO installs a catalogue when the user does not say.
//
// If any conventional location already holds one, that wins, so importing a new
// message set extends the existing catalogue instead of quietly starting a
// second one somewhere else.
func DefaultDir() string {
	dirs := DefaultDirs()
	for _, d := range dirs {
		if IsCatalog(d) {
			return d
		}
	}
	if len(dirs) > 0 {
		return dirs[0]
	}
	return ""
}

// HomeDir returns the user's home directory, preferring an explicitly set HOME.
//
// os.UserHomeDir reads USERPROFILE on Windows and ignores HOME entirely, so a
// caller that sets HOME — a test isolating itself, or a shell environment that
// defines it — would silently get the real profile instead. AskISO writes a
// catalogue into this directory, so that difference is the gap between an
// isolated run and one that scribbles on the actual user account.
func HomeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return h
}

// DefaultDirs lists every conventional location, in preference order.
//
// An explicitly set XDG_DATA_HOME comes first, because that is the user saying
// where data belongs. The platform convention follows. macOS keeps both:
// XDG_DATA_HOME is commonly set there by dotfile managers, and honouring it
// alone would hide a catalogue sitting in Application Support. Windows uses
// LocalAppData, which is where a catalogue of this size belongs — it is machine
// state, not roaming profile data.
func DefaultDirs() []string {
	var dirs []string
	seen := map[string]bool{}
	add := func(p string) {
		if p == "" || seen[p] {
			return
		}
		seen[p] = true
		dirs = append(dirs, p)
	}

	home := HomeDir()

	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		add(filepath.Join(xdg, "askiso", "catalog"))
	}
	if runtime.GOOS == "darwin" && home != "" {
		add(filepath.Join(home, "Library", "Application Support", "askiso", "catalog"))
	}
	if runtime.GOOS == "windows" {
		if local := os.Getenv("LocalAppData"); local != "" {
			add(filepath.Join(local, "askiso", "catalog"))
		}
	}
	if home != "" {
		add(filepath.Join(home, ".local", "share", "askiso", "catalog"))
	}
	return dirs
}

// searchPaths lists candidate roots in precedence order.
func searchPaths(override string) []string {
	var paths []string
	add := func(p string) {
		if p == "" {
			return
		}
		for _, existing := range paths {
			if existing == p {
				return
			}
		}
		paths = append(paths, p)
	}

	add(override)
	add(os.Getenv(EnvCatalog))
	for _, d := range DefaultDirs() {
		add(d)
	}

	// Walk up from the working directory so running inside a catalogue tree
	// (or a repository that vendors one) keeps working.
	if wd, err := os.Getwd(); err == nil {
		dir := wd
		for i := 0; i < 6; i++ {
			add(dir)
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}

	return paths
}

// Resolve locates a usable catalogue root. override wins when non-empty.
func Resolve(override string) (string, error) {
	candidates := searchPaths(override)
	for _, dir := range candidates {
		if IsCatalog(dir) {
			return dir, nil
		}
	}
	return "", &NotFoundError{Searched: candidates}
}

// IsCatalog reports whether dir looks like an ISO 20022 catalogue: at least one
// category directory holding a "Version N" directory with a Schemas folder.
func IsCatalog(dir string) bool {
	if dir == "" {
		return false
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, cat := range entries {
		if !cat.IsDir() || strings.HasPrefix(cat.Name(), ".") {
			continue
		}
		if skipDir(cat.Name()) {
			continue
		}
		versions, err := os.ReadDir(filepath.Join(dir, cat.Name()))
		if err != nil {
			continue
		}
		for _, v := range versions {
			if !v.IsDir() || !strings.HasPrefix(v.Name(), "Version") {
				continue
			}
			schemas := filepath.Join(dir, cat.Name(), v.Name(), "Schemas")
			if st, err := os.Stat(schemas); err == nil && st.IsDir() {
				return true
			}
		}
	}
	return false
}

// EvictedError reports a file that iCloud Drive has evicted to a placeholder.
// Reading it would either block on a network fetch or yield a stub, so AskISO
// fails loudly instead of mis-parsing.
type EvictedError struct {
	Path string
}

func (e *EvictedError) Error() string {
	return fmt.Sprintf("%s is evicted from local storage by iCloud Drive\n\n"+
		"Download it, or move the catalogue out of iCloud:\n\n"+
		"  brctl download %q\n", e.Path, filepath.Dir(e.Path))
}

// CheckEvicted returns an *EvictedError if path is an iCloud placeholder rather
// than real content. iCloud replaces "name.xsd" with a hidden ".name.xsd.icloud".
func CheckEvicted(path string) error {
	if st, err := os.Stat(path); err == nil {
		if st.Size() > 0 {
			return nil
		}
	}
	stub := filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+".icloud")
	if _, err := os.Stat(stub); err == nil {
		return &EvictedError{Path: path}
	}
	return nil
}
