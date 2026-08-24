// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package schemagen

import (
	"fmt"
	"strconv"
	"strings"
)

// Generating a message from a schema means generating values that satisfy the
// schema's patterns. Going the other way from a regular expression to a string
// it accepts is not possible in general, but it is entirely possible for the
// vocabulary ISO 20022 actually uses: 125 distinct patterns across all 4,746
// schemas, built from character classes, bounded repeats, groups and
// alternation. Nothing else appears -- no backreferences, no lookaround, no
// anchors.
//
// So this is a parser for that subset and a sampler over the result. Every
// value it produces is checked against the real pattern by the validator before
// it is used, so a gap here surfaces as a test failure rather than as an
// invalid message.

// patternNode is one item in a parsed pattern.
type patternNode interface {
	sample(*sampler, *strings.Builder)
}

// alternation is "a|b": any one branch.
type alternation struct{ branches []patternNode }

// sequence is "ab": every item in order.
type sequence struct{ items []patternNode }

// repeat is "x{n,m}", "x?", "x*" or "x+".
type repeat struct {
	item     patternNode
	min, max int
}

// charClass is "[a-z]" or "[^/]".
type charClass struct {
	ranges  []runeRange
	negated bool
}

type runeRange struct{ lo, hi rune }

// literal is a single character.
type literal struct{ r rune }

// unbounded marks a repeat with no upper limit.
const unbounded = -1

// parsePattern reads the XSD pattern subset. It fails rather than guessing on
// anything outside it, because a wrong guess would produce an invalid message.
func parsePattern(p string) (patternNode, error) {
	ps := &patternParser{src: []rune(p)}
	node, err := ps.parseAlternation()
	if err != nil {
		return nil, err
	}
	if ps.pos != len(ps.src) {
		return nil, fmt.Errorf("unexpected %q at offset %d", string(ps.src[ps.pos]), ps.pos)
	}
	return node, nil
}

type patternParser struct {
	src []rune
	pos int
}

func (p *patternParser) eof() bool { return p.pos >= len(p.src) }

func (p *patternParser) peek() rune {
	if p.eof() {
		return 0
	}
	return p.src[p.pos]
}

func (p *patternParser) next() rune {
	r := p.src[p.pos]
	p.pos++
	return r
}

func (p *patternParser) parseAlternation() (patternNode, error) {
	first, err := p.parseSequence()
	if err != nil {
		return nil, err
	}
	if p.eof() || p.peek() != '|' {
		return first, nil
	}

	branches := []patternNode{first}
	for !p.eof() && p.peek() == '|' {
		p.next()
		branch, err := p.parseSequence()
		if err != nil {
			return nil, err
		}
		branches = append(branches, branch)
	}
	return &alternation{branches: branches}, nil
}

