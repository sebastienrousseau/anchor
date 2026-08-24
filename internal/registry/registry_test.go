// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package registry

import (
	"bytes"
	"compress/gzip"
	"strings"
	"testing"
)

func TestLoadEmbedded(t *testing.T) {
	r, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(r.Sets) == 0 {
		t.Fatal("registry has no message sets")
	}
	if len(r.Messages) < 2000 {
		t.Errorf("registry has only %d messages; the standard defines thousands", len(r.Messages))
	}
}

// Load caches, so a second call must return the same pointer rather than
// re-decoding the blob on every command.
func TestLoadIsCached(t *testing.T) {
	a, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	b, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Error("Load should return the cached registry")
	}
}

func TestLookupKnownMessages(t *testing.T) {
	r := MustLoad()

	for _, tc := range []struct{ id, base, domain string }{
		{"pacs.008.001.10", "pacs.008", "pacs"},
		{"pain.001.001.11", "pain.001", "pain"},
		{"camt.053.001.11", "camt.053", "camt"},
		{"head.001.001.02", "head.001", "head"},
	} {
		m, ok := r.Lookup(tc.id)
		if !ok {
			t.Errorf("%s should be in the registry", tc.id)
			continue
		}
		if m.BaseCode != tc.base {
			t.Errorf("%s: BaseCode = %s, want %s", tc.id, m.BaseCode, tc.base)
		}
		if m.Domain != tc.domain {
			t.Errorf("%s: Domain = %s, want %s", tc.id, m.Domain, tc.domain)
		}
		if len(m.SetIDs) == 0 {
			t.Errorf("%s: no publishing message set recorded", tc.id)
		}
	}

	if _, ok := r.Lookup("nope.999.999.99"); ok {
		t.Error("an unknown identifier must not resolve")
	}
}

// The point of the registry is telling users where to get a schema they do not
// have, so every message must resolve to a real download URL.
func TestSetsForYieldsDownloadURL(t *testing.T) {
	r := MustLoad()

	sets := r.SetsFor("pacs.008.001.10")
	if len(sets) == 0 {
		t.Fatal("pacs.008.001.10 should name at least one message set")
	}
	s := sets[0]
	if s.Name == "" {
		t.Error("message set should have a display name")
	}
	url := s.DownloadURL()
	if !strings.HasPrefix(url, "https://www.iso20022.org/message-set/") || !strings.HasSuffix(url, "/download") {
		t.Errorf("unexpected download URL: %s", url)
	}
	if !strings.Contains(url, s.ID) {
		t.Errorf("download URL %s should contain set id %s", url, s.ID)
	}

	if got := r.SetsFor("nope.999.999.99"); got != nil {
		t.Errorf("unknown message should yield no sets, got %v", got)
	}
}

// Every message must point at a set that actually exists in the same blob,
// otherwise the guidance AskIso prints would be a dead end.
func TestEverySetReferenceResolves(t *testing.T) {
	r := MustLoad()
	orphans := 0
	for _, m := range r.Messages {
		for _, sid := range m.SetIDs {
			if _, ok := r.Set(sid); !ok {
				if orphans < 5 {
					t.Errorf("%s references unknown message set %q", m.ID, sid)
				}
				orphans++
			}
		}
	}
	if orphans > 0 {
		t.Errorf("%d dangling set references", orphans)
	}
}

func TestSearchRanking(t *testing.T) {
	r := MustLoad()

	exact := r.Search("pacs.008.001.10")
	if len(exact) == 0 || exact[0].ID != "pacs.008.001.10" {
		t.Errorf("exact identifier should rank first, got %v", firstIDs(exact, 3))
	}

	base := r.Search("pacs.008")
	if len(base) < 5 {
		t.Errorf("pacs.008 should match several versions, got %d", len(base))
	}
	for _, m := range base {
		if m.BaseCode != "pacs.008" {
			t.Errorf("unexpected match for pacs.008: %s", m.ID)
			break
		}
	}

	if got := r.Search("zzz-not-a-message"); len(got) != 0 {
		t.Errorf("expected no results, got %d", len(got))
	}
	if got := r.Search(""); len(got) != len(r.Messages) {
		t.Errorf("empty query should return everything: got %d, want %d", len(got), len(r.Messages))
	}
}

func TestSearchIsCaseInsensitive(t *testing.T) {
	r := MustLoad()
	if got := r.Search("PACS.008.001.10"); len(got) == 0 || got[0].ID != "pacs.008.001.10" {
		t.Errorf("search should be case-insensitive, got %v", firstIDs(got, 3))
	}
}

func TestDomains(t *testing.T) {
	r := MustLoad()
	d := r.Domains()
	for _, want := range []string{"pacs", "pain", "camt", "seev", "sese"} {
		if d[want] == 0 {
			t.Errorf("domain %q should have messages", want)
		}
	}
	total := 0
	for _, n := range d {
		total += n
	}
	if total != len(r.Messages) {
		t.Errorf("domain counts sum to %d, want %d", total, len(r.Messages))
	}
}

func TestDecodeRejectsMalformedBlob(t *testing.T) {
	for name, body := range map[string]string{
		"empty":            "",
		"no sections":      "pacs.008.001.10\t1\n",
		"short set record": "#SETS\n1\tName\n",
		"short message":    "#SETS\n1\tName\tslug\tv01\t5\n#MESSAGES\npacs.008.001.10\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := decode(gzipBytes(t, body)); err == nil {
				t.Errorf("decode should reject %s", name)
			}
		})
	}
}

func TestDecodeRoundTrip(t *testing.T) {
	blob := "#SETS\n" +
		"7\tPayments Clearing and Settlement\tpayments-clearing\tv11\t42\n" +
		"#MESSAGES\n" +
		"pacs.008.001.10\t7\n"

	r, err := decode(gzipBytes(t, blob))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(r.Sets) != 1 || len(r.Messages) != 1 {
		t.Fatalf("got %d sets and %d messages, want 1 and 1", len(r.Sets), len(r.Messages))
	}
	s, ok := r.Set("7")
	if !ok || s.FilesCount != 42 || s.Version != "v11" {
		t.Errorf("set decoded incorrectly: %+v", s)
	}
	if s.String() != "Payments Clearing and Settlement v11" {
		t.Errorf("String() = %q", s.String())
	}
}

func gzipBytes(t *testing.T, s string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write([]byte(s)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func firstIDs(ms []Message, n int) []string {
	var out []string
	for i, m := range ms {
		if i == n {
			break
		}
		out = append(out, m.ID)
	}
	return out
}
