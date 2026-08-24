// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package swift

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/sebastienrousseau/anchor/internal/converter"
)

// Going the other way is the harder direction, and the one people need during
// coexistence: a bank that receives ISO 20022 and has a downstream system that
// only speaks MT.
//
// It is also lossier, and lossy in a way that matters more. MT to MX loses
// nothing a schema requires; MX to MT loses things a regulator now requires. A
// structured address collapses into free text, a purpose code has nowhere to
// go, an LEI disappears, a 35-character reference is cut to 16. Every one of
// those is reported, because a conversion that quietly shortened a mandate
// reference would be worse than one that refused.

// ConvertMX translates an ISO 20022 message to its SWIFT MT equivalent.
//
//	pain.001 -> MT101   request for transfer
//	pacs.008 -> MT103   customer credit transfer
//	pain.008 -> MT104   request for debit transfer
//	pacs.009 -> MT202   financial institution transfer
//	pacs.010 -> MT204   financial markets direct debit
//	camt.053 -> MT940   customer statement
func ConvertMX(document []byte) (*Conversion, error) {
	root, err := converter.Parse(document)
	if err != nil {
		return nil, fmt.Errorf("reading the message: %w", err)
	}

	msgID := messageIDOf(document)
	base := baseOf(msgID)

	switch base {
	case "pacs.008":
		return convertPacs008(root, msgID)
	case "pacs.009":
		return convertPacs009(root, msgID)
	case "pacs.010":
		return convertPacs010(root, msgID)
	case "pain.001":
		return convertPain001(root, msgID)
	case "pain.008":
		return convertPain008(root, msgID)
	case "camt.053":
		return convertCamt053(root, msgID)
	}

	if msgID == "" {
		return nil, fmt.Errorf("the document's namespace does not name an ISO 20022 message; " +
			"MX to MT conversion needs to know which message this is")
	}
	return nil, fmt.Errorf("%s has no MT conversion yet (supported: %s)",
		msgID, strings.Join(SupportedMX(), ", "))
}

// SupportedMX lists the ISO 20022 messages ConvertMX can translate.
func SupportedMX() []string {
	return []string{"pain.001", "pacs.008", "pain.008", "pacs.009", "pacs.010", "camt.053"}
}

var mxNamespace = regexp.MustCompile(`urn:iso:std:iso:20022:tech:xsd:([a-z]{4}\.\d{3}\.\d{3}\.\d{2})`)

func messageIDOf(document []byte) string {
	if m := mxNamespace.FindSubmatch(document); m != nil {
		return string(m[1])
	}
	return ""
}

func baseOf(msgID string) string {
	parts := strings.Split(msgID, ".")
	if len(parts) >= 2 {
		return parts[0] + "." + parts[1]
	}
	return ""
}

// ---------------------------------------------------------------------------
// The MT document under construction
// ---------------------------------------------------------------------------

// mtBuilder assembles an MT message and records what each source element cost.
type mtBuilder struct {
	fields  []string
	reports []FieldReport
}

// set writes a field, reporting the source element it came from and whether the
// value had to be shortened to fit.
func (b *mtBuilder) set(tag, sourcePath, value string, max int) {
	trimmed, cut := truncate(strings.TrimSpace(value), max)
	if trimmed == "" {
		return
	}
	b.fields = append(b.fields, ":"+tag+":"+trimmed)

	if cut {
		b.reports = append(b.reports, FieldReport{
			Tag: sourcePath, Path: ":" + tag + ":", Fidelity: FidelityTruncated,
			Note: fmt.Sprintf("field %s permits %d characters; the value is %d",
				tag, max, len([]rune(strings.TrimSpace(value)))),
			Value: strings.TrimSpace(value),
		})
		return
	}
	b.reports = append(b.reports, FieldReport{
		Tag: sourcePath, Path: ":" + tag + ":", Fidelity: FidelityMapped, Value: trimmed,
	})
}

