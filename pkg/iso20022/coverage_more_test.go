// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package iso20022

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebastienrousseau/askiso/internal/catalog"
)

func TestValidateStreamRejectsAnUnreadableSchema(t *testing.T) {
	if _, err := ValidateStream(strings.NewReader(`<Document/>`), filepath.Join(t.TempDir(), "missing.xsd")); err == nil {
		t.Fatal("ValidateStream accepted a missing schema")
	}
}

func TestMessageIDFallsBackToANestedISONamespace(t *testing.T) {
	data := []byte(`<Envelope><Payload xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10"/></Envelope>`)
	id, err := MessageIDFromInstance(data)
	if err != nil || id != "pacs.008.001.10" {
		t.Fatalf("MessageIDFromInstance = %q, %v", id, err)
	}
	if _, err := MessageIDFromInstance([]byte(`<Envelope><broken></Envelope>`)); err == nil {
		t.Fatal("malformed XML did not produce an error")
	}
}

func TestDiffNamesMalformedInstalledSchemas(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.xsd")
	bad := filepath.Join(dir, "bad.xsd")
	if err := os.WriteFile(good, []byte(`<?xml version="1.0"?><xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" xmlns="urn:test" targetNamespace="urn:test" elementFormDefault="qualified"><xs:element name="Document" type="xs:string"/></xs:schema>`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bad, []byte(`<not-a-schema>`), 0o600); err != nil {
		t.Fatal(err)
	}
	message := func(id, path string) catalog.Message {
		return catalog.Message{ID: id, BaseCode: id[:8], XSDPath: path}
	}
	fromID, toID := "pacs.008.001.09", "pacs.008.001.10"
	from := message(fromID, bad)
	to := message(toID, good)
	cat := &Catalogue{idx: &catalog.Index{MessageMap: map[string]catalog.Message{fromID: from, toID: to}}}
	if _, err := cat.Diff(fromID, toID); err == nil || !strings.Contains(err.Error(), fromID) {
		t.Fatalf("bad source schema error = %v", err)
	}

	from.XSDPath, to.XSDPath = good, bad
	cat.idx.MessageMap[fromID], cat.idx.MessageMap[toID] = from, to
	if _, err := cat.Diff(fromID, toID); err == nil || !strings.Contains(err.Error(), toID) {
		t.Fatalf("bad target schema error = %v", err)
	}
}

func TestImportExternalCodesReportsStorageFailure(t *testing.T) {
	dir := t.TempDir()
	publication := filepath.Join(dir, "codes.json")
	if err := os.WriteFile(publication, []byte(`[{"set":"ExternalPurposeCode","code":"SALA","name":"Salary"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	blockedRoot := filepath.Join(dir, "not-a-directory")
	if err := os.WriteFile(blockedRoot, []byte("blocked"), 0o600); err != nil {
		t.Fatal(err)
	}
	cat := &Catalogue{idx: &catalog.Index{RootDir: blockedRoot}}
	if _, err := cat.ImportExternalCodes(publication); err == nil {
		t.Fatal("external code import hid its storage failure")
	}
}

func TestCheckCBPRPackRejectsAnInvalidSource(t *testing.T) {
	if _, err := CheckCBPRPack([]byte(`<Document/>`), filepath.Join(t.TempDir(), "missing.pdf"), "message.xml"); err == nil {
		t.Fatal("CBPR pack check accepted a missing source")
	}
}
