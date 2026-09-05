// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package mcp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/sebastienrousseau/askiso/internal/mcp"
	"github.com/sebastienrousseau/askiso/pkg/iso20022"
)

// session drives a server over a scripted set of requests and returns the
// replies, decoded. This is the whole protocol surface: one JSON object per
// line, in and out.
func session(t *testing.T, requests ...string) []map[string]any {
	t.Helper()

	var out, errOut bytes.Buffer
	in := strings.NewReader(strings.Join(requests, "\n") + "\n")

	s := mcp.New(in, &out, &errOut)
	s.SetVersion("test")
	if err := s.Serve(context.Background()); err != nil {
		t.Fatalf("Serve: %v", err)
	}

	var replies []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("a reply is not valid JSON: %v\n%s", err, line)
		}
		replies = append(replies, m)
	}
	return replies
}

const initialize = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","clientInfo":{"name":"test","version":"1"}}}` + "\n" +
	`{"jsonrpc":"2.0","method":"notifications/initialized"}`

// call builds a tools/call request.
func call(id int, name string, args map[string]any) string {
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": id, "method": "tools/call",
		"params": map[string]any{"name": name, "arguments": args},
	})
	if err != nil {
		panic(err)
	}
	return string(body)
}

// result extracts a tool's structured content, failing when the tool errored.
func result(t *testing.T, reply map[string]any) map[string]any {
	t.Helper()
	if e, ok := reply["error"]; ok {
		t.Fatalf("the call failed at the protocol level: %v", e)
	}
	res, ok := reply["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result in %v", reply)
	}
	if isErr, _ := res["isError"].(bool); isErr {
		t.Fatalf("the tool reported an error: %v", text(res))
	}
	sc, ok := res["structuredContent"].(map[string]any)
	if !ok {
		t.Fatalf("no structured content in %v", res)
	}
	return sc
}

// text pulls the first text block out of a tool result.
func text(res map[string]any) string {
	content, _ := res["content"].([]any)
	if len(content) == 0 {
		return ""
	}
	block, _ := content[0].(map[string]any)
	s, _ := block["text"].(string)
	return s
}

// toolError asserts that a call reported a failure the model can read, rather
// than a protocol error the model never sees.
func toolError(t *testing.T, reply map[string]any) string {
	t.Helper()
	res, ok := reply["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result in %v", reply)
	}
	if isErr, _ := res["isError"].(bool); !isErr {
		t.Fatalf("the call succeeded, but should have failed: %v", res)
	}
	return text(res)
}

func TestInitializeHandshake(t *testing.T) {
	replies := session(t, initialize)
	if len(replies) != 1 {
		t.Fatalf("got %d replies, want 1", len(replies))
	}

	res := replies[0]["result"].(map[string]any)
	if res["protocolVersion"] != mcp.ProtocolVersion {
		t.Errorf("protocolVersion = %v, want %s", res["protocolVersion"], mcp.ProtocolVersion)
	}

	info := res["serverInfo"].(map[string]any)
	if info["name"] != "askiso" || info["version"] != "test" {
		t.Errorf("serverInfo = %v", info)
	}

	// The instructions are what tell a model when to reach for these tools
	// rather than answering from memory, so their absence is a real defect.
	instructions, _ := res["instructions"].(string)
	if !strings.Contains(instructions, "structured-address") {
		t.Errorf("the instructions do not mention the address rule: %q", instructions)
	}
	// And that it has moved. A model told the requirement without the deferral
	// repeats a date that stopped being true on 27 August 2026, which is worse
	// than saying nothing: it is confidently wrong to somebody planning a
	// migration around it.
	if !strings.Contains(instructions, "deferred") {
		t.Errorf("the instructions state the rule without stating that its timing "+
			"was deferred: %q", instructions)
	}

	caps := res["capabilities"].(map[string]any)
	if _, ok := caps["tools"]; !ok {
		t.Errorf("the server does not advertise tools: %v", caps)
	}
}