// setLines writes a multi-line field, wrapping to the MT line width.
func (b *mtBuilder) setLines(tag, sourcePath string, lines []string, maxLines, width int) {
	wrapped := wrapLines(lines, width)
	if len(wrapped) == 0 {
		return
	}

	dropped := 0
	if len(wrapped) > maxLines {
		dropped = len(wrapped) - maxLines
		wrapped = wrapped[:maxLines]
	}
	b.fields = append(b.fields, ":"+tag+":"+strings.Join(wrapped, "\n"))

	if dropped > 0 {
		b.reports = append(b.reports, FieldReport{
			Tag: sourcePath, Path: ":" + tag + ":", Fidelity: FidelityTruncated,
			Note: fmt.Sprintf("field %s permits %d lines of %d characters; %d line(s) did not fit",
				tag, maxLines, width, dropped),
			Value: strings.Join(lines, " / "),
		})
		return
	}
	b.reports = append(b.reports, FieldReport{
		Tag: sourcePath, Path: ":" + tag + ":", Fidelity: FidelityMapped,
		Value: strings.Join(wrapped, " / "),
	})
}

func (b *mtBuilder) derived(tag, note string) {
	b.reports = append(b.reports, FieldReport{
		Tag: "(derived)", Path: ":" + tag + ":", Fidelity: FidelityDerived, Note: note,
	})
}

// lost records a source element MT cannot carry at all.
func (b *mtBuilder) lost(sourcePath, note, value string) {
	b.reports = append(b.reports, FieldReport{
		Tag: sourcePath, Fidelity: FidelityUnmapped, Note: note, Value: value,
	})
}

// message wraps the text block in the header and trailer blocks.
func (b *mtBuilder) message(msgType, sender, receiver, uetr string) string {
	var out strings.Builder

	fmt.Fprintf(&out, "{1:F01%s0000000000}", logicalTerminal(sender, "A"))
	fmt.Fprintf(&out, "{2:I%s%sN}", msgType, logicalTerminal(receiver, "X"))
	if uetr != "" {
		fmt.Fprintf(&out, "{3:{121:%s}}", uetr)
	}

	out.WriteString("{4:\n")
	for _, f := range b.fields {
		out.WriteString(f + "\n")
	}
	out.WriteString("-}")
	return out.String()
}

// logicalTerminal expands a BIC into the twelve-character address a header
// carries: eight characters, a terminal identifier, then the branch.
func logicalTerminal(bic, terminal string) string {
	b := strings.ToUpper(strings.TrimSpace(bic))
	if b == "" {
		b = "NOTPROVD"
	}
	for len(b) < 8 {
		b += "X"
	}
	branch := "XXX"
	if len(b) >= 11 {
		branch = b[8:11]
	}
	return b[:8] + terminal + branch
}

