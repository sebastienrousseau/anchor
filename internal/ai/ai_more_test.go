// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package ai

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebastienrousseau/askiso/internal/catalog"
)

func engine(t *testing.T) *Engine {
	t.Helper()
	return New(fixtureIndex(t))
}

// Every branch of the offline router must answer, never panic, and never return
// an empty body -- an assistant that says nothing is worse than one that says
// "I don't know".
func TestQueryCoversEveryBranch(t *testing.T) {
	e := engine(t)

	prompts := []string{
		"",
		"   ",
		"pacs.008 vs pacs.009",
		"compare pacs008 and pacs009",
		"pain.001 vs pacs.008",
		"pain001 versus pacs008",
		"camt.052 vs camt.053",
		"camt052 camt053",
		"show me the pacs.008 xml sample",
		"pacs.008 xsd schema",
		"what is camt.053",
		"pacs",
		"seev",
		"securities settlement",
		"what is the business application header",
		"credit transfer between banks",
		"zzz nothing matches this at all",
		"tell me about pacs 008 001 10",
	}
	for _, p := range prompts {
		t.Run(p, func(t *testing.T) {
			ans := e.Query(p)
			if strings.TrimSpace(ans.Summary) == "" && strings.TrimSpace(ans.Details) == "" {
				t.Error("the assistant returned nothing at all")
			}
		})
	}
}

func TestNormalizeQuery(t *testing.T) {
	cases := map[string]string{
		"pacs008":       "pacs.008",
		"pacs 008":      "pacs.008",
		"pacs-008":      "pacs.008",
		"pacs_008":      "pacs.008",
		"pacs008001 10": "pacs.008.001.10",
		"PACS008":       "pacs.008",
		"no codes here": "no codes here",
		"":              "",
	}
	for in, want := range cases {
		if got := NormalizeQuery(in); got != want {
			t.Errorf("NormalizeQuery(%q) = %q, want %q", in, got, want)
		}
	}
}

// An OpenAI-compatible endpoint is used when one is configured.
func TestQueryUsesConfiguredOpenAIEndpoint(t *testing.T) {
	var gotAuth, gotModel string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		var body struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotModel = body.Model

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"A reply that is comfortably long enough."}}]}`))
	}))
	defer srv.Close()

	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("OPENAI_BASE_URL", srv.URL)
	t.Setenv("OPENAI_MODEL", "test-model")
	t.Setenv("OLLAMA_HOST", "http://127.0.0.1:1") // refused immediately

	ans := engine(t).Query("What is pacs.008?")
	if !strings.Contains(ans.Details, "comfortably long enough") {
		t.Errorf("the model reply should be used:\n%s", ans.Details)
	}
	if gotAuth != "Bearer test-key" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	if gotModel != "test-model" {
		t.Errorf("model = %q, want configured model", gotModel)
	}
}

func TestQueryFallsBackWhenEndpointFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream is down", http.StatusInternalServerError)
	}))
	defer srv.Close()

	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("OPENAI_BASE_URL", srv.URL)
	t.Setenv("OLLAMA_HOST", "http://127.0.0.1:1")

	// The offline router must still answer.
	ans := engine(t).Query("What is pacs.008?")
	if strings.TrimSpace(ans.Details) == "" {
		t.Error("a failing endpoint must fall back to the offline answer")
	}
	if !strings.Contains(ans.ProviderWarning, "OpenAI-compatible") {
		t.Errorf("configured provider failure should be observable: %q", ans.ProviderWarning)
	}
}

func TestInvalidConfiguredProviderEndpointIsReported(t *testing.T) {
	t.Setenv("OLLAMA_HOST", "://bad")
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("OPENAI_BASE_URL", "file:///tmp/socket")

	ans := engine(t).Query("What is pacs.008?")
	for _, want := range []string{"OLLAMA_HOST", "OPENAI_BASE_URL"} {
		if !strings.Contains(ans.ProviderWarning, want) {
			t.Errorf("warning %q does not mention %s", ans.ProviderWarning, want)
		}
	}
}

func TestUnconfiguredLocalProviderFallbackIsQuiet(t *testing.T) {
	t.Setenv("OLLAMA_HOST", "")
	t.Setenv("OPENAI_API_KEY", "")
	ans := engine(t).Query("What is pacs.008?")
	if ans.ProviderWarning != "" {
		t.Errorf("default local-provider absence should stay quiet: %q", ans.ProviderWarning)
	}
}

func TestQueryUsesOllamaWhenAvailable(t *testing.T) {
	var gotModel string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/api/generate") {
			http.NotFound(w, r)
			return
		}
		var body struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotModel = body.Model
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"response":"Ollama answered with enough text to be used."}`))
	}))
	defer srv.Close()

	t.Setenv("OLLAMA_HOST", srv.URL)
	t.Setenv("OLLAMA_MODEL", "local-test-model")
	t.Setenv("OPENAI_API_KEY", "")

	ans := engine(t).Query("What is pacs.008?")
	if !strings.Contains(ans.Details, "Ollama answered") {
		t.Errorf("the Ollama reply should be used:\n%s", ans.Details)
	}
	if gotModel != "local-test-model" {
		t.Errorf("model = %q, want configured model", gotModel)
	}
}

