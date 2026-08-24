// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package lsp

import (
	"encoding/xml"
	"io"
	"strings"
	"unicode/utf16"
)

// An editor works in positions; the rest of Anchor works in element paths and
// values. This file is the bridge. It indexes a document once per change, so a
// diagnostic that knows only "the IBAN is wrong" can still be underlined in the
// right place, and a hover at a cursor can name the element under it.

// Element is one element occurrence in a document, with the byte offsets of its
// start tag and of its text content.
type Element struct {
	// Path is the element path from the root, for example
	// "/Document/FIToFICstmrCdtTrf/GrpHdr/MsgId".
	Path string
	// Name is the local name, without a namespace prefix.
	Name string
	// TagStart and TagEnd bracket the start tag.
	TagStart, TagEnd int
	// ValueStart and ValueEnd bracket the text content. They are equal when the
	// element has none.
	ValueStart, ValueEnd int
	// Value is the trimmed text content.
	Value string
	// Depth is how far the element sits from the root, which is 0.
	Depth int
}

// HasValue reports whether the element carries text rather than child elements.
func (e Element) HasValue() bool { return e.ValueEnd > e.ValueStart }

// Document is an indexed XML instance.
type Document struct {
	// Text is the document as the editor holds it.
	Text string
	// Elements are every element occurrence, in document order.
	Elements []Element
	// Wellformed reports whether the document parsed to the end. An editor sees
	// half-typed documents constantly, so this is the normal case, not an error.
	Wellformed bool
	// ParseError describes why parsing stopped, when it did.
	ParseError string

	byPath map[string][]int
	byName map[string][]int
	// lineStarts holds the byte offset of each line, for position conversion.
	lineStarts []int
}

// Parse indexes a document. A malformed document is indexed as far as it got,
// because an editor is mostly showing documents that are not finished yet.
func Parse(text string) *Document {
	doc := &Document{
		Text:       text,
		byPath:     map[string][]int{},
		byName:     map[string][]int{},
		Wellformed: true,
	}
	doc.indexLines()

	dec := xml.NewDecoder(strings.NewReader(text))
	dec.Strict = false
	dec.Entity = xml.HTMLEntity

	var stack []string
	// open maps a stack depth to the index of the element that opened it, so
	// character data can be attached to the element it belongs to.
	open := map[int]int{}

	for {
		before := int(dec.InputOffset())
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			doc.Wellformed = false
			doc.ParseError = err.Error()
			break
		}
		after := int(dec.InputOffset())

		switch t := tok.(type) {
		case xml.StartElement:
			stack = append(stack, t.Name.Local)
			el := Element{
				Path:       "/" + strings.Join(stack, "/"),
				Name:       t.Name.Local,
				TagStart:   before,
				TagEnd:     after,
				ValueStart: after,
				ValueEnd:   after,
				Depth:      len(stack) - 1,
			}
			idx := len(doc.Elements)
			doc.Elements = append(doc.Elements, el)
			doc.byPath[el.Path] = append(doc.byPath[el.Path], idx)
			doc.byName[el.Name] = append(doc.byName[el.Name], idx)
			open[len(stack)] = idx

		case xml.CharData:
			if len(stack) == 0 {
				continue
			}
			idx, ok := open[len(stack)]
			if !ok {
				continue
			}
			value := strings.TrimSpace(string(t))
			if value == "" {
				continue
			}
			// Trim the offsets to the text itself, so whitespace and indentation
			// are not underlined along with the value.
			raw := text[before:after]
			lead := len(raw) - len(strings.TrimLeft(raw, " \t\r\n"))
			trail := len(raw) - len(strings.TrimRight(raw, " \t\r\n"))
			doc.Elements[idx].ValueStart = before + lead
			doc.Elements[idx].ValueEnd = after - trail
			doc.Elements[idx].Value = value

		case xml.EndElement:
			if len(stack) == 0 {
				continue
			}
			delete(open, len(stack))
			stack = stack[:len(stack)-1]
		}
	}
	return doc
}

func (d *Document) indexLines() {
	d.lineStarts = []int{0}
	for i := 0; i < len(d.Text); i++ {
		if d.Text[i] == '\n' {
			d.lineStarts = append(d.lineStarts, i+1)
		}
	}
}

// Root returns the document element's name, or the empty string.
func (d *Document) Root() string {
	if len(d.Elements) == 0 {
		return ""
	}
	return d.Elements[0].Name
}

// ByPath returns every element at an exact path.
func (d *Document) ByPath(path string) []Element {
	return d.collect(d.byPath[path])
}

// ByName returns every element with a local name.
func (d *Document) ByName(name string) []Element {
	return d.collect(d.byName[name])
}

func (d *Document) collect(indices []int) []Element {
	out := make([]Element, 0, len(indices))
	for _, i := range indices {
		out = append(out, d.Elements[i])
	}
	return out
}

