// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sebastienrousseau/askiso/internal/catalog"
)

// The Bubble Tea model is a pure function of its messages, so the whole TUI can
// be driven from a test without a terminal: feed Update, inspect View.

func fixtureIndex(t *testing.T) *catalog.Index {
	t.Helper()
	root := t.TempDir()

	base := filepath.Join(root, "Payments Clearing and Settlement", "Version 11.0")
	schemas := filepath.Join(base, "Schemas")
	samples := filepath.Join(base, "Sample Messages")
	for _, d := range []string{schemas, samples} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	for _, id := range []string{"pacs.008.001.10", "pacs.009.001.10", "camt.053.001.11"} {
		if err := os.WriteFile(filepath.Join(schemas, id+".xsd"),
			[]byte(`<?xml version="1.0"?><xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"/>`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(samples, "pacs.008.001.10.xml"),
		[]byte(`<?xml version="1.0"?><Document/>`), 0o644); err != nil {
		t.Fatal(err)
	}

	idx, err := catalog.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	return idx
}

// newSizedModel returns a model that has been given a window size, as the real
// program does before anything renders.
func newSizedModel(t *testing.T) Model {
	t.Helper()
	m := NewModel(fixtureIndex(t))
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return next.(Model)
}

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	case "ctrl+a":
		return tea.KeyMsg{Type: tea.KeyCtrlA}
	case "ctrl+k":
		return tea.KeyMsg{Type: tea.KeyCtrlK}
	case "ctrl+s":
		return tea.KeyMsg{Type: tea.KeyCtrlS}
	case "ctrl+y":
		return tea.KeyMsg{Type: tea.KeyCtrlY}
	case " ":
		return tea.KeyMsg{Type: tea.KeySpace}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

// send feeds a sequence of keys and returns the resulting model.
func send(m Model, keys ...string) Model {
	for _, k := range keys {
		next, _ := m.Update(key(k))
		m = next.(Model)
	}
	return m
}

// typeText enters a string one rune at a time, as a user would.
func typeText(m Model, text string) Model {
	for _, r := range text {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = next.(Model)
	}
	return m
}

func TestNewModelInitialState(t *testing.T) {
	idx := fixtureIndex(t)
	m := NewModel(idx)

	if m.idx != idx {
		t.Error("the model should hold the index it was given")
	}
	if m.mode != modeTable {
		t.Error("the model should start on the table")
	}
	if len(m.filteredMsgs) != len(idx.Messages) {
		t.Errorf("all %d messages should be listed initially, got %d",
			len(idx.Messages), len(m.filteredMsgs))
	}
	if m.selected == nil {
		t.Error("the selection set should be initialised")
	}
}

func TestInitReturnsACommand(t *testing.T) {
	if NewModel(fixtureIndex(t)).Init() == nil {
		t.Error("Init should start the spinner")
	}
}

func TestWindowSizeIsApplied(t *testing.T) {
	m := NewModel(fixtureIndex(t))
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = next.(Model)

	if m.width != 100 || m.height != 30 {
		t.Errorf("size = %dx%d, want 100x30", m.width, m.height)
	}
	if strings.TrimSpace(m.View()) == "" {
		t.Error("a sized model should render something")
	}
}

func TestViewRendersInEveryMode(t *testing.T) {
	m := newSizedModel(t)

	if !strings.Contains(m.View(), "pacs.008.001.10") {
		t.Error("table view should list messages")
	}

	ask := send(m, "ctrl+a")
	if ask.mode != modeAsk {
		t.Fatal("ctrl+a should switch to the assistant")
	}
	if strings.TrimSpace(ask.View()) == "" {
		t.Error("ask view rendered nothing")
	}

	m.mode = modeViewer
	m.viewingTitle = "pacs.008.001.10.xsd"
	m.viewingContent = "<xs:schema/>"
	if strings.TrimSpace(m.View()) == "" {
		t.Error("viewer rendered nothing")
	}
}

func TestHelpPanelToggles(t *testing.T) {
	m := newSizedModel(t)

	withHelp := send(m, "?")
	if !withHelp.showHelp {
		t.Fatal("'?' should open help")
	}
	if !strings.Contains(withHelp.View(), "/") {
		t.Error("help should list slash commands")
	}

	if send(withHelp, "?").showHelp {
		t.Error("'?' should close help again")
	}
}

func TestFilteringNarrowsTheTable(t *testing.T) {
	m := newSizedModel(t)
	all := len(m.filteredMsgs)

	m = typeText(m, "camt")
	if m.filter != "camt" {
		t.Fatalf("filter = %q, want camt", m.filter)
	}
	if m.mode != modeTable {
		t.Fatal("typing should not leave the table")
	}
	if len(m.filteredMsgs) >= all {
		t.Errorf("filtering should narrow the list: %d of %d", len(m.filteredMsgs), all)
	}
	before := m.filter
	m = send(m, "backspace")
	if m.filter != before[:len(before)-1] {
		t.Errorf("backspace should delete one rune: %q -> %q", before, m.filter)
	}
}

func TestSelectionToggles(t *testing.T) {
	m := newSizedModel(t)

	m = send(m, " ")
	if countSelected(m) != 1 {
		t.Errorf("space should select the current row, got %d selected", countSelected(m))
	}
	m = send(m, " ")
	if countSelected(m) != 0 {
		t.Errorf("space should deselect again, got %d selected", countSelected(m))
	}
}

func TestQuitKeys(t *testing.T) {
	for _, k := range []string{"q", "ctrl+c"} {
		m := newSizedModel(t)
		_, cmd := m.Update(key(k))
		if cmd == nil {
			t.Errorf("%q should quit", k)
		}
	}
}

func TestEscapeReturnsToTable(t *testing.T) {
	m := newSizedModel(t)
	m.mode = modeViewer
	m.viewingContent = "x"

	back := send(m, "esc")
	if back.mode != modeTable {
		t.Error("esc should return to the table")
	}
}

func TestOpenSchemaAndSample(t *testing.T) {
	m := newSizedModel(t)

	// ctrl+s opens the schema for the highlighted row. It is modified rather
	// than a bare letter because every letter belongs to the filter.
	viewer := send(m, "ctrl+s")
	if viewer.mode != modeViewer {
		t.Fatalf("ctrl+s should open the viewer, mode = %v", viewer.mode)
	}
	if viewer.viewingContent == "" {
		t.Error("the viewer should have content")
	}

	// Enter opens the sample.
	sample := send(newSizedModel(t), "enter")
	if sample.mode != modeViewer && sample.cmdErr == "" {
		t.Error("enter should either open a sample or explain why it cannot")
	}
}

func TestSlashCommands(t *testing.T) {
	cases := []struct {
		cmd    string
		expect func(t *testing.T, m Model)
	}{
		{"/help", func(t *testing.T, m Model) {
			if !m.showHelp {
				t.Error("/help should open the help panel")
			}
		}},
		{"/ask", func(t *testing.T, m Model) {
			if m.mode != modeAsk {
				t.Error("/ask should switch to the assistant")
			}
		}},
		{"/table", func(t *testing.T, m Model) {
			if m.mode != modeTable {
				t.Error("/table should return to the table")
			}
		}},
		{"/stats", func(t *testing.T, m Model) {
			if m.viewingContent == "" && m.mode != modeViewer {
				t.Error("/stats should show something")
			}
		}},
		{"/doctor", func(t *testing.T, m Model) {
			if m.viewingContent == "" && m.mode != modeViewer {
				t.Error("/doctor should show something")
			}
		}},
		{"/flow", nil},
		{"/graph", nil},
		{"/code AC04", nil},
		{"/sort id", nil},
		{"/sort cat", nil},
		{"/sort ver", nil},
		{"/all", func(t *testing.T, m Model) {
			if countSelected(m) == 0 {
				t.Error("/all should select everything")
			}
		}},
		{"/none", func(t *testing.T, m Model) {
			if countSelected(m) != 0 {
				t.Error("/none should clear the selection")
			}
		}},
		{"/clear", nil},
		{"/nonsense", func(t *testing.T, m Model) {
			if m.cmdErr == "" {
				t.Error("an unknown command should report something")
			}
		}},
	}

	for _, tc := range cases {
		t.Run(tc.cmd, func(t *testing.T) {
			m := newSizedModel(t)
			if tc.cmd == "/none" {
				m.executeSlashCommand("/all")
			}
			m.executeSlashCommand(tc.cmd)
			if tc.expect != nil {
				tc.expect(t, m)
			}
			// Whatever happened, the model must still render.
			if strings.TrimSpace(m.View()) == "" {
				t.Errorf("View is empty after %q", tc.cmd)
			}
		})
	}
}

func TestExitSlashCommandQuits(t *testing.T) {
	m := newSizedModel(t)
	if cmd := m.executeSlashCommand("/exit"); cmd == nil {
		t.Error("/exit should quit")
	}
}

func TestOpenMarkdownAndFile(t *testing.T) {
	m := newSizedModel(t)

	// openMarkdown fills the viewer; switching mode is the caller's job.
	m.openMarkdown("# Title\n\nBody", "Doc")
	if m.viewingTitle != "Doc" || m.viewingContent == "" {
		t.Errorf("openMarkdown did not load the viewer: title=%q content=%d bytes",
			m.viewingTitle, len(m.viewingContent))
	}

	dir := t.TempDir()
	p := filepath.Join(dir, "sample.xml")
	if err := os.WriteFile(p, []byte("<Document/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	m.openFile(p, "Sample")
	if m.viewingContent == "" {
		t.Error("openFile produced no content")
	}

	// A missing file must be reported, not panic.
	m.openFile(filepath.Join(dir, "missing.xml"), "Missing")
	if m.cmdErr == "" && m.viewingContent == "" {
		t.Error("a missing file should be reported")
	}
}

func TestAskModeConversation(t *testing.T) {
	m := newSizedModel(t)
	m = send(m, "a")
	m = typeText(m, "What is pacs.008?")
	m = send(m, "enter")

	if len(m.askHistory) == 0 {
		t.Error("the exchange should be recorded in the history")
	}
	if strings.TrimSpace(m.renderAsk()) == "" {
		t.Error("ask view rendered nothing after a question")
	}
}

func TestSpinnerAndProgressMessages(t *testing.T) {
	m := newSizedModel(t)

	// Unknown message types must be handled without panicking.
	next, _ := m.Update(struct{ unknown bool }{})
	if strings.TrimSpace(next.(Model).View()) == "" {
		t.Error("an unrecognised message should leave the view intact")
	}
}

func TestFooterAndViewerRendering(t *testing.T) {
	m := newSizedModel(t)

	if strings.TrimSpace(m.renderFooter()) == "" {
		t.Error("footer rendered nothing")
	}
	m.cmdErr = "something went wrong"
	if !strings.Contains(m.View(), "something went wrong") {
		t.Error("the table view should surface the last command error")
	}

	m.viewingContent = "<xs:schema/>"
	m.viewingTitle = "schema"
	if strings.TrimSpace(m.renderViewer()) == "" {
		t.Error("viewer rendered nothing")
	}
	if strings.TrimSpace(m.renderHelpPanel()) == "" {
		t.Error("help panel rendered nothing")
	}
}

func TestUpdateTableRowsAndFilter(t *testing.T) {
	m := newSizedModel(t)

	m.filter = "pacs"
	m.applyFilter()
	if len(m.filteredMsgs) == 0 {
		t.Fatal("applyFilter dropped everything")
	}
	for _, msg := range m.filteredMsgs {
		if !strings.Contains(msg.ID, "pacs") {
			t.Errorf("unexpected match: %s", msg.ID)
		}
	}

	m.filter = "no-such-message"
	m.applyFilter()
	if len(m.filteredMsgs) != 0 {
		t.Errorf("expected no matches, got %d", len(m.filteredMsgs))
	}
	// Rendering an empty table must still work.
	m.updateTableRows()
	if strings.TrimSpace(m.View()) == "" {
		t.Error("an empty result set should still render")
	}
}

func TestStyledLogoCarriesName(t *testing.T) {
	logo := GetStyledLogo()
	if !strings.Contains(logo, "AskIso") {
		t.Error("the logo should carry the product name")
	}
}

func TestGlamourStyleIsCustomised(t *testing.T) {
	style := getCustomGlamourStyle()
	if style.Document.Margin == nil || *style.Document.Margin != 0 {
		t.Error("the document margin should be zeroed for terminal rendering")
	}
}

func countSelected(m Model) int {
	n := 0
	for _, v := range m.selected {
		if v {
			n++
		}
	}
	return n
}

// Regression: 'a' and 'c' are shortcuts only when the filter is empty. Twelve of
// the roughly thirty ISO 20022 domains begin with one of those letters, so
// treating them as shortcuts unconditionally made those messages unfilterable.
func TestShortcutsDoNotShadowFiltering(t *testing.T) {
	m := newSizedModel(t)

	// Every domain must be typeable, including those starting with the letters
	// that used to be shortcuts.
	for _, domain := range []string{"camt", "acmt", "colr", "auth", "casp", "catm"} {
		typed := typeText(newSizedModel(t), domain)
		if typed.mode != modeTable {
			t.Errorf("typing %q left the table (mode %v)", domain, typed.mode)
		}
		if typed.filter != domain {
			t.Errorf("filter = %q, want %q", typed.filter, domain)
		}
	}

	// The assistant is still reachable, without shadowing a letter.
	if send(m, "ctrl+a").mode != modeAsk {
		t.Error("ctrl+a should open the assistant")
	}
	m2 := newSizedModel(t)
	m2.executeSlashCommand("/ask")
	if m2.mode != modeAsk {
		t.Error("/ask should open the assistant")
	}
}
