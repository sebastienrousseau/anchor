// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

// Package diff compares two ISO 20022 schemas structurally.
//
// Comparing the sets of type names two versions declare says almost nothing
// useful: a rename shows as one addition and one removal, and a field that
// silently became mandatory does not show at all. What a migration actually
// needs to know is which paths appeared, which disappeared, which tightened,
// and which of those changes will reject a message that used to be accepted.
//
// So the comparison walks both schemas from their root element, flattens them
// into element paths, and classifies each difference as breaking or not. A
// change is breaking when a message valid against the old schema can be
// rejected by the new one, or when a receiver relying on the old schema loses
// data it used to get.
package diff

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/sebastienrousseau/anchor/internal/xsd"
)

// maxDepth bounds the walk. ISO 20022 types nest deeply and a few are
// recursive, so the walk needs a stop even with cycle detection.
const maxDepth = 24

// Node is one element position in a flattened schema.
type Node struct {
	// Path is the element path from the document root, for example
	// "/Document/FIToFICstmrCdtTrf/CdtTrfTxInf/PmtId/UETR".
	Path string `json:"path"`
	// Type is the declared type name.
	Type string `json:"type"`
	// Min and Max are the occurrence bounds; Max is xsd.Unbounded for "unbounded".
	Min int `json:"min"`
	Max int `json:"max"`
	// Optional records that the element sits inside an optional choice or
	// sequence within its own type, so its minOccurs overstates how mandatory
	// it is even when the containing element is present.
	Optional bool `json:"optional"`
}

// Model is a schema flattened to element paths.
type Model struct {
	// Root is the document root element name.
	Root string
	// Nodes maps a path to its node.
	Nodes map[string]*Node
	// Order lists paths in document order, which is how a reader expects to see
	// them and how the XSD requires them to appear.
	Order []string

	schema *xsd.Schema
}

// Flatten walks a schema from its root element and records every path.
func Flatten(s *xsd.Schema) *Model {
	m := &Model{Nodes: map[string]*Node{}, schema: s}
	if s == nil {
		return m
	}

	root, ok := s.RootElement()
	if !ok {
		return m
	}
	m.Root = root.Name

	m.add(&Node{Path: "/" + root.Name, Type: root.Type, Min: root.MinOccurs, Max: root.MaxOccurs})
	m.walkType(root.Type, "/"+root.Name, map[string]bool{root.Type: true}, 1, false)
	return m
}

func (m *Model) add(n *Node) {
	if _, seen := m.Nodes[n.Path]; seen {
		return
	}
	m.Nodes[n.Path] = n
	m.Order = append(m.Order, n.Path)
}

// walkType descends into a complex type, recording each element it contains.
// visiting holds the type names on the current path, so a recursive type stops
// rather than looping.
func (m *Model) walkType(typeName, path string, visiting map[string]bool, depth int, optional bool) {
	if depth > maxDepth {
		return
	}
	ct, ok := m.schema.ResolveComplex(typeName)
	if !ok || ct.Content == nil {
		return
	}
	m.walkParticle(ct.Content, path, visiting, depth, optional)
}

func (m *Model) walkParticle(p xsd.Particle, path string, visiting map[string]bool, depth int, optional bool) {
	switch t := p.(type) {
	case *xsd.Element:
		child := path + "/" + t.Name
		m.add(&Node{
			Path: child, Type: t.Type,
			Min: t.MinOccurs, Max: t.MaxOccurs,
			Optional: optional,
		})
		if visiting[t.Type] {
			// A recursive type: the path is recorded, but not descended again.
			return
		}
		// Optionality is not propagated through the element: a child of an
		// optional parent is still mandatory whenever that parent is sent, and
		// a migration needs to know that. Whether a whole new branch has to be
		// populated is decided separately, by whether its parent already
		// existed.
		visiting[t.Type] = true
		m.walkType(t.Type, child, visiting, depth+1, false)
		delete(visiting, t.Type)

	case *xsd.Sequence:
		inner := optional || t.MinOccurs == 0
		for _, c := range t.Particles {
			m.walkParticle(c, path, visiting, depth, inner)
		}

	case *xsd.Choice:
		// Every branch of a choice is optional on its own: picking one means not
		// picking the others.
		for _, c := range t.Particles {
			m.walkParticle(c, path, visiting, depth, true)
		}

	case *xsd.Any:
		// A wildcard has no path of its own to record.
	}
}

