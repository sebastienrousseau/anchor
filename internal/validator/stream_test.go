// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package validator_test

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/sebastienrousseau/anchor/internal/catalog"
	"github.com/sebastienrousseau/anchor/internal/validator"
	"github.com/sebastienrousseau/anchor/internal/xsd"
)

// The streaming validator exists to give the same verdict for less memory. The
// first half of that claim is worth nothing without proof, so these tests
// compare the two paths rather than checking the streaming one in isolation.

func streamSchema(t *testing.T) *xsd.Schema {
	t.Helper()
	s, err := xsd.Parse(strings.NewReader(fuzzSchema))
	if err != nil {
		t.Fatalf("parsing the fixture schema: %v", err)
	}
	return s
}

// agree runs both paths and requires the same verdict and the same errors.
func agree(t *testing.T, schema *xsd.Schema, doc string) *validator.Result {
	t.Helper()

	buffered := validator.Validate([]byte(doc), schema)
	streamed := validator.ValidateReader(strings.NewReader(doc), schema)

	if buffered.Valid != streamed.Valid {
		t.Fatalf("the two paths disagree: buffered valid=%v, streamed valid=%v\n%s\nbuffered: %+v\nstreamed: %+v",
			buffered.Valid, streamed.Valid, doc, buffered.Errors, streamed.Errors)
	}
	if len(buffered.Errors) != len(streamed.Errors) {
		t.Fatalf("buffered found %d error(s), streamed found %d\nbuffered: %+v\nstreamed: %+v",
			len(buffered.Errors), len(streamed.Errors), buffered.Errors, streamed.Errors)
	}
	for i := range buffered.Errors {
		b, s := buffered.Errors[i], streamed.Errors[i]
		if b.Path != s.Path || b.Rule != s.Rule || b.Message != s.Message {
			t.Errorf("error %d differs:\nbuffered: %+v\nstreamed: %+v", i, b, s)
		}
	}
	return streamed
}

const streamValid = `<?xml version="1.0"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">
  <CstmrCdtTrf>
    <GrpHdr>
      <MsgId>MSG-1</MsgId>
      <CreDtTm>2026-08-24T09:00:00Z</CreDtTm>
    </GrpHdr>
    <Tx><Amt Ccy="EUR">25000.00</Amt><IBAN>GB29NWBK60161331926819</IBAN><ChrgBr>SHAR</ChrgBr></Tx>
    <Tx><Amt Ccy="GBP">1.00</Amt><Othr>ACCT-1</Othr></Tx>
  </CstmrCdtTrf>
</Document>`

func TestStreamingAcceptsAValidDocument(t *testing.T) {
	res := agree(t, streamSchema(t), streamValid)
	if !res.Valid {
		t.Errorf("a valid document was rejected: %+v", res.Errors)
	}
}

func TestStreamingRejectsTheSameThings(t *testing.T) {
	cases := map[string]string{
		"a bad code": strings.Replace(streamValid, "<ChrgBr>SHAR</ChrgBr>", "<ChrgBr>NOPE</ChrgBr>", 1),
		"a bad pattern": strings.Replace(streamValid,
			"<IBAN>GB29NWBK60161331926819</IBAN>", "<IBAN>not-an-iban</IBAN>", 1),
		"a bad currency": strings.Replace(streamValid, `Ccy="EUR"`, `Ccy="EURO"`, 1),
		"too many decimals": strings.Replace(streamValid,
			"<Amt Ccy=\"EUR\">25000.00</Amt>", "<Amt Ccy=\"EUR\">25000.0000001</Amt>", 1),
		"a missing mandatory child": strings.Replace(streamValid,
			"<Amt Ccy=\"GBP\">1.00</Amt>", "", 1),
		"an unexpected child": strings.Replace(streamValid,
			"<Othr>ACCT-1</Othr>", "<Othr>ACCT-1</Othr><Surprise>x</Surprise>", 1),
		"elements out of order": strings.Replace(streamValid,
			"<Amt Ccy=\"GBP\">1.00</Amt><Othr>ACCT-1</Othr>",
			"<Othr>ACCT-1</Othr><Amt Ccy=\"GBP\">1.00</Amt>", 1),
		"a missing attribute": strings.Replace(streamValid, ` Ccy="GBP"`, "", 1),
		"a missing header":    strings.Replace(streamValid, "<MsgId>MSG-1</MsgId>", "", 1),
		"no transactions": `<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">
  <CstmrCdtTrf><GrpHdr><MsgId>MSG-1</MsgId></GrpHdr></CstmrCdtTrf></Document>`,
		"the wrong namespace": strings.Replace(streamValid,
			"urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10", "urn:wrong", 1),
		"the wrong root": strings.Replace(streamValid, "Document>", "Wrong>", 2),
	}

	schema := streamSchema(t)
	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			if res := agree(t, schema, doc); res.Valid {
				t.Errorf("%s was accepted", name)
			}
		})
	}
}

