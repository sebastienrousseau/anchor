// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package lsp

import (
	"strings"
	"testing"

	"github.com/sebastienrousseau/askiso/internal/validator"
)

// A schema error is turned into an editor range by preferring the element path,
// because a line and column reported against the last parse drift as soon as
// the user types. These are the three fallbacks, in order.
func TestRangeForSchemaErrorPrefersThePath(t *testing.T) {
	doc := Parse("<Document>\n  <GrpHdr>\n    <MsgId>x</MsgId>\n  </GrpHdr>\n</Document>")

	elements := doc.ByPath("/Document/GrpHdr/MsgId")
	if len(elements) == 0 {
		t.Skip("the document index does not expose this path")
	}
	want := doc.RangeOf(elements[0])

	got := doc.rangeForSchemaError(validator.Error{
		Path: "/Document/GrpHdr/MsgId",
		// A deliberately wrong line: the path must win over it.
		Line:   99,
		Column: 99,
	})
	if got != want {
		t.Errorf("range = %+v, want the range of the element at the path %+v", got, want)
	}
}

func TestRangeForSchemaErrorFallsBackToTheLine(t *testing.T) {
	doc := Parse("<Document>\n  <GrpHdr/>\n</Document>")

	got := doc.rangeForSchemaError(validator.Error{Line: 2, Column: 3})
	want := doc.LineRange(2, 3)
	if got != want {
		t.Errorf("range = %+v, want %+v", got, want)
	}
}

// With neither a path that resolves nor a line, the diagnostic still has to
// land somewhere the editor will accept.
func TestRangeForSchemaErrorFallsBackToTheFirstLine(t *testing.T) {
	doc := Parse("<Document/>")

	got := doc.rangeForSchemaError(validator.Error{Path: "/No/Such/Path"})
	want := doc.LineRange(1, 0)
	if got != want {
		t.Errorf("range = %+v, want %+v", got, want)
	}
}

func TestSchemaMessageAppendsWhatWasExpectedAndFound(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  validator.Error
		want []string
		omit []string
	}{
		{
			name: "message only",
			err:  validator.Error{Message: "unexpected element"},
			want: []string{"unexpected element"},
			omit: []string{"expected:", "found:"},
		},
		{
			name: "expected only",
			err:  validator.Error{Message: "wrong element", Expected: "GrpHdr"},
			want: []string{"wrong element", "expected: GrpHdr"},
			omit: []string{"found:"},
		},
		{
			name: "both",
			err: validator.Error{
				Message: "wrong element", Expected: "GrpHdr", Actual: "Hdr",
			},
			want: []string{"wrong element", "expected: GrpHdr", "found: Hdr"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := schemaMessage(tc.err)
			for _, w := range tc.want {
				if !strings.Contains(got, w) {
					t.Errorf("message %q is missing %q", got, w)
				}
			}
			for _, o := range tc.omit {
				if strings.Contains(got, o) {
					t.Errorf("message %q should not carry %q", got, o)
				}
			}
		})
	}
}
