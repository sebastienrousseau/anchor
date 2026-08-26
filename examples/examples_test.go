// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

// Package examples_test exercises the runnable examples.
//
// Examples that no longer compile are worse than no examples: somebody copies
// one into their own project and finds out it was abandoned. Building each one
// on every test run is what stops that happening quietly.
package examples_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// binaryName returns a name the host will actually execute. Windows refuses to
// run a file without an executable extension, so a build to a bare name
// produced a binary that could not be started -- which looked like every
// example failing rather than like a test-harness bug.
func binaryName(dir string) string {
	if runtime.GOOS == "windows" {
		return dir + ".exe"
	}
	return dir
}

// eachExample lists the example directories, so a new one is covered by every
// test below the moment it is added rather than when somebody remembers.
func eachExample(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading the examples directory: %v", err)
	}
	var dirs []string
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, e.Name())
		}
	}
	if len(dirs) == 0 {
		t.Fatal("no examples found, so this test proves nothing")
	}
	return dirs
}

func TestEveryExampleBuilds(t *testing.T) {
	for _, dir := range eachExample(t) {
		t.Run(dir, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), binaryName(dir))
			cmd := exec.Command("go", "build", "-o", out, "./"+dir)
			if b, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%s does not build: %v\n%s", dir, err, b)
			}
		})
	}
}

// Every example must explain itself when run with no arguments, and must not
// panic. A program that exits silently on bad input teaches the reader nothing.
func TestEveryExampleExplainsItself(t *testing.T) {
	for _, dir := range eachExample(t) {
		t.Run(dir, func(t *testing.T) {
			bin := filepath.Join(t.TempDir(), binaryName(dir))
			if b, err := exec.Command("go", "build", "-o", bin, "./"+dir).CombinedOutput(); err != nil {
				t.Fatalf("building: %v\n%s", err, b)
			}

			cmd := exec.Command(bin)
			out, err := cmd.CombinedOutput()
			text := string(out)
			// An ExitError is expected -- these exit non-zero on bad input.
			// Anything else means the binary could not be started at all, and
			// reporting that as "no usage printed" hides the real cause.
			if err != nil {
				if _, isExit := err.(*exec.ExitError); !isExit {
					t.Fatalf("%s could not be run: %v\n%s", dir, err, text)
				}
			}

			if strings.Contains(text, "panic:") {
				t.Fatalf("%s panicked on no arguments:\n%s", dir, text)
			}
			// ci-sarif and batch-audit default to the working directory rather
			// than requiring one, so they are allowed to do work instead of
			// printing usage.
			if dir == "ci-sarif" || dir == "batch-audit" {
				return
			}
			if !strings.Contains(strings.ToLower(text), "usage:") {
				t.Errorf("%s does not print usage when run with no arguments:\n%s", dir, text)
			}
		})
	}
}

// The README is the index somebody reads first, so an example missing from it
// is an example nobody finds.
func TestReadmeListsEveryExample(t *testing.T) {
	readme, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatalf("reading README.md: %v", err)
	}
	for _, dir := range eachExample(t) {
		if !strings.Contains(string(readme), dir) {
			t.Errorf("examples/README.md does not mention %q", dir)
		}
	}
}