// Severity says whether a change can reject a previously valid message or lose
// data a receiver relied on.
type Severity string

const (
	// Breaking means a message that satisfied the old schema may fail the new
	// one, or a receiver loses a field it used to get.
	Breaking Severity = "breaking"
	// Compatible means the change relaxes a rule or adds something optional.
	Compatible Severity = "compatible"
)

// Kind names what changed.
type Kind string

const (
	KindAdded       Kind = "added"
	KindRemoved     Kind = "removed"
	KindCardinality Kind = "cardinality"
	KindType        Kind = "type"
	KindFacet       Kind = "facet"
	KindEnumeration Kind = "enumeration"
)

// Change is one structural difference.
type Change struct {
	Path     string   `json:"path"`
	Kind     Kind     `json:"kind"`
	Severity Severity `json:"severity"`
	// Detail explains the change in the terms a migration cares about.
	Detail string `json:"detail"`
	// From and To carry the old and new values where a comparison has them.
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
}

// Report is the outcome of comparing two schemas.
type Report struct {
	From string `json:"from"`
	To   string `json:"to"`
	// Common counts paths present in both, unchanged or not.
	Common  int      `json:"common"`
	Changes []Change `json:"changes"`
}

// Breaking returns only the changes that can reject a message.
func (r *Report) Breaking() []Change {
	var out []Change
	for _, c := range r.Changes {
		if c.Severity == Breaking {
			out = append(out, c)
		}
	}
	return out
}

// Counts summarises the report by severity.
func (r *Report) Counts() (breaking, compatible int) {
	for _, c := range r.Changes {
		if c.Severity == Breaking {
			breaking++
			continue
		}
		compatible++
	}
	return breaking, compatible
}

// Identical reports whether the two schemas have the same structure.
func (r *Report) Identical() bool { return len(r.Changes) == 0 }

// Compare walks both schemas and classifies every difference.
//
// The direction matters: from is the schema a message was built against, to is
// the one it must now satisfy.
func Compare(from, to *xsd.Schema, fromName, toName string) *Report {
	a, b := Flatten(from), Flatten(to)
	rep := &Report{From: fromName, To: toName}

	// Removals and changes, in the order the old schema declared them.
	for _, path := range a.Order {
		oldNode := a.Nodes[path]
		newNode, ok := b.Nodes[path]
		if !ok {
			rep.Changes = append(rep.Changes, Change{
				Path: path, Kind: KindRemoved, Severity: Breaking,
				Detail: "the element no longer exists; a message carrying it is rejected, " +
					"and a receiver expecting it loses the data",
				From: occurs(oldNode),
			})
			continue
		}
		rep.Common++
		rep.Changes = append(rep.Changes, compareNodes(from, to, oldNode, newNode)...)
		rep.Changes = append(rep.Changes, compareTypes(from, to, oldNode, newNode)...)
	}

	// Additions, in the order the new schema declares them.
	for _, path := range b.Order {
		if _, ok := a.Nodes[path]; ok {
			continue
		}
		n := b.Nodes[path]

		// A mandatory element only forces work on a sender if it hangs off a
		// path that already existed. When the parent is new too, the whole
		// branch is new, and a branch nobody was sending has nothing mandatory
		// about it.
		_, parentExisted := a.Nodes[parentPath(path)]
		if mandatory(n) && parentExisted {
			rep.Changes = append(rep.Changes, Change{
				Path: path, Kind: KindAdded, Severity: Breaking,
				Detail: "a new mandatory element; a message carrying its parent is rejected without it",
				To:     occurs(n),
			})
			continue
		}

		detail := "a new optional element"
		if mandatory(n) {
			detail = "a new element, mandatory within a branch that is itself new"
		}
		rep.Changes = append(rep.Changes, Change{
			Path: path, Kind: KindAdded, Severity: Compatible,
			Detail: detail, To: occurs(n),
		})
	}

	sortChanges(rep.Changes)
	return rep
}

