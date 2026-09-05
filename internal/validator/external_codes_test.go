// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package validator_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebastienrousseau/askiso/internal/codes"
	"github.com/sebastienrousseau/askiso/internal/validator"
	"github.com/sebastienrousseau/askiso/internal/xsd"
)

const externalCodeSchema = `<?xml version="1.0"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:test" xmlns="urn:test" elementFormDefault="qualified">
  <xs:simpleType name="ExternalPurpose1Code"><xs:restriction base="xs:string"><xs:minLength value="1"/><xs:maxLength value="4"/></xs:restriction></xs:simpleType>
  <xs:complexType name="DocumentType"><xs:sequence><xs:element name="Purpose" type="ExternalPurpose1Code"/></xs:sequence></xs:complexType>
  <xs:element name="Document" type="DocumentType"/>
</xs:schema>`

func externalFixture(t *testing.T) (*xsd.Schema, *codes.ExternalSets) {
	t.Helper()
	schema, err := xsd.Parse(strings.NewReader(externalCodeSchema))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "codes.json")
	if err := os.WriteFile(path, []byte(`{"ExternalPurpose1Code":[{"code":"SALA"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	sets, err := codes.ImportExternalSets(path)
	if err != nil {
		t.Fatal(err)
	}
	return schema, sets
}

func TestExternalCodesAreEnforcedByBothValidators(t *testing.T) {
	schema, sets := externalFixture(t)
	valid := `<Document xmlns="urn:test"><Purpose>SALA</Purpose></Document>`
	invalid := `<Document xmlns="urn:test"><Purpose>NOPE</Purpose></Document>`

	if result := validator.ValidateWithExternalSets([]byte(valid), schema, sets); !result.Valid {
		t.Fatalf("known external code rejected: %+v", result.Errors)
	}
	result := validator.ValidateWithExternalSets([]byte(invalid), schema, sets)
	if result.Valid || len(result.Errors) != 1 || result.Errors[0].Rule != "external code set" {
		t.Fatalf("unknown external code result: %+v", result)
	}
	if result := validator.Validate([]byte(invalid), schema); !result.Valid {
		t.Fatalf("base XSD validation should not invent an external-code enumeration: %+v", result.Errors)
	}

	streamed := validator.ValidateReaderWithExternalSets(strings.NewReader(invalid), schema, sets)
	if streamed.Valid || len(streamed.Errors) != 1 || streamed.Errors[0].Rule != "external code set" {
		t.Fatalf("streamed unknown external code result: %+v", streamed)
	}
}
