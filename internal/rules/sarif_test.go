// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package rules_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/sebastienrousseau/askiso/internal/rules"
)

type sarifDoc struct {
	Schema  string `json:"$schema"`
	Version string `json:"version"`
	Runs    []struct {
		Tool struct {
			Driver struct {
				Name  string `json:"name"`
				Rules []struct {
					ID              string                `json:"id"`
					Name            string                `json:"name"`
					HelpURI         string                `json:"helpUri"`
					FullDescription struct{ Text string } `json:"fullDescription"`
					Help            struct{ Text string } `json:"help"`
				} `json:"rules"`
			} `json:"driver"`
		} `json:"tool"`
		Results []struct {
			RuleID    string                `json:"ruleId"`
			Level     string                `json:"level"`
			Message   struct{ Text string } `json:"message"`
			Locations []struct {
				PhysicalLocation struct {
					ArtifactLocation struct{ URI string } `json:"artifactLocation"`
				} `json:"physicalLocation"`
				LogicalLocations []struct {
					FullyQualifiedName string `json:"fullyQualifiedName"`
				} `json:"logicalLocations"`
			} `json:"locations"`
		} `json:"results"`
	} `json:"runs"`
}

func renderSARIF(t *testing.T, results ...*rules.Result) sarifDoc {
	t.Helper()
	var buf strings.Builder
	if err := rules.WriteSARIF(&buf, results...); err != nil {
		t.Fatalf("WriteSARIF: %v", err)
	}
	var doc sarifDoc
	if err := json.Unmarshal([]byte(buf.String()), &doc); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}
	return doc
}

func TestSARIFShape(t *testing.T) {
	res := check(t, "pacs.008.001.10",
		`<Dbtr><PstlAdr><AdrLine>12 High Street</AdrLine></PstlAdr></Dbtr>`)

	doc := renderSARIF(t, res)

	if doc.Version != "2.1.0" {
		t.Errorf("version = %q, want 2.1.0", doc.Version)
	}
	if !strings.Contains(doc.Schema, "sarif-2.1.0") {
		t.Errorf("schema = %q", doc.Schema)
	}
	if len(doc.Runs) != 1 {
		t.Fatalf("got %d runs, want 1", len(doc.Runs))
	}
	run := doc.Runs[0]
	if run.Tool.Driver.Name != "askiso" {
		t.Errorf("driver name = %q", run.Tool.Driver.Name)
	}
	if len(run.Results) == 0 {
		t.Fatal("no results emitted")
	}

	// Every result must reference a described rule.
	described := map[string]bool{}
	for _, r := range run.Tool.Driver.Rules {
		described[r.ID] = true
		if r.Name == "" || r.FullDescription.Text == "" || r.Help.Text == "" {
			t.Errorf("rule %s is incompletely described: %+v", r.ID, r)
		}
		if !strings.HasPrefix(r.HelpURI, "http") {
			t.Errorf("rule %s should cite a reference, got %q", r.ID, r.HelpURI)
		}
	}
	for _, r := range run.Results {
		if !described[r.RuleID] {
			t.Errorf("result cites undescribed rule %s", r.RuleID)
		}
		if r.Message.Text == "" {
			t.Error("a result should carry a message")
		}
		if len(r.Locations) == 0 {
			t.Fatal("a result should carry a location")
		}
		loc := r.Locations[0]
		if loc.PhysicalLocation.ArtifactLocation.URI == "" {
			t.Error("a result should name its file")
		}
		// The XPath is the useful location in XML.
		if len(loc.LogicalLocations) == 0 || !strings.HasPrefix(loc.LogicalLocations[0].FullyQualifiedName, "/") {
			t.Errorf("a result should carry the element path: %+v", loc.LogicalLocations)
		}
	}
}

func TestSARIFLevels(t *testing.T) {
	// A hybrid address produces an informational finding, which SARIF calls a note.
	res := check(t, "pacs.008.001.10",
		`<Dbtr><PstlAdr><AdrLine>a</AdrLine><TwnNm>London</TwnNm><Ctry>GB</Ctry></PstlAdr></Dbtr>`)

	doc := renderSARIF(t, res)
	var sawNote bool
	for _, r := range doc.Runs[0].Results {
		if r.Level == "note" {
			sawNote = true
		}
		switch r.Level {
		case "error", "warning", "note":
		default:
			t.Errorf("unexpected SARIF level %q", r.Level)
		}
	}
	if !sawNote {
		t.Error("an informational finding should map to note")
	}
}

func TestSARIFMergesSeveralResults(t *testing.T) {
	a := check(t, "pacs.008.001.10", `<Dbtr><PstlAdr><AdrLine>a</AdrLine></PstlAdr></Dbtr>`)
	a.File = "a.xml"
	b := check(t, "pacs.008.001.10", `<Cdtr><PstlAdr><AdrLine>b</AdrLine></PstlAdr></Cdtr>`)
	b.File = "b.xml"

	doc := renderSARIF(t, a, b, nil)

	files := map[string]bool{}
	for _, r := range doc.Runs[0].Results {
		files[r.Locations[0].PhysicalLocation.ArtifactLocation.URI] = true
	}
	if len(files) != 2 {
		t.Errorf("both files should appear in one document: %v", files)
	}
	// A rule is described once, not once per file.
	seen := map[string]int{}
	for _, r := range doc.Runs[0].Tool.Driver.Rules {
		seen[r.ID]++
	}
	for id, n := range seen {
		if n != 1 {
			t.Errorf("rule %s described %d times", id, n)
		}
	}
}

func TestSARIFWithNoFindings(t *testing.T) {
	res := check(t, "pacs.008.001.10",
		`<Dbtr><PstlAdr><StrtNm>High St</StrtNm><TwnNm>London</TwnNm><Ctry>GB</Ctry></PstlAdr></Dbtr>`)

	doc := renderSARIF(t, res)
	if len(doc.Runs[0].Results) != 0 {
		t.Errorf("a clean message should produce no results: %+v", doc.Runs[0].Results)
	}
	// The document must still be well formed, with an empty rules array rather
	// than null, or some consumers reject it.
	var raw map[string]any
	var buf strings.Builder
	_ = rules.WriteSARIF(&buf, res)
	_ = json.Unmarshal([]byte(buf.String()), &raw)
	runs := raw["runs"].([]any)[0].(map[string]any)
	if runs["results"] == nil {
		t.Error("results should be an empty array, not null")
	}
	driver := runs["tool"].(map[string]any)["driver"].(map[string]any)
	if driver["rules"] == nil {
		t.Error("rules should be an empty array, not null")
	}
}