// mandatory reports whether an element must be present whenever its parent is.
// An element inside an optional choice or sequence is not, however its own
// minOccurs reads.
func mandatory(n *Node) bool { return n.Min > 0 && !n.Optional }

// parentPath returns the path of the containing element, or "" for the root.
func parentPath(path string) string {
	if i := strings.LastIndexByte(path, '/'); i > 0 {
		return path[:i]
	}
	return ""
}

// compareNodes reports cardinality and type differences.
func compareNodes(from, to *xsd.Schema, oldNode, newNode *Node) []Change {
	var out []Change

	if oldNode.Type != newNode.Type {
		// ISO 20022 renumbers its types on almost every version bump, so a
		// changed type name says nothing on its own -- CashAccount38 becoming
		// CashAccount40 is routine. What actually changed shows up as element
		// and facet differences at the paths beneath, which this walk already
		// reports. Treating the rename itself as breaking would bury those.
		sev, detail := Compatible, "the type was renamed; its content and constraints are compared path by path"
		if shapeChanged(from, to, oldNode.Type, newNode.Type) {
			sev = Breaking
			detail = "the element changed between a simple value and a structured type"
		}
		out = append(out, Change{
			Path: oldNode.Path, Kind: KindType, Severity: sev,
			Detail: detail, From: oldNode.Type, To: newNode.Type,
		})
	}

	switch {
	case !mandatory(oldNode) && mandatory(newNode):
		out = append(out, Change{
			Path: oldNode.Path, Kind: KindCardinality, Severity: Breaking,
			Detail: "the element became mandatory",
			From:   occurs(oldNode), To: occurs(newNode),
		})
	case mandatory(oldNode) && !mandatory(newNode):
		out = append(out, Change{
			Path: oldNode.Path, Kind: KindCardinality, Severity: Compatible,
			Detail: "the element became optional",
			From:   occurs(oldNode), To: occurs(newNode),
		})
	}

	if oldNode.Max != newNode.Max {
		sev := Compatible
		detail := "the element may now repeat more often"
		if narrower(oldNode.Max, newNode.Max) {
			sev = Breaking
			detail = "the element may repeat fewer times; a message with more occurrences is rejected"
		}
		out = append(out, Change{
			Path: oldNode.Path, Kind: KindCardinality, Severity: sev,
			Detail: detail, From: occurs(oldNode), To: occurs(newNode),
		})
	}

	return out
}

// shapeChanged reports whether an element went from carrying a value to
// carrying a structure, or the other way round. That is a real break; a mere
// renumbering is not.
func shapeChanged(from, to *xsd.Schema, oldType, newType string) bool {
	_, oldComplex := from.ResolveComplex(oldType)
	_, newComplex := to.ResolveComplex(newType)
	return oldComplex != newComplex
}

// narrower reports whether a maxOccurs bound got tighter.
func narrower(oldMax, newMax int) bool {
	if oldMax == xsd.Unbounded {
		return newMax != xsd.Unbounded
	}
	if newMax == xsd.Unbounded {
		return false
	}
	return newMax < oldMax
}