func (p *patternParser) parseSequence() (patternNode, error) {
	var items []patternNode
	for !p.eof() && p.peek() != '|' && p.peek() != ')' {
		item, err := p.parseRepeat()
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if len(items) == 1 {
		return items[0], nil
	}
	return &sequence{items: items}, nil
}

func (p *patternParser) parseRepeat() (patternNode, error) {
	atom, err := p.parseAtom()
	if err != nil {
		return nil, err
	}
	if p.eof() {
		return atom, nil
	}

	switch p.peek() {
	case '?':
		p.next()
		return &repeat{item: atom, min: 0, max: 1}, nil
	case '*':
		p.next()
		return &repeat{item: atom, min: 0, max: unbounded}, nil
	case '+':
		p.next()
		return &repeat{item: atom, min: 1, max: unbounded}, nil
	case '{':
		return p.parseBraces(atom)
	}
	return atom, nil
}

func (p *patternParser) parseBraces(atom patternNode) (patternNode, error) {
	p.next() // '{'

	closing := -1
	for i := p.pos; i < len(p.src); i++ {
		if p.src[i] == '}' {
			closing = i
			break
		}
	}
	if closing < 0 {
		return nil, fmt.Errorf("unclosed quantifier at offset %d", p.pos)
	}

	body := string(p.src[p.pos:closing])
	p.pos = closing + 1

	lo, hi, found := strings.Cut(body, ",")
	min, err := strconv.Atoi(strings.TrimSpace(lo))
	if err != nil {
		return nil, fmt.Errorf("quantifier %q is not a number", body)
	}
	if !found {
		return &repeat{item: atom, min: min, max: min}, nil
	}
	if strings.TrimSpace(hi) == "" {
		return &repeat{item: atom, min: min, max: unbounded}, nil
	}
	max, err := strconv.Atoi(strings.TrimSpace(hi))
	if err != nil {
		return nil, fmt.Errorf("quantifier %q is not a range", body)
	}
	return &repeat{item: atom, min: min, max: max}, nil
}

func (p *patternParser) parseAtom() (patternNode, error) {
	switch r := p.peek(); r {
	case '(':
		p.next()
		inner, err := p.parseAlternation()
		if err != nil {
			return nil, err
		}
		if p.eof() || p.peek() != ')' {
			return nil, fmt.Errorf("unclosed group at offset %d", p.pos)
		}
		p.next()
		return inner, nil

	case '[':
		return p.parseClass()

	case '\\':
		p.next()
		if p.eof() {
			return nil, fmt.Errorf("trailing backslash")
		}
		return escapeAtom(p.next())

	case '.':
		p.next()
		// Any character. The sampler picks a safe one.
		return &charClass{ranges: []runeRange{{0x20, 0x7E}}}, nil

	case ')':
		return nil, fmt.Errorf("unbalanced ) at offset %d", p.pos)
	}

	return &literal{r: p.next()}, nil
}

// escapeAtom expands a backslash escape outside a character class.
func escapeAtom(r rune) (patternNode, error) {
	switch r {
	case 'd':
		return &charClass{ranges: []runeRange{{'0', '9'}}}, nil
	case 'w':
		return &charClass{ranges: []runeRange{{'A', 'Z'}, {'a', 'z'}, {'0', '9'}, {'_', '_'}}}, nil
	case 's':
		return &charClass{ranges: []runeRange{{' ', ' '}}}, nil
	case 'n':
		return &literal{r: '\n'}, nil
	case 'r':
		return &literal{r: '\r'}, nil
	case 't':
		return &literal{r: '\t'}, nil
	}
	// Everything else is the character itself: "\-", "\.", "\(" and so on.
	return &literal{r: r}, nil
}

func (p *patternParser) parseClass() (patternNode, error) {
	p.next() // '['

	cls := &charClass{}
	if !p.eof() && p.peek() == '^' {
		p.next()
		cls.negated = true
	}

	for {
		if p.eof() {
			return nil, fmt.Errorf("unclosed character class")
		}
		if p.peek() == ']' {
			p.next()
			break
		}

		lo, err := p.classRune()
		if err != nil {
			return nil, err
		}

		// A hyphen between two characters is a range; one before the closing
		// bracket is a literal hyphen.
		if !p.eof() && p.peek() == '-' && p.pos+1 < len(p.src) && p.src[p.pos+1] != ']' {
			p.next()
			hi, err := p.classRune()
			if err != nil {
				return nil, err
			}
			cls.ranges = append(cls.ranges, runeRange{lo, hi})
			continue
		}
		cls.ranges = append(cls.ranges, runeRange{lo, lo})
	}

	if len(cls.ranges) == 0 {
		return nil, fmt.Errorf("empty character class")
	}
	return cls, nil
}

// classRune reads one member of a character class, expanding escapes. A shape
// such as "\d" inside a class contributes its whole range, which is why it is
// handled here rather than by the caller.
func (p *patternParser) classRune() (rune, error) {
	r := p.next()
	if r != '\\' {
		return r, nil
	}
	if p.eof() {
		return 0, fmt.Errorf("trailing backslash in a character class")
	}

	switch e := p.next(); e {
	case 'n':
		return '\n', nil
	case 'r':
		return '\r', nil
	case 't':
		return '\t', nil
	case 'd':
		// "[\d]" occurs in the catalogue. Contributing '0' is enough: the class
		// only has to yield one usable character.
		return '0', nil
	case 's':
		return ' ', nil
	case 'w':
		return 'A', nil
	default:
		return e, nil
	}
}

// ---------------------------------------------------------------------------
// Sampling
// ---------------------------------------------------------------------------

// sampler holds the choices a generated value depends on.
type sampler struct {
	// reps is how many times an unbounded or ranged repeat is taken beyond its
	// minimum. Growing it is how a value is lengthened to satisfy minLength.
	reps int
}

func (a *alternation) sample(s *sampler, b *strings.Builder) {
	// The first branch, always: a generated message has to be reproducible, and
	// the first branch is the one a schema author wrote first.
	if len(a.branches) > 0 {
		a.branches[0].sample(s, b)
	}
}

func (q *sequence) sample(s *sampler, b *strings.Builder) {
	for _, item := range q.items {
		item.sample(s, b)
	}
}

func (r *repeat) sample(s *sampler, b *strings.Builder) {
	count := r.min
	// Take extra repetitions when asked, up to whatever the pattern allows.
	if s.reps > 0 {
		count += s.reps
		if r.max != unbounded && count > r.max {
			count = r.max
		}
	}
	// A repeat that may occur but need not still has to produce something when
	// the value would otherwise be empty; that is decided by the caller growing
	// reps, not here.
	for i := 0; i < count; i++ {
		r.item.sample(s, b)
	}
}

func (l *literal) sample(_ *sampler, b *strings.Builder) { b.WriteRune(l.r) }

func (c *charClass) sample(_ *sampler, b *strings.Builder) { b.WriteRune(c.pick()) }

// pick chooses a character from a class, preferring ones that read as data
// rather than as punctuation: a generated identifier of slashes and question
// marks is valid and useless.
func (c *charClass) pick() rune {
	preferred := []rune{
		'A', 'B', 'C', 'D', 'E', 'X', 'Z',
		'0', '1', '2', '9',
		'a', 'b', 'c', 'z',
		' ', '-', '.', '_',
	}
	for _, r := range preferred {
		if c.contains(r) {
			return r
		}
	}

	// Nothing preferred fits. Take the first printable member, then the first
	// member of any kind.
	if c.negated {
		for r := rune(0x21); r <= 0x7E; r++ {
			if c.contains(r) {
				return r
			}
		}
		return 'A'
	}
	for _, rng := range c.ranges {
		for r := rng.lo; r <= rng.hi; r++ {
			if r >= 0x20 && r <= 0x7E {
				return r
			}
		}
	}
	return c.ranges[0].lo
}

func (c *charClass) contains(r rune) bool {
	inRanges := false
	for _, rng := range c.ranges {
		if r >= rng.lo && r <= rng.hi {
			inRanges = true
			break
		}
	}
	if c.negated {
		return !inRanges
	}
	return inRanges
}

// SamplePattern produces a string the pattern accepts, at least minLength runes
// long where it can manage it.
//
// The caller is expected to verify the result against the real pattern. This
// generates; it does not certify.
func SamplePattern(pattern string, minLength int) (string, error) {
	node, err := parsePattern(pattern)
	if err != nil {
		return "", fmt.Errorf("pattern %q: %w", pattern, err)
	}

	// Grow the repeats until the value is long enough, or until growing stops
	// helping -- a pattern of fixed length cannot be lengthened, and asking
	// forever would not change that.
	best := ""
	for reps := 0; reps <= minLength+1; reps++ {
		var b strings.Builder
		node.sample(&sampler{reps: reps}, &b)
		got := b.String()

		if len([]rune(got)) >= minLength && got != "" {
			return got, nil
		}
		if len([]rune(got)) <= len([]rune(best)) && reps > 0 {
			// Growing stopped making a difference.
			break
		}
		best = got
	}

	if best == "" {
		// A pattern that only matches the empty string is legal but useless as
		// a sample; the caller decides what to do with that.
		return "", nil
	}
	return best, nil
}