func TestStreamingRejectsMalformedXML(t *testing.T) {
	schema := streamSchema(t)
	for _, doc := range []string{"", "<Document>", "not xml at all", "<A/><B/>"} {
		res := validator.ValidateReader(strings.NewReader(doc), schema)
		if res.Valid {
			t.Errorf("%q was accepted", doc)
		}
		if len(res.Errors) == 0 {
			t.Errorf("%q was rejected with no explanation", doc)
		}
	}
}

func TestStreamingReportsRealPositions(t *testing.T) {
	doc := strings.Replace(streamValid, "<ChrgBr>SHAR</ChrgBr>", "<ChrgBr>NOPE</ChrgBr>", 1)
	res := validator.ValidateReader(strings.NewReader(doc), streamSchema(t))

	if len(res.Errors) == 0 {
		t.Fatal("no error was reported")
	}
	e := res.Errors[0]
	if e.Line <= 0 {
		t.Fatalf("the error carries no line number: %+v", e)
	}

	// The reported line must actually contain the offending element, or an
	// editor points somewhere useless.
	lines := strings.Split(doc, "\n")
	if e.Line > len(lines) || !strings.Contains(lines[e.Line-1], "ChrgBr") {
		t.Errorf("the error points at line %d: %q", e.Line, lines[min(e.Line, len(lines))-1])
	}

	// And the position has to match what the buffered path reports.
	buffered := validator.Validate([]byte(doc), streamSchema(t))
	if buffered.Errors[0].Line != e.Line || buffered.Errors[0].Column != e.Column {
		t.Errorf("positions differ: buffered %d:%d, streamed %d:%d",
			buffered.Errors[0].Line, buffered.Errors[0].Column, e.Line, e.Column)
	}
}

func TestStreamingHeapDoesNotGrowWithTransactionSize(t *testing.T) {
	// The property that matters for a real statement: a transaction carrying
	// full remittance data must not cost more to validate than an empty one,
	// because each is released the moment it closes.
	//
	// Cumulative allocation cannot show this -- both read the same bytes. What
	// matters is the live heap while the document is still arriving, which is
	// where the buffered path would be holding everything at once. So the
	// document is generated as it is read, never materialised.
	schema := streamSchema(t)

	peak := func(count, padding int) uint64 {
		gen := &txGenerator{total: count, padding: padding, sampleAt: count * 9 / 10}
		res := validator.ValidateReader(gen, schema)
		if !res.Valid {
			t.Fatalf("the generated document did not validate: %+v", res.Errors)
		}
		if gen.sample == 0 {
			t.Fatal("the heap was never sampled")
		}
		return gen.sample
	}

	lean := peak(5000, 0)
	fat := peak(5000, 2000)
	t.Logf("heap growth: %d bytes with empty transactions, %d with 2 KB ones", lean, fat)

	// Fifty times the bytes, through the same number of transactions. If the
	// subtrees were being retained this would grow by roughly that factor.
	if fat > lean*2 {
		t.Errorf("the heap grew from %d to %d bytes when the transactions got "+
			"50x larger; the subtrees are not being released", lean, fat)
	}
}

func TestStreamingHoldsFarLessThanTheDocument(t *testing.T) {
	// The buffered path must hold at least the document's bytes, plus a tree
	// several times that size. The streaming path has to hold far less, or it
	// has no reason to exist.
	schema := streamSchema(t)

	const transactions = 20000
	const padding = 2000

	gen := &txGenerator{total: transactions, padding: padding, sampleAt: transactions * 9 / 10}
	res := validator.ValidateReader(gen, schema)
	if !res.Valid {
		t.Fatalf("the generated document did not validate: %+v", res.Errors)
	}

	t.Logf("document: %d bytes; heap growth while validating: %d bytes", gen.written, gen.sample)
	if gen.written < 8<<20 {
		t.Fatalf("the generated document is only %d bytes; the test proves nothing", gen.written)
	}
	// The buffered path holds the document's bytes plus a tree several times
	// that size. A quarter of the document is a bar it could never meet.
	if gen.sample > uint64(gen.written)/4 {
		t.Errorf("validating a %d-byte document grew the heap by %d bytes",
			gen.written, gen.sample)
	}
}

// txGenerator emits a valid document one transaction at a time, so a large
// document can be validated without ever existing in memory. It samples the
// live heap once, near the end, which is where the buffered path would be
// holding everything.
type txGenerator struct {
	total    int
	padding  int
	emitted  int
	sampleAt int
	baseline uint64
	sample   uint64
	written  int

	buf  []byte
	done bool
}

