// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

// Package validator checks an ISO 20022 XML instance against a parsed schema.
//
// It is pure Go, so `anchor validate` works with no libxml2 and no cgo, on any
// platform Go targets — including WebAssembly, which is what lets the website
// validate a pasted message without uploading it.
//
// Diagnostics carry the element path, the schema rule that fired, and what was
// expected versus what was found, rather than the single line xmllint prints.
package validator

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sebastienrousseau/anchor/internal/xsd"
)

// Error is one schema violation.
type Error struct {
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Path     string `json:"path"`
	Rule     string `json:"rule"`
	Expected string `json:"expected,omitempty"`
	Actual   string `json:"actual,omitempty"`
	Message  string `json:"message"`
}

func (e Error) String() string {
	if e.Line > 0 {
		return fmt.Sprintf("%d:%d %s: %s", e.Line, e.Column, e.Path, e.Message)
	}
	return fmt.Sprintf("%s: %s", e.Path, e.Message)
}

// Result is the outcome of validating one document.
type Result struct {
	Valid  bool    `json:"valid"`
	Errors []Error `json:"errors,omitempty"`
}

// maxErrors bounds a cascade so one structural mistake near the top of a large
// statement does not produce thousands of lines.
const maxErrors = 100

// node is a parsed instance element.
type node struct {
	Name     string
	Space    string
	Attrs    []xml.Attr
	Text     string
	Children []*node
	Line     int
	Column   int
	// done marks a subtree the streaming validator has already checked and
	// released. Its name and position survive so the parent's content model can
	// still be verified; its contents are gone and must not be re-checked.
	done bool
}

// Validate checks an XML instance against a schema.
func Validate(instance []byte, schema *xsd.Schema) *Result {
	res := &Result{Valid: true}

	root, err := parseInstance(instance)
	if err != nil {
		res.Valid = false
		res.Errors = append(res.Errors, Error{
			Path:    "/",
			Rule:    "well-formedness",
			Message: err.Error(),
		})
		return res
	}

	v := &validation{schema: schema, res: res}

	decl, ok := schema.Elements[root.Name]
	if !ok {
		names := make([]string, 0, len(schema.Elements))
		for n := range schema.Elements {
			names = append(names, n)
		}
		v.fail(root, "/"+root.Name, "root element", strings.Join(names, ", "), root.Name,
			fmt.Sprintf("no global element named %q is declared in this schema", root.Name))
		res.Valid = len(res.Errors) == 0
		return res
	}

	if schema.TargetNamespace != "" && root.Space != schema.TargetNamespace {
		found := root.Space
		if found == "" {
			found = "(none)"
		}
		v.fail(root, "/"+root.Name, "namespace", schema.TargetNamespace, found,
			"the document is in the wrong namespace for this schema")
	}

	v.element(decl, root, "/"+root.Name)

	res.Valid = len(res.Errors) == 0
	return res
}

type validation struct {
	schema *xsd.Schema
	res    *Result
}

func (v *validation) fail(n *node, path, rule, expected, actual, msg string) {
	if len(v.res.Errors) >= maxErrors {
		return
	}
	e := Error{Path: path, Rule: rule, Expected: expected, Actual: actual, Message: msg}
	if n != nil {
		e.Line, e.Column = n.Line, n.Column
	}
	v.res.Errors = append(v.res.Errors, e)
}

func (v *validation) tooMany() bool { return len(v.res.Errors) >= maxErrors }

// element validates one instance node against its declaration.
func (v *validation) element(decl *xsd.Element, n *node, path string) {
	if v.tooMany() {
		return
	}
	// A released subtree was validated when it closed; re-walking the stub would
	// report every element it no longer holds as missing.
	if n.done {
		return
	}

	// A complex type: children, attributes, or simple content plus attributes.
	if ct, ok := v.schema.ResolveComplex(decl.Type); ok {
		v.attributes(ct, n, path)

		if ct.SimpleBase != "" {
			if len(n.Children) > 0 {
				v.fail(n, path, "content model", "text only", n.Children[0].Name,
					fmt.Sprintf("<%s> carries a value and attributes, so it must not have child elements", n.Name))
				return
			}
			v.value(ct.SimpleBase, n.Text, n, path)
			return
		}

		if ct.Content == nil {
			if len(n.Children) > 0 {
				v.fail(n, path, "content model", "empty", n.Children[0].Name,
					fmt.Sprintf("<%s> is declared empty but has child elements", n.Name))
			}
			return
		}

		pos := v.particle(ct.Content, n, n.Children, 0, path)
		if pos < len(n.Children) && !v.tooMany() {
			extra := n.Children[pos]
			v.fail(extra, path+"/"+extra.Name, "content model",
				expectedAt(ct.Content, v.schema), extra.Name,
				fmt.Sprintf("unexpected element <%s> in <%s>", extra.Name, n.Name))
		}
		return
	}

	// Otherwise a simple value.
	if len(n.Children) > 0 {
		v.fail(n, path, "content model", "text only", n.Children[0].Name,
			fmt.Sprintf("<%s> holds a value, so it must not have child elements", n.Name))
		return
	}
	v.value(decl.Type, n.Text, n, path)
}