func TestInitializeAcceptsAnyClientVersion(t *testing.T) {
	// The specification says to answer with a version the server supports and
	// let the client decide, rather than refusing outright.
	replies := session(t,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"1999-01-01"}}`)
	res := replies[0]["result"].(map[string]any)
	if res["protocolVersion"] != mcp.ProtocolVersion {
		t.Errorf("protocolVersion = %v", res["protocolVersion"])
	}
}

func TestInitializeWithoutParams(t *testing.T) {
	replies := session(t, `{"jsonrpc":"2.0","id":1,"method":"initialize"}`)
	if _, ok := replies[0]["result"]; !ok {
		t.Errorf("initialize without params failed: %v", replies[0])
	}
}

func TestToolsList(t *testing.T) {
	replies := session(t, initialize, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	res := replies[1]["result"].(map[string]any)
	tools := res["tools"].([]any)

	if len(tools) < 8 {
		t.Fatalf("got %d tools, want the full set", len(tools))
	}

	seen := map[string]map[string]any{}
	for _, raw := range tools {
		tool := raw.(map[string]any)
		name, _ := tool["name"].(string)
		seen[name] = tool

		// A tool with no description is a tool a model will not call correctly.
		if desc, _ := tool["description"].(string); len(desc) < 40 {
			t.Errorf("%s has a thin description: %q", name, desc)
		}
		schema, ok := tool["inputSchema"].(map[string]any)
		if !ok || schema["type"] != "object" {
			t.Errorf("%s has no object input schema: %v", name, tool["inputSchema"])
		}
	}

	for _, want := range []string{
		"askiso_search", "askiso_info", "askiso_lint", "askiso_check_profile",
		"askiso_validate", "askiso_generate", "askiso_translate", "askiso_code",
		"askiso_diff", "askiso_convert",
	} {
		if _, ok := seen[want]; !ok {
			t.Errorf("%s is missing", want)
		}
	}

	// The tools that read the user's own schemas must say so, because that is
	// why a call may report that nothing is installed.
	for _, name := range []string{"askiso_validate", "askiso_diff"} {
		desc := seen[name]["description"].(string)
		if !strings.Contains(desc, "iso20022.org") {
			t.Errorf("%s does not explain that it needs a catalogue: %q", name, desc)
		}
	}
}

func TestCallsBeforeInitialize(t *testing.T) {
	replies := session(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`,
		call(2, "askiso_lint", map[string]any{"xml": "<Document/>"}))

	for _, r := range replies {
		e, ok := r["error"].(map[string]any)
		if !ok {
			t.Fatalf("a call before initialize succeeded: %v", r)
		}
		if !strings.Contains(e["message"].(string), "initialize") {
			t.Errorf("error = %v", e)
		}
	}
}

func TestToolsRequireInitializedNotification(t *testing.T) {
	rawInitialize := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`
	replies := session(t,
		rawInitialize,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/list"}`,
	)
	if len(replies) != 3 {
		t.Fatalf("got %d replies, want 3: %v", len(replies), replies)
	}
	if _, refused := replies[1]["error"]; !refused {
		t.Fatalf("tools/list entered operation phase before initialized notification: %v", replies[1])
	}
	if _, accepted := replies[2]["result"]; !accepted {
		t.Fatalf("tools/list stayed unavailable after initialized notification: %v", replies[2])
	}

	unsolicited := session(t,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/list"}`,
	)
	if len(unsolicited) != 1 || unsolicited[0]["error"] == nil {
		t.Fatalf("unsolicited initialized notification unlocked tools: %v", unsolicited)
	}
}

