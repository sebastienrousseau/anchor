// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

// Package registry holds AskIso's embedded knowledge of the ISO 20022 message
// catalogue: which message identifiers exist, which message set publishes each
// one, and where the Registration Authority hosts the download.
//
// It contains no schema, report, or specification text. That means AskIso's
// search, lookup, and guidance work with no setup at all, while anything
// needing the actual XSDs uses a catalogue the user supplies.
//
// registry.tsv.gz is generated, and manifest.json at the repository root is the
// input it is generated from — an index of the Registration Authority's message
// sets: identifier, title, category, version, download URL, and the schema
// filenames each set contains. Keeping it in the repository is what makes the
// generated file reproducible rather than something only its author can rebuild.
//
// Its "files" lists carry .xsd names only. The generator reads nothing else from
// them — it maps each schema filename back to the set that publishes it — so the
// Word, PDF and spreadsheet filenames the Registration Authority also ships were
// parsed and discarded. They are omitted here, which is 176,069 fewer strings
// and 96% less of a file, with no effect on the output: regenerating produces a
// byte-identical registry.tsv.gz.
//
//go:generate go run ../../scripts/gen-registry -manifest ../../manifest.json -out registry.tsv.gz
package registry

import (
	"bufio"
	"bytes"
	"compress/gzip"
	_ "embed"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"
)

//go:embed registry.tsv.gz
var packed []byte

// Set is a message set as published by the RA.
type Set struct {
	ID         string `json:"id"`      // RA message-set id, e.g. "691"
	Name       string `json:"name"`    // display name, e.g. "Account Switching"
	Slug       string `json:"slug"`    // url slug, e.g. "account-switching"
	Version    string `json:"version"` // e.g. "v03"
	FilesCount int    `json:"files_count"`
	// URL is the Registration Authority download location. It is derived, and
	// populated on marshal so JSON consumers do not have to build it.
	URL string `json:"url"`
}

// DownloadURL is where the RA hosts this set. AskIso never mirrors it.
func (s Set) DownloadURL() string {
	return "https://www.iso20022.org/message-set/" + s.ID + "/download"
}

// String renders "Account Switching v03".
func (s Set) String() string { return s.Name + " " + s.Version }

// Message is a message definition known to exist in the standard.
type Message struct {
	ID       string   `json:"id"`        // e.g. "pacs.008.001.10"
	BaseCode string   `json:"base_code"` // e.g. "pacs.008"
	Domain   string   `json:"domain"`    // e.g. "pacs"
	SetIDs   []string `json:"set_ids"`
}

// Registry is the decoded embedded index.
type Registry struct {
	Sets       []Set
	Messages   []Message
	setByID    map[string]Set
	messageMap map[string]Message
}

var (
	once    sync.Once
	loaded  *Registry
	loadErr error
)

// Load decodes the embedded registry. The result is cached.
func Load() (*Registry, error) {
	once.Do(func() { loaded, loadErr = decode(packed) })
	return loaded, loadErr
}

// MustLoad is Load for callers that cannot proceed without it. The data is
// embedded at build time, so a failure means a corrupt binary.
func MustLoad() *Registry {
	r, err := Load()
	if err != nil {
		panic("registry: " + err.Error())
	}
	return r
}

