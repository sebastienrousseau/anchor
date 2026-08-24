// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebastienrousseau/anchor/internal/ai"
)

// withStdin replaces os.Stdin with a file holding the given script. A regular
// file has no character-device bit, so the command sees piped input -- exactly
// as it would behind a shell pipe.
func withStdin(t *testing.T, script string) {
	t.Helper()

	p := filepath.Join(t.TempDir(), "stdin")
	if err := os.WriteFile(p, []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}

	prev := os.Stdin
	os.Stdin = f
	t.Cleanup(func() {
		os.Stdin = prev
		_ = f.Close()
	})
}

func TestAskReadsPipedQuestion(t *testing.T) {
	withCatalogue(t)
	withStdin(t, "What is pacs.008?\n")

	out, err := run(t, "ask")
	if err != nil {
		t.Fatalf("ask with piped input: %v", err)
	}
	if strings.TrimSpace(out) == "" {
		t.Error("piped input produced no answer")
	}
}

// The REPL loop is a function of its reader, so a scripted session drives every
// in-session command without a terminal.
func TestAskLoopSession(t *testing.T) {
	idx := askFixtureIndex(t)
	eng := ai.New(idx)

	script := strings.Join([]string{
		"", // ignored in the REPL
		"What is pacs.008?",
		"1",   // run the first suggestion
		"[2]", // bracketed index
		"999", // out of range: treated as a question
		"/help",
		"/info pacs.008.001.10",
		"/info",
		"/xml pacs.008.001.10",
		"/xml",
		"/xsd pacs.008.001.10",
		"/xsd",
		"/clear",
		"/nonsense",
		"compare pacs.008 vs pacs.009",
		"/exit",
	}, "\n") + "\n"

	out := captureStdout(t, func() {
		askLoop(eng, idx, strings.NewReader(script), []string{"What is pacs.008?", "What is camt.053?"}, true)
	})

	for _, want := range []string{"pacs.008", "Goodbye", "Unknown command", "Usage: /info"} {
		if !strings.Contains(out, want) {
			t.Errorf("REPL output should contain %q", want)
		}
	}
}

func TestAskLoopQuitWords(t *testing.T) {
	idx := askFixtureIndex(t)
	eng := ai.New(idx)

	for _, quit := range []string{"q", "exit", "quit", "bye", ":q", "/exit", "/quit", "/q", "QUIT"} {
		t.Run(quit, func(t *testing.T) {
			out := captureStdout(t, func() {
				askLoop(eng, idx, strings.NewReader(quit+"\n"), nil, true)
			})
			if !strings.Contains(out, "Goodbye") {
				t.Errorf("%q should end the session", quit)
			}
		})
	}
}

// In the follow-up prompt (not the full REPL) a blank line ends the session and
// slash commands are not offered.
func TestAskLoopFollowUpMode(t *testing.T) {
	idx := askFixtureIndex(t)
	eng := ai.New(idx)

	out := captureStdout(t, func() {
		askLoop(eng, idx, strings.NewReader("\n"), []string{"What is pacs.008?"}, false)
	})
	if !strings.Contains(out, "Goodbye") {
		t.Error("a blank line should end the follow-up prompt")
	}

	// A slash command is just text here.
	out = captureStdout(t, func() {
		askLoop(eng, idx, strings.NewReader("/help\nq\n"), nil, false)
	})
	if strings.Contains(out, "Available REPL Commands") {
		t.Error("slash commands should not be handled in follow-up mode")
	}
}

func TestAskLoopStopsAtEOF(t *testing.T) {
	idx := askFixtureIndex(t)
	captureStdout(t, func() {
		askLoop(ai.New(idx), idx, strings.NewReader(""), nil, true)
	})
}

func TestIsQuitWord(t *testing.T) {
	for _, w := range []string{"q", "Q", "exit", "EXIT", "quit", "bye", ":q", "/exit", "/quit", "/q"} {
		if !isQuitWord(w) {
			t.Errorf("%q should be a quit word", w)
		}
	}
	for _, w := range []string{"question", "quitting", "", "/help"} {
		if isQuitWord(w) {
			t.Errorf("%q should not be a quit word", w)
		}
	}
}

// The command still accepts a piped question end to end.
func TestAskReplSession(t *testing.T) {
	withCatalogue(t)

	script := strings.Join([]string{
		"What is pacs.008?",
		"",  // blank line is ignored
		"1", // run the first suggestion
		"/help",
		"/info pacs.008.001.10",
		"/info", // missing argument
		"/xml pacs.008.001.10",
		"/xml", // missing argument
		"/xsd pacs.008.001.10",
		"/xsd", // missing argument
		"/clear",
		"/unknown-command",
		"compare pacs.008 vs pacs.009",
		"/exit",
	}, "\n") + "\n"

	withStdin(t, script)

	out, err := run(t, "ask")
	if err != nil {
		t.Fatalf("REPL session: %v", err)
	}
	for _, want := range []string{"pacs.008"} {
		if !strings.Contains(out, want) {
			t.Errorf("REPL output should mention %q", want)
		}
	}
}

func TestAskReplQuitVariants(t *testing.T) {
	withCatalogue(t)
	for _, quit := range []string{"q", "exit", "quit", "/quit", "/q"} {
		t.Run(quit, func(t *testing.T) {
			withStdin(t, "What is pacs.008?\n"+quit+"\n")
			if _, err := run(t, "ask"); err != nil {
				t.Errorf("REPL should exit cleanly on %q: %v", quit, err)
			}
		})
	}
}

// A one-shot question followed by a suggestion index, which is the interactive
// follow-up path.
func TestAskOneShotThenFollowUp(t *testing.T) {
	withCatalogue(t)
	withStdin(t, "2\nq\n")

	if _, err := run(t, "ask", "What is pacs.008?"); err != nil {
		t.Fatalf("one-shot with follow-up: %v", err)
	}
}

// Stdin that closes immediately must not hang or panic.
func TestAskHandlesEmptyStdin(t *testing.T) {
	withCatalogue(t)
	withStdin(t, "")

	if _, err := run(t, "ask"); err != nil {
		t.Errorf("empty stdin should exit cleanly: %v", err)
	}
}
