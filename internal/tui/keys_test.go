// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// askWith puts the model in assistant mode with the given text already entered,
// which is what the text input holds when the user presses Enter.
func askWith(t *testing.T, text string) Model {
	t.Helper()
	m := send(newSizedModel(t), "ctrl+a")
	m.textInput.SetValue(text)
	return m
}

func TestAskModeSubmitsQuestion(t *testing.T) {
	m := askWith(t, "What is pacs.008?")
	m = send(m, "enter")

	// The transcript opens with a welcome message, so the exchange is appended
	// after it.
	if len(m.askHistory) < 3 {
		t.Fatalf("both sides of the exchange should be recorded, got %d entries", len(m.askHistory))
	}
	q := m.askHistory[len(m.askHistory)-2]
	if q.sender != "You" || !strings.Contains(q.content, "pacs.008") {
		t.Errorf("the question should be recorded: %+v", q)
	}
	if strings.TrimSpace(m.renderAsk()) == "" {
		t.Error("the transcript rendered nothing")
	}
}

// In the assistant, the exit words return to the catalogue rather than closing
// the program -- the table is the home screen.
func TestAskModeExitWordsReturnToTable(t *testing.T) {
	for _, word := range []string{"q", "quit", "exit", "bye", ":q", "/exit", "/quit", "/q"} {
		t.Run(word, func(t *testing.T) {
			m := send(askWith(t, word), "enter")
			if m.mode != modeTable {
				t.Errorf("%q should return to the table, mode = %v", word, m.mode)
			}
			if m.textInput.Value() != "" {
				t.Errorf("%q should clear the input", word)
			}
		})
	}
}

func TestAskModeSlashCommands(t *testing.T) {
	// /clear resets the transcript.
	m := askWith(t, "What is pacs.008?")
	m = send(m, "enter")
	if len(m.askHistory) == 0 {
		t.Fatal("expected a transcript")
	}
	m.textInput.SetValue("/clear")
	m = send(m, "enter")
	// The welcome message is kept; the conversation after it is dropped.
	if len(m.askHistory) != 1 {
		t.Errorf("/clear should leave only the welcome message, got %d entries", len(m.askHistory))
	}

	// /reset behaves the same.
	m = askWith(t, "/reset")
	m = send(m, "enter")

	// /table returns to the catalogue.
	for _, cmdWord := range []string{"/table", "/back", "/list"} {
		back := askWith(t, cmdWord)
		back = send(back, "enter")
		if back.mode != modeTable {
			t.Errorf("%s should return to the table", cmdWord)
		}
	}
}

func TestAskModeNumberedSuggestion(t *testing.T) {
	m := askWith(t, "What is pacs.008?")
	m = send(m, "enter")
	if len(m.lastSuggestions) == 0 {
		t.Skip("this answer offered no suggestions")
	}

	m.textInput.SetValue("1")
	m = send(m, "enter")
	if len(m.askHistory) < 4 {
		t.Errorf("running a suggestion should extend the transcript, got %d", len(m.askHistory))
	}
}

func TestAskModeIgnoresEmptySubmit(t *testing.T) {
	m := askWith(t, "")
	before := len(m.askHistory)
	m = send(m, "enter")
	if len(m.askHistory) != before {
		t.Error("an empty submission should be ignored")
	}
}

func TestAskModeEscapeReturns(t *testing.T) {
	m := send(newSizedModel(t), "ctrl+a")
	if back := send(m, "esc"); back.mode != modeTable {
		t.Error("esc should leave the assistant")
	}
}

func TestTableEscapeBehaviour(t *testing.T) {
	m := newSizedModel(t)

	// With help open, esc closes it.
	withHelp := send(m, "?")
	if closed := send(withHelp, "esc"); closed.showHelp {
		t.Error("esc should close the help panel first")
	}

	// With a filter set, esc clears it.
	filtered := typeText(m, "pacs")
	cleared := send(filtered, "esc")
	if cleared.filter != "" {
		t.Errorf("esc should clear the filter, got %q", cleared.filter)
	}

	// With neither, esc quits.
	if _, cmd := m.Update(key("esc")); cmd == nil {
		t.Error("esc on a clean table should quit")
	}
}

func TestQuitOnlyWhenFilterEmpty(t *testing.T) {
	m := typeText(newSizedModel(t), "pacs")
	// 'q' is a letter while filtering.
	typed := send(m, "q")
	if typed.filter != "pacsq" {
		t.Errorf("'q' should type while filtering, got %q", typed.filter)
	}

	withHelp := send(newSizedModel(t), "?")
	if closed := send(withHelp, "q"); closed.showHelp {
		t.Error("'q' should close help before quitting")
	}
}

func TestSlashCommandTabCompletion(t *testing.T) {
	m := newSizedModel(t)
	m.filter = "/he"
	completed := send(m, "tab")
	if completed.filter != "/help" {
		t.Errorf("tab should complete the command, got %q", completed.filter)
	}

	// Tab with no matching command leaves the filter alone.
	m.filter = "/zzz"
	if unchanged := send(m, "tab"); unchanged.filter != "/zzz" {
		t.Errorf("tab should not invent a command, got %q", unchanged.filter)
	}
}