func (v *validation) attributes(ct *xsd.ComplexType, n *node, path string) {
	for _, a := range ct.Attributes {
		found := ""
		present := false
		for _, got := range n.Attrs {
			if got.Name.Local == a.Name {
				found, present = got.Value, true
				break
			}
		}
		if !present {
			if a.Required {
				v.fail(n, path+"/@"+a.Name, "required attribute", a.Name, "(absent)",
					fmt.Sprintf("<%s> is missing the required attribute %q", n.Name, a.Name))
			}
			continue
		}
		v.value(a.Type, found, n, path+"/@"+a.Name)
	}
}

// particle matches children[pos:] against p, returning the new position.
//
// ISO 20022 content models are deterministic: no two branches of a sequence
// begin with the same element name, so greedy matching is correct here and much
// simpler than backtracking.
func (v *validation) particle(p xsd.Particle, parent *node, children []*node, pos int, path string) int {
	switch t := p.(type) {

	case *xsd.Element:
		count := 0
		for pos < len(children) && children[pos].Name == t.Name {
			if t.MaxOccurs != xsd.Unbounded && count >= t.MaxOccurs {
				break
			}
			child := children[pos]
			childPath := path + "/" + child.Name
			if count > 0 {
				childPath = fmt.Sprintf("%s/%s[%d]", path, child.Name, count+1)
			}
			v.element(t, child, childPath)
			pos++
			count++
		}
		if count < t.MinOccurs {
			at := parent
			if pos < len(children) {
				at = children[pos]
			}
			v.fail(at, path+"/"+t.Name, "cardinality",
				fmt.Sprintf("at least %d <%s>", t.MinOccurs, t.Name),
				fmt.Sprintf("%d", count),
				fmt.Sprintf("<%s> requires %s but found %d", parent.Name, occursPhrase(t), count))
		}
		return pos

	case *xsd.Sequence:
		reps := 0
		for t.MaxOccurs == xsd.Unbounded || reps < t.MaxOccurs {
			before := pos
			next := pos
			for _, sub := range t.Particles {
				next = v.particle(sub, parent, children, next, path)
				if v.tooMany() {
					return next
				}
			}
			pos = next
			reps++
			// A repeatable group that consumed nothing would loop forever.
			if pos == before {
				break
			}
			if t.MaxOccurs == 1 {
				break
			}
		}
		return pos

	case *xsd.Choice:
		reps := 0
		for t.MaxOccurs == xsd.Unbounded || reps < t.MaxOccurs {
			if pos >= len(children) {
				break
			}
			branch := pickBranch(t, children[pos])
			if branch == nil {
				break
			}
			before := pos
			pos = v.particle(branch, parent, children, pos, path)
			reps++
			if pos == before || t.MaxOccurs == 1 {
				break
			}
		}
		if reps == 0 && t.MinOccurs > 0 {
			at := parent
			if pos < len(children) {
				at = children[pos]
			}
			got := "(nothing)"
			if pos < len(children) {
				got = children[pos].Name
			}
			v.fail(at, path, "choice",
				"one of "+strings.Join(branchNames(t), ", "), got,
				fmt.Sprintf("<%s> requires one of %s", parent.Name, strings.Join(branchNames(t), ", ")))
		}
		return pos

	case *xsd.Any:
		count := 0
		for pos < len(children) {
			if t.MaxOccurs != xsd.Unbounded && count >= t.MaxOccurs {
				break
			}
			pos++
			count++
		}
		if count < t.MinOccurs {
			v.fail(parent, path, "cardinality",
				fmt.Sprintf("at least %d element(s)", t.MinOccurs), strconv.Itoa(count),
				fmt.Sprintf("<%s> requires at least %d element(s)", parent.Name, t.MinOccurs))
		}
		return pos
	}
	return pos
}