// wrapLines folds text to the MT line width, dropping anything empty.
func wrapLines(lines []string, width int) []string {
	var out []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		for len([]rune(line)) > width {
			r := []rune(line)
			out = append(out, string(r[:width]))
			line = strings.TrimSpace(string(r[width:]))
		}
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Reading an MX document
// ---------------------------------------------------------------------------

// find returns the first descendant with a name, and its path.
func find(n *converter.Node, name string) (*converter.Node, string, bool) {
	var found *converter.Node
	var path string

	var walk func(*converter.Node, string)
	walk = func(cur *converter.Node, cur1 string) {
		if found != nil {
			return
		}
		if cur.Name == name {
			found, path = cur, cur1
			return
		}
		for _, c := range cur.Children {
			walk(c, cur1+"/"+c.Name)
		}
	}
	walk(n, "/"+n.Name)

	return found, path, found != nil
}

// child returns a direct child by name.
func child(n *converter.Node, name string) (*converter.Node, bool) {
	if n == nil {
		return nil, false
	}
	for _, c := range n.Children {
		if c.Name == name {
			return c, true
		}
	}
	return nil, false
}

// childrenNamed returns every direct child with a name.
func childrenNamed(n *converter.Node, name string) []*converter.Node {
	if n == nil {
		return nil
	}
	var out []*converter.Node
	for _, c := range n.Children {
		if c.Name == name {
			out = append(out, c)
		}
	}
	return out
}

// text returns the trimmed text of a direct child.
func text(n *converter.Node, name string) string {
	if c, ok := child(n, name); ok {
		return strings.TrimSpace(c.Text)
	}
	return ""
}

// deepText returns the trimmed text of the first descendant with a name.
func deepText(n *converter.Node, name string) string {
	if c, _, ok := find(n, name); ok {
		return strings.TrimSpace(c.Text)
	}
	return ""
}

// attr returns an attribute value.
func attr(n *converter.Node, name string) string {
	if n == nil {
		return ""
	}
	for _, a := range n.Attrs {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

// agentBIC reads the BIC out of an agent element.
func agentBIC(n *converter.Node, name string) string {
	agent, ok := child(n, name)
	if !ok {
		return ""
	}
	return deepText(agent, "BICFI")
}

// accountID reads an account identifier, IBAN or otherwise.
func accountID(n *converter.Node, name string) string {
	acct, ok := child(n, name)
	if !ok {
		return ""
	}
	if iban := deepText(acct, "IBAN"); iban != "" {
		return iban
	}
	if othr, _, ok := find(acct, "Othr"); ok {
		return text(othr, "Id")
	}
	return ""
}

// mxParty is a party read out of an MX document, with everything MT cannot
// carry noted alongside.
type mxParty struct {
	Name    string
	Account string
	// Address is the postal address flattened to lines, which is what MT has.
	Address []string
	// Structured records that the source address was structured, so the loss
	// can be reported rather than passing unnoticed.
	Structured bool
	// LEI and other identifiers MT has no field for.
	LEI string
}

// readParty flattens an MX party into what an MT field can hold.
func readParty(parent *converter.Node, name string) (mxParty, bool) {
	node, ok := child(parent, name)
	if !ok {
		return mxParty{}, false
	}

	p := mxParty{Name: text(node, "Nm")}
	p.LEI = deepText(node, "LEI")

	if addr, ok := child(node, "PstlAdr"); ok {
		// A structured address has to be flattened into address lines, which is
		// the loss the 2026 rules exist to prevent going the other way.
		var structured []string
		for _, field := range []string{"StrtNm", "BldgNb", "PstCd", "TwnNm", "CtrySubDvsn", "Ctry"} {
			if v := text(addr, field); v != "" {
				structured = append(structured, v)
				p.Structured = true
			}
		}
		for _, line := range childrenNamed(addr, "AdrLine") {
			if v := strings.TrimSpace(line.Text); v != "" {
				p.Address = append(p.Address, v)
			}
		}
		if len(structured) > 0 {
			p.Address = append(p.Address, strings.Join(structured, " "))
		}
	}
	return p, true
}

// partyLinesFor renders a party as the lines an MT party field carries: an
// account on the first line, then the name and address.
func partyLinesFor(p mxParty) []string {
	var lines []string
	if p.Account != "" {
		lines = append(lines, "/"+p.Account)
	}
	if p.Name != "" {
		lines = append(lines, p.Name)
	}
	return append(lines, p.Address...)
}

// reportPartyLosses records what an MT party field cannot carry.
func (b *mtBuilder) reportPartyLosses(p mxParty, sourcePath string) {
	if p.Structured {
		b.reports = append(b.reports, FieldReport{
			Tag: sourcePath + "/PstlAdr", Fidelity: FidelityTruncated,
			Note: "the address was structured and MT has only free-text lines; " +
				"converting back will not restore TwnNm and Ctry",
			Value: strings.Join(p.Address, " / "),
		})
	}
	if p.LEI != "" {
		b.lost(sourcePath+"/Id/OrgId/LEI",
			"MT has no field for a legal entity identifier", p.LEI)
	}
}
