// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package cmd

import (
	"path/filepath"
	"strings"
	"testing"
)

// Reading the head of an instance document decides which schema validates it,
// so what it returns is the difference between validating a message and
// refusing one. The streaming path is the interesting half: it asks for exactly
// headBytes and a message shorter than that cannot supply them, so io.ReadFull
// reports ErrUnexpectedEOF on every small document. Treating that as a failure
// would refuse to validate the ordinary case — a single credit transfer is a
// few kilobytes against a 64KB head — while the large documents streaming
// exists for would pass, which is the wrong way round for a bug to appear.

func TestReadInstanceHeadReadsAShortFileWhole(t *testing.T) {
	body := `<?xml version="1.0" encoding="UTF-8"?>` +
		`<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10"/>`
	path := writeTemp(t, "short.xml", body)

	for _, streaming := range []bool{false, true} {
		got, err := readInstanceHead(path, streaming)
		if err != nil {
			t.Fatalf("streaming=%v: a document shorter than the head was rejected: %v", streaming, err)
		}
		if string(got) != body {
			t.Errorf("streaming=%v: got %d bytes, want the whole %d-byte document",
				streaming, len(got), len(body))
		}
	}
}

// The namespace has to survive the cut, which is the whole reason a fixed
// number of bytes is read rather than the file.
func TestReadInstanceHeadStopsAtTheHeadOfALongFile(t *testing.T) {
	head := `<?xml version="1.0" encoding="UTF-8"?>` +
		`<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">`
	body := head + strings.Repeat("<CdtTrfTxInf/>", headBytes/8) + "</Document>"
	if len(body) <= headBytes {
		t.Fatalf("the fixture is not longer than the head: %d bytes", len(body))
	}
	path := writeTemp(t, "long.xml", body)

	got, err := readInstanceHead(path, true)
	if err != nil {
		t.Fatalf("reading the head of a long document: %v", err)
	}
	if len(got) != headBytes {
		t.Errorf("read %d bytes, want exactly headBytes (%d)", len(got), headBytes)
	}
	if !strings.Contains(string(got), "pacs.008.001.10") {
		t.Error("the namespace did not survive the cut, so no schema can be resolved")
	}

	// And the whole file when not streaming, so the two modes disagree only
	// about how much is read, never about what.
	whole, err := readInstanceHead(path, false)
	if err != nil {
		t.Fatalf("reading the whole document: %v", err)
	}
	if len(whole) != len(body) {
		t.Errorf("read %d bytes, want the whole %d-byte document", len(whole), len(body))
	}
}

// An empty file is not an error to read; it is a document with no namespace in
// it, and saying so is the schema resolver's job rather than this one's.
func TestReadInstanceHeadAcceptsAnEmptyFile(t *testing.T) {
	path := writeTemp(t, "empty.xml", "")
	for _, streaming := range []bool{false, true} {
		got, err := readInstanceHead(path, streaming)
		if err != nil {
			t.Errorf("streaming=%v: an empty file was reported as unreadable: %v", streaming, err)
		}
		if len(got) != 0 {
			t.Errorf("streaming=%v: got %d bytes from an empty file", streaming, len(got))
		}
	}
}

// The path belongs in the message. "no such file or directory" on its own
// leaves somebody who passed three files guessing which one was wrong.
func TestReadInstanceHeadNamesTheFileItCouldNotRead(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent.xml")
	for _, streaming := range []bool{false, true} {
		_, err := readInstanceHead(missing, streaming)
		if err == nil {
			t.Fatalf("streaming=%v: a missing file was read without error", streaming)
		}
		if !strings.Contains(err.Error(), "absent.xml") {
			t.Errorf("streaming=%v: the error does not name the file: %v", streaming, err)
		}
	}
}