// pickBranch finds the branch of a choice that can start with this element.
func pickBranch(c *xsd.Choice, child *node) xsd.Particle {
	for _, p := range c.Particles {
		if startsWith(p, child.Name) {
			return p
		}
	}
	return nil
}

func startsWith(p xsd.Particle, name string) bool {
	switch t := p.(type) {
	case *xsd.Element:
		return t.Name == name
	case *xsd.Any:
		return true
	case *xsd.Sequence:
		for _, sub := range t.Particles {
			if startsWith(sub, name) {
				return true
			}
			if !optional(sub) {
				return false
			}
		}
	case *xsd.Choice:
		for _, sub := range t.Particles {
			if startsWith(sub, name) {
				return true
			}
		}
	}
	return false
}

func optional(p xsd.Particle) bool {
	switch t := p.(type) {
	case *xsd.Element:
		return t.MinOccurs == 0
	case *xsd.Sequence:
		return t.MinOccurs == 0
	case *xsd.Choice:
		return t.MinOccurs == 0
	case *xsd.Any:
		return t.MinOccurs == 0
	}
	return false
}

func branchNames(c *xsd.Choice) []string {
	var out []string
	for _, p := range c.Particles {
		out = append(out, firstNames(p)...)
	}
	if len(out) > 8 {
		out = append(out[:8], "…")
	}
	return out
}

func firstNames(p xsd.Particle) []string {
	switch t := p.(type) {
	case *xsd.Element:
		return []string{t.Name}
	case *xsd.Any:
		return []string{"(any)"}
	case *xsd.Sequence:
		for _, sub := range t.Particles {
			if n := firstNames(sub); len(n) > 0 {
				return n
			}
		}
	case *xsd.Choice:
		var out []string
		for _, sub := range t.Particles {
			out = append(out, firstNames(sub)...)
		}
		return out
	}
	return nil
}

// expectedAt lists what could legally appear, for an "unexpected element" message.
func expectedAt(p xsd.Particle, _ *xsd.Schema) string {
	names := firstNames(p)
	if len(names) > 8 {
		names = append(names[:8], "…")
	}
	return strings.Join(names, ", ")
}

func occursPhrase(e *xsd.Element) string {
	switch {
	case e.MinOccurs == 1 && e.MaxOccurs == 1:
		return fmt.Sprintf("exactly one <%s>", e.Name)
	case e.MaxOccurs == xsd.Unbounded:
		return fmt.Sprintf("at least %d <%s>", e.MinOccurs, e.Name)
	default:
		return fmt.Sprintf("%d to %d <%s>", e.MinOccurs, e.MaxOccurs, e.Name)
	}
}

// ---------------------------------------------------------------------------
// Simple values
// ---------------------------------------------------------------------------

func (v *validation) value(typeName, text string, n *node, path string) {
	if v.tooMany() {
		return
	}
	facets, base := v.schema.EffectiveFacets(typeName)

	if !checkBase(base, text) {
		v.fail(n, path, "type", base, text,
			fmt.Sprintf("%q is not a valid %s", truncate(text), base))
		return
	}

	if len(facets.Enumeration) > 0 {
		found := false
		for _, e := range facets.Enumeration {
			if e == text {
				found = true
				break
			}
		}
		if !found {
			allowed := facets.Enumeration
			if len(allowed) > 12 {
				allowed = append(append([]string{}, allowed[:12]...), "…")
			}
			v.fail(n, path, "enumeration", strings.Join(allowed, ", "), text,
				fmt.Sprintf("%q is not one of the permitted values", truncate(text)))
			return
		}
	}

	length := len([]rune(text))
	if facets.Length != nil && length != *facets.Length {
		v.fail(n, path, "length", strconv.Itoa(*facets.Length), strconv.Itoa(length),
			fmt.Sprintf("must be exactly %d characters, got %d", *facets.Length, length))
	}
	if facets.MinLength != nil && length < *facets.MinLength {
		v.fail(n, path, "minLength", strconv.Itoa(*facets.MinLength), strconv.Itoa(length),
			fmt.Sprintf("must be at least %d characters, got %d", *facets.MinLength, length))
	}
	if facets.MaxLength != nil && length > *facets.MaxLength {
		v.fail(n, path, "maxLength", strconv.Itoa(*facets.MaxLength), strconv.Itoa(length),
			fmt.Sprintf("must be at most %d characters, got %d", *facets.MaxLength, length))
	}

	for _, p := range facets.Pattern {
		re, err := compilePattern(p)
		if err != nil {
			continue // an unusable pattern must not fail an otherwise valid document
		}
		if !re.match(text) {
			v.fail(n, path, "pattern", p, text,
				fmt.Sprintf("%q does not match the required format", truncate(text)))
			break
		}
	}

	v.numericFacets(facets, base, text, n, path)
}

