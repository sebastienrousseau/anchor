// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package importer

import (
	"path/filepath"
	"strings"
	"testing"
)

// The Registration Authority nests zips inside zips, so the importer opens
// archives it finds inside archives. That is exactly the shape a zip bomb
// takes, and the input is a file the user downloaded from a website — so the
// limits are a security control, not a tidiness measure, and they deserve a
// test that actually trips them.

func TestNestedArchiveOverTheSizeLimitIsRefused(t *testing.T) {
	dir := t.TempDir()

	inner := buildZip(t, entry{"Payments/Version 1.0/Schemas/a.xsd", []byte("<xs:schema/>")})
	outer := writeZip(t, dir, "outer.zip", entry{"nested.zip", inner})

	limits := DefaultLimits()
	limits.MaxFileSize = 10 // smaller than the nested archive

	_, err := ImportArchive(outer, Options{Root: filepath.Join(dir, "out"), Limits: limits})
	if err == nil {
		t.Fatal("an oversized nested archive was accepted")
	}
	if !strings.Contains(err.Error(), "nested.zip") {
		t.Errorf("the error does not name the offending entry: %v", err)
	}
}

func TestNestedArchiveWithinTheLimitIsRead(t *testing.T) {
	dir := t.TempDir()

	inner := buildZip(t, entry{"Payments/Version 1.0/Schemas/a.xsd", []byte("<xs:schema/>")})
	outer := writeZip(t, dir, "outer.zip", entry{"nested.zip", inner})

	res, err := ImportArchive(outer, Options{Root: filepath.Join(dir, "out")})
	if err != nil {
		t.Fatalf("a nested archive within the limits was refused: %v", err)
	}
	if res.Schemas == 0 {
		t.Errorf("the schema inside the nested archive was not imported: %+v", res)
	}
}

// An entry over the limit inside a plain archive takes a different path from
// the nested case, and must be refused just as clearly.
func TestOversizedEntryIsRefused(t *testing.T) {
	dir := t.TempDir()
	big := strings.Repeat("x", 500)
	z := writeZip(t, dir, "big.zip",
		entry{"Payments/Version 1.0/Schemas/big.xsd", []byte(big)})

	limits := DefaultLimits()
	limits.MaxFileSize = 100

	if _, err := ImportArchive(z, Options{Root: filepath.Join(dir, "out"), Limits: limits}); err == nil {
		t.Error("an oversized entry was accepted")
	}
}
