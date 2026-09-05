// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

// Package converter translates between ISO 20022 XML and JSON.
//
// Document order is preserved in both directions. That is not a nicety: ISO
// 20022 complex types are xs:sequence, so the order of children is part of the
// contract. Marshalling through a Go map sorts keys alphabetically and silently
// produces a document that no longer validates, which is why this package
// carries its own ordered representation instead.
package converter

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Node is one element of an XML document, with its children in document order.
type Node struct {
	Name  string
	QName string
	// Space is the resolved namespace URI. QName preserves the spelling used
	// in the source; Space lets validators distinguish a real ISO element from
	// a foreign element that happens to use the same local name.
	Space    string
	Attrs    []xml.Attr
	Text     string
	Children []*Node
}

// ErrInterleaved reports sibling elements of the same name that are not
// adjacent. JSON objects cannot express that ordering, so rather than silently
// reordering the document the conversion stops.
type ErrInterleaved struct {
	Parent string
	Child  string
}

func (e *ErrInterleaved) Error() string {
	return fmt.Sprintf("<%s> repeats non-adjacently inside <%s>; JSON cannot represent that order",
		e.Child, e.Parent)
}

// Parse reads an XML document into an ordered tree.
func Parse(data []byte) (*Node, error) {
	if err := validateXML(data); err != nil {
		return nil, err
	}
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = true
	// encoding/xml never resolves external entities; this rejects internal ones
	// too, so a DTD cannot influence the conversion.
	dec.Entity = xml.HTMLEntity

	var root *Node
	var stack []*Node
	var namespaces []*namespaceFrame

	for {
		tok, err := dec.RawToken()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("xml decode error: %w", err)
		}

		switch t := tok.(type) {
		case xml.StartElement:
			// Go's decoder is lenient about names; the specification is not, and
			// a name AskISO accepts here is one it would have to emit later.
			// Refusing it now is better than producing JSON that cannot be
			// converted back.
			qname := rawName(t.Name)
			if !validXMLName(qname) {
				return nil, fmt.Errorf("xml decode error: %q is not a valid XML element name", qname)
			}
			for _, a := range t.Attr {
				if !validXMLName(rawName(a.Name)) {
					return nil, fmt.Errorf("xml decode error: %q is not a valid XML attribute name", rawName(a.Name))
				}
			}
			frame := &namespaceFrame{bindings: map[string]string{}}
			if len(namespaces) > 0 {
				frame.parent = namespaces[len(namespaces)-1]
			}
			for _, attr := range t.Attr {
				switch {
				case attr.Name.Space == "" && attr.Name.Local == "xmlns":
					frame.bindings[""] = attr.Value
				case attr.Name.Space == "xmlns":
					frame.bindings[attr.Name.Local] = attr.Value
				}
			}
			n := &Node{Name: t.Name.Local, QName: qname, Space: frame.resolve(t.Name.Space), Attrs: append([]xml.Attr(nil), t.Attr...)}
			if len(stack) == 0 {
				if root != nil {
					return nil, fmt.Errorf("document has more than one root element")
				}
				root = n
			} else {
				parent := stack[len(stack)-1]
				parent.Children = append(parent.Children, n)
			}
			stack = append(stack, n)
			namespaces = append(namespaces, frame)

		case xml.EndElement:
			if len(stack) == 0 || stack[len(stack)-1].QName != rawName(t.Name) {
				return nil, fmt.Errorf("xml decode error: unexpected closing element </%s>", rawName(t.Name))
			}
			stack = stack[:len(stack)-1]
			namespaces = namespaces[:len(namespaces)-1]

		case xml.CharData:
			if len(stack) > 0 {
				stack[len(stack)-1].Text += string(t)
			}
		}
	}

	if root == nil {
		return nil, fmt.Errorf("document has no elements")
	}
	if len(stack) != 0 {
		return nil, fmt.Errorf("xml decode error: unclosed element <%s>", stack[len(stack)-1].QName)
	}
	normalise(root)
	return root, nil
}

type namespaceFrame struct {
	parent   *namespaceFrame
	bindings map[string]string
}

func (f *namespaceFrame) resolve(prefix string) string {
	if prefix == "xml" {
		return "http://www.w3.org/XML/1998/namespace"
	}
	for current := f; current != nil; current = current.parent {
		if uri, ok := current.bindings[prefix]; ok {
			return uri
		}
	}
	return ""
}

func validateXML(data []byte) error {
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = true
	dec.Entity = xml.HTMLEntity
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("xml decode error: %w", err)
		}
		if start, ok := tok.(xml.StartElement); ok {
			if !validXMLName(start.Name.Local) {
				return fmt.Errorf("xml decode error: %q is not a valid XML element name", start.Name.Local)
			}
			for _, attr := range start.Attr {
				if !validXMLName(attr.Name.Local) {
					return fmt.Errorf("xml decode error: %q is not a valid XML attribute name", attr.Name.Local)
				}
			}
		}
	}
}

