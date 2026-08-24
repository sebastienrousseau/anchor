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
	driver := sarifDriver{
		Name:           "anchor",
		InformationURI: "https://github.com/sebastienrousseau/anchor",
	}

	// Describe every rule that produced a result, once.
	described := map[string]bool{}
	byID := map[string]Rule{}
	for _, p := range profiles {
		for _, r := range p.Rules {
			byID[r.ID] = r
		}
	}

	run := sarifRun{Results: []sarifResult{}}

	for _, res := range results {
		if res == nil {
			continue
		}
		for _, f := range res.Findings {
			if !described[f.RuleID] {
				described[f.RuleID] = true
				rule := byID[f.RuleID]
				help := rule.Remediation
				if help == "" {
					help = f.Remediation
				}
				driver.Rules = append(driver.Rules, sarifRule{
					ID:               f.RuleID,
					Name:             f.Rule,
					ShortDescription: sarifText{Text: f.Rule},
					FullDescription:  sarifText{Text: rule.Description},
					HelpURI:          f.Reference,
					Help:             sarifText{Text: help},
					Properties:       map[string]string{"profile": res.Profile},
				})
			}

			run.Results = append(run.Results, sarifResult{
				RuleID:  f.RuleID,
				Level:   sarifLevel(f.Severity),
				Message: sarifText{Text: f.Message},
				Locations: []sarifLocation{{
					PhysicalLocation: sarifPhysical{
						ArtifactLocation: sarifArtifact{URI: res.File},
					},
					// The XPath is the useful location in an XML document;
					// SARIF carries it as a logical location.
					LogicalLocations: []sarifLogic{{
						FullyQualifiedName: f.Path,
						Kind:               "member",
					}},
				}},
			})
		}
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
