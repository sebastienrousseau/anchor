package tui

import (
	"strings"
	"testing"

	"github.com/sebastienrousseau/askiso/internal/catalog"
)

// The logo is drawn as a fixed-width block of braille cells, so the TUI can
// centre it without measuring. Symmetry is not asserted: the mark is a question
// mark, which is asymmetric by nature.
func TestLogoIsAWellFormedBlock(t *testing.T) {
	if len(logoLines) == 0 {
		t.Fatal("the logo has no lines")
	}

	width := len([]rune(logoLines[0]))
	if width == 0 {
		t.Fatal("the first logo line is empty")
	}

	var inked int
	for i, line := range logoLines {
		runes := []rune(line)
		if len(runes) != width {
			t.Errorf("line %d is %d cells wide, want %d — the block must be rectangular",
				i, len(runes), width)
		}
		for j, r := range runes {
			if r == ' ' {
				continue
			}
			if r < 0x2800 || r > 0x28FF {
				t.Errorf("line %d position %d holds %q, which is neither a space nor a braille cell",
					i, j, r)
			}
			inked++
		}
	}

	if inked == 0 {
		t.Error("the logo is entirely blank")
	}
}

func TestStyledLogo(t *testing.T) {
	logo := GetStyledLogo()
	if !strings.Contains(logo, "AskIso") {
		t.Errorf("expected logo to contain 'AskIso', got:\n%s", logo)
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
	if !strings.Contains(view, "AskIso") {
		t.Errorf("Expected logo 'AskIso' in Ask view, got:\n%s", view)
	}
	if !strings.Contains(view, "┃") {
		t.Errorf("Expected vertical delimiter '┃' in Ask view, got:\n%s", view)
	}
	if !strings.Contains(view, "Back") {
		t.Errorf("Expected footer to contain 'Back' in Ask view, got:\n%s", view)
	}
}