func (v *validation) numericFacets(f xsd.Facets, base, text string, n *node, path string) {
	if base != "decimal" && base != "integer" {
		return
	}
	if f.TotalDigits == nil && f.FractionDigits == nil && f.MinInclusive == nil && f.MaxInclusive == nil {
		return
	}

	digits, fraction := countDigits(text)
	if f.TotalDigits != nil && digits > *f.TotalDigits {
		v.fail(n, path, "totalDigits", strconv.Itoa(*f.TotalDigits), strconv.Itoa(digits),
			fmt.Sprintf("at most %d digits are permitted, got %d", *f.TotalDigits, digits))
	}
	if f.FractionDigits != nil && fraction > *f.FractionDigits {
		v.fail(n, path, "fractionDigits", strconv.Itoa(*f.FractionDigits), strconv.Itoa(fraction),
			fmt.Sprintf("at most %d decimal place(s) permitted, got %d", *f.FractionDigits, fraction))
	}

	got, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
	if err != nil {
		return
	}
	if f.MinInclusive != nil {
		if min, err := strconv.ParseFloat(*f.MinInclusive, 64); err == nil && got < min {
			v.fail(n, path, "minInclusive", *f.MinInclusive, text,
				fmt.Sprintf("must be at least %s", *f.MinInclusive))
		}
	}
	if f.MaxInclusive != nil {
		if max, err := strconv.ParseFloat(*f.MaxInclusive, 64); err == nil && got > max {
			v.fail(n, path, "maxInclusive", *f.MaxInclusive, text,
				fmt.Sprintf("must be at most %s", *f.MaxInclusive))
		}
	}
}

// countDigits returns the total significant digits and the fractional digits,
// matching how XSD counts them (sign and leading zeros excluded).
func countDigits(text string) (total, fraction int) {
	s := strings.TrimSpace(text)
	s = strings.TrimPrefix(strings.TrimPrefix(s, "+"), "-")

	intPart, fracPart, hasFrac := strings.Cut(s, ".")
	intPart = strings.TrimLeft(intPart, "0")
	if hasFrac {
		fracPart = strings.TrimRight(fracPart, "0")
	}
	return len(intPart) + len(fracPart), len(fracPart)
}

var (
	dateRe      = regexp.MustCompile(`^-?\d{4}-\d{2}-\d{2}(Z|[+-]\d{2}:\d{2})?$`)
	timeRe      = regexp.MustCompile(`^\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})?$`)
	dateTimeRe  = regexp.MustCompile(`^-?\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})?$`)
	gYearRe     = regexp.MustCompile(`^-?\d{4}(Z|[+-]\d{2}:\d{2})?$`)
	gYearMonRe  = regexp.MustCompile(`^-?\d{4}-\d{2}(Z|[+-]\d{2}:\d{2})?$`)
	gMonthRe    = regexp.MustCompile(`^--\d{2}(Z|[+-]\d{2}:\d{2})?$`)
	decimalRe   = regexp.MustCompile(`^[+-]?(\d+(\.\d*)?|\.\d+)$`)
	integerRe   = regexp.MustCompile(`^[+-]?\d+$`)
	nonNegIntRe = regexp.MustCompile(`^\+?\d+$`)
)

// checkBase validates a lexical form against a builtin type.
func checkBase(base, text string) bool {
	s := strings.TrimSpace(text)
	switch base {
	case "string", "normalizedString", "token", "anyURI", "anyType", "anySimpleType",
		"base64Binary", "hexBinary":
		return true

	case "boolean":
		return s == "true" || s == "false" || s == "1" || s == "0"

	case "decimal", "float", "double":
		return decimalRe.MatchString(s)

	case "integer", "long", "int", "short", "byte", "negativeInteger", "nonPositiveInteger":
		return integerRe.MatchString(s)

	case "nonNegativeInteger", "positiveInteger",
		"unsignedLong", "unsignedInt", "unsignedShort", "unsignedByte":
		return nonNegIntRe.MatchString(s)

	case "date":
		return dateRe.MatchString(s) && realDate(s)
	case "time":
		return timeRe.MatchString(s)
	case "dateTime":
		return dateTimeRe.MatchString(s) && realDateTime(s)
	case "gYear":
		return gYearRe.MatchString(s)
	case "gYearMonth":
		return gYearMonRe.MatchString(s)
	case "gMonth":
		return gMonthRe.MatchString(s)
	}
	// An unrecognised base is not a reason to reject a document.
	return true
}

