// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package converter

import (
	"encoding/json"
	"encoding/xml"
	"io"
	"strings"
	"testing"
)

func TestRepeatedSiblingsBecomeArrays(t *testing.T) {
	out, err := XMLToJSON([]byte(`<Doc><Tx>1</Tx><Tx>2</Tx><Tx>3</Tx></Doc>`))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	doc := m["Doc"].(map[string]any)
	list, ok := doc["Tx"].([]any)
	if !ok {
		t.Fatalf("repeated elements should become an array, got %T", doc["Tx"])
	}
	if len(list) != 3 {
		t.Errorf("got %d entries, want 3", len(list))
	}
}

func TestAttributesAndMixedContent(t *testing.T) {
	out, err := XMLToJSON([]byte(`<Doc><Amt Ccy="EUR">10.00</Amt><Empty/></Doc>`))
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, `"@Ccy"`) {
		t.Errorf("attributes should be prefixed with @: %s", s)
	}
	if !strings.Contains(s, `"#text"`) {
		t.Errorf("text beside an attribute should be kept: %s", s)
	}
}

func TestXMLToJSONRejectsMalformed(t *testing.T) {
	for _, in := range []string{"<not-closed>", "", "{{{"} {
		if _, err := XMLToJSON([]byte(in)); err == nil {
			t.Errorf("malformed input should be rejected: %q", in)
		}
	}
}

func TestJSONToXMLShapes(t *testing.T) {
	in := `{"Doc":{"@xmlns":"urn:t","Str":"a","Num":1.5,"Bool":true,
	         "Nested":{"Inner":"x"},"List":["one","two"],"WithAttr":{"@k":"v","#text":"body"}}}`
	out, err := JSONToXML([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{`xmlns="urn:t"`, "<Str>a</Str>", "1.5", "true",
		"<Inner>x</Inner>", "<List>one</List>", "<List>two</List>", `k="v"`, ">body<"} {
		if !strings.Contains(s, want) {
			t.Errorf("output should contain %q:\n%s", want, s)
		}
	}
}

func TestJSONToXMLRejectsMalformed(t *testing.T) {
	if _, err := JSONToXML([]byte("{{{")); err == nil {
		t.Error("malformed JSON should be rejected")
	}
}

func TestDeeplyNestedRoundTrip(t *testing.T) {
	in := `<A><B><C><D>leaf</D></C></B></A>`
	j, err := XMLToJSON([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	x, err := JSONToXML(j)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(x), "<D>leaf</D>") {
		t.Errorf("the leaf should survive:\n%s", x)
	}
}

func TestErrInterleavedMessage(t *testing.T) {
	e := &ErrInterleaved{Parent: "Doc", Child: "Tx"}
	msg := e.Error()
	if !strings.Contains(msg, "Tx") || !strings.Contains(msg, "Doc") {
		t.Errorf("the message should name both elements: %s", msg)
	}
}

func TestParseRejectsStructuralProblems(t *testing.T) {
	cases := map[string]string{
		"two roots":   `<A/><B/>`,
		"no elements": `just text`,
		"unclosed":    `<A>`,
		"entity":      `<!DOCTYPE d [<!ENTITY x "y">]><A>&x;</A>`,
	}
	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse([]byte(doc)); err == nil {
				t.Errorf("Parse should reject %s", name)
			}
		})
	}
}

func TestParseKeepsDocumentOrder(t *testing.T) {
	root, err := Parse([]byte(`<A><C/><B/><D>x</D></A>`))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, c := range root.Children {
		names = append(names, c.Name)
	}
	if strings.Join(names, ",") != "C,B,D" {
		t.Errorf("order = %v, want C,B,D", names)
	}
	if root.Children[2].Text != "x" {
		t.Errorf("leaf text lost: %q", root.Children[2].Text)
	}
}

func TestJSONToXMLRejectsStructuralProblems(t *testing.T) {
	cases := map[string]string{
		"not an object":    `["a","b"]`,
		"scalar at top":    `"hello"`,
		"empty object":     `{}`,
		"truncated object": `{"A":`,
		"truncated array":  `{"A":[1,`,
		"bad token":        `{"A":@}`,
		"unterminated":     `{"A":{"B":"c"}`,
		"multiple roots":   `{"A":"1","B":"2"}`,
		"trailing value":   `{"A":"1"} {"B":"2"}`,
	}
	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := JSONToXML([]byte(doc)); err == nil {
				t.Errorf("JSONToXML should reject %s", name)
			}
		})
	}
}

