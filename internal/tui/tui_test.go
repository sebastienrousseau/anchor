package tui

import (
	"strings"
	"testing"

	"github.com/sebastienrousseau/anchor/internal/catalog"
)

func mirrorBrailleRune(r rune) rune {
	if r == ' ' {
		return ' '
	}
	code := int(r) - 0x2800
	if code < 0 || code > 255 {
		return r
	}
	d1 := (code >> 0) & 1
	d2 := (code >> 1) & 1
	d3 := (code >> 2) & 1
	d7 := (code >> 6) & 1
	d4 := (code >> 3) & 1
	d5 := (code >> 4) & 1
	d6 := (code >> 5) & 1
	d8 := (code >> 7) & 1
	newCode := (d4 << 0) | (d5 << 1) | (d6 << 2) | (d8 << 6) | (d1 << 3) | (d2 << 4) | (d3 << 5) | (d7 << 7)
	return rune(0x2800 + newCode)
}

func TestLogoSymmetry(t *testing.T) {
	for i, line := range logoLines {
		runes := []rune(line)
		n := len(runes)
		for j := 0; j < n/2; j++ {
			left := runes[j]
			right := runes[n-1-j]
			expectedRight := mirrorBrailleRune(left)
			if right != expectedRight {
				t.Errorf("Line %d is not symmetric at position %d: left '%c', right '%c', expected right '%c'",
					i, j, left, right, expectedRight)
			}
		}
	}
}

func TestStyledLogo(t *testing.T) {
	logo := GetStyledLogo()
	if !strings.Contains(logo, "Anchor ⚓") {
		t.Errorf("expected logo to contain 'Anchor ⚓', got:\n%s", logo)
	}
}

func TestNewModel(t *testing.T) {
	idx := &catalog.Index{
		Categories: []catalog.Category{
			{Name: "Payments", TotalSchemas: 1},
		},
		Messages: []catalog.Message{
			{ID: "pacs.008.001.01", Category: "Payments", Version: "Version 1.0"},
		},
		MessageMap: map[string]catalog.Message{
			"pacs.008.001.01": {ID: "pacs.008.001.01"},
		},
	}

	m := NewModel(idx)
	if m.mode != modeTable {
		t.Errorf("expected initial mode modeTable, got %d", m.mode)
	}

	view := m.View()
	if len(view) == 0 {
		t.Errorf("expected non-empty view")
	}
}

func TestAskViewRendering(t *testing.T) {
	idx := &catalog.Index{
		Categories: []catalog.Category{
			{Name: "Payments", TotalSchemas: 1},
		},
		Messages: []catalog.Message{
			{ID: "pacs.008.001.01", Category: "Payments", Version: "Version 1.0"},
		},
	}
	m := NewModel(idx)
	m.mode = modeAsk
	m.width = 80
	m.height = 24

	view := m.View()
	if !strings.Contains(view, "Anchor ⚓") {
		t.Errorf("Expected logo 'Anchor ⚓' in Ask view, got:\n%s", view)
	}
	if !strings.Contains(view, "┃") {
		t.Errorf("Expected vertical delimiter '┃' in Ask view, got:\n%s", view)
	}
	if !strings.Contains(view, "Back") {
		t.Errorf("Expected footer to contain 'Back' in Ask view, got:\n%s", view)
	}
}