func TestOversizedLLMResponseFallsBack(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"response":"` + strings.Repeat("x", maxLLMResponse) + `"}`))
	}))
	defer srv.Close()
	t.Setenv("OLLAMA_HOST", srv.URL)
	t.Setenv("OPENAI_API_KEY", "")
	ans := engine(t).Query("What is pacs.008?")
	if strings.Contains(ans.Details, strings.Repeat("x", 100)) {
		t.Fatal("oversized provider response was accepted")
	}
}

func TestOllamaMalformedResponseIsIgnored(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	t.Setenv("OLLAMA_HOST", srv.URL)
	t.Setenv("OPENAI_API_KEY", "")

	if ans := engine(t).Query("What is pacs.008?"); strings.TrimSpace(ans.Details) == "" {
		t.Error("a malformed reply must fall back to the offline answer")
	}
}

func TestOpenAIEmptyChoicesIgnored(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer srv.Close()

	t.Setenv("OPENAI_API_KEY", "k")
	t.Setenv("OPENAI_BASE_URL", srv.URL)
	t.Setenv("OLLAMA_HOST", "http://127.0.0.1:1")

	if ans := engine(t).Query("What is pacs.008?"); strings.TrimSpace(ans.Details) == "" {
		t.Error("an empty choice list must fall back to the offline answer")
	}
}

// A non-HTTP endpoint must be refused rather than dialled.
func TestInvalidEndpointsAreRejected(t *testing.T) {
	for _, raw := range []string{"", "ftp://example.com", "file:///etc/passwd", "localhost:11434"} {
		if isValidEndpoint(raw) {
			t.Errorf("isValidEndpoint(%q) should be false", raw)
		}
	}
	for _, raw := range []string{"http://localhost:11434", "https://api.openai.com/v1"} {
		if !isValidEndpoint(raw) {
			t.Errorf("isValidEndpoint(%q) should be true", raw)
		}
	}

	t.Setenv("OLLAMA_HOST", "ftp://nope")
	t.Setenv("OPENAI_API_KEY", "k")
	t.Setenv("OPENAI_BASE_URL", "file:///etc/passwd")

	if ans := engine(t).Query("What is pacs.008?"); strings.TrimSpace(ans.Details) == "" {
		t.Error("invalid endpoints must fall back to the offline answer")
	}
}

func TestShortLLMReplyIsIgnored(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"response":"no"}`))
	}))
	defer srv.Close()

	t.Setenv("OLLAMA_HOST", srv.URL)
	t.Setenv("OPENAI_API_KEY", "")

	ans := engine(t).Query("What is pacs.008?")
	if strings.Contains(ans.Details, "no") && len(ans.Details) < 15 {
		t.Error("a trivially short model reply should not replace the offline answer")
	}
}

// The remaining branches of the offline router: the sample-preview path (which
// reads the file from disk), the cover-payment explanation, and the
// single-message explainers for each domain.
func TestQueryRemainingBranches(t *testing.T) {
	e := engine(t)

	for _, p := range []string{
		"show me the xml for pacs.008.001.10",
		"pacs.008.001.10 sample",
		"explain cover payments in pacs.009",
		"what is a cover payment pacs009",
		"what is pacs.009",
		"pacs009 explain",
		"what is pain.001",
		"pain001 explain",
		"camt.053 xsd",
		"acmt",
		"colr",
	} {
		t.Run(p, func(t *testing.T) {
			ans := e.Query(p)
			if strings.TrimSpace(ans.Summary) == "" && strings.TrimSpace(ans.Details) == "" {
				t.Error("the assistant returned nothing")
			}
		})
	}
}

// NormalizeQuery on a code with only a partial version tail.
func TestNormalizeQueryPartialCodes(t *testing.T) {
	if got := NormalizeQuery("head001"); got != "head.001" {
		t.Errorf("got %q", got)
	}
	if got := NormalizeQuery("seev 031 001 14"); got != "seev.031.001.14" {
		t.Errorf("got %q", got)
	}
}

// The OpenAI base URL defaults when the variable is empty.
func TestOpenAIDefaultBaseURL(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "k")
	t.Setenv("OPENAI_BASE_URL", "")
	t.Setenv("OLLAMA_HOST", "http://127.0.0.1:1")

	// api.openai.com is unreachable from the test environment, so this exercises
	// the default-URL path and the failure fallback together.
	if ans := engine(t).Query("What is pacs.008?"); strings.TrimSpace(ans.Details) == "" {
		t.Error("the offline answer should still be produced")
	}
}

