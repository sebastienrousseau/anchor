// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package codes_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebastienrousseau/askiso/internal/catalog"
	"github.com/sebastienrousseau/askiso/internal/codes"
)

const schemaWithCodes = `<?xml version="1.0" encoding="UTF-8"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:t" xmlns="urn:t">
  <xs:element name="Document" type="Doc"/>
  <xs:complexType name="Doc"><xs:sequence/></xs:complexType>

  <xs:simpleType name="ChargeBearerType1Code">
    <xs:restriction base="xs:string">
      <xs:enumeration value="DEBT">
        <xs:annotation><xs:documentation>Borne by the debtor.</xs:documentation></xs:annotation>
      </xs:enumeration>
      <xs:enumeration value="CRED"/>
      <xs:enumeration value="SHAR"/>
      <xs:enumeration value="SLEV"/>
    </xs:restriction>
  </xs:simpleType>

  <xs:simpleType name="CreditDebitCode">
    <xs:restriction base="xs:string">
      <xs:enumeration value="CRDT"/>
      <xs:enumeration value="DBIT"/>
    </xs:restriction>
  </xs:simpleType>

  <!-- Not a code set: the suffix decides. -->
  <xs:simpleType name="Max35Text">
    <xs:restriction base="xs:string"><xs:maxLength value="35"/></xs:restriction>
  </xs:simpleType>
</xs:schema>`

func fixtureCatalogue(t *testing.T) *catalog.Index {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "Payments Clearing and Settlement", "Version 11.0", "Schemas")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"pacs.008.001.10", "pain.001.001.11"} {
		if err := os.WriteFile(filepath.Join(dir, id+".xsd"), []byte(schemaWithCodes), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	idx, err := catalog.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	return idx
}

func TestBuildIndexExtractsCodeSets(t *testing.T) {
	idx, err := codes.BuildIndex(fixtureCatalogue(t))
	if err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}

	if idx.Total() != 2 {
		t.Errorf("got %d code sets, want 2 (Max35Text is not one)", idx.Total())
	}
	if idx.Codes != 6 {
		t.Errorf("got %d codes, want 6", idx.Codes)
	}

	members := idx.Set("ChargeBearerType1Code")
	if len(members) != 4 {
		t.Fatalf("got %d members, want 4", len(members))
	}
	// Sorted by code.
	if members[0].Code != "CRED" {
		t.Errorf("members should be sorted, got %s first", members[0].Code)
	}

	var debt codes.SchemaCode
	for _, m := range members {
		if m.Code == "DEBT" {
			debt = m
		}
	}
	if !strings.Contains(debt.Description, "debtor") {
		t.Errorf("documentation should be captured: %q", debt.Description)
	}
	if len(debt.Messages) == 0 {
		t.Error("the messages using the set should be recorded")
	}
	for _, m := range debt.Messages {
		if strings.Count(m, ".") != 1 {
			t.Errorf("messages should be base codes, got %q", m)
		}
	}
}

func TestSchemaIndexLookupAndSearch(t *testing.T) {
	idx, err := codes.BuildIndex(fixtureCatalogue(t))
	if err != nil {
		t.Fatal(err)
	}

	if got := idx.Lookup("DEBT"); len(got) != 1 || got[0].Set != "ChargeBearerType1Code" {
		t.Errorf("Lookup(DEBT) = %+v", got)
	}
	if got := idx.Lookup("debt"); len(got) != 1 {
		t.Error("lookup should be case-insensitive")
	}
	if got := idx.Lookup("NOPE"); len(got) != 0 {
		t.Errorf("an unknown code should not resolve: %+v", got)
	}

	// Search by code, by set name, and by description.
	if got := idx.Search("CRDT"); len(got) == 0 {
		t.Error("search by code failed")
	}
	if got := idx.Search("ChargeBearer"); len(got) != 4 {
		t.Errorf("search by set name = %d, want 4", len(got))
	}
	if got := idx.Search("debtor"); len(got) == 0 {
		t.Error("search by description failed")
	}
	if got := idx.Search(""); got != nil {
		t.Error("an empty query should return nothing")
	}
	if got := idx.Search("zzz-nothing"); len(got) != 0 {
		t.Errorf("no match expected, got %d", len(got))
	}

	// An exact code sorts before a partial match.
	hits := idx.Search("CRED")
	if len(hits) == 0 || hits[0].Code != "CRED" {
		t.Errorf("exact matches should come first: %+v", hits)
	}
}

func TestSchemaIndexSetLookup(t *testing.T) {
	idx, err := codes.BuildIndex(fixtureCatalogue(t))
	if err != nil {
		t.Fatal(err)
	}

	if got := idx.Set("chargebearertype1code"); len(got) != 4 {
		t.Errorf("set lookup should be case-insensitive, got %d", len(got))
	}
	if got := idx.Set("NoSuchSet"); got != nil {
		t.Errorf("an unknown set should return nothing: %+v", got)
	}

	names := idx.SetNames()
	if len(names) != 2 {
		t.Fatalf("got %d names, want 2", len(names))
	}
	if names[0] > names[1] {
		t.Errorf("names should be sorted: %v", names)
	}
}

func TestNilSchemaIndexIsSafe(t *testing.T) {
	var idx *codes.SchemaIndex
	if idx.Total() != 0 || idx.Lookup("X") != nil || idx.Search("X") != nil ||
		idx.Set("X") != nil || idx.SetNames() != nil {
		t.Error("a nil index should answer emptily rather than panic")
	}
}

func TestBuildIndexRequiresACatalogue(t *testing.T) {
	if _, err := codes.BuildIndex(nil); err == nil {
		t.Error("BuildIndex(nil) should be an error")
	}
	if _, err := codes.LoadIndex(nil); err == nil {
		t.Error("LoadIndex(nil) should be an error")
	}
}

func TestLoadIndexCaches(t *testing.T) {
	cat := fixtureCatalogue(t)

	a, err := codes.LoadIndex(cat)
	if err != nil {
		t.Fatal(err)
	}
	b, err := codes.LoadIndex(cat)
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Error("the index should be cached per catalogue root")
	}
}

func TestUnreadableSchemaDoesNotAbortTheScan(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "Cat", "Version 1.0", "Schemas")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pacs.008.001.10.xsd"), []byte(schemaWithCodes), 0o644); err != nil {
		t.Fatal(err)
	}
	// A schema that is not XML at all.
	if err := os.WriteFile(filepath.Join(dir, "pacs.009.001.10.xsd"), []byte("\x00\x01 not xml"), 0o644); err != nil {
		t.Fatal(err)
	}

	cat, err := catalog.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	idx, err := codes.BuildIndex(cat)
	if err != nil {
		t.Fatalf("a bad schema should not abort the scan: %v", err)
	}
	if idx.Total() == 0 {
		t.Error("the readable schema should still have been indexed")
	}
}

// The curated dictionary is unaffected by the schema-backed index.
func TestCuratedDictionaryStillWorks(t *testing.T) {
	if got := codes.Lookup("AC04"); len(got) == 0 || got[0].Code != "AC04" {
		t.Errorf("curated lookup broke: %+v", got)
	}
	if len(codes.GetAllCodes()) == 0 {
		t.Error("the curated set should not be empty")
	}
}