// realDate rejects lexically well-formed but impossible dates such as 2026-02-30.
func realDate(s string) bool {
	if len(s) < 10 {
		return false
	}
	_, err := time.Parse("2006-01-02", s[:10])
	return err == nil
}

func realDateTime(s string) bool {
	if len(s) < 10 {
		return false
	}
	return realDate(s[:10])
}

// ---------------------------------------------------------------------------
// XSD regular expressions
// ---------------------------------------------------------------------------

var (
	patternCache sync.Map // pattern string -> *matcher or error
	repeatRe     = regexp.MustCompile(`\{(\d+),(\d+)\}`)
)

// goMaxRepeat is Go's cap on a repetition count. It applies to the product
// across nesting, so "(?:X{1,1000}){1,2}" is rejected too -- a bound above it
// simply cannot be expressed as a Go regexp.
const goMaxRepeat = 1000

var errBadPattern = fmt.Errorf("pattern could not be compiled")

// matcher is a compiled XSD pattern.
//
// maxRunes carries an upper bound the regexp itself could not express: where a
// pattern such as "([0-9A-F][0-9A-F]){1,10000}" exceeds Go's repeat cap, the
// bound is relaxed to unbounded in the expression and reinstated here, computed
// from the repeat count and the atom's fixed width. When the atom has no fixed
// width the bound is dropped, and an accompanying maxLength facet -- which the
// schemas declare in that case -- keeps the check exact.
type matcher struct {
	re       *regexp.Regexp
	maxRunes int // 0 means no additional limit
}

func (m *matcher) match(s string) bool {
	if m.maxRunes > 0 && len([]rune(s)) > m.maxRunes {
		return false
	}
	return m.re.MatchString(s)
}

// compilePattern turns an XSD pattern into a matcher. XSD patterns are anchored
// to the whole value, so the expression is wrapped.
func compilePattern(p string) (*matcher, error) {
	if cached, ok := patternCache.Load(p); ok {
		if m, ok := cached.(*matcher); ok {
			return m, nil
		}
		return nil, errBadPattern
	}

	expr, maxRunes := relaxBigRepeats(p)
	re, err := regexp.Compile(`^(?:` + expr + `)$`)
	if err != nil {
		patternCache.Store(p, err)
		return nil, errBadPattern
	}
	m := &matcher{re: re, maxRunes: maxRunes}
	patternCache.Store(p, m)
	return m, nil
}

// relaxBigRepeats replaces "{n,m}" where m exceeds Go's cap with "{n,}", and
// returns the largest value length the original bound allowed, or 0 when that
// cannot be determined.
func relaxBigRepeats(p string) (string, int) {
	var out strings.Builder
	maxRunes := 0
	i := 0

	for {
		loc := repeatRe.FindStringSubmatchIndex(p[i:])
		if loc == nil {
			out.WriteString(p[i:])
			return out.String(), maxRunes
		}
		start, end := i+loc[0], i+loc[1]
		lo := p[i+loc[2] : i+loc[3]]
		hi, err := strconv.Atoi(p[i+loc[4] : i+loc[5]])

		if err != nil || hi <= goMaxRepeat {
			out.WriteString(p[i:end])
			i = end
			continue
		}

		out.WriteString(p[i:start])
		fmt.Fprintf(&out, "{%s,}", lo)
		i = end

		if width := atomWidth(p, start); width > 0 {
			if bound := hi * width; maxRunes == 0 || bound < maxRunes {
				maxRunes = bound
			}
		}
	}
}