// A body the transport cannot deliver exercises the request-construction and
// decode error paths.
func TestLLMTransportErrors(t *testing.T) {
	// A server that closes the connection mid-response.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "500")
		_, _ = w.Write([]byte(`{"response":"truncated`))
	}))
	defer srv.Close()

	t.Setenv("OLLAMA_HOST", srv.URL)
	t.Setenv("OPENAI_API_KEY", "")
	if ans := engine(t).Query("What is pacs.008?"); strings.TrimSpace(ans.Details) == "" {
		t.Error("a truncated reply must fall back")
	}

	// An endpoint that is syntactically valid but unroutable.
	t.Setenv("OLLAMA_HOST", "http://127.0.0.1:1")
	t.Setenv("OPENAI_API_KEY", "k")
	t.Setenv("OPENAI_BASE_URL", "http://127.0.0.1:1")
	if ans := engine(t).Query("What is pacs.008?"); strings.TrimSpace(ans.Details) == "" {
		t.Error("an unreachable endpoint must fall back")
	}
}

// The sample-preview branch reads the instance from disk and truncates it.
func TestQueryShowsSamplePreview(t *testing.T) {
	idx := fixtureIndexWithLongSample(t)
	ans := New(idx).Query("show the xml sample for pacs.008.001.10")
	if strings.TrimSpace(ans.Details) == "" {
		t.Fatal("no answer produced")
	}
}

func TestNormalizeQueryLeavesUnmatchedText(t *testing.T) {
	if got := NormalizeQuery("just words"); got != "just words" {
		t.Errorf("got %q", got)
	}
}

func TestQueryGenericMessageIdentifierFallback(t *testing.T) {
	root := t.TempDir()
	schemas := filepath.Join(root, "Administration", "Version 2.0", "Schemas")
	if err := os.MkdirAll(schemas, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(schemas, "admi.004.001.02.xsd"), []byte("<schema/>"), 0o600); err != nil {
		t.Fatal(err)
	}
	idx, err := catalog.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	ans := New(idx).Query("tell me about admi.004.001.02")
	if !strings.Contains(ans.Summary, "admi.004.001.02") || !strings.Contains(ans.Details, "Administration") {
		t.Fatalf("generic identifier answer = %+v", ans)
	}
}

func TestQueryShortSampleAndDirectCatalogueFallback(t *testing.T) {
	idx := fixtureIndex(t)
	short := filepath.Join(t.TempDir(), "short.xml")
	if err := os.WriteFile(short, []byte("<Document/>"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := idx.MessageMap["pacs.008.001.10"]
	m.XMLSamplePath = short
	idx.MessageMap[m.ID] = m
	for i := range idx.Messages {
		if idx.Messages[i].ID == m.ID {
			idx.Messages[i] = m
		}
	}
	ans := New(idx).Query("show the xml sample for pacs.008.001.10")
	if !strings.Contains(ans.Details, "<Document/>") || strings.Contains(ans.Details, "truncated") {
		t.Fatalf("short sample preview = %+v", ans)
	}

	m.Category = "Obscure Widget"
	idx.MessageMap[m.ID] = m
	for i := range idx.Messages {
		if idx.Messages[i].ID == m.ID {
			idx.Messages[i] = m
		}
	}
	ans = New(idx).Query("Obscure Widget")
	if len(ans.RelatedMsgs) == 0 || !strings.Contains(ans.Details, "Found") {
		t.Fatalf("direct catalogue fallback = %+v", ans)
	}
}

func TestProviderCallsRejectMalformedRequestURLs(t *testing.T) {
	e := engine(t)
	if _, ok := e.callOllama("://bad", "question"); ok {
		t.Fatal("Ollama accepted a malformed request URL")
	}
	if _, ok := e.callOpenAI("key", "://bad", "question"); ok {
		t.Fatal("OpenAI accepted a malformed request URL")
	}
}

func TestProviderHTTPAndTruncatedBodyFailures(t *testing.T) {
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer ollama.Close()
	t.Setenv("OLLAMA_HOST", ollama.URL)
	t.Setenv("OPENAI_API_KEY", "")
	if _, ok, _ := engine(t).queryExternalLLM("question"); ok {
		t.Fatal("non-200 Ollama response was accepted")
	}

	openai := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "500")
		_, _ = w.Write([]byte(`{"choices":[`))
	}))
	defer openai.Close()
	t.Setenv("OLLAMA_HOST", "http://127.0.0.1:1")
	t.Setenv("OPENAI_API_KEY", "key")
	t.Setenv("OPENAI_BASE_URL", openai.URL)
	if _, ok, _ := engine(t).queryExternalLLM("question"); ok {
		t.Fatal("truncated OpenAI response was accepted")
	}
}
