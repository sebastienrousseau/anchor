// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package catalog

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// os.UserHomeDir reads USERPROFILE on Windows and ignores HOME. A test that
// isolates itself with HOME would therefore have been handed the real profile
// there, and anything installed into DefaultDir would have been written into
// the actual user account instead of the temporary directory — leaking a
// catalogue into every later run on the same machine.
func TestHomeDirPrefersAnExplicitHOME(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if got := HomeDir(); got != home {
		t.Errorf("HomeDir() = %q, want the HOME that was set, %q", got, home)
	}
}

func TestHomeDirFallsBackWhenHOMEIsUnset(t *testing.T) {
	t.Setenv("HOME", "")

	// The fallback is the platform's own answer; it just has to be non-empty
	// on any machine that can run the tests at all.
	if got := HomeDir(); got == "" {
		t.Skip("this platform reports no home directory")
	}
}

// Isolating HOME is only half the job on Windows, where the conventional
// location is LocalAppData rather than a dotted directory under the profile.
func TestDefaultDirsUsesLocalAppDataOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("LocalAppData is a Windows convention")
	}
	local := t.TempDir()
	t.Setenv("LocalAppData", local)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", "")

	want := filepath.Join(local, "anchor", "catalog")
	for _, d := range DefaultDirs() {
		if d == want {
			return
		}
	}
	t.Errorf("DefaultDirs() = %v, want it to include %s", DefaultDirs(), want)
}

// Whatever the platform, every candidate must sit under a directory the caller
// controls once HOME, LocalAppData and XDG_DATA_HOME are all pointed at one.
// That is what makes a test run unable to touch the real user account.
func TestDefaultDirsAreFullyIsolable(t *testing.T) {
	sandbox := t.TempDir()
	t.Setenv("HOME", sandbox)
	t.Setenv("LocalAppData", sandbox)
	t.Setenv("XDG_DATA_HOME", sandbox)

	dirs := DefaultDirs()
	if len(dirs) == 0 {
		t.Fatal("no conventional locations at all")
	}
	for _, d := range dirs {
		if !strings.HasPrefix(d, sandbox) {
			t.Errorf("%s escapes the sandbox %s", d, sandbox)
		}
	}
}
