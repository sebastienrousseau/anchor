// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package lsp

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/sebastienrousseau/askiso/internal/xsd"
)

// Hover, completion and the symbol outline all answer the same question from
// different angles: what is this element, and what belongs here? The answers
// come from the schema when the user has it installed, and from the document
// itself when they do not.

type textDocumentPosition struct {
	TextDocument struct {
		URI string `json:"uri"`
	} `json:"textDocument"`
	Position Position `json:"position"`
}

// ---------------------------------------------------------------------------
// Hover
// ---------------------------------------------------------------------------

func (s *Server) hover(params json.RawMessage) (any, error) {
	var p textDocumentPosition
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("hover parameters: %w", err)
	}

	doc, ok := s.document(p.TextDocument.URI)
	if !ok {
		return nil, nil
	}
	el, ok := doc.ElementAt(doc.OffsetAt(p.Position))
	if !ok {
		return nil, nil
	}

	schema, msgID, haveSchema := s.schemaFor(doc)
	markdown := s.describeElement(el, schema, msgID, haveSchema)

	return map[string]any{
		"contents": map[string]any{"kind": "markdown", "value": markdown},
		"range":    doc.RangeOf(el),
	}, nil
}

// describeElement builds the hover text: what the element is, what it may
// contain, and where the answer came from.
func (s *Server) describeElement(el Element, schema *xsd.Schema, msgID string, haveSchema bool) string {
	var b strings.Builder

	fmt.Fprintf(&b, "**%s**\n\n`%s`\n", el.Name, el.Path)
	if el.HasValue() {
		fmt.Fprintf(&b, "\n```\n%s\n```\n", el.Value)
	}

	if !haveSchema {
		b.WriteString("\n---\n\n")
		if msgID != "" {
			fmt.Fprintf(&b, "The schema for `%s` is not installed, so this is what the document says "+
				"rather than what the specification requires.\n\n", msgID)
		} else {
			b.WriteString("This document's namespace does not name an ISO 20022 message, " +
				"so no schema applies.\n\n")
		}
		b.WriteString("Install one: download the message set from https://www.iso20022.org/ " +
			"then `askiso catalog add <downloaded.zip>`.\n")
		return b.String()
	}

	decl, found := declarationFor(schema, el.Path)
	if !found {
		fmt.Fprintf(&b, "\n---\n\n`%s` is not declared at this position in `%s`.\n", el.Name, msgID)
		return b.String()
	}

	fmt.Fprintf(&b, "\n---\n\n**Type** `%s`  \n**Occurs** %s\n", decl.Type, occursOf(decl))

	facets, base := schema.EffectiveFacets(decl.Type)
	if constraints := describeFacets(facets, base); constraints != "" {
		fmt.Fprintf(&b, "\n**Constraints**\n\n%s", constraints)
	}
	if children := childNames(schema, decl.Type); len(children) > 0 {
		fmt.Fprintf(&b, "\n**Contains** %s\n", "`"+strings.Join(children, "`, `")+"`")
	}

	fmt.Fprintf(&b, "\nFrom the `%s` schema you installed.\n", msgID)
	return b.String()
}

func occursOf(el *xsd.Element) string {
	max := strconv.Itoa(el.MaxOccurs)
	if el.MaxOccurs == xsd.Unbounded {
		max = "unbounded"
	}
	if el.MinOccurs == 0 {
		return "0.." + max + " (optional)"
	}
	return strconv.Itoa(el.MinOccurs) + ".." + max + " (mandatory)"
}