func decode(blob []byte) (*Registry, error) {
	zr, err := gzip.NewReader(bytes.NewReader(blob))
	if err != nil {
		return nil, fmt.Errorf("opening embedded registry: %w", err)
	}
	defer func() { _ = zr.Close() }()

	raw, err := io.ReadAll(zr)
	if err != nil {
		return nil, fmt.Errorf("reading embedded registry: %w", err)
	}

	r := &Registry{
		setByID:    map[string]Set{},
		messageMap: map[string]Message{},
	}

	const (
		sectionNone = iota
		sectionSets
		sectionMessages
	)
	section := sectionNone

	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		switch line {
		case "#SETS":
			section = sectionSets
			continue
		case "#MESSAGES":
			section = sectionMessages
			continue
		}
		if line == "" {
			continue
		}

		switch section {
		case sectionSets:
			f := strings.Split(line, "\t")
			if len(f) != 5 {
				return nil, fmt.Errorf("malformed set record: %q", line)
			}
			n, _ := strconv.Atoi(f[4])
			s := Set{ID: f[0], Name: f[1], Slug: f[2], Version: f[3], FilesCount: n}
			s.URL = s.DownloadURL()
			r.Sets = append(r.Sets, s)
			r.setByID[s.ID] = s

		case sectionMessages:
			f := strings.Split(line, "\t")
			if len(f) != 2 {
				return nil, fmt.Errorf("malformed message record: %q", line)
			}
			m := Message{
				ID:       f[0],
				BaseCode: baseCode(f[0]),
				Domain:   domain(f[0]),
				SetIDs:   strings.Split(f[1], ","),
			}
			r.Messages = append(r.Messages, m)
			r.messageMap[m.ID] = m

		default:
			return nil, fmt.Errorf("record outside any section: %q", line)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("scanning embedded registry: %w", err)
	}
	if len(r.Messages) == 0 {
		return nil, fmt.Errorf("embedded registry contains no messages")
	}
	return r, nil
}

// Lookup returns the message with exactly this identifier.
func (r *Registry) Lookup(id string) (Message, bool) {
	m, ok := r.messageMap[strings.ToLower(strings.TrimSpace(id))]
	return m, ok
}

// Set returns the message set with this RA id.
func (r *Registry) Set(id string) (Set, bool) {
	s, ok := r.setByID[id]
	return s, ok
}

// SetsFor returns the message sets that publish a message, newest first.
func (r *Registry) SetsFor(msgID string) []Set {
	m, ok := r.Lookup(msgID)
	if !ok {
		return nil
	}
	out := make([]Set, 0, len(m.SetIDs))
	for _, sid := range m.SetIDs {
		if s, ok := r.setByID[sid]; ok {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version > out[j].Version })
	return out
}

// Search ranks messages the same way the catalogue index does, so results are
// consistent whether or not a catalogue is installed.
func (r *Registry) Search(query string) []Message {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return r.Messages
	}

	type scored struct {
		m Message
		s int
	}
	var hits []scored
	for _, m := range r.Messages {
		score := 0
		switch {
		case m.ID == q:
			score = 100
		case m.BaseCode == q:
			score = 80
		case strings.HasPrefix(m.ID, q):
			score = 60
		case strings.Contains(m.ID, q):
			score = 40
		case strings.Contains(m.BaseCode, q):
			score = 30
		case m.Domain == q:
			score = 20
		default:
			// The registry carries no per-message name -- the Registration
			// Authority publishes titles per message set, not per message -- so
			// a keyword query is matched against the set names instead. That is
			// what makes "mandates" or "corporate actions" find anything.
			for _, id := range m.SetIDs {
				if set, ok := r.setByID[id]; ok && strings.Contains(strings.ToLower(set.Name), q) {
					score = 15
					break
				}
			}
		}
		if score > 0 {
			hits = append(hits, scored{m, score})
		}
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].s != hits[j].s {
			return hits[i].s > hits[j].s
		}
		return hits[i].m.ID < hits[j].m.ID
	})

	out := make([]Message, len(hits))
	for i, h := range hits {
		out[i] = h.m
	}
	return out
}

// Domains counts message definitions per business domain.
func (r *Registry) Domains() map[string]int {
	counts := make(map[string]int, 32)
	for _, m := range r.Messages {
		counts[m.Domain]++
	}
	return counts
}

func baseCode(id string) string {
	p := strings.Split(id, ".")
	if len(p) >= 2 {
		return p[0] + "." + p[1]
	}
	return id
}

func domain(id string) string {
	if i := strings.IndexByte(id, '.'); i > 0 {
		return id[:i]
	}
	return id
}