func TestNotificationsAreNotAnswered(t *testing.T) {
	replies := session(t,
		initialize,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":1}}`,
		`{"jsonrpc":"2.0","id":9,"method":"ping"}`)

	// Only initialize and ping carry an id, so only those get replies.
	if len(replies) != 2 {
		t.Fatalf("got %d replies, want 2: %v", len(replies), replies)
	}
	if replies[1]["id"].(float64) != 9 {
		t.Errorf("the second reply is not the ping: %v", replies[1])
	}
}

func TestProtocolErrors(t *testing.T) {
	cases := []struct {
		name, request, want string
	}{
		{"not JSON", "this is not json", "not valid JSON"},
		{"wrong version", `{"jsonrpc":"1.0","id":1,"method":"ping"}`, `"jsonrpc"`},
		{"unknown method", `{"jsonrpc":"2.0","id":1,"method":"does/not/exist"}`, "no such method"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			replies := session(t, tc.request)
			if len(replies) != 1 {
				t.Fatalf("got %d replies, want 1", len(replies))
			}
			e, ok := replies[0]["error"].(map[string]any)
			if !ok {
				t.Fatalf("no error in %v", replies[0])
			}
			if !strings.Contains(e["message"].(string), tc.want) {
				t.Errorf("message = %q, want it to mention %q", e["message"], tc.want)
			}
		})
	}
}

func TestBlankLinesAreIgnored(t *testing.T) {
	replies := session(t, "", "   ", initialize, "")
	if len(replies) != 1 {
		t.Fatalf("got %d replies, want 1", len(replies))
	}
}

func TestUnknownToolIsAProtocolError(t *testing.T) {
	// Naming a tool that does not exist is the client's mistake, not a failure
	// the model should reason about.
	replies := session(t, initialize, call(2, "askiso_nope", nil))
	e, ok := replies[1]["error"].(map[string]any)
	if !ok {
		t.Fatalf("no error in %v", replies[1])
	}
	if !strings.Contains(e["message"].(string), "askiso_nope") {
		t.Errorf("message = %v", e["message"])
	}
}

