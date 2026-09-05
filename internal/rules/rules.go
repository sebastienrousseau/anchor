// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

// Package rules checks ISO 20022 messages against scheme-level requirements.
//
// Schema validity says a message is well formed; it says nothing about whether a
// clearing system will accept it. These are the rules that sit on top: what
// CBPR+ requires, what a rail expects, what a regulator mandates from a given
// date. They are expressed as data so that adding one touches no engine code.
package rules

import (
	"fmt"
	"sort"
	"strings"

	"github.com/sebastienrousseau/askiso/internal/converter"
)

// Severity classifies a finding.
type Severity string

const (
	SeverityError   Severity = "ERROR"
	SeverityWarning Severity = "WARNING"
	SeverityInfo    Severity = "INFO"
)

// Finding is one rule violation.
type Finding struct {
	RuleID      string   `json:"rule_id"`
	Rule        string   `json:"rule"`
	Severity    Severity `json:"severity"`
	Path        string   `json:"path"`
	Message     string   `json:"message"`
	Found       string   `json:"found,omitempty"`
	Expected    string   `json:"expected,omitempty"`
	Remediation string   `json:"remediation,omitempty"`
	Reference   string   `json:"reference,omitempty"`
}

// Rule is one check.
type Rule struct {
	ID          string
	Name        string
	Severity    Severity
	Description string
	Remediation string
	Reference   string

	// Exempt reports message identifiers the rule does not apply to. A nil
	// function means the rule applies everywhere.
	Exempt func(msgID string) bool

	// Check inspects the document and returns any violations. Paths are
	// populated by the engine, so a rule only has to describe the fault.
	Check func(ctx *Context) []Finding
}

// Context is what a rule sees.
type Context struct {
	Root    *converter.Node
	MsgID   string
	Profile string
}

// Walk returns every element in document order, paired with its absolute path.
// Unlike FindAll it is useful for rules that apply to a class of values rather
// than to one element name (for example every amount carrying a Ccy attribute).
func Walk(root *converter.Node) []Located {
	if root == nil {
		return nil
	}
	var out []Located
	var walk func(n *converter.Node, path string)
	walk = func(n *converter.Node, path string) {
		out = append(out, Located{Node: n, Path: path})
		counts := map[string]int{}
		for _, c := range n.Children {
			counts[c.Name]++
		}
		seen := map[string]int{}
		for _, c := range n.Children {
			childPath := path + "/" + c.Name
			if counts[c.Name] > 1 {
				seen[c.Name]++
				childPath = fmt.Sprintf("%s/%s[%d]", path, c.Name, seen[c.Name])
			}
			walk(c, childPath)
		}
	}
	walk(root, "/"+root.Name)
	return out
}

// Profile is a named set of rules.
type Profile struct {
	Name        string
	Description string
	Rules       []Rule
	Pack        *CBPRPackInfo
}

// Result is the outcome of running a profile.
type Result struct {
	Profile     string        `json:"profile"`
	File        string        `json:"file"`
	Description string        `json:"description,omitempty"`
	Pack        *CBPRPackInfo `json:"cbpr_pack,omitempty"`
	Findings    []Finding     `json:"findings"`
	Errors      int           `json:"error_count"`
	Warnings    int           `json:"warning_count"`
	Checked     int           `json:"rules_checked"`
	Skipped     int           `json:"rules_skipped"`
}

// Valid reports whether the message passed with no errors.
func (r *Result) Valid() bool { return r.Errors == 0 }

// Run applies a profile to a parsed message.
func Run(p Profile, root *converter.Node, msgID, filename string) *Result {
	res := &Result{Profile: p.Name, File: filename, Description: p.Description, Pack: p.Pack, Findings: []Finding{}}
	ctx := &Context{Root: root, MsgID: msgID, Profile: p.Name}

	for _, rule := range p.Rules {
		if rule.Exempt != nil && rule.Exempt(msgID) {
			res.Skipped++
			continue
		}
		res.Checked++

		for _, f := range rule.Check(ctx) {
			if f.RuleID == "" {
				f.RuleID = rule.ID
			}
			if f.Rule == "" {
				f.Rule = rule.Name
			}
			if f.Severity == "" {
				f.Severity = rule.Severity
			}
			if f.Remediation == "" {
				f.Remediation = rule.Remediation
			}
			if f.Reference == "" {
				f.Reference = rule.Reference
			}
			res.Findings = append(res.Findings, f)

			switch f.Severity {
			case SeverityError:
				res.Errors++
			case SeverityWarning:
				res.Warnings++
			}
		}
	}

	sort.SliceStable(res.Findings, func(i, j int) bool {
		return res.Findings[i].Path < res.Findings[j].Path
	})
	return res
}

// ---------------------------------------------------------------------------
// Document walking
// ---------------------------------------------------------------------------

// Located pairs a node with its path in the document.
type Located struct {
	Node *converter.Node
	Path string
}

// FindAll returns every element with the given name, with its path.
func FindAll(root *converter.Node, name string) []Located {
	var out []Located
	var walk func(n *converter.Node, path string)
	walk = func(n *converter.Node, path string) {
		if n.Name == name {
			out = append(out, Located{Node: n, Path: path})
		}
		counts := map[string]int{}
		for _, c := range n.Children {
			counts[c.Name]++
		}
		seen := map[string]int{}
		for _, c := range n.Children {
			childPath := path + "/" + c.Name
			if counts[c.Name] > 1 {
				seen[c.Name]++
				childPath = fmt.Sprintf("%s/%s[%d]", path, c.Name, seen[c.Name])
			}
			walk(c, childPath)
		}
	}
	if root != nil {
		walk(root, "/"+root.Name)
	}
	return out
}

// Child returns the first child with the given name.
func Child(n *converter.Node, name string) (*converter.Node, bool) {
	for _, c := range n.Children {
		if c.Name == name {
			return c, true
		}
	}
	return nil, false
}

// ChildText returns the trimmed text of the first child with the given name.
func ChildText(n *converter.Node, name string) string {
	if c, ok := Child(n, name); ok {
		return strings.TrimSpace(c.Text)
	}
	return ""
}

// Children returns every child with the given name.
func Children(n *converter.Node, name string) []*converter.Node {
	var out []*converter.Node
	for _, c := range n.Children {
		if c.Name == name {
			out = append(out, c)
		}
	}
	return out
}
