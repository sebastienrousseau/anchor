// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package rules

import (
	"encoding/json"
	"io"
	"sort"
)

// SARIF is the Static Analysis Results Interchange Format, which GitHub code
// scanning and most CI dashboards ingest directly. Emitting it means a failing
// address rule shows up as an annotation on the pull request rather than as a
// line buried in a build log.
//
// Reference: https://docs.oasis-open.org/sarif/sarif/v2.1.0/sarif-v2.1.0.html
const (
	sarifVersion = "2.1.0"
	sarifSchema  = "https://json.schemastore.org/sarif-2.1.0.json"
)

type sarifLog struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool    sarifTool     `json:"tool"`
	Results []sarifResult `json:"results"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name           string      `json:"name"`
	InformationURI string      `json:"informationUri"`
	Rules          []sarifRule `json:"rules"`
}

type sarifRule struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	ShortDescription sarifText         `json:"shortDescription"`
	FullDescription  sarifText         `json:"fullDescription"`
	HelpURI          string            `json:"helpUri,omitempty"`
	Help             sarifText         `json:"help"`
	Properties       map[string]string `json:"properties,omitempty"`
}

type sarifText struct {
	Text string `json:"text"`
}

type sarifResult struct {
	RuleID    string          `json:"ruleId"`
	Level     string          `json:"level"`
	Message   sarifText       `json:"message"`
	Locations []sarifLocation `json:"locations"`
}

type sarifLocation struct {
	PhysicalLocation sarifPhysical `json:"physicalLocation"`
	LogicalLocations []sarifLogic  `json:"logicalLocations,omitempty"`
}

type sarifPhysical struct {
	ArtifactLocation sarifArtifact `json:"artifactLocation"`
}

type sarifArtifact struct {
	URI string `json:"uri"`
}

type sarifLogic struct {
	FullyQualifiedName string `json:"fullyQualifiedName"`
	Kind               string `json:"kind"`
}

// SARIFDiagnostic is a source-independent finding. It lets batch reporting put
// linter, schema, and scheme-profile failures in the same SARIF document.
type SARIFDiagnostic struct {
	RuleID      string
	Name        string
	Description string
	HelpURI     string
	Help        string
	Severity    Severity
	Message     string
	File        string
	Path        string
	Properties  map[string]string
}

// sarifLevel maps a severity onto the three levels SARIF defines.
func sarifLevel(s Severity) string {
	switch s {
	case SeverityError:
		return "error"
	case SeverityWarning:
		return "warning"
	default:
		return "note"
	}
}

// WriteSARIF renders results as a SARIF log.
//
// Several results may be passed, so a batch run over a directory produces one
// document rather than one per file.
func WriteSARIF(w io.Writer, results ...*Result) error {
	byID := map[string]Rule{}
	for _, p := range profiles {
		for _, r := range p.Rules {
			byID[r.ID] = r
		}
	}

	var diagnostics []SARIFDiagnostic
	for _, res := range results {
		if res == nil {
			continue
		}
		for _, f := range res.Findings {
			rule := byID[f.RuleID]
			help := rule.Remediation
			if help == "" {
				help = f.Remediation
			}
			diagnostics = append(diagnostics, SARIFDiagnostic{
				RuleID: f.RuleID, Name: f.Rule, Description: rule.Description,
				HelpURI: f.Reference, Help: help, Severity: f.Severity,
				Message: f.Message, File: res.File, Path: f.Path,
				Properties: map[string]string{"profile": res.Profile},
			})
		}
	}
	return WriteDiagnosticsSARIF(w, diagnostics)
}

// WriteDiagnosticsSARIF renders diagnostics from every AskISO checking engine.
func WriteDiagnosticsSARIF(w io.Writer, diagnostics []SARIFDiagnostic) error {
	driver := sarifDriver{
		Name:           "askiso",
		InformationURI: "https://github.com/sebastienrousseau/askiso",
	}
	run := sarifRun{Results: []sarifResult{}}
	described := map[string]bool{}
	for _, d := range diagnostics {
		if !described[d.RuleID] {
			described[d.RuleID] = true
			driver.Rules = append(driver.Rules, sarifRule{
				ID: d.RuleID, Name: d.Name,
				ShortDescription: sarifText{Text: d.Name},
				FullDescription:  sarifText{Text: d.Description},
				HelpURI:          d.HelpURI, Help: sarifText{Text: d.Help},
				Properties: d.Properties,
			})
		}
		location := sarifLocation{
			PhysicalLocation: sarifPhysical{ArtifactLocation: sarifArtifact{URI: d.File}},
		}
		if d.Path != "" {
			location.LogicalLocations = []sarifLogic{{FullyQualifiedName: d.Path, Kind: "member"}}
		}
		run.Results = append(run.Results, sarifResult{
			RuleID: d.RuleID, Level: sarifLevel(d.Severity), Message: sarifText{Text: d.Message},
			Locations: []sarifLocation{location},
		})
	}

	sort.Slice(driver.Rules, func(i, j int) bool { return driver.Rules[i].ID < driver.Rules[j].ID })
	if driver.Rules == nil {
		driver.Rules = []sarifRule{}
	}
	run.Tool = sarifTool{Driver: driver}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(sarifLog{
		Schema:  sarifSchema,
		Version: sarifVersion,
		Runs:    []sarifRun{run},
	})
}