func rawName(name xml.Name) string {
	if name.Space != "" {
		return name.Space + ":" + name.Local
	}
	return name.Local
}

func nodeName(n *Node) string {
	if n.QName != "" {
		return n.QName
	}
	return n.Name
}

// normalise drops whitespace-only text. An element with children never carries a
// value in ISO 20022.
func normalise(n *Node) {
	if len(n.Children) > 0 {
		n.Text = ""
	} else {
		n.Text = strings.TrimSpace(n.Text)
	}
	for _, c := range n.Children {
		normalise(c)
	}
}

// XMLToJSON converts an ISO 20022 XML document to JSON, keeping element order.
//
// Attributes are prefixed with "@". An element with both a value and attributes
// carries its value under "#text". Adjacent repeats of the same element become a
// JSON array.
func XMLToJSON(xmlData []byte) ([]byte, error) {
	root, err := Parse(xmlData)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	buf.WriteString("{\n")
	if err := writeMember(&buf, nodeName(root), root, 1); err != nil {
		return nil, err
	}
	buf.WriteString("\n}")
	return buf.Bytes(), nil
}

// writeMember emits `"name": <value>` at the given indent, without a trailing
// newline so the caller controls separators.
func writeMember(buf *bytes.Buffer, name string, n *Node, depth int) error {
	writeIndent(buf, depth)
	buf.WriteString(quote(name))
	buf.WriteString(": ")
	return writeNode(buf, n, depth)
}

func writeNode(buf *bytes.Buffer, n *Node, depth int) error {
	// A leaf with no attributes is a bare string.
	if len(n.Children) == 0 && len(n.Attrs) == 0 {
		buf.WriteString(quote(n.Text))
		return nil
	}

	buf.WriteString("{\n")
	first := true

	emit := func(write func() error) error {
		if !first {
			buf.WriteString(",\n")
		}
		first = false
		return write()
	}

	for _, a := range n.Attrs {
		attr := a
		if err := emit(func() error {
			writeIndent(buf, depth+1)
			buf.WriteString(quote("@" + rawName(attr.Name)))
			buf.WriteString(": ")
			buf.WriteString(quote(attr.Value))
			return nil
		}); err != nil {
			return err
		}
	}

	if len(n.Children) == 0 {
		if n.Text != "" {
			if err := emit(func() error {
				writeIndent(buf, depth+1)
				buf.WriteString(quote("#text"))
				buf.WriteString(": ")
				buf.WriteString(quote(n.Text))
				return nil
			}); err != nil {
				return err
			}
		}
		buf.WriteString("\n")
		writeIndent(buf, depth)
		buf.WriteString("}")
		return nil
	}

	// Walk children in document order, collapsing adjacent same-name runs into
	// an array. A non-adjacent repeat cannot be represented and is an error.
	seen := map[string]bool{}
	for i := 0; i < len(n.Children); {
		name := nodeName(n.Children[i])
		if seen[name] {
			return &ErrInterleaved{Parent: n.Name, Child: name}
		}
		seen[name] = true

		j := i
		for j < len(n.Children) && nodeName(n.Children[j]) == name {
			j++
		}
		run := n.Children[i:j]

		if err := emit(func() error {
			if len(run) == 1 {
				return writeMember(buf, name, run[0], depth+1)
			}
			writeIndent(buf, depth+1)
			buf.WriteString(quote(name))
			buf.WriteString(": [\n")
			for k, child := range run {
				if k > 0 {
					buf.WriteString(",\n")
				}
				writeIndent(buf, depth+2)
				if err := writeNode(buf, child, depth+2); err != nil {
					return err
				}
			}
			buf.WriteString("\n")
			writeIndent(buf, depth+1)
			buf.WriteString("]")
			return nil
		}); err != nil {
			return err
		}
		i = j
	}

	buf.WriteString("\n")
	writeIndent(buf, depth)
	buf.WriteString("}")
	return nil
}

func writeIndent(buf *bytes.Buffer, depth int) {
	for i := 0; i < depth; i++ {
		buf.WriteString("  ")
	}
}

func quote(s string) string {
	b, _ := json.Marshal(s) // encoding a string cannot fail
	return string(b)
}

// JSONToXML converts JSON produced by XMLToJSON back into XML.
//
// Keys are emitted in the order the JSON document declares them, which is what
// makes the round trip faithful: Go's map type would sort them.
func JSONToXML(jsonData []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(jsonData))
	dec.UseNumber()

	value, err := decodeValue(dec)
	if err != nil {
		return nil, fmt.Errorf("json unmarshal error: %w", err)
	}
	if tok, err := dec.Token(); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("json unmarshal error: unexpected trailing token %v", tok)
		}
		return nil, fmt.Errorf("json unmarshal error: %w", err)
	}

	obj, ok := value.(*object)
	if !ok {
		return nil, fmt.Errorf("json unmarshal error: top level must be an object")
	}
	if len(obj.keys) == 0 {
		return nil, fmt.Errorf("json unmarshal error: document is empty")
	}
	if len(obj.keys) != 1 {
		return nil, fmt.Errorf("json unmarshal error: document must contain exactly one root element")
	}

	// A key that is not a valid XML name would produce a document AskISO could
	// not read back, which is worse than refusing it.
	if err := checkNames(obj); err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	buf.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	writeXMLNode(&buf, obj.keys[0], obj.values[0], 0)
	return buf.Bytes(), nil
}