func describeFacets(f xsd.Facets, base string) string {
	var lines []string
	// The base type is worth naming only when it is not the default: every
	// text element restricts a string, and saying so on all of them is noise.
	if base != "" && base != "string" && base != "xs:string" {
		lines = append(lines, "- base `"+base+"`")
	}
	if f.MinLength != nil || f.MaxLength != nil || f.Length != nil {
		switch {
		case f.Length != nil:
			lines = append(lines, fmt.Sprintf("- exactly %d character(s)", *f.Length))
		case f.MinLength != nil && f.MaxLength != nil:
			lines = append(lines, fmt.Sprintf("- %d to %d characters", *f.MinLength, *f.MaxLength))
		case f.MaxLength != nil:
			lines = append(lines, fmt.Sprintf("- at most %d characters", *f.MaxLength))
		default:
			lines = append(lines, fmt.Sprintf("- at least %d characters", *f.MinLength))
		}
	}
	if f.TotalDigits != nil {
		lines = append(lines, fmt.Sprintf("- at most %d digits", *f.TotalDigits))
	}
	if f.FractionDigits != nil {
		lines = append(lines, fmt.Sprintf("- at most %d decimal place(s)", *f.FractionDigits))
	}
	for _, p := range f.Pattern {
		lines = append(lines, "- pattern `"+p+"`")
	}
	if len(f.Enumeration) > 0 {
		codes := f.Enumeration
		// A long code set is unreadable in a hover; the first few plus a count
		// is what a reader can use.
		const shown = 12
		if len(codes) > shown {
			lines = append(lines, fmt.Sprintf("- one of `%s` and %d more",
				strings.Join(codes[:shown], "`, `"), len(codes)-shown))
		} else {
			lines = append(lines, "- one of `"+strings.Join(codes, "`, `")+"`")
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

// ---------------------------------------------------------------------------
// Completion
// ---------------------------------------------------------------------------

// Completion item kinds, from the protocol.
const (
	kindField    = 5
	kindProperty = 10
	kindEnum     = 13
	kindValue    = 12
)

func (s *Server) completion(params json.RawMessage) (any, error) {
	var p textDocumentPosition
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("completion parameters: %w", err)
	}

	doc, ok := s.document(p.TextDocument.URI)
	if !ok {
		return nil, nil
	}
	schema, _, haveSchema := s.schemaFor(doc)
	if !haveSchema {
		// Without the schema there is nothing to suggest that the user does not
		// already know. Suggesting element names from elsewhere in the document
		// would be guessing.
		return map[string]any{"isIncomplete": false, "items": []any{}}, nil
	}

	offset := doc.OffsetAt(p.Position)

	// A cursor inside an element's text is asking for a value, not a child.
	if el, ok := doc.ElementAt(offset); ok && el.HasValue() &&
		offset >= el.ValueStart && offset <= el.ValueEnd {
		if items := valueCompletions(schema, el.Path); items != nil {
			return map[string]any{"isIncomplete": false, "items": items}, nil
		}
	}

	parent := doc.EnclosingPath(offset)
	items := childCompletions(schema, parent)
	return map[string]any{"isIncomplete": false, "items": items}, nil
}

// childCompletions lists the elements the schema allows inside a path.
func childCompletions(schema *xsd.Schema, path string) []map[string]any {
	decl, found := declarationFor(schema, path)
	if !found {
		return []map[string]any{}
	}

	ct, ok := schema.ResolveComplex(decl.Type)
	if !ok || ct.Content == nil {
		return []map[string]any{}
	}

	children := childElements(ct.Content)
	items := make([]map[string]any, 0, len(children))
	for i, child := range children {
		detail := child.Type + "  " + occursOf(child)
		kind := kindField
		if _, complex := schema.ResolveComplex(child.Type); complex {
			kind = kindProperty
		}
		items = append(items, map[string]any{
			"label":  child.Name,
			"kind":   kind,
			"detail": detail,
			// ISO 20022 content models are ordered sequences, so the order the
			// schema declares is the order the elements must appear in. Sorting
			// by index preserves it in the completion list.
			"sortText":         fmt.Sprintf("%04d", i),
			"insertText":       child.Name + ">$0</" + child.Name + ">",
			"insertTextFormat": 2, // snippet
			"documentation": map[string]any{
				"kind": "markdown",
				"value": fmt.Sprintf("`%s/%s`\n\nType `%s`, %s.",
					path, child.Name, child.Type, occursOf(child)),
			},
		})
	}
	return items
}

// valueCompletions lists the codes an enumerated element accepts.
func valueCompletions(schema *xsd.Schema, path string) []map[string]any {
	decl, found := declarationFor(schema, path)
	if !found {
		return nil
	}
	facets, _ := schema.EffectiveFacets(decl.Type)
	if len(facets.Enumeration) == 0 {
		return nil
	}

	items := make([]map[string]any, 0, len(facets.Enumeration))
	for _, code := range facets.Enumeration {
		items = append(items, map[string]any{
			"label":  code,
			"kind":   kindEnum,
			"detail": decl.Type,
		})
	}
	return items
}

// ---------------------------------------------------------------------------
// Document symbols
// ---------------------------------------------------------------------------

// symbol is an LSP DocumentSymbol.
type symbol struct {
	Name           string   `json:"name"`
	Detail         string   `json:"detail,omitempty"`
	Kind           int      `json:"kind"`
	Range          Range    `json:"range"`
	SelectionRange Range    `json:"selectionRange"`
	Children       []symbol `json:"children,omitempty"`
}

func (s *Server) documentSymbol(params json.RawMessage) (any, error) {
	var p struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("documentSymbol parameters: %w", err)
	}

	doc, ok := s.document(p.TextDocument.URI)
	if !ok || len(doc.Elements) == 0 {
		return []symbol{}, nil
	}

	// The elements are in document order, so one pass with a stack rebuilds the
	// tree without re-parsing.
	var roots []symbol
	var stack []*symbol

	for _, el := range doc.Elements {
		kind := kindField
		if el.HasValue() {
			kind = kindValue
		}
		node := symbol{
			Name:           el.Name,
			Detail:         el.Value,
			Kind:           kind,
			Range:          Range{Start: doc.PositionAt(el.TagStart), End: doc.PositionAt(el.ValueEnd)},
			SelectionRange: Range{Start: doc.PositionAt(el.TagStart), End: doc.PositionAt(el.TagEnd)},
		}

		for len(stack) > el.Depth {
			stack = stack[:len(stack)-1]
		}
		if len(stack) == 0 {
			roots = append(roots, node)
			stack = append(stack, &roots[len(roots)-1])
			continue
		}
		parent := stack[len(stack)-1]
		parent.Children = append(parent.Children, node)
		stack = append(stack, &parent.Children[len(parent.Children)-1])
	}
	return roots, nil
}

// ---------------------------------------------------------------------------
// Schema navigation
// ---------------------------------------------------------------------------

// declarationFor resolves an element path against a schema, returning the
// declaration at that position.
func declarationFor(schema *xsd.Schema, path string) (*xsd.Element, bool) {
	if schema == nil || path == "" {
		return nil, false
	}
	names := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(names) == 0 {
		return nil, false
	}

	root, ok := schema.RootElement()
	if !ok || root.Name != names[0] {
		return nil, false
	}

	current := root
	for _, name := range names[1:] {
		ct, ok := schema.ResolveComplex(current.Type)
		if !ok || ct.Content == nil {
			return nil, false
		}
		next, ok := findChild(ct.Content, name)
		if !ok {
			return nil, false
		}
		current = next
	}
	return current, true
}

// findChild looks for a named element among a particle's immediate children.
func findChild(p xsd.Particle, name string) (*xsd.Element, bool) {
	for _, child := range childElements(p) {
		if child.Name == name {
			return child, true
		}
	}
	return nil, false
}

// childElements flattens a particle to the elements it may contain, in
// declaration order. The order is the content model's, which is what an editor
// must preserve.
func childElements(p xsd.Particle) []*xsd.Element {
	switch t := p.(type) {
	case *xsd.Element:
		return []*xsd.Element{t}
	case *xsd.Sequence:
		var out []*xsd.Element
		for _, c := range t.Particles {
			out = append(out, childElements(c)...)
		}
		return out
	case *xsd.Choice:
		var out []*xsd.Element
		for _, c := range t.Particles {
			out = append(out, childElements(c)...)
		}
		return out
	}
	return nil
}

// childNames lists the element names a type contains, for a hover summary.
func childNames(schema *xsd.Schema, typeName string) []string {
	ct, ok := schema.ResolveComplex(typeName)
	if !ok || ct.Content == nil {
		return nil
	}
	children := childElements(ct.Content)

	const shown = 16
	out := make([]string, 0, len(children))
	for i, c := range children {
		if i == shown {
			out = append(out, fmt.Sprintf("and %d more", len(children)-shown))
			break
		}
		out = append(out, c.Name)
	}
	return out
}
