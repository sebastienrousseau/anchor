// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package codes

import (
	"encoding/json"
	"strings"
	"testing"
)

func FuzzExternalJSONSemanticShapes(f *testing.F) {
	f.Add("ExternalPurposeCode", "SALA", "Salary", "Payment of salary", uint8(0))
	f.Add("ExternalStatusReason1Code", "AC04", "Closed", "Closed account", uint8(1))
	f.Add("ExternalLocalInstrumentCode", "INST", "Instrument", "Local instrument", uint8(2))

	f.Fuzz(func(t *testing.T, rawSet, rawCode, rawName, rawDefinition string, shape uint8) {
		set := boundedExternalIdentifier(rawSet, "ExternalPurposeCode", 128)
		code := boundedExternalIdentifier(rawCode, "CODE", 32)
		name := boundedExternalText(rawName, "Name")
		definition := boundedExternalText(rawDefinition, "Definition")
		secondCode := code + "2"
		if len(secondCode) > 260 {
			secondCode = "SECOND"
		}

		var document any
		switch shape % 3 {
		case 0:
			document = []map[string]string{
				{"set": set, "code": code, "name": name, "definition": definition},
				{"codeSet": set, "codeValue": secondCode, "codeName": name},
			}
		case 1:
			document = map[string]any{"definitions": map[string]any{
				set: map[string]any{"type": "string", "enum": []string{code, secondCode}},
			}}
		case 2:
			document = map[string]any{set: []map[string]string{
				{"code": code, "name": name, "definition": definition},
				{"code": secondCode, "name": name},
			}}
		}
		encoded, err := json.Marshal(document)
		if err != nil {
			t.Fatal(err)
		}
		sets, err := parseExternalJSON("external.json", encoded)
		if err != nil {
			t.Fatalf("generated publication was rejected: %v\n%s", err, encoded)
		}
		sets.index()
		if sets.Total() != 2 {
			t.Fatalf("imported %d codes, want 2: %+v", sets.Total(), sets.Codes)
		}
		if got := sets.Lookup(strings.ToLower(code)); len(got) == 0 || got[0].Code != code {
			t.Fatalf("case-insensitive lookup %q = %+v", code, got)
		}
		members := sets.Set(strings.ToLower(set))
		if len(members) != 2 {
			t.Fatalf("case-insensitive set %q = %+v", set, members)
		}
		members[0].Code = "CALLER-MUTATION"
		if fresh := sets.Set(set); len(fresh) != 2 || fresh[0].Code == "CALLER-MUTATION" {
			t.Fatal("Set exposed mutable index storage")
		}
		matches := sets.Search(code)
		if len(matches) == 0 || matches[0].Code != code {
			t.Fatalf("exact search did not rank %q first: %+v", code, matches)
		}
	})
}

func boundedExternalIdentifier(value, fallback string, limit int) string {
	identifier := make([]byte, 0, len(value))
	for index := 0; index < len(value) && len(identifier) < limit; index++ {
		character := value[index]
		if character >= 'A' && character <= 'Z' || character >= 'a' && character <= 'z' ||
			character >= '0' && character <= '9' {
			identifier = append(identifier, character)
		}
	}
	if len(identifier) == 0 {
		return fallback
	}
	return string(identifier)
}

func boundedExternalText(value, fallback string) string {
	value = strings.ToValidUTF8(value, "X")
	runes := []rune(value)
	if len(runes) > 256 {
		value = string(runes[:256])
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}
