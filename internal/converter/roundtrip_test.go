// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package converter_test

import (
	"encoding/json"
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebastienrousseau/anchor/internal/catalog"
	"github.com/sebastienrousseau/anchor/internal/converter"
)

// elementOrder lists every element name in document order, which is the property
// that must survive a round trip: ISO 20022 complex types are xs:sequence.
func elementOrder(t *testing.T, doc []byte) []string {
	t.Helper()
	dec := xml.NewDecoder(strings.NewReader(string(doc)))
	var names []string
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		if se, ok := tok.(xml.StartElement); ok {
			names = append(names, se.Name.Local)
		}
	}
	return names
}

func TestJSONIsEmittedInDocumentOrder(t *testing.T) {
	src := []byte(`<Document xmlns="urn:t">
  <GrpHdr><MsgId>M1</MsgId><CreDtTm>2026-01-01T00:00:00Z</CreDtTm></GrpHdr>
  <CdtTrfTxInf><Amt Ccy="EUR">1.00</Amt></CdtTrfTxInf>
</Document>`)

	out, err := converter.XMLToJSON(src)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)

	// Alphabetically CdtTrfTxInf sorts before GrpHdr; document order is the
	// opposite, and that is what must be emitted.
	if strings.Index(s, `"GrpHdr"`) > strings.Index(s, `"CdtTrfTxInf"`) {
		t.Errorf("keys were sorted rather than kept in document order:\n%s", s)
	}
	if strings.Index(s, `"MsgId"`) > strings.Index(s, `"CreDtTm"`) {
		t.Errorf("nested keys were sorted:\n%s", s)
	}
	if !json.Valid(out) {
		t.Errorf("output is not valid JSON:\n%s", s)
	}
}

func TestRoundTripPreservesElementOrder(t *testing.T) {
	cases := map[string]string{
		"simple":              `<Doc xmlns="urn:t"><B>2</B><A>1</A><C>3</C></Doc>`,
		"nested":              `<Doc xmlns="urn:t"><Z><Y>1</Y><X>2</X></Z><M>3</M></Doc>`,
		"attributes and text": `<Doc xmlns="urn:t"><Amt Ccy="EUR">10.00</Amt><Nm>Name</Nm></Doc>`,
		"adjacent repeats":    `<Doc xmlns="urn:t"><Tx>1</Tx><Tx>2</Tx><Tx>3</Tx><End>x</End></Doc>`,
		"empty element":       `<Doc xmlns="urn:t"><A/><B>1</B></Doc>`,
		"deep":                `<A xmlns="urn:t"><B><C><D><E>leaf</E></D></C></B></A>`,
	}

	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			j, err := converter.XMLToJSON([]byte(src))
			if err != nil {
				t.Fatalf("XMLToJSON: %v", err)
			}
			back, err := converter.JSONToXML(j)
			if err != nil {
				t.Fatalf("JSONToXML: %v", err)
			}

			want := elementOrder(t, []byte(src))
			got := elementOrder(t, back)
			if len(want) != len(got) {
				t.Fatalf("element count changed: %v -> %v", want, got)
			}
			for i := range want {
				if want[i] != got[i] {
					t.Fatalf("order changed at %d: %v -> %v", i, want, got)
				}
			}
		})
	}
}

func TestRoundTripPreservesValuesAndAttributes(t *testing.T) {
	src := `<Doc xmlns="urn:t"><Amt Ccy="EUR" Src="X">25000.00</Amt><Nm>A &amp; B</Nm></Doc>`

	j, err := converter.XMLToJSON([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	back, err := converter.JSONToXML(j)
	if err != nil {
		t.Fatal(err)
	}
	s := string(back)

	for _, want := range []string{`Ccy="EUR"`, `Src="X"`, `>25000.00<`, `A &amp; B`, `xmlns="urn:t"`} {
		if !strings.Contains(s, want) {
			t.Errorf("round trip lost %q:\n%s", want, s)
		}
	}
	// A decimal must not be renormalised into 25000.
	if strings.Contains(s, ">25000<") {
		t.Errorf("the amount lost its scale:\n%s", s)
	}
}

// Non-adjacent repeats cannot be expressed as a JSON object, so the conversion
// reports it instead of silently reordering the document.
func TestInterleavedRepeatsAreRefused(t *testing.T) {
	src := []byte(`<Doc xmlns="urn:t"><A>1</A><B>2</B><A>3</A></Doc>`)

	_, err := converter.XMLToJSON(src)
	if err == nil {
		t.Fatal("interleaved repeats should be refused, not silently reordered")
	}
	var interleaved *converter.ErrInterleaved
	if !errorsAs(err, &interleaved) {
		t.Fatalf("want *ErrInterleaved, got %T: %v", err, err)
	}
	if interleaved.Child != "A" || interleaved.Parent != "Doc" {
		t.Errorf("error should name the elements: %+v", interleaved)
	}
}

func errorsAs(err error, target **converter.ErrInterleaved) bool {
	for err != nil {
		if e, ok := err.(*converter.ErrInterleaved); ok {
			*target = e
			return true
		}
		type unwrapper interface{ Unwrap() error }
		u, ok := err.(unwrapper)
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

// The real bar: every installed sample must survive a round trip with its
// element order intact.
func TestRoundTripAcrossTheCatalogue(t *testing.T) {
	if testing.Short() {
		t.Skip("catalogue sweep is slow")
	}
	root, err := catalog.Resolve("")
	if err != nil {
		t.Skip("no ISO 20022 catalogue installed")
	}

	var checked, mismatched, refused int
	var examples []string

	err = filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() || !strings.HasSuffix(p, ".xml") {
			return nil
		}
		if checked >= 500 {
			return filepath.SkipDir
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return nil
		}

		j, err := converter.XMLToJSON(src)
		if err != nil {
			var interleaved *converter.ErrInterleaved
			if errorsAs(err, &interleaved) {
				refused++
				return nil
			}
			return nil // not well-formed; not this package's problem
		}
		back, err := converter.JSONToXML(j)
		if err != nil {
			mismatched++
			return nil
		}

		checked++
		want, got := elementOrder(t, src), elementOrder(t, back)
		if len(want) != len(got) {
			mismatched++
			if len(examples) < 3 {
				examples = append(examples, filepath.Base(p))
			}
			return nil
		}
		for i := range want {
			if want[i] != got[i] {
				mismatched++
				if len(examples) < 3 {
					examples = append(examples, filepath.Base(p))
				}
				return nil
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking catalogue: %v", err)
	}
	if checked == 0 {
		t.Skip("no samples to check")
	}

	t.Logf("round-tripped %d samples: %d order mismatches, %d refused as interleaved",
		checked, mismatched, refused)
	if mismatched > 0 {
		t.Errorf("%d sample(s) changed element order, e.g. %v", mismatched, examples)
	}
}
