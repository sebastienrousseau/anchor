// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// invoke runs the command with the given arguments and input, returning its
// exit code and both streams.
func invoke(t *testing.T, args []string, stdin string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := run(args, strings.NewReader(stdin), &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestVersionFlag(t *testing.T) {
	code, out, _ := invoke(t, []string{"--version"}, "")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if !strings.HasPrefix(out, "anchor-mcp ") {
		t.Errorf("output = %q", out)
	}
}

func TestToolsFlag(t *testing.T) {
	code, out, _ := invoke(t, []string{"--tools"}, "")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	for _, want := range []string{"anchor_lint", "anchor_translate", "anchor_diff"} {
		if !strings.Contains(out, want) {
			t.Errorf("the listing is missing %s:\n%s", want, out)
		}
	}
}

func TestHelpExitsCleanly(t *testing.T) {
	code, _, errOut := invoke(t, []string{"--help"}, "")
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if !strings.Contains(errOut, "Model Context Protocol") {
		t.Errorf("usage = %q", errOut)
	}
}

func TestUnknownFlag(t *testing.T) {
	code, _, _ := invoke(t, []string{"--nope"}, "")
	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
}

func TestServesAHandshake(t *testing.T) {
	requests := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
	}, "\n") + "\n"

	code, out, errOut := invoke(t, nil, requests)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, errOut)
	}

	// Exactly two replies: the notification is not answered. Nothing else may
	// reach stdout, because a stray line corrupts the stream.
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines on stdout, want 2:\n%s", len(lines), out)
	}
	for _, line := range lines {
		var reply map[string]any
		if err := json.Unmarshal([]byte(line), &reply); err != nil {
			t.Fatalf("a reply is not valid JSON: %v\n%s", err, line)
		}
		if reply["jsonrpc"] != "2.0" {
			t.Errorf("reply = %v", reply)
		}
	}
	if errOut != "" {
		t.Errorf("a clean run wrote to stderr: %q", errOut)
	}
}

func TestServeReportsAFailure(t *testing.T) {
	// A request longer than the transport allows must end the process with a
	// message on stderr, not a silent success.
	huge := `{"jsonrpc":"2.0","id":1,"method":"ping","params":"` + strings.Repeat("A", 9<<20) + `"}`

	code, _, errOut := invoke(t, nil, huge)
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(errOut, "anchor-mcp:") {
		t.Errorf("stderr = %q", errOut)
	}
}
