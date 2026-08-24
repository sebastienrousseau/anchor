// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package ai

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
	t.Setenv("OLLAMA_HOST", "http://127.0.0.1:1") // refused immediately

	ans := engine(t).Query("What is pacs.008?")
	if !strings.Contains(ans.Details, "comfortably long enough") {
		t.Errorf("the model reply should be used:\n%s", ans.Details)
	}
	if gotAuth != "Bearer test-key" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	if gotModel == "" {
		t.Error("a model should be requested")
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
}

func TestQueryUsesOllamaWhenAvailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/api/generate") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"response":"Ollama answered with enough text to be used."}`))
	}))
	defer srv.Close()

	t.Setenv("OLLAMA_HOST", srv.URL)
	t.Setenv("OPENAI_API_KEY", "")

	ans := engine(t).Query("What is pacs.008?")
	if !strings.Contains(ans.Details, "Ollama answered") {
		t.Errorf("the Ollama reply should be used:\n%s", ans.Details)
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
