// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package cmd

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebastienrousseau/askiso/internal/codes"
)

// The external code sets are the Registration Authority's own publication.
// AskISO imports the file the user downloaded and never carries one itself,
// which is the same arrangement the schemas are under.

// writeCodePublication builds a spreadsheet in the shape the publication takes.
func writeCodePublication(t *testing.T, path string, rows [][]string) {
	t.Helper()

	var strs []string
	index := map[string]int{}
	intern := func(s string) int {
		if i, ok := index[s]; ok {
			return i
		}
		index[s] = len(strs)
		strs = append(strs, s)
		return index[s]
	}

	all := append([][]string{{"Code Set", "Code Value", "Code Name", "Code Definition"}}, rows...)
	var sheet strings.Builder
	for r, row := range all {
		fmt.Fprintf(&sheet, `<row r="%d">`, r+1)
		for c, cell := range row {
			fmt.Fprintf(&sheet, `<c r="%c%d" t="s"><v>%d</v></c>`, 'A'+c, r+1, intern(cell))
		}
		sheet.WriteString("</row>")
	}

	var shared strings.Builder
	shared.WriteString(`<?xml version="1.0"?><sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">`)
	for _, s := range strs {
		fmt.Fprintf(&shared, "<si><t>%s</t></si>",
			strings.NewReplacer("&", "&amp;", "<", "&lt;").Replace(s))
	}
	shared.WriteString("</sst>")

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	zw := zip.NewWriter(f)
	for name, body := range map[string]string{
		"xl/sharedStrings.xml": shared.String(),
		"xl/worksheets/sheet1.xml": `<?xml version="1.0"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">` +
			`<sheetData>` + sheet.String() + `</sheetData></worksheet>`,
	} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}

var samplePublication = [][]string{
	{"ExternalPurposeCode", "GSCB", "Gas Bill", "Transaction is the payment of a gas bill."},
	{"ExternalPurposeCode", "WTER", "Water Bill", "Transaction is the payment of a water bill."},
	{"ExternalStatusReason1Code", "AM09", "Wrong Amount", "Amount received is not the amount agreed."},
}

// importPublication puts a publication into the fixture catalogue.
func importPublication(t *testing.T, root string) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "ExternalCodeSets.xlsx")
	writeCodePublication(t, path, samplePublication)

	if _, err := run(t, "code", "--import", path); err != nil {
		t.Fatalf("code --import: %v", err)
	}
	codes.ForgetExternalSets(root)
}

func TestCodeImport(t *testing.T) {
	root := withCatalogue(t)

	path := filepath.Join(t.TempDir(), "ExternalCodeSets.xlsx")
	writeCodePublication(t, path, samplePublication)

	out, err := run(t, "code", "--import", path)
	if err != nil {
		t.Fatalf("code --import: %v", err)
	}
	wantContains(t, out, "EXTERNAL CODE SETS", "3 code(s)", "2 set(s)", "stored")

	// It lands beside the catalogue, not in the repository: it is the user's
	// copy of a publication AskISO does not redistribute.
	stored := codes.ExternalCodesPath(root)
	if _, err := os.Stat(stored); err != nil {
		t.Fatalf("the publication was not stored at %s: %v", stored, err)
	}
}

func TestCodeFindsAnImportedCode(t *testing.T) {
	root := withCatalogue(t)
	importPublication(t, root)

	// GSCB is in no curated dictionary and no schema enumeration; it can only
	// have come from the imported publication.
	out, err := run(t, "code", "GSCB")
	if err != nil {
		t.Fatalf("code GSCB: %v", err)
	}
	wantContains(t, out, "GSCB", "Gas Bill", "ExternalPurposeCode",
		"Transaction is the payment of a gas bill.")
}

func TestCodeShowsAnImportedSet(t *testing.T) {
	root := withCatalogue(t)
	importPublication(t, root)

	out, err := run(t, "code", "--set", "ExternalPurposeCode")
	if err != nil {
		t.Fatalf("code --set: %v", err)
	}
	wantContains(t, out, "EXTERNAL CODE SET", "GSCB", "WTER")

	// Matched case-insensitively, because nobody types these from memory.
	if _, err := run(t, "code", "--set", "externalpurposecode"); err != nil {
		t.Errorf("a case-insensitive set lookup failed: %v", err)
	}
}

func TestCodeImportedSetAsJSON(t *testing.T) {
	root := withCatalogue(t)
	importPublication(t, root)

	out, err := run(t, "code", "--set", "ExternalStatusReason1Code", "--json")
	if err != nil {
		t.Fatalf("code --set --json: %v", err)
	}

	var members []codes.ExternalCode
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &members); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if len(members) != 1 || members[0].Code != "AM09" {
		t.Errorf("members = %+v", members)
	}
}