// FindValue locates the element with a name and text value, which is how a
// diagnostic that knows only "the BIC BANKGB2LXXX is malformed" is placed.
// Elements are searched by name first, then by name and value; without a match
// the second return value is false.
func (d *Document) FindValue(name, value string) (Element, bool) {
	candidates := d.byName[name]
	if len(candidates) == 0 {
		// A field may be reported by path fragment rather than by element name.
		if i := strings.LastIndexByte(name, '/'); i >= 0 {
			candidates = d.byName[name[i+1:]]
		}
	}

	want := strings.TrimSpace(value)
	for _, i := range candidates {
		if want == "" || d.Elements[i].Value == want {
			return d.Elements[i], true
		}
	}
	if len(candidates) > 0 {
		return d.Elements[candidates[0]], true
	}
	return Element{}, false
}

// ElementAt returns the innermost element whose start tag or text contains an
// offset, which is what a hover or a completion needs.
func (d *Document) ElementAt(offset int) (Element, bool) {
	best := -1
	for i, el := range d.Elements {
		within := (offset >= el.TagStart && offset <= el.TagEnd) ||
			(el.HasValue() && offset >= el.ValueStart && offset <= el.ValueEnd)
		if !within {
			continue
		}
		if best < 0 || el.Depth >= d.Elements[best].Depth {
			best = i
		}
	}
	if best < 0 {
		return Element{}, false
	}
	return d.Elements[best], true
}

// EnclosingPath returns the path of the element that contains an offset,
// including elements whose start tag is before it and whose end tag is after.
// That is what completion needs: the cursor is between children, not on one.
func (d *Document) EnclosingPath(offset int) string {
	var stack []string
	depthEnd := map[int]int{}

	// Walking the elements in order, an element encloses the offset when its tag
	// starts before it and the next element at the same or lower depth starts
	// after it.
	for i, el := range d.Elements {
		if el.TagStart > offset {
			break
		}
		for len(stack) > el.Depth {
			stack = stack[:len(stack)-1]
		}
		stack = append(stack, el.Name)
		depthEnd[el.Depth] = i
	}
	if len(stack) == 0 {
		return ""
	}
	return "/" + strings.Join(stack, "/")
}

// ---------------------------------------------------------------------------
// Positions
// ---------------------------------------------------------------------------

// Position is an LSP position: a zero-based line, and a character offset
// counted in UTF-16 code units, which is what the protocol specifies.
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// Range is a span between two positions.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// PositionAt converts a byte offset to an LSP position.
func (d *Document) PositionAt(offset int) Position {
	if offset < 0 {
		offset = 0
	}
	if offset > len(d.Text) {
		offset = len(d.Text)
	}

	// The last line start at or before the offset.
	line := 0
	lo, hi := 0, len(d.lineStarts)-1
	for lo <= hi {
		mid := (lo + hi) / 2
		if d.lineStarts[mid] <= offset {
			line = mid
			lo = mid + 1
			continue
		}
		hi = mid - 1
	}

	// The protocol counts characters in UTF-16 code units, so a message
	// carrying an emoji or a Chinese name still highlights the right span.
	prefix := d.Text[d.lineStarts[line]:offset]
	return Position{Line: line, Character: len(utf16.Encode([]rune(prefix)))}
}

// OffsetAt converts an LSP position to a byte offset.
func (d *Document) OffsetAt(pos Position) int {
	if pos.Line < 0 {
		return 0
	}
	if pos.Line >= len(d.lineStarts) {
		return len(d.Text)
	}

	start := d.lineStarts[pos.Line]
	end := len(d.Text)
	if pos.Line+1 < len(d.lineStarts) {
		end = d.lineStarts[pos.Line+1]
	}

	// Walk the line one rune at a time, counting UTF-16 code units.
	units := 0
	for i, r := range d.Text[start:end] {
		if units >= pos.Character {
			return start + i
		}
		units += len(utf16.Encode([]rune{r}))
	}
	return end
}

// RangeOf returns the span of an element's value, falling back to its start tag
// when it has none.
func (d *Document) RangeOf(el Element) Range {
	if el.HasValue() {
		return Range{Start: d.PositionAt(el.ValueStart), End: d.PositionAt(el.ValueEnd)}
	}
	return Range{Start: d.PositionAt(el.TagStart), End: d.PositionAt(el.TagEnd)}
}

// LineRange returns the span of a one-based line and column, which is the shape
// the schema validator reports. A column of zero underlines the whole line.
func (d *Document) LineRange(line, column int) Range {
	idx := line - 1
	if idx < 0 || idx >= len(d.lineStarts) {
		return Range{}
	}

	start := d.lineStarts[idx]
	end := len(d.Text)
	if idx+1 < len(d.lineStarts) {
		end = d.lineStarts[idx+1] - 1
	}

	text := d.Text[start:end]
	if column > 1 {
		offset := start + column - 1
		if offset > end {
			offset = end
		}
		return Range{Start: d.PositionAt(offset), End: d.PositionAt(end)}
	}

	// Skip the indentation, so a whole-line diagnostic does not underline
	// leading whitespace.
	lead := len(text) - len(strings.TrimLeft(text, " \t"))
	return Range{Start: d.PositionAt(start + lead), End: d.PositionAt(end)}
}