// compareTypes compares the value constraints behind two elements of the same
// path. A tighter facet rejects values the old schema accepted.
func compareTypes(from, to *xsd.Schema, oldNode, newNode *Node) []Change {
	oldFacets, _ := from.EffectiveFacets(oldNode.Type)
	newFacets, _ := to.EffectiveFacets(newNode.Type)

	var out []Change
	add := func(kind Kind, sev Severity, detail, a, b string) {
		out = append(out, Change{Path: oldNode.Path, Kind: kind, Severity: sev, Detail: detail, From: a, To: b})
	}

	// Length and digit bounds.
	for _, f := range []struct {
		name             string
		oldV, newV       *int
		tighterIsSmaller bool
	}{
		{"maxLength", oldFacets.MaxLength, newFacets.MaxLength, true},
		{"minLength", oldFacets.MinLength, newFacets.MinLength, false},
		{"totalDigits", oldFacets.TotalDigits, newFacets.TotalDigits, true},
		{"fractionDigits", oldFacets.FractionDigits, newFacets.FractionDigits, true},
	} {
		if same(f.oldV, f.newV) {
			continue
		}
		sev := Compatible
		detail := f.name + " was relaxed"
		if tighter(f.oldV, f.newV, f.tighterIsSmaller) {
			sev = Breaking
			detail = f.name + " was tightened; values the old schema accepted are now rejected"
		}
		add(KindFacet, sev, detail, intStr(f.oldV), intStr(f.newV))
	}

	// Patterns. Comparing two regular expressions for containment is not
	// decidable in general, so any change is reported as breaking: it is the
	// answer that cannot mislead a migration.
	if strings.Join(oldFacets.Pattern, "|") != strings.Join(newFacets.Pattern, "|") {
		add(KindFacet, Breaking, "the pattern changed; check whether values you send still match",
			strings.Join(oldFacets.Pattern, " "), strings.Join(newFacets.Pattern, " "))
	}

	// Enumerations.
	removed, added := diffStrings(oldFacets.Enumeration, newFacets.Enumeration)
	if len(removed) > 0 {
		add(KindEnumeration, Breaking,
			fmt.Sprintf("%d code(s) were withdrawn; a message using one is rejected", len(removed)),
			strings.Join(removed, ", "), "")
	}
	if len(added) > 0 {
		add(KindEnumeration, Compatible,
			fmt.Sprintf("%d code(s) were added", len(added)), "", strings.Join(added, ", "))
	}

	return out
}

func same(a, b *int) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// tighter reports whether a bound got stricter. smallerIsTighter distinguishes
// an upper bound (maxLength) from a lower one (minLength). A bound that appears
// where none existed is always tighter.
func tighter(a, b *int, smallerIsTighter bool) bool {
	if b == nil {
		return false
	}
	if a == nil {
		return true
	}
	if smallerIsTighter {
		return *b < *a
	}
	return *b > *a
}

func intStr(v *int) string {
	if v == nil {
		return ""
	}
	return strconv.Itoa(*v)
}

// diffStrings reports which members of a are absent from b, and vice versa.
func diffStrings(a, b []string) (removed, added []string) {
	inB := make(map[string]bool, len(b))
	for _, s := range b {
		inB[s] = true
	}
	inA := make(map[string]bool, len(a))
	for _, s := range a {
		inA[s] = true
		if !inB[s] {
			removed = append(removed, s)
		}
	}
	for _, s := range b {
		if !inA[s] {
			added = append(added, s)
		}
	}
	sort.Strings(removed)
	sort.Strings(added)
	return removed, added
}

// occurs renders a cardinality the way a schema reader expects to see it.
func occurs(n *Node) string {
	max := strconv.Itoa(n.Max)
	if n.Max == xsd.Unbounded {
		max = "unbounded"
	}
	s := fmt.Sprintf("%d..%s", n.Min, max)
	if n.Optional {
		s += " (within an optional group)"
	}
	return s
}

// sortChanges puts breaking changes first, then orders by path, so the output
// leads with what will stop a migration.
func sortChanges(list []Change) {
	sort.SliceStable(list, func(i, j int) bool {
		if (list[i].Severity == Breaking) != (list[j].Severity == Breaking) {
			return list[i].Severity == Breaking
		}
		if list[i].Path != list[j].Path {
			return list[i].Path < list[j].Path
		}
		return list[i].Kind < list[j].Kind
	})
}