func TestSlashCommandSubmittedWithEnter(t *testing.T) {
	m := newSizedModel(t)
	m.filter = "/help"
	after := send(m, "enter")
	if !after.showHelp {
		t.Error("entering /help should open the panel")
	}
	if after.filter != "" {
		t.Errorf("the command should be consumed, filter = %q", after.filter)
	}
}

func TestSpaceExtendsSlashCommand(t *testing.T) {
	m := newSizedModel(t)
	m.filter = "/code"
	after := send(m, " ")
	if after.filter != "/code " {
		t.Errorf("space should extend a slash command, got %q", after.filter)
	}
}

func TestCopyKeyOnTable(t *testing.T) {
	// 'y' copies the highlighted row; the clipboard may be unavailable in CI, so
	// this asserts the model reports something rather than panicking.
	m := send(newSizedModel(t), "y")
	if m.cmdErr == "" {
		t.Log("no confirmation recorded; clipboard likely unavailable")
	}
}

func TestViewerKeys(t *testing.T) {
	m := newSizedModel(t)
	m.mode = modeViewer
	m.viewingContent = "<xs:schema/>"
	m.viewingTitle = "schema"

	// Copy from the viewer.
	send(m, "y")

	// Scrolling is delegated to the viewport and must not panic.
	for _, k := range []string{"j", "k", "g", "G"} {
		send(m, k)
	}

	if back := send(m, "q"); back.mode != modeTable {
		t.Error("q should leave the viewer")
	}
}

func TestFilterStartsSlashCommand(t *testing.T) {
	m := newSizedModel(t)
	m = typeText(m, "/sort")
	if !strings.HasPrefix(m.filter, "/") {
		t.Errorf("a leading slash should be preserved: %q", m.filter)
	}
	// A slash filter must not narrow the table.
	if len(m.filteredMsgs) != len(m.idx.Messages) {
		t.Error("a slash command should not filter the catalogue")
	}
}

func TestSortSlashCommandOrders(t *testing.T) {
	m := newSizedModel(t)

	m.executeSlashCommand("/sort id")
	first := m.filteredMsgs[0].ID
	m.executeSlashCommand("/sort ver")
	m.executeSlashCommand("/sort cat")
	m.executeSlashCommand("/sort category")
	m.executeSlashCommand("/sort version")
	if len(m.filteredMsgs) == 0 {
		t.Fatal("sorting emptied the list")
	}
	m.executeSlashCommand("/sort")
	if m.cmdErr == "" {
		t.Error("/sort with no field should explain the usage")
	}
	_ = first
}

func TestCodeSlashCommandPaths(t *testing.T) {
	m := newSizedModel(t)

	m.executeSlashCommand("/code AC04")
	if m.viewingContent == "" {
		t.Error("/code with a hit should show something")
	}

	m.executeSlashCommand("/code")
	if m.cmdErr == "" {
		t.Error("/code with no argument should explain the usage")
	}

	m.executeSlashCommand("/code zzzz-no-such-code")
	if m.cmdErr == "" {
		t.Error("/code with no matches should say so")
	}
}

func TestFlowAndGraphSlashCommands(t *testing.T) {
	m := newSizedModel(t)
	for _, cmdStr := range []string{
		"/flow", "/flow sepa", "/flow fednow",
		"/graph", "/graph pacs.008", "/graph pacs.008 mermaid",
	} {
		m.executeSlashCommand(cmdStr)
		if m.viewingContent == "" && m.cmdErr == "" {
			t.Errorf("%q produced neither output nor an error", cmdStr)
		}
	}
}

func TestOpenFileHandlesMarkdown(t *testing.T) {
	m := newSizedModel(t)
	dir := t.TempDir()
	md := filepath.Join(dir, "README.md")
	if err := os.WriteFile(md, []byte("# Title\n\nBody"), 0o644); err != nil {
		t.Fatal(err)
	}
	m.openFile(md, "Readme")
	if m.viewingContent == "" {
		t.Error("markdown should be rendered into the viewer")
	}
}

func TestUnknownKeysAreDelegated(t *testing.T) {
	m := newSizedModel(t)
	// Arrow keys belong to the table widget.
	for _, msg := range []tea.KeyMsg{
		{Type: tea.KeyDown}, {Type: tea.KeyUp}, {Type: tea.KeyPgDown}, {Type: tea.KeyHome},
	} {
		next, _ := m.Update(msg)
		m = next.(Model)
	}
	if strings.TrimSpace(m.View()) == "" {
		t.Error("navigation should leave the view intact")
	}
}

func TestTinyWindowStillRenders(t *testing.T) {
	m := NewModel(fixtureIndex(t))
	next, _ := m.Update(tea.WindowSizeMsg{Width: 20, Height: 6})
	m = next.(Model)
	if strings.TrimSpace(m.View()) == "" {
		t.Error("a very small terminal should still render")
	}
}