// atomWidth returns how many characters the atom ending at end-1 always matches,
// or 0 when that is not fixed.
func atomWidth(p string, end int) int {
	start := atomStartIndex(p, end)
	if start < 0 {
		return 0
	}
	atom := p[start:end]

	// A group: sum the widths of its parts, provided it has no alternation or
	// nested quantifier.
	if strings.HasPrefix(atom, "(") && strings.HasSuffix(atom, ")") {
		inner := atom[1 : len(atom)-1]
		if strings.ContainsAny(inner, "|*+?{") {
			return 0
		}
		width, i := 0, 0
		for i < len(inner) {
			n := atomLen(inner, i)
			if n == 0 {
				return 0
			}
			width++
			i += n
		}
		return width
	}

	// A character class or a single (possibly escaped) character.
	if atomLen(atom, 0) == len(atom) {
		return 1
	}
	return 0
}

// atomLen returns the byte length of the single-character atom at s[i:], or 0.
func atomLen(s string, i int) int {
	if i >= len(s) {
		return 0
	}
	switch s[i] {
	case '[':
		for j := i + 1; j < len(s); j++ {
			if s[j] == ']' && !escaped(s, j) {
				return j - i + 1
			}
		}
		return 0
	case '\\':
		if i+1 < len(s) {
			return 2
		}
		return 0
	case '(', ')', '|', '*', '+', '?', '{', '}':
		return 0
	default:
		return 1
	}
}

// atomStartIndex finds where the regex atom ending at end-1 begins.
func atomStartIndex(p string, end int) int {
	if end <= 0 {
		return -1
	}
	switch p[end-1] {
	case ')':
		depth := 0
		for i := end - 1; i >= 0; i-- {
			if escaped(p, i) {
				continue
			}
			switch p[i] {
			case ')':
				depth++
			case '(':
				depth--
				if depth == 0 {
					return i
				}
			}
		}
		return -1

	case ']':
		for i := end - 2; i >= 0; i-- {
			if p[i] == '[' && !escaped(p, i) {
				return i
			}
		}
		return -1

	default:
		if end >= 2 && escaped(p, end-1) {
			return end - 2
		}
		return end - 1
	}
}

// escaped reports whether p[i] is preceded by an odd number of backslashes.
func escaped(p string, i int) bool {
	n := 0
	for j := i - 1; j >= 0 && p[j] == '\\'; j-- {
		n++
	}
	return n%2 == 1
}

func truncate(s string) string {
	const limit = 60
	if len([]rune(s)) <= limit {
		return s
	}
	return string([]rune(s)[:limit]) + "…"
}

// ---------------------------------------------------------------------------
// Instance parsing
// ---------------------------------------------------------------------------

func parseInstance(data []byte) (*node, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = true
	// Reject DTD entity references outright: encoding/xml never fetches external
	// entities, and this stops internal ones being expanded too.
	dec.Entity = xml.HTMLEntity

	lines := newLineIndex(data)

	var root *node
	var stack []*node

	for {
		offset := dec.InputOffset()
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			line, col := lines.at(offset)
			n := &node{
				Name:   t.Name.Local,
				Space:  t.Name.Space,
				Attrs:  append([]xml.Attr(nil), t.Attr...),
				Line:   line,
				Column: col,
			}
			if len(stack) == 0 {
				if root != nil {
					return nil, fmt.Errorf("more than one root element")
				}
				root = n
			} else {
				parent := stack[len(stack)-1]
				parent.Children = append(parent.Children, n)
			}
			stack = append(stack, n)

		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}

		case xml.CharData:
			if len(stack) > 0 {
				stack[len(stack)-1].Text += string(t)
			}
		}
	}

	if root == nil {
		return nil, fmt.Errorf("document has no elements")
	}
	trimText(root)
	return root, nil
}

// trimText normalises whitespace-only text so indentation is not mistaken for a
// value. Elements with children never carry a value in ISO 20022.
func trimText(n *node) {
	if len(n.Children) > 0 {
		n.Text = ""
	} else {
		n.Text = strings.TrimSpace(n.Text)
	}
	for _, c := range n.Children {
		trimText(c)
	}
}

// lineIndex converts a byte offset to a 1-based line and column.
type lineIndex struct{ starts []int64 }

func newLineIndex(data []byte) *lineIndex {
	starts := []int64{0}
	for i, b := range data {
		if b == '\n' {
			starts = append(starts, int64(i)+1)
		}
	}
	return &lineIndex{starts: starts}
}

func (l *lineIndex) at(offset int64) (line, col int) {
	lo, hi := 0, len(l.starts)-1
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if l.starts[mid] <= offset {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return lo + 1, int(offset-l.starts[lo]) + 1
}
