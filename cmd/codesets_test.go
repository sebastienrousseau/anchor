// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/sebastienrousseau/anchor/internal/codes"
)

// Code sets come out of the schemas the user installed, so these tests need a
// catalogue. Without one the commands must explain how to get one rather than
// failing obscurely.

func TestCodeSetsListing(t *testing.T) {
	withCatalogue(t)

	out, err := run(t, "code", "--sets")
	if err != nil {
		t.Fatalf("code --sets: %v", err)
	}
	wantContains(t, out, "CODE SETS", "ChargeBearerType1Code")
}

func TestCodeSetMembers(t *testing.T) {
	withCatalogue(t)

	out, err := run(t, "code", "--set", "ChargeBearerType1Code")
	if err != nil {
		t.Fatalf("code --set: %v", err)
	}
	wantContains(t, out, "CODE SET", "DEBT", "BorneByDebtor", "SHAR")
}

func TestCodeSetMembersJSON(t *testing.T) {
	withCatalogue(t)

	// The set name is matched case-insensitively, because nobody types
	// ChargeBearerType1Code from memory.
	out, err := run(t, "code", "--set", "chargebearertype1code", "--json")
	if err != nil {
		t.Fatalf("code --set --json: %v", err)
	}

	var members []codes.SchemaCode
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &members); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if len(members) != 3 {
		t.Fatalf("got %d members, want 3: %+v", len(members), members)
	}
	if members[0].Code != "CRED" {
		t.Errorf("members are not sorted: %+v", members)
	}
}

func TestCodeSetUnknown(t *testing.T) {
	withCatalogue(t)

	_, err := run(t, "code", "--set", "NoSuchType1Code")
	if err == nil {
		t.Fatal("expected an error for an unknown set")
	}
	if !strings.Contains(err.Error(), "--sets") {
		t.Errorf("error = %q; it should point at the listing command", err)
	}
}

func TestCodeSetsWithoutCatalogue(t *testing.T) {
	isolate(t)

	for _, args := range [][]string{{"code", "--sets"}, {"code", "--set", "ChargeBearerType1Code"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			_, err := run(t, args...)
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), "iso20022.org") {
				t.Errorf("error = %q; it should say where to download schemas", err)
			}
		})
	}
}

func TestCodeSearchesInstalledSchemas(t *testing.T) {
	withCatalogue(t)

	// "BorneByDebtor" is the schema's own documentation text, so a hit can only
	// have come from the user's installed schemas.
	out, err := run(t, "code", "BorneByDebtor")
	if err != nil {
		t.Fatalf("code BorneByDebtor: %v", err)
	}
	wantContains(t, out, "DEBT", "ChargeBearerType1Code", "BorneByDebtor")
}

func TestCodeAllSearchesSchemasToo(t *testing.T) {
	withCatalogue(t)

	// DEBT is curated, so without --all the schemas are never consulted.
	plain, err := run(t, "code", "DEBT")
	if err != nil {
		t.Fatalf("code DEBT: %v", err)
	}
	if strings.Contains(plain, "ChargeBearerType1Code") {
		t.Error("the schema index was built even though the curated set answered")
	}

	// With --all both sources are reported, under a heading that separates them.
	out, err := run(t, "code", "DEBT", "--all")
	if err != nil {
		t.Fatalf("code --all: %v", err)
	}
	wantContains(t, out, "DEBT", "ChargeBearerType1Code", "installed schemas")
}

func TestCodeLimitCapsSchemaResults(t *testing.T) {
	withCatalogue(t)

	// A query matching the whole set, capped to one result, must say what it hid
	// rather than quietly truncating.
	out, err := run(t, "code", "ChargeBearer", "--limit", "1")
	if err != nil {
		t.Fatalf("code --limit: %v", err)
	}
	wantContains(t, out, "and 2 more", "--limit")
}

func TestCapCodes(t *testing.T) {
	list := []codes.SchemaCode{{Code: "A"}, {Code: "B"}, {Code: "C"}}
	if got := capCodes(list, 0); len(got) != 3 {
		t.Errorf("a limit of zero should not cap: %v", got)
	}
	if got := capCodes(list, 10); len(got) != 3 {
		t.Errorf("a limit above the length should not cap: %v", got)
	}
	if got := capCodes(list, 2); len(got) != 2 {
		t.Errorf("capCodes = %v, want two entries", got)
	}
}

func TestValidScenario(t *testing.T) {
	if !validScenario("reject-ac01") {
		t.Error("reject-ac01 should be a valid scenario")
	}
	if validScenario("not-a-scenario") {
		t.Error("an unknown scenario was accepted")
	}
}