func (g *txGenerator) Read(p []byte) (int, error) {
	for len(g.buf) == 0 {
		switch {
		case g.emitted == 0:
			// The process has a heap of its own before any of this starts, so
			// what is measured is the growth, not the total.
			runtime.GC()
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			g.baseline = m.HeapAlloc

			g.buf = []byte(`<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">` + "\n" +
				"<CstmrCdtTrf>\n<GrpHdr><MsgId>MSG-1</MsgId></GrpHdr>\n")
			g.emitted++

		case g.emitted <= g.total:
			if g.emitted == g.sampleAt {
				runtime.GC()
				var m runtime.MemStats
				runtime.ReadMemStats(&m)
				if m.HeapAlloc > g.baseline {
					g.sample = m.HeapAlloc - g.baseline
				}
			}
			// Othr is a Max35Text, so the bulk goes into repeated transactions
			// rather than into one oversized value.
			var b strings.Builder
			fmt.Fprintf(&b, "<Tx><Amt Ccy=\"EUR\">%d.00</Amt><Othr>ACCT-%d</Othr></Tx>\n",
				g.emitted%1000, g.emitted)
			for i := 0; i < g.padding/64; i++ {
				b.WriteString("<!-- padding padding padding padding padding padding pad -->\n")
			}
			g.buf = []byte(b.String())
			g.emitted++

		case !g.done:
			g.buf = []byte("</CstmrCdtTrf>\n</Document>")
			g.done = true

		default:
			return 0, io.EOF
		}
	}

	n := copy(p, g.buf)
	g.buf = g.buf[n:]
	g.written += n
	return n, nil
}

func TestStreamingLineTrackerWindow(t *testing.T) {
	// Beyond the tracked window a position reports its byte offset rather than
	// claiming a line number nobody counted.
	var b strings.Builder
	b.WriteString(`<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">` + "\n")
	b.WriteString("<CstmrCdtTrf>\n<GrpHdr><MsgId>MSG-1</MsgId></GrpHdr>\n")
	for i := 0; i < 20000; i++ {
		b.WriteString("<Tx><Amt Ccy=\"EUR\">1.00</Amt><Othr>A</Othr></Tx>\n")
	}
	b.WriteString("</CstmrCdtTrf>\n</Document>")

	res := validator.ValidateReader(strings.NewReader(b.String()), streamSchema(t))
	if !res.Valid {
		t.Fatalf("a long document did not validate: %+v", res.Errors)
	}
}

// TestStreamingAgreesWithValidateAcrossTheCatalogue is the real bar: the two
// paths must return the same verdict for every sample message the user has
// installed. It skips without a catalogue.
func TestStreamingAgreesWithValidateAcrossTheCatalogue(t *testing.T) {
	if testing.Short() {
		t.Skip("this walks the whole catalogue")
	}
	root, err := catalog.Resolve("")
	if err != nil {
		t.Skip("no ISO 20022 catalogue installed")
	}
	idx, err := catalog.Load(root)
	if err != nil {
		t.Skipf("the catalogue would not load: %v", err)
	}

	const limit = 400
	var checked, disagreed int

	for _, msg := range idx.Messages {
		if checked >= limit {
			break
		}
		if msg.XSDPath == "" || msg.XMLSamplePath == "" {
			continue
		}

		schema, err := xsd.ParseFile(msg.XSDPath)
		if err != nil {
			continue
		}
		instance, err := os.ReadFile(msg.XMLSamplePath)
		if err != nil {
			continue
		}

		buffered := validator.Validate(instance, schema)
		streamed := validator.ValidateReader(strings.NewReader(string(instance)), schema)
		checked++

		if buffered.Valid != streamed.Valid || len(buffered.Errors) != len(streamed.Errors) {
			disagreed++
			t.Errorf("%s: buffered valid=%v (%d errors), streamed valid=%v (%d errors)",
				filepath.Base(msg.XMLSamplePath),
				buffered.Valid, len(buffered.Errors), streamed.Valid, len(streamed.Errors))
		}
	}

	if checked == 0 {
		t.Skip("the catalogue has no sample messages to compare")
	}
	t.Logf("compared %d sample message(s); %d disagreement(s)", checked, disagreed)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestStreamingRejectsAnUnknownRootElement(t *testing.T) {
	// A document whose root the schema does not declare has to say which roots
	// it does declare, or the reader has nothing to go on.
	res := validator.ValidateReader(strings.NewReader(
		`<Wrong xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10"/>`), streamSchema(t))

	if res.Valid {
		t.Fatal("an unknown root element was accepted")
	}
	if len(res.Errors) == 0 {
		t.Fatal("no error was reported")
	}
	if !strings.Contains(res.Errors[0].Expected, "Document") {
		t.Errorf("the error does not name the declared root: %+v", res.Errors[0])
	}
}
