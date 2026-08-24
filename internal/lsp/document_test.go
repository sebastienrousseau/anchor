// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package lsp_test

import (
	"strings"
	"testing"

	"github.com/sebastienrousseau/anchor/internal/lsp"
)

const instance = `<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">
  <FIToFICstmrCdtTrf>
    <GrpHdr>
      <MsgId>MSG-0001</MsgId>
      <CreDtTm>2026-08-24T09:00:00Z</CreDtTm>
    </GrpHdr>
    <CdtTrfTxInf>
      <ChrgBr>SHAR</ChrgBr>
      <Cdtr>
        <Nm>MUELLER GMBH</Nm>
      </Cdtr>
    </CdtTrfTxInf>
  </FIToFICstmrCdtTrf>
</Document>`

func TestParseIndexesPaths(t *testing.T) {
	doc := lsp.Parse(instance)

	if !doc.Wellformed {
		t.Fatalf("the fixture did not parse: %s", doc.ParseError)
	}
	if doc.Root() != "Document" {
		t.Errorf("Root = %q", doc.Root())
	}

	msgID := doc.ByPath("/Document/FIToFICstmrCdtTrf/GrpHdr/MsgId")
	if len(msgID) != 1 {
		t.Fatalf("got %d MsgId elements, want 1", len(msgID))
	}
	if msgID[0].Value != "MSG-0001" {
		t.Errorf("value = %q", msgID[0].Value)
	}
	if !msgID[0].HasValue() {
		t.Error("HasValue is false for an element with text")
	}
	if msgID[0].Depth != 3 {
		t.Errorf("Depth = %d, want 3", msgID[0].Depth)
	}

	// A container has no text of its own.
	if grp := doc.ByPath("/Document/FIToFICstmrCdtTrf/GrpHdr"); grp[0].HasValue() {
		t.Errorf("GrpHdr was given a value: %q", grp[0].Value)
	}

	if byName := doc.ByName("Nm"); len(byName) != 1 || byName[0].Value != "MUELLER GMBH" {
		t.Errorf("ByName(Nm) = %+v", byName)
	}
	if len(doc.ByPath("/no/such/path")) != 0 {
		t.Error("an unknown path matched")
	}
}

func TestValueRangeExcludesWhitespace(t *testing.T) {
	doc := lsp.Parse(instance)
	el := doc.ByPath("/Document/FIToFICstmrCdtTrf/GrpHdr/MsgId")[0]

	// The offsets must bracket the value itself, or an editor underlines the
	// indentation along with it.
	if got := doc.Text[el.ValueStart:el.ValueEnd]; got != "MSG-0001" {
		t.Errorf("the value span is %q", got)
	}

	rng := doc.RangeOf(el)
	line := strings.Split(instance, "\n")[rng.Start.Line]
	if !strings.Contains(line, "MSG-0001") {
		t.Errorf("the range points at line %d: %q", rng.Start.Line, line)
	}
	if rng.End.Character-rng.Start.Character != len("MSG-0001") {
		t.Errorf("the range spans %d characters", rng.End.Character-rng.Start.Character)
	}
}

func TestRangeOfEmptyElement(t *testing.T) {
	doc := lsp.Parse(instance)
	el := doc.ByPath("/Document/FIToFICstmrCdtTrf/GrpHdr")[0]

	// With no value to point at, the start tag is what gets highlighted.
	rng := doc.RangeOf(el)
	if rng.Start == rng.End {
		t.Error("an empty range was produced for a container element")
	}
}

func TestPositionsUseUTF16(t *testing.T) {
	// The protocol counts characters in UTF-16 code units, so a name outside
	// the basic plane must not shift every column after it.
	doc := lsp.Parse(`<Document><Nm>M` + "\U0001F600" + `LLER</Nm></Document>`)
	el := doc.ByName("Nm")[0]

	rng := doc.RangeOf(el)
	// "M" plus a surrogate pair plus "LLER" is 7 UTF-16 units.
	if got := rng.End.Character - rng.Start.Character; got != 7 {
		t.Errorf("the value spans %d UTF-16 units, want 7", got)
	}

	// Converting back must land on the same byte offset.
	if off := doc.OffsetAt(rng.Start); off != el.ValueStart {
		t.Errorf("OffsetAt round trip = %d, want %d", off, el.ValueStart)
	}
	if off := doc.OffsetAt(rng.End); off != el.ValueEnd {
		t.Errorf("OffsetAt round trip = %d, want %d", off, el.ValueEnd)
	}
}

func TestPositionRoundTrip(t *testing.T) {
	doc := lsp.Parse(instance)
	for _, offset := range []int{0, 40, 120, len(instance) - 1, len(instance)} {
		if got := doc.OffsetAt(doc.PositionAt(offset)); got != offset {
			t.Errorf("offset %d round-tripped to %d", offset, got)
		}
	}

	// Out-of-range inputs are clamped rather than panicking, because an editor
	// sends positions from a document state the server may not have yet.
	if p := doc.PositionAt(-5); p.Line != 0 || p.Character != 0 {
		t.Errorf("PositionAt(-5) = %+v", p)
	}
	if p := doc.PositionAt(len(instance) + 100); p.Line == 0 {
		t.Errorf("PositionAt past the end = %+v", p)
	}
	if off := doc.OffsetAt(lsp.Position{Line: -1}); off != 0 {
		t.Errorf("OffsetAt of a negative line = %d", off)
	}
	if off := doc.OffsetAt(lsp.Position{Line: 9999}); off != len(instance) {
		t.Errorf("OffsetAt past the end = %d", off)
	}
	if off := doc.OffsetAt(lsp.Position{Line: 1, Character: 9999}); off == 0 {
		t.Error("a character past the end of a line collapsed to zero")
	}
}