func TestCodeListsImportedSets(t *testing.T) {
	root := withCatalogue(t)
	importPublication(t, root)

	out, err := run(t, "code", "--sets")
	if err != nil {
		t.Fatalf("code --sets: %v", err)
	}
	// Both sources are listed, and told apart.
	wantContains(t, out,
		"EXTERNAL CODE SETS", "ExternalPurposeCode",
		"CODE SETS", "ChargeBearerType1Code")
}

func TestCodeCombinesEverySource(t *testing.T) {
	root := withCatalogue(t)
	importPublication(t, root)

	// AC04 is curated; --all adds the imported publication and the schemas
	// under headings that say where each came from.
	out, err := run(t, "code", "AC04", "--all")
	if err != nil {
		t.Fatalf("code --all: %v", err)
	}
	wantContains(t, out, "AC04", "Account Closed")
}

func TestCodeImportRejects(t *testing.T) {
	withCatalogue(t)

	if _, err := run(t, "code", "--import", filepath.Join(t.TempDir(), "absent.xlsx")); err == nil {
		t.Error("a missing file was accepted")
	}

	other := filepath.Join(t.TempDir(), "codes.csv")
	if err := os.WriteFile(other, []byte("a,b"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := run(t, "code", "--import", other)
	if err == nil {
		t.Fatal("a .csv was accepted")
	}
	if !strings.Contains(err.Error(), "iso20022.org") {
		t.Errorf("the error does not say where to download the publication: %v", err)
	}
}

func TestCodeImportWithoutACatalogue(t *testing.T) {
	isolate(t)

	path := filepath.Join(t.TempDir(), "ExternalCodeSets.xlsx")
	writeCodePublication(t, path, samplePublication)

	_, err := run(t, "code", "--import", path)
	if err == nil {
		t.Fatal("an import with no catalogue was accepted")
	}
	// It is stored beside the catalogue, so there has to be one.
	if !strings.Contains(err.Error(), "catalogue") {
		t.Errorf("the error does not explain why: %v", err)
	}
}

func TestCodeUnknownSaysWhatWouldWidenTheSearch(t *testing.T) {
	isolate(t)

	_, err := run(t, "code", "not-a-code-anywhere")
	if err == nil {
		t.Fatal("an unknown code was accepted")
	}
	// With neither source installed, both are offered.
	wantErr := err.Error()
	for _, want := range []string{"catalog add", "code --import"} {
		if !strings.Contains(wantErr, want) {
			t.Errorf("the error does not offer %q: %v", want, err)
		}
	}
}

func TestListCodeSetsWithOnlyAnImport(t *testing.T) {
	// A catalogue whose schemas enumerate nothing still lists what was
	// imported. The command reaches this only when the schema index cannot be
	// built, so it is exercised directly.
	path := filepath.Join(t.TempDir(), "codes.xlsx")
	writeCodePublication(t, path, samplePublication)

	sets, err := codes.ImportExternalSets(path)
	if err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		if err := listCodeSets(nil, sets); err != nil {
			t.Errorf("listCodeSets: %v", err)
		}
	})
	wantContains(t, out, "EXTERNAL CODE SETS", "ExternalPurposeCode",
		"No schemas are installed")

	// Neither source is an error the user can act on without being told how.
	err = listCodeSets(nil, nil)
	if err == nil {
		t.Fatal("listing with nothing installed was accepted")
	}
	for _, want := range []string{"catalog add", "code --import"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error does not offer %q: %v", want, err)
		}
	}
}

func TestFirstOfDefinitionPrefersTheDefinition(t *testing.T) {
	full := codes.ExternalCode{Name: "Salary", Definition: "Salary payment"}
	if got := firstOfDefinition(full); got != "Salary payment" {
		t.Errorf("got %q, want the definition", got)
	}
	// The RA does not always publish a definition; the name is the fallback.
	nameOnly := codes.ExternalCode{Name: "Salary"}
	if got := firstOfDefinition(nameOnly); got != "Salary" {
		t.Errorf("got %q, want the name as fallback", got)
	}
	if got := firstOfDefinition(codes.ExternalCode{}); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

// A limit of zero or less means "no limit" — capping to nothing would silently
// hide every code the user asked to see.
func TestCapExternal(t *testing.T) {
	list := []codes.ExternalCode{{Code: "A"}, {Code: "B"}, {Code: "C"}}

	if got := capExternal(list, 0); len(got) != 3 {
		t.Errorf("limit 0 gave %d, want all 3", len(got))
	}
	if got := capExternal(list, -1); len(got) != 3 {
		t.Errorf("a negative limit gave %d, want all 3", len(got))
	}
	if got := capExternal(list, 2); len(got) != 2 || got[1].Code != "B" {
		t.Errorf("limit 2 gave %v, want the first two", got)
	}
	if got := capExternal(list, 10); len(got) != 3 {
		t.Errorf("a limit above the length gave %d, want all 3", len(got))
	}
}
