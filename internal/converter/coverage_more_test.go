// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package converter

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNodeAndNamespaceFallbacks(t *testing.T) {
	if got := nodeName(&Node{Name: "Local"}); got != "Local" {
		t.Fatalf("nodeName fallback = %q", got)
	}
	root, err := Parse([]byte(`<Document xml:lang="en"/>`))
	if err != nil {
		t.Fatal(err)
	}
	if len(root.Attrs) != 1 || root.Attrs[0].Name.Space != "xml" {
		t.Fatalf("XML namespace attribute was not retained: %+v", root.Attrs)
	}
}

func TestInterleavedErrorInsideArrayItem(t *testing.T) {
	_, err := XMLToJSON([]byte(`<Doc><Wrap><A/><B/><A/></Wrap><Wrap><A/></Wrap></Doc>`))
	if err == nil || !strings.Contains(err.Error(), "repeats non-adjacently") {
		t.Fatalf("nested array interleave error = %v", err)
	}
}

func TestJSONTrailingSyntaxAndDecoderDelimiters(t *testing.T) {
	if _, err := JSONToXML([]byte(`{"A":"ok"} @`)); err == nil {
		t.Fatal("invalid trailing token was accepted")
	}
	dec := json.NewDecoder(strings.NewReader(`[]`))
	if _, err := dec.Token(); err != nil {
		t.Fatal(err)
	}
	if _, err := decodeValue(dec); err == nil || !strings.Contains(err.Error(), "unexpected delimiter") {
		t.Fatalf("closing delimiter as value = %v", err)
	}
	if _, err := decodeValue(json.NewDecoder(strings.NewReader(`[`))); err == nil {
		t.Fatal("unclosed empty array should fail while reading its closing token")
	}
}

func TestScalarFallbackKinds(t *testing.T) {
	if got := scalar(float64(1.25)); got != "1.25" {
		t.Fatalf("float scalar = %q", got)
	}
	if got := scalar(struct{ N int }{N: 3}); got != "{3}" {
		t.Fatalf("fallback scalar = %q", got)
	}
}