func TestMalformedToolParams(t *testing.T) {
	replies := session(t, initialize,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":"not an object"}`)
	if _, ok := replies[1]["error"]; !ok {
		t.Errorf("malformed parameters were accepted: %v", replies[1])
	}
}

// ---------------------------------------------------------------------------
// Tools
// ---------------------------------------------------------------------------

const sampleMT103 = `{1:F01BANKGB2LAXXX0000000000}{2:I103BANKDEFFXXXXN}{3:{121:f3a1b2c4-5d6e-4f70-8a91-2b3c4d5e6f70}}{4:
:20:REF20260824001
:32A:260824EUR25000,00
:50K:/GB29NWBK60161331926819
ACME TRADING LIMITED
14 GRESHAM STREET
LONDON EC2V 7NN
:59:/DE89370400440532013000
MUELLER GMBH
:71A:SHA
-}`

func TestSearchTool(t *testing.T) {
	replies := session(t, initialize,
		call(2, "askiso_search", map[string]any{"query": "pacs.008", "limit": 3}))

	sc := result(t, replies[1])
	messages := sc["messages"].([]any)
	if len(messages) == 0 || len(messages) > 3 {
		t.Fatalf("got %d messages, want at most 3", len(messages))
	}
	first := messages[0].(map[string]any)
	if !strings.HasPrefix(first["id"].(string), "pacs.008") {
		t.Errorf("first result = %v", first)
	}
}

func TestSearchToolRequiresQuery(t *testing.T) {
	replies := session(t, initialize, call(2, "askiso_search", map[string]any{}))
	if msg := toolError(t, replies[1]); !strings.Contains(msg, "query") {
		t.Errorf("message = %q; it should name the missing argument", msg)
	}
}

func TestInfoTool(t *testing.T) {
	replies := session(t, initialize,
		call(2, "askiso_info", map[string]any{"message_id": "pacs.008.001.10"}))

	sc := result(t, replies[1])
	if sc["id"] != "pacs.008.001.10" {
		t.Errorf("id = %v", sc["id"])
	}
	// A message set has to be named whether or not the schema is installed, so
	// the answer always says where the specification comes from.
	if sets, _ := sc["message_sets"].([]any); len(sets) == 0 {
		t.Errorf("no message set was named: %v", sc)
	}
}

func TestLintTool(t *testing.T) {
	// A generated message must lint clean; a broken IBAN must not.
	gen := session(t, initialize,
		call(2, "askiso_generate", map[string]any{"message_type": "pacs.008", "preset": "sepa"}))
	xmlDoc := result(t, gen[1])["xml"].(string)

	clean := session(t, initialize, call(2, "askiso_lint", map[string]any{"xml": xmlDoc}))
	if errs := result(t, clean[1])["error_count"].(float64); errs != 0 {
		t.Errorf("a generated message did not lint clean: %v", text(clean[1]["result"].(map[string]any)))
	}

	broken := strings.Replace(xmlDoc, "<IBAN>", "<IBAN>ZZ", 1)
	dirty := session(t, initialize, call(2, "askiso_lint", map[string]any{"xml": broken}))
	if errs := result(t, dirty[1])["error_count"].(float64); errs == 0 {
		t.Error("a corrupted IBAN linted clean")
	}
}

func TestLintToolRequiresXML(t *testing.T) {
	replies := session(t, initialize, call(2, "askiso_lint", map[string]any{}))
	if msg := toolError(t, replies[1]); !strings.Contains(msg, "xml") {
		t.Errorf("message = %q", msg)
	}
}

func TestCheckProfileTool(t *testing.T) {
	// With no profile named, the tool lists what is available rather than
	// guessing.
	listed := session(t, initialize,
		call(2, "askiso_check_profile", map[string]any{"xml": "<Document/>"}))
	if profiles, _ := result(t, listed[1])["profiles"].([]any); len(profiles) == 0 {
		t.Fatal("no profiles were listed")
	}

	// An unstructured address is exactly what cbpr-2026 exists to catch.
	doc := `<?xml version="1.0"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">
  <FIToFICstmrCdtTrf><CdtTrfTxInf><Cdtr>
    <Nm>MUELLER GMBH</Nm>
    <PstlAdr><AdrLine>HAUPTSTRASSE 12</AdrLine><AdrLine>FRANKFURT</AdrLine></PstlAdr>
  </Cdtr></CdtTrfTxInf></FIToFICstmrCdtTrf>
</Document>`
	checked := session(t, initialize,
		call(2, "askiso_check_profile", map[string]any{"xml": doc, "profile": "cbpr-2026"}))
	if errs := result(t, checked[1])["error_count"].(float64); errs == 0 {
		t.Errorf("an unstructured address passed cbpr-2026: %v",
			text(checked[1]["result"].(map[string]any)))
	}
}

func TestGenerateTool(t *testing.T) {
	replies := session(t, initialize, call(2, "askiso_generate", map[string]any{
		"message_type": "pacs.008",
		"preset":       "target2",
		"amount":       "1234.56",
		"currency":     "EUR",
		"with_bah":     true,
	}))

	sc := result(t, replies[1])
	doc := sc["xml"].(string)
	for _, want := range []string{"<AppHdr", "1234.56", "urn:iso:std:iso:20022:tech:xsd:pacs.008"} {
		if !strings.Contains(doc, want) {
			t.Errorf("the generated message is missing %q:\n%s", want, doc)
		}
	}

	bad := session(t, initialize,
		call(2, "askiso_generate", map[string]any{"message_type": "nope.999"}))
	toolError(t, bad[1])
}

func TestTranslateToolConverts(t *testing.T) {
	replies := session(t, initialize,
		call(2, "askiso_translate", map[string]any{"mt_message": sampleMT103}))

	sc := result(t, replies[1])
	if sc["source_type"] != "MT103" || sc["target_type"] != "pacs.008.001.10" {
		t.Errorf("got %v -> %v", sc["source_type"], sc["target_type"])
	}
	if sc["lossless"].(bool) {
		t.Error("a message with unstructured addresses was reported as lossless")
	}

	// The fidelity report is the point of the tool; without it a model cannot
	// tell what the conversion dropped.
	report := sc["report"].([]any)
	if len(report) < 5 {
		t.Fatalf("the report has %d entries", len(report))
	}
	summary := sc["summary"].(map[string]any)
	total := 0
	for _, v := range summary {
		total += int(v.(float64))
	}
	if total != len(report) {
		t.Errorf("the summary counts %d fields but the report has %d", total, len(report))
	}
}

func TestTranslateToolCrossReference(t *testing.T) {
	replies := session(t, initialize,
		call(2, "askiso_translate", map[string]any{"code": "MT103"}))
	sc := result(t, replies[1])
	if !strings.HasPrefix(sc["MXCode"].(string), "pacs.008") {
		t.Errorf("MXCode = %v", sc["MXCode"])
	}

	// With no arguments at all it says what it can do.
	listed := session(t, initialize, call(2, "askiso_translate", map[string]any{}))
	described := result(t, listed[1])
	if convertible, _ := described["convertible"].([]any); len(convertible) != 10 {
		t.Errorf("convertible = %v, want every supported MT type", convertible)
	}
	// Both directions have to be described, or a model only ever reaches for
	// one of them.
	if reverse, _ := described["convertible_mx"].([]any); len(reverse) != 6 {
		t.Errorf("convertible_mx = %v, want every supported ISO 20022 message", reverse)
	}

	unknown := session(t, initialize,
		call(2, "askiso_translate", map[string]any{"code": "MT999"}))
	toolError(t, unknown[1])
}

func TestTranslateToolRejectsGarbage(t *testing.T) {
	replies := session(t, initialize,
		call(2, "askiso_translate", map[string]any{"mt_message": "not an MT message"}))
	toolError(t, replies[1])
}

func TestCodeTool(t *testing.T) {
	replies := session(t, initialize, call(2, "askiso_code", map[string]any{"query": "AC04"}))
	sc := result(t, replies[1])
	codes := sc["codes"].([]any)
	if len(codes) == 0 || codes[0].(map[string]any)["code"] != "AC04" {
		t.Errorf("codes = %v", codes)
	}

	missing := session(t, initialize,
		call(2, "askiso_code", map[string]any{"query": "not-a-code-anywhere"}))
	toolError(t, missing[1])

	empty := session(t, initialize, call(2, "askiso_code", map[string]any{}))
	toolError(t, empty[1])
}

func TestConvertTool(t *testing.T) {
	doc := `<?xml version="1.0"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">
  <MsgId>MSG-1</MsgId>
</Document>`

	// The target format is inferred from the content when it is not named.
	toJSON := session(t, initialize, call(2, "askiso_convert", map[string]any{"content": doc}))
	sc := result(t, toJSON[1])
	if sc["format"] != "json" || !strings.Contains(sc["content"].(string), "MsgId") {
		t.Fatalf("XML did not convert to JSON: %v", sc)
	}

	back := session(t, initialize,
		call(2, "askiso_convert", map[string]any{"content": sc["content"]}))
	round := result(t, back[1])
	if round["format"] != "xml" || !strings.Contains(round["content"].(string), "<MsgId>MSG-1</MsgId>") {
		t.Errorf("the round trip lost the message: %v", round)
	}

	bad := session(t, initialize,
		call(2, "askiso_convert", map[string]any{"content": doc, "to": "yaml"}))
	if msg := toolError(t, bad[1]); !strings.Contains(msg, "yaml") {
		t.Errorf("message = %q", msg)
	}

	broken := session(t, initialize,
		call(2, "askiso_convert", map[string]any{"content": "<unclosed>", "to": "json"}))
	toolError(t, broken[1])

	brokenJSON := session(t, initialize,
		call(2, "askiso_convert", map[string]any{"content": "{oops", "to": "xml"}))
	toolError(t, brokenJSON[1])

	empty := session(t, initialize, call(2, "askiso_convert", map[string]any{}))
	toolError(t, empty[1])
}

func TestValidateAndDiffNeedArguments(t *testing.T) {
	// These two read the user's own schemas, so they may legitimately report
	// that nothing is installed. A missing argument must still be caught first.
	replies := session(t, initialize,
		call(2, "askiso_validate", map[string]any{}),
		call(3, "askiso_diff", map[string]any{"to": "pacs.008.001.10"}),
		call(4, "askiso_diff", map[string]any{"from": "pacs.008.001.09"}))

	if msg := toolError(t, replies[1]); !strings.Contains(msg, "xml") {
		t.Errorf("validate: %q", msg)
	}
	if msg := toolError(t, replies[2]); !strings.Contains(msg, "from") {
		t.Errorf("diff: %q", msg)
	}
	if msg := toolError(t, replies[3]); !strings.Contains(msg, "to") {
		t.Errorf("diff: %q", msg)
	}
}

func TestToolNames(t *testing.T) {
	s := mcp.New(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	names := s.ToolNames()
	if len(names) != len(s.Tools()) {
		t.Errorf("ToolNames returned %d of %d tools", len(names), len(s.Tools()))
	}
	for i := 1; i < len(names); i++ {
		if names[i-1] > names[i] {
			t.Fatalf("ToolNames is unsorted at %d: %q then %q", i, names[i-1], names[i])
		}
	}
}

func TestSetVersionIgnoresEmpty(t *testing.T) {
	s := mcp.New(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	s.SetVersion("")
	if s.Version != "dev" {
		t.Errorf("Version = %q, want the default kept", s.Version)
	}
}

func TestServeStopsOnCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var out bytes.Buffer
	s := mcp.New(strings.NewReader(initialize+"\n"), &out, &bytes.Buffer{})
	if err := s.Serve(ctx); err == nil {
		t.Error("Serve returned nil for a cancelled context")
	}
	if out.Len() != 0 {
		t.Errorf("a cancelled server still replied: %s", out.String())
	}
}

func TestServeCancellationInterruptsBlockedRead(t *testing.T) {
	reader, writer := io.Pipe()
	defer func() { _ = writer.Close() }()
	s := mcp.New(reader, &bytes.Buffer{}, &bytes.Buffer{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Serve(ctx) }()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Serve returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Serve stayed blocked after cancellation")
	}
}

func TestOversizedRequestIsRefused(t *testing.T) {
	// The transport has no framing header, so a client that never sends a
	// newline must not be able to exhaust memory.
	huge := `{"jsonrpc":"2.0","id":1,"method":"ping","params":"` + strings.Repeat("A", 9<<20) + `"}`

	var out bytes.Buffer
	s := mcp.New(strings.NewReader(huge), &out, &bytes.Buffer{})
	err := s.Serve(context.Background())
	if err == nil || !strings.Contains(err.Error(), "limit") {
		t.Errorf("Serve accepted an oversized request: %v", err)
	}
}

func TestWriteFailureIsReportedOnStderr(t *testing.T) {
	var errOut bytes.Buffer
	s := mcp.New(strings.NewReader(initialize+"\n"), failingWriter{}, &errOut)
	if err := s.Serve(context.Background()); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	// A broken stdout must not be silent, and must not be reported on stdout.
	if !strings.Contains(errOut.String(), "could not write") {
		t.Errorf("stderr = %q", errOut.String())
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errWrite }

var errWrite = &writeError{}

type writeError struct{}

func (*writeError) Error() string { return "the stream is closed" }

// ---------------------------------------------------------------------------
// Tools that read the user's own schemas
// ---------------------------------------------------------------------------

// sessionWith drives a server whose catalogue is supplied by the test.
func sessionWith(t *testing.T, open mcp.CatalogueFunc, requests ...string) []map[string]any {
	t.Helper()

	var out, errOut bytes.Buffer
	in := strings.NewReader(strings.Join(requests, "\n") + "\n")

	s := mcp.New(in, &out, &errOut)
	s.SetCatalogue(open)
	s.SetCatalogue(nil) // a nil replacement is ignored rather than disarming the server
	if err := s.Serve(context.Background()); err != nil {
		t.Fatalf("Serve: %v", err)
	}

	var replies []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("a reply is not valid JSON: %v\n%s", err, line)
		}
		replies = append(replies, m)
	}
	return replies
}

// noCatalogue stands in for a machine with nothing downloaded.
func noCatalogue() (*iso20022.Catalogue, error) {
	return nil, errNoCatalogue
}

var errNoCatalogue = errors.New("no catalogue found")

// installed opens the developer's real catalogue, skipping when there is none.
func installed(t *testing.T) mcp.CatalogueFunc {
	t.Helper()
	cat, err := iso20022.OpenCatalogue("")
	if err != nil {
		t.Skip("no ISO 20022 catalogue installed")
	}
	return func() (*iso20022.Catalogue, error) { return cat, nil }
}

func TestSchemaToolsWithoutACatalogue(t *testing.T) {
	replies := sessionWith(t, noCatalogue, initialize,
		call(2, "askiso_validate", map[string]any{"xml": "<Document/>"}),
		call(3, "askiso_diff", map[string]any{"from": "pacs.008.001.09", "to": "pacs.008.001.10"}))

	// Neither may fail obscurely: both must name the download that fixes it.
	for _, reply := range replies[1:] {
		msg := toolError(t, reply)
		if !strings.Contains(msg, "iso20022.org") || !strings.Contains(msg, "catalog add") {
			t.Errorf("the message does not say how to install schemas: %q", msg)
		}
	}
}

func TestSearchAndInfoWorkWithoutACatalogue(t *testing.T) {
	// Light mode: the embedded registry answers, and says nothing is installed.
	replies := sessionWith(t, noCatalogue, initialize,
		call(2, "askiso_search", map[string]any{"query": "camt.053"}),
		call(3, "askiso_info", map[string]any{"message_id": "camt.053.001.11"}))

	messages := result(t, replies[1])["messages"].([]any)
	if len(messages) == 0 {
		t.Fatal("search returned nothing in light mode")
	}
	if messages[0].(map[string]any)["installed"].(bool) {
		t.Error("a message was reported as installed with no catalogue")
	}

	info := result(t, replies[2])
	if info["installed"].(bool) {
		t.Error("info reported an installed schema with no catalogue")
	}
	if sets, _ := info["message_sets"].([]any); len(sets) == 0 {
		t.Error("info named no message set to download")
	}
}

func TestValidateToolAgainstRealSchemas(t *testing.T) {
	open := installed(t)

	gen := session(t, initialize,
		call(2, "askiso_generate", map[string]any{"message_type": "pacs.008", "preset": "sepa"}))
	doc := result(t, gen[1])["xml"].(string)

	replies := sessionWith(t, open, initialize, call(2, "askiso_validate", map[string]any{"xml": doc}))
	res := replies[1]["result"].(map[string]any)
	if isErr, _ := res["isError"].(bool); isErr {
		t.Skipf("pacs.008 is not installed in this catalogue: %s", text(res))
	}
	if valid, _ := result(t, replies[1])["valid"].(bool); !valid {
		t.Errorf("a generated message did not validate: %s", text(res))
	}

	// A message whose namespace names no installed schema is reported, not
	// guessed at.
	unknown := strings.Replace(doc, "pacs.008.001.10", "zzzz.999.999.99", 1)
	missing := sessionWith(t, open, initialize,
		call(2, "askiso_validate", map[string]any{"xml": unknown}))
	toolError(t, missing[1])
}

func TestDiffToolAgainstRealSchemas(t *testing.T) {
	open := installed(t)

	replies := sessionWith(t, open, initialize,
		call(2, "askiso_diff", map[string]any{"from": "pacs.008.001.09", "to": "pacs.008.001.10"}),
		call(3, "askiso_diff", map[string]any{
			"from": "pacs.008.001.10", "to": "pacs.008.001.13", "breaking_only": true}),
		call(4, "askiso_diff", map[string]any{"from": "pacs.008.001.10", "to": "zzzz.999.999.99"}))

	res := replies[1]["result"].(map[string]any)
	if isErr, _ := res["isError"].(bool); isErr {
		t.Skipf("both versions must be installed: %s", text(res))
	}

	step := result(t, replies[1])
	if step["common_paths"].(float64) == 0 {
		t.Error("two versions of the same message share no paths")
	}
	if step["breaking_changes"].(float64) != 0 {
		t.Errorf("pacs.008.001.09 -> .10 was reported as breaking: %s", text(res))
	}

	jump := replies[2]["result"].(map[string]any)
	if isErr, _ := jump["isError"].(bool); !isErr {
		sc := result(t, replies[2])
		if sc["breaking_changes"].(float64) == 0 {
			t.Error("pacs.008.001.10 -> .13 was reported as entirely compatible")
		}
		for _, raw := range sc["changes"].([]any) {
			if raw.(map[string]any)["severity"] != "breaking" {
				t.Errorf("breaking_only returned a compatible change: %v", raw)
			}
		}
	}

	toolError(t, replies[3])
}

func TestSearchLimitDefaults(t *testing.T) {
	// A query matching many messages is capped, so a model is not handed
	// hundreds of rows it has no use for.
	replies := session(t, initialize, call(2, "askiso_search", map[string]any{"query": "camt"}))
	if n := len(result(t, replies[1])["messages"].([]any)); n != 20 {
		t.Errorf("got %d messages, want the default cap of 20", n)
	}

	// A limit of zero means no cap.
	all := session(t, initialize,
		call(2, "askiso_search", map[string]any{"query": "camt", "limit": 0}))
	if n := len(result(t, all[1])["messages"].([]any)); n <= 20 {
		t.Errorf("a limit of zero capped the results at %d", n)
	}
}

func TestToolCallWithoutArguments(t *testing.T) {
	// A client may omit the arguments object entirely; the tool must report the
	// missing argument rather than dereferencing a nil map.
	replies := session(t, initialize,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"askiso_info"}}`)
	if msg := toolError(t, replies[1]); !strings.Contains(msg, "message_id") {
		t.Errorf("message = %q", msg)
	}
}

func TestRequiredArgumentsAreNamed(t *testing.T) {
	// Every tool that needs an argument has to say which one is missing, or a
	// model retries blindly.
	cases := map[string]string{
		"askiso_check_profile": "xml",
		"askiso_generate":      "message_type",
		"askiso_translate":     "",
	}
	for tool, want := range cases {
		t.Run(tool, func(t *testing.T) {
			replies := session(t, initialize, call(2, tool, map[string]any{}))
			if want == "" {
				// askiso_translate with nothing to do describes itself instead.
				if _, ok := result(t, replies[1])["mappings"]; !ok {
					t.Errorf("the tool did not describe itself: %v", replies[1])
				}
				return
			}
			if msg := toolError(t, replies[1]); !strings.Contains(msg, want) {
				t.Errorf("message = %q, want it to name %q", msg, want)
			}
		})
	}
}

func TestServeReportsAReadFailure(t *testing.T) {
	s := mcp.New(failingReader{}, &bytes.Buffer{}, &bytes.Buffer{})
	if err := s.Serve(context.Background()); err == nil {
		t.Error("Serve returned nil for a broken input stream")
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errRead }

var errRead = errors.New("the stream is broken")