func TestRoundTripPreservesNamespacePrefixes(t *testing.T) {
	src := []byte(`<env:Envelope xmlns:env="urn:envelope" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns:p="urn:payment"><p:Document xsi:nil="true"><p:Amt Ccy="EUR">1.00</p:Amt></p:Document></env:Envelope>`)
	j, err := XMLToJSON(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"env:Envelope"`, `"@xmlns:env"`, `"@xmlns:xsi"`, `"p:Document"`, `"@xsi:nil"`} {
		if !strings.Contains(string(j), want) {
			t.Errorf("namespace name %s missing from JSON:\n%s", want, j)
		}
	}
	back, err := JSONToXML(j)
	if err != nil {
		t.Fatal(err)
	}
	dec := xml.NewDecoder(strings.NewReader(string(back)))
	var sawEnvelope, sawDocument, sawNil bool
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("round-tripped XML is malformed: %v\n%s", err, back)
		}
		if start, ok := tok.(xml.StartElement); ok {
			if start.Name.Local == "Envelope" && start.Name.Space == "urn:envelope" {
				sawEnvelope = true
			}
			if start.Name.Local == "Document" && start.Name.Space == "urn:payment" {
				sawDocument = true
				for _, attr := range start.Attr {
					if attr.Name.Local == "nil" && attr.Name.Space == "http://www.w3.org/2001/XMLSchema-instance" {
						sawNil = true
					}
				}
			}
		}
	}
	if !sawEnvelope || !sawDocument || !sawNil {
		t.Errorf("namespace semantics changed: envelope=%v document=%v nil=%v\n%s", sawEnvelope, sawDocument, sawNil, back)
	}
}

func TestJSONToXMLScalarKinds(t *testing.T) {
	out, err := JSONToXML([]byte(`{"Doc":{"S":"s","N":1.5,"I":42,"B":true,"Null":null}}`))
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{"<S>s</S>", "<N>1.5</N>", "<I>42</I>", "<B>true</B>", "<Null></Null>"} {
		if !strings.Contains(s, want) {
			t.Errorf("output should contain %q:\n%s", want, s)
		}
	}
}

// Numbers keep their original notation so a monetary amount does not lose its
// scale on the way back to XML.
func TestAmountScaleSurvives(t *testing.T) {
	out, err := JSONToXML([]byte(`{"Doc":{"Amt":25000.00,"Rate":1.500}}`))
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "<Amt>25000.00</Amt>") {
		t.Errorf("the amount lost its scale:\n%s", s)
	}
	if !strings.Contains(s, "<Rate>1.500</Rate>") {
		t.Errorf("the rate lost its scale:\n%s", s)
	}
}

func TestXMLToJSONEscapesSpecialCharacters(t *testing.T) {
	out, err := XMLToJSON([]byte(`<A><B>quote " and \ backslash</B></A>`))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
}

func TestInterleavedDetectedWhenNested(t *testing.T) {
	// The repeat is inside a child, not at the root.
	_, err := XMLToJSON([]byte(`<Doc><Wrap><A>1</A><B>2</B><A>3</A></Wrap></Doc>`))
	if err == nil {
		t.Fatal("a nested interleave should also be refused")
	}
	if !strings.Contains(err.Error(), "Wrap") {
		t.Errorf("the error should name the containing element: %v", err)
	}
}

func TestArrayOfObjectsRoundTrips(t *testing.T) {
	src := `<Doc><Tx><Id>1</Id></Tx><Tx><Id>2</Id></Tx></Doc>`
	j, err := XMLToJSON([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(j), "[") {
		t.Errorf("repeats should become an array:\n%s", j)
	}
	back, err := JSONToXML(j)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(back), "<Tx>") != 2 {
		t.Errorf("both entries should survive:\n%s", back)
	}
}

func TestParseRetainsResolvedNamespaces(t *testing.T) {
	root, err := Parse([]byte(`<e:Envelope xmlns:e="urn:env" xmlns="urn:business"><AppHdr xmlns="urn:header"><BizSvc>x</BizSvc></AppHdr><Document><p:Foreign xmlns:p="urn:foreign"/></Document></e:Envelope>`))
	if err != nil {
		t.Fatal(err)
	}
	if root.Space != "urn:env" {
		t.Errorf("Envelope namespace = %q", root.Space)
	}
	header := root.Children[0]
	if header.Space != "urn:header" || header.Children[0].Space != "urn:header" {
		t.Errorf("default header namespace was not inherited: %+v", header)
	}
	document := root.Children[1]
	if document.Space != "urn:business" {
		t.Errorf("Document namespace = %q", document.Space)
	}
	if document.Children[0].Space != "urn:foreign" {
		t.Errorf("prefixed namespace = %q", document.Children[0].Space)
	}
}
