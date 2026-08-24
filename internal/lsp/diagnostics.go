// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package lsp

import (
	"fmt"
	"sort"

	"github.com/sebastienrousseau/anchor/internal/rules"
	"github.com/sebastienrousseau/anchor/internal/validator"
	"github.com/sebastienrousseau/anchor/pkg/iso20022"
)

// Severity is an LSP diagnostic severity.
type Severity int

const (
	SeverityError   Severity = 1
	SeverityWarning Severity = 2
	SeverityInfo    Severity = 3
	SeverityHint    Severity = 4
)

// Diagnostic is one problem reported against a document.
type Diagnostic struct {
	Range    Range    `json:"range"`
	Severity Severity `json:"severity"`
	// Code is the rule identifier, so an editor can group or suppress by rule.
	Code   string `json:"code,omitempty"`
	Source string `json:"source"`
	// Message is what the user reads. It says what is wrong and, where the rule
	// has one, how to fix it.
	Message string `json:"message"`
}

// publish rechecks a document and sends the result to the client.
func (s *Server) publish(uri string) {
	doc, ok := s.document(uri)
	if !ok {
		return
	}

	diagnostics := s.diagnose(doc)
	_ = s.conn.notify("textDocument/publishDiagnostics", map[string]any{
		"uri":         uri,
		"diagnostics": diagnostics,
	})
}

// Diagnose runs every check that applies to a document. It is exported so the
// checks can be tested without a client on the other end of a pipe.
func (s *Server) Diagnose(doc *Document) []Diagnostic {
	return s.diagnose(doc)
}

func (s *Server) diagnose(doc *Document) []Diagnostic {
	out := []Diagnostic{}

	// A document that does not parse cannot be checked any further, and saying
	// so is more useful than a list of consequential errors.
	if !doc.Wellformed {
		return append(out, Diagnostic{
			Range:    doc.errorRange(),
			Severity: SeverityError,
			Code:     "xml",
			Source:   "anchor",
			Message:  "this is not well-formed XML: " + doc.ParseError,
		})
	}
	if len(doc.Elements) == 0 {
		return out
	}

	out = append(out, s.lintDiagnostics(doc)...)
	out = append(out, s.schemaDiagnostics(doc)...)
	out = append(out, s.profileDiagnostics(doc)...)

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Range.Start.Line != out[j].Range.Start.Line {
			return out[i].Range.Start.Line < out[j].Range.Start.Line
		}
		return out[i].Range.Start.Character < out[j].Range.Start.Character
	})
	return out
}

// errorRange points at where parsing stopped. Without a position the whole
// first line is marked, which is at least somewhere the user can see.
func (d *Document) errorRange() Range {
	if len(d.Elements) == 0 {
		return d.LineRange(1, 0)
	}
	last := d.Elements[len(d.Elements)-1]
	return Range{Start: d.PositionAt(last.TagStart), End: d.PositionAt(last.TagEnd)}
}

// lintDiagnostics runs the business-rule linter. Its findings name a field and
// a value rather than a position, so each is located in the document.
func (s *Server) lintDiagnostics(doc *Document) []Diagnostic {
	res, err := iso20022.Lint([]byte(doc.Text), "")
	if err != nil {
		return nil
	}

	out := make([]Diagnostic, 0, len(res.Issues))
	for _, issue := range res.Issues {
		severity := SeverityWarning
		if issue.Severity == iso20022.SeverityError {
			severity = SeverityError
		}

		rng := doc.RangeOf(doc.Elements[0])
		if el, ok := doc.FindValue(trimNamespace(issue.Field), issue.Value); ok {
			rng = doc.RangeOf(el)
		}

		out = append(out, Diagnostic{
			Range:    rng,
			Severity: severity,
			Code:     issue.Rule,
			Source:   "anchor",
			Message:  issue.Message,
		})
	}
	return out
}

// schemaDiagnostics validates against the schema the document's namespace
// names. Without an installed catalogue this reports nothing rather than
// pretending the document is valid.
func (s *Server) schemaDiagnostics(doc *Document) []Diagnostic {
	schema, _, ok := s.schemaFor(doc)
	if !ok {
		return nil
	}

	res := validator.Validate([]byte(doc.Text), schema)
	out := make([]Diagnostic, 0, len(res.Errors))
	for _, e := range res.Errors {
		out = append(out, Diagnostic{
			Range:    doc.rangeForSchemaError(e),
			Severity: SeverityError,
			Code:     e.Rule,
			Source:   "anchor/schema",
			Message:  schemaMessage(e),
		})
	}
	return out
}

// rangeForSchemaError prefers the element path, because a path is exact where a
// reported line and column can drift on a document the editor is mid-edit.
func (d *Document) rangeForSchemaError(e validator.Error) Range {
	if e.Path != "" {
		if elements := d.ByPath(e.Path); len(elements) > 0 {
			return d.RangeOf(elements[0])
		}
	}
	if e.Line > 0 {
		return d.LineRange(e.Line, e.Column)
	}
	return d.LineRange(1, 0)
}

func schemaMessage(e validator.Error) string {
	msg := e.Message
	if e.Expected != "" {
		msg += fmt.Sprintf("\nexpected: %s", e.Expected)
	}
	if e.Actual != "" {
		msg += fmt.Sprintf("\nfound: %s", e.Actual)
	}
	return msg
}

// profileDiagnostics applies the scheme rule profile. This is where the
// 14 November 2026 address rules surface, in the editor, before anything is
// sent anywhere.
func (s *Server) profileDiagnostics(doc *Document) []Diagnostic {
	if s.Profile == "" {
		return nil
	}
	res, err := iso20022.CheckProfile([]byte(doc.Text), s.Profile, "")
	if err != nil {
		return nil
	}

	out := make([]Diagnostic, 0, len(res.Findings))
	for _, f := range res.Findings {
		severity := SeverityWarning
		if f.Severity == rules.SeverityError {
			severity = SeverityError
		}

		rng := doc.RangeOf(doc.Elements[0])
		if elements := doc.ByPath(f.Path); len(elements) > 0 {
			rng = doc.RangeOf(elements[0])
		} else if parent := parentOf(f.Path); parent != "" {
			// A rule reporting a missing element names a path that is not in
			// the document; its parent is, and that is where the fix goes.
			if elements := doc.ByPath(parent); len(elements) > 0 {
				rng = doc.RangeOf(elements[0])
			}
		}

		message := f.Message
		if f.Expected != "" {
			message += "\nexpected: " + f.Expected
		}
		if f.Remediation != "" {
			message += "\n\n" + f.Remediation
		}

		out = append(out, Diagnostic{
			Range:    rng,
			Severity: severity,
			Code:     f.RuleID,
			Source:   "anchor/" + s.Profile,
			Message:  message,
		})
	}
	return out
}

func parentOf(path string) string {
	for i := len(path) - 1; i > 0; i-- {
		if path[i] == '/' {
			return path[:i]
		}
	}
	return ""
}