func TestElementAt(t *testing.T) {
	doc := lsp.Parse(instance)
	chrgBr := doc.ByName("ChrgBr")[0]

	// Inside the start tag.
	if el, ok := doc.ElementAt(chrgBr.TagStart + 2); !ok || el.Name != "ChrgBr" {
		t.Errorf("ElementAt in the tag = %+v, %v", el, ok)
	}
	// Inside the value.
	if el, ok := doc.ElementAt(chrgBr.ValueStart + 1); !ok || el.Name != "ChrgBr" {
		t.Errorf("ElementAt in the value = %+v, %v", el, ok)
	}
	// Before anything.
	if _, ok := doc.ElementAt(0); ok {
		t.Error("ElementAt matched inside the XML declaration")
	}
}

func TestEnclosingPath(t *testing.T) {
	doc := lsp.Parse(instance)
	grpHdr := doc.ByPath("/Document/FIToFICstmrCdtTrf/GrpHdr")[0]

	// A cursor just after <GrpHdr> is asking what belongs inside it.
	got := doc.EnclosingPath(grpHdr.TagEnd + 1)
	if got != "/Document/FIToFICstmrCdtTrf/GrpHdr" {
		t.Errorf("EnclosingPath = %q", got)
	}
	if doc.EnclosingPath(0) != "" {
		t.Errorf("EnclosingPath before the root = %q", doc.EnclosingPath(0))
	}
}

func TestFindValue(t *testing.T) {
	doc := lsp.Parse(instance)

	if el, ok := doc.FindValue("Nm", "MUELLER GMBH"); !ok || el.Path != "/Document/FIToFICstmrCdtTrf/CdtTrfTxInf/Cdtr/Nm" {
		t.Errorf("FindValue = %+v, %v", el, ok)
	}
	// A field named by path fragment still resolves.
	if el, ok := doc.FindValue("Cdtr/Nm", "MUELLER GMBH"); !ok || el.Name != "Nm" {
		t.Errorf("FindValue by fragment = %+v, %v", el, ok)
	}
	// A value that does not match falls back to the first element of that name,
	// because underlining the right element with the wrong value beats
	// underlining nothing.
	if el, ok := doc.FindValue("Nm", "SOMETHING ELSE"); !ok || el.Name != "Nm" {
		t.Errorf("FindValue with a wrong value = %+v, %v", el, ok)
	}
	if _, ok := doc.FindValue("NoSuchElement", ""); ok {
		t.Error("FindValue matched an element that is not there")
	}
}

func TestMalformedDocumentIndexesWhatItCan(t *testing.T) {
	// An editor shows half-typed documents constantly; the index has to survive
	// them and say where it stopped.
	doc := lsp.Parse(`<Document><GrpHdr><MsgId>MSG-1</MsgId>`)
	if doc.Wellformed {
		t.Error("an unclosed document was reported as well-formed")
	}
	if doc.ParseError == "" {
		t.Error("no parse error was recorded")
	}
	if len(doc.ByName("MsgId")) != 1 {
		t.Error("the elements before the error were not indexed")
	}
}

func TestEmptyDocument(t *testing.T) {
	doc := lsp.Parse("")
	if doc.Root() != "" || len(doc.Elements) != 0 {
		t.Errorf("an empty document produced %+v", doc.Elements)
	}
	if rng := doc.LineRange(1, 0); rng.Start != rng.End {
		t.Errorf("LineRange on an empty document = %+v", rng)
	}
}

func TestLineRange(t *testing.T) {
	doc := lsp.Parse(instance)

	// Line 5 (one-based) is the MsgId line; a column of zero skips the indent.
	rng := doc.LineRange(5, 0)
	if rng.Start.Line != 4 {
		t.Errorf("Start.Line = %d, want 4", rng.Start.Line)
	}
	if rng.Start.Character == 0 {
		t.Error("the leading indentation was included")
	}

	// An explicit column starts there instead.
	if col := doc.LineRange(5, 8); col.Start.Character != 7 {
		t.Errorf("Start.Character = %d, want 7", col.Start.Character)
	}
	// A column past the end of the line is clamped.
	if far := doc.LineRange(5, 9999); far.Start != far.End {
		t.Errorf("a column past the line end = %+v", far)
	}
	// A line that does not exist yields an empty range rather than panicking.
	if none := doc.LineRange(9999, 1); none.Start != none.End {
		t.Errorf("LineRange past the end = %+v", none)
	}
	if none := doc.LineRange(0, 1); none.Start != none.End {
		t.Errorf("LineRange(0) = %+v", none)
	}
}