// checkNames walks a decoded object and refuses any key that cannot become an
// XML element or attribute name.
func checkNames(v any) error {
	switch t := v.(type) {
	case *object:
		for i, k := range t.keys {
			name := k
			switch {
			case k == "#text":
				continue
			case strings.HasPrefix(k, "@"):
				name = strings.TrimPrefix(k, "@")
			}
			if !validXMLName(name) {
				return fmt.Errorf("json unmarshal error: %q is not a valid XML name, "+
					"so it cannot become an element or attribute", k)
			}
			if err := checkNames(t.values[i]); err != nil {
				return err
			}
		}
	case []any:
		for _, item := range t {
			if err := checkNames(item); err != nil {
				return err
			}
		}
	}
	return nil
}

// validXMLName reports whether a string satisfies the XML Name production.
//
// A colon is permitted anywhere, because a namespace prefix arrives as part of
// the name. Everything outside ASCII is accepted: the ranges the specification
// carves out there exclude nothing an ISO 20022 schema uses, and rejecting a
// name for being foreign would be worse than accepting one that is unusual.
func validXMLName(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r == ':' || r == '_' ||
			(r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || r > 0x7F:
			// A name-start character, valid in any position.
		case i > 0 && (r == '-' || r == '.' || (r >= '0' && r <= '9')):
			// Valid after the first character only.
		default:
			return false
		}
	}
	return true
}

// object is a JSON object with its keys in document order.
type object struct {
	keys   []string
	values []any
}

// decodeValue reads one JSON value from the token stream, preserving key order.
func decodeValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}

	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '{':
			obj := &object{}
			for dec.More() {
				keyTok, err := dec.Token()
				if err != nil {
					return nil, err
				}
				key, ok := keyTok.(string)
				if !ok {
					return nil, fmt.Errorf("object key is not a string")
				}
				val, err := decodeValue(dec)
				if err != nil {
					return nil, err
				}
				obj.keys = append(obj.keys, key)
				obj.values = append(obj.values, val)
			}
			if _, err := dec.Token(); err != nil { // closing brace
				return nil, err
			}
			return obj, nil

		case '[':
			var list []any
			for dec.More() {
				val, err := decodeValue(dec)
				if err != nil {
					return nil, err
				}
				list = append(list, val)
			}
			if _, err := dec.Token(); err != nil { // closing bracket
				return nil, err
			}
			return list, nil
		}
		return nil, fmt.Errorf("unexpected delimiter %v", t)

	default:
		return tok, nil
	}
}

func writeXMLNode(buf *bytes.Buffer, name string, value any, depth int) {
	indent := strings.Repeat("  ", depth)

	switch v := value.(type) {
	case *object:
		var attrs []string
		var text string
		type child struct {
			name  string
			value any
		}
		var children []child

		for i, k := range v.keys {
			switch {
			case strings.HasPrefix(k, "@"):
				// %q would apply Go's quoting rules, which are not XML's: a
				// value containing an ampersand, a quote or a control
				// character would come out as invalid XML or as the literal
				// text of a Go escape. EscapeText handles all three, and is
				// safe inside an attribute as well as in element content.
				attrs = append(attrs, fmt.Sprintf(`%s="%s"`,
					strings.TrimPrefix(k, "@"), escapeText(scalar(v.values[i]))))
			case k == "#text":
				text = scalar(v.values[i])
			default:
				children = append(children, child{k, v.values[i]})
			}
		}

		attrStr := ""
		if len(attrs) > 0 {
			attrStr = " " + strings.Join(attrs, " ")
		}

		if len(children) == 0 {
			fmt.Fprintf(buf, "%s<%s%s>%s</%s>\n", indent, name, attrStr, escapeText(text), name)
			return
		}

		fmt.Fprintf(buf, "%s<%s%s>\n", indent, name, attrStr)
		for _, c := range children {
			writeXMLNode(buf, c.name, c.value, depth+1)
		}
		fmt.Fprintf(buf, "%s</%s>\n", indent, name)

	case []any:
		for _, item := range v {
			writeXMLNode(buf, name, item, depth)
		}

	default:
		fmt.Fprintf(buf, "%s<%s>%s</%s>\n", indent, name, escapeText(scalar(value)), name)
	}
}

// scalar renders a JSON scalar as text. Numbers keep their original notation, so
// "25000.00" does not become "25000".
func scalar(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case json.Number:
		return t.String()
	case bool:
		return strconv.FormatBool(t)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		return fmt.Sprintf("%v", t)
	}
}

func escapeText(s string) string {
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, []byte(s)) // bytes.Buffer writes cannot fail
	return buf.String()
}
