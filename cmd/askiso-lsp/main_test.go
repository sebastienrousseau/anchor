// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
)

func invoke(t *testing.T, args []string, stdin string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := run(args, strings.NewReader(stdin), &out, &errOut)
	return code, out.String(), errOut.String()
}

func framed(body string) string {
	return fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body)
}

// replies decodes the framed messages a run produced.
func replies(t *testing.T, stream string) []map[string]any {
	t.Helper()

	var out []map[string]any
	data := []byte(stream)
	for i := 0; i < len(data); {
		sep := bytes.Index(data[i:], []byte("\r\n\r\n"))
		if sep < 0 {
			break
		}
		header := string(data[i : i+sep])

		length := -1
		for _, line := range strings.Split(header, "\r\n") {
			name, value, found := strings.Cut(line, ":")
			if found && strings.EqualFold(strings.TrimSpace(name), "content-length") {
				n, err := strconv.Atoi(strings.TrimSpace(value))
				if err != nil {
					t.Fatalf("Content-Length is not a number: %q", value)
				}
				length = n
			}
		}
		if length < 0 {
			t.Fatalf("a reply has no Content-Length: %q", header)
		}

		start := i + sep + 4
		var msg map[string]any
		if err := json.Unmarshal(data[start:start+length], &msg); err != nil {
			t.Fatalf("a reply is not valid JSON: %v", err)
		}
		out = append(out, msg)
		i = start + length
	}
	return out
}

func TestVersionFlag(t *testing.T) {
	code, out, _ := invoke(t, []string{"--version"}, "")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if !strings.HasPrefix(out, "askiso-lsp ") {
		t.Errorf("output = %q", out)
	}
}

func TestHelpExitsCleanly(t *testing.T) {
	code, _, errOut := invoke(t, []string{"--help"}, "")
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if !strings.Contains(errOut, "Language Server Protocol") {
		t.Errorf("usage = %q", errOut)
	}
	// The usage has to list the profiles, or --profile is unguessable.
	if !strings.Contains(errOut, "cbpr-2026") {
		t.Errorf("the usage does not list the profiles: %q", errOut)
	}
}

func TestUnknownFlag(t *testing.T) {
	if code, _, _ := invoke(t, []string{"--nope"}, ""); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
}

func TestUnknownProfileIsRefused(t *testing.T) {
	// Starting with a profile that does not exist would silently check nothing,
	// so it is refused with the list of what is available.
	code, _, errOut := invoke(t, []string{"--profile", "no-such-profile"}, "")
	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(errOut, "cbpr-2026") {
		t.Errorf("stderr does not list the profiles: %q", errOut)
	}
}

func TestProfileCanBeDisabled(t *testing.T) {
	// An empty profile is valid: it turns the scheme rules off and leaves the
	// linter and the schema validator running.
	stream := framed(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"capabilities":{}}}`) +
		framed(`{"jsonrpc":"2.0","method":"exit"}`)

	code, out, errOut := invoke(t, []string{"--profile", ""}, stream)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, errOut)
	}
	if len(replies(t, out)) != 1 {
		t.Errorf("got %d replies, want 1", len(replies(t, out)))
	}
}

func TestServesAHandshake(t *testing.T) {
	doc := `<?xml version="1.0"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">
  <FIToFICstmrCdtTrf><CdtTrfTxInf><Cdtr>
    <Nm>MUELLER GMBH</Nm>
    <PstlAdr><AdrLine>HAUPTSTRASSE 12</AdrLine></PstlAdr>
  </Cdtr></CdtTrfTxInf></FIToFICstmrCdtTrf>
</Document>`
	open, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "method": "textDocument/didOpen",
		"params": map[string]any{"textDocument": map[string]any{
			"uri": "file:///a.xml", "languageId": "xml", "version": 1, "text": doc,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	stream := framed(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"capabilities":{}}}`) +
		framed(`{"jsonrpc":"2.0","method":"initialized","params":{}}`) +
		framed(string(open)) +
		framed(`{"jsonrpc":"2.0","id":2,"method":"shutdown"}`) +
		framed(`{"jsonrpc":"2.0","method":"exit"}`)

	code, out, errOut := invoke(t, nil, stream)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, errOut)
	}

	msgs := replies(t, out)
	if len(msgs) != 3 {
		t.Fatalf("got %d replies, want 3: %v", len(msgs), msgs)
	}

	// The unstructured address must have produced diagnostics, published
	// without being asked for.
	params := msgs[1]["params"].(map[string]any)
	if msgs[1]["method"] != "textDocument/publishDiagnostics" {
		t.Fatalf("the second message is %v", msgs[1]["method"])
	}
	if len(params["diagnostics"].([]any)) == 0 {
		t.Error("an unstructured address produced no diagnostics")
	}
}

func TestFramingFailureExitsNonZero(t *testing.T) {
	code, _, errOut := invoke(t, nil, "Content-Length: not-a-number\r\n\r\n{}")
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(errOut, "askiso-lsp:") {
		t.Errorf("stderr = %q", errOut)
	}
}
