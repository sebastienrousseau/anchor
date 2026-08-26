// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package cmd

import (
	"fmt"
	"strings"

	"github.com/sebastienrousseau/askiso/internal/catalog"
	"github.com/sebastienrousseau/askiso/internal/registry"
)

// AskISO runs in two modes.
//
// Light mode is the default on a fresh install: the embedded registry answers
// what exists, what it is called, and where the Registration Authority
// publishes it. Everything that does not need the XSD bytes works.
//
// Full mode adds a catalogue the user downloaded from iso20022.org, unlocking
// the schema text itself.
//
// A command that needs full mode must never fail with a bare error. It says
// which message set to download and which command installs it, so light mode is
// a starting point rather than a dead end.

// NotInstalledError reports a message AskISO knows about but has no schema for.
type NotInstalledError struct {
	Query   string
	Known   bool
	Sets    []registry.Set
	Reason  error
	Purpose string // e.g. "read its schema"
}

func (e *NotInstalledError) Error() string {
	var b strings.Builder

	if !e.Known {
		fmt.Fprintf(&b, "no ISO 20022 message matches %q\n\n", e.Query)
		b.WriteString("AskISO knows every message identifier in the published standard,\n")
		b.WriteString("so this is probably a typo. Try: askiso search " + firstToken(e.Query) + "\n")
		return b.String()
	}

	purpose := e.Purpose
	if purpose == "" {
		purpose = "use this message"
	}

	fmt.Fprintf(&b, "%s is part of the ISO 20022 standard, but its schema is not installed.\n\n", e.Query)
	fmt.Fprintf(&b, "To %s, download the message set from the Registration Authority:\n\n", purpose)

	for i, s := range e.Sets {
		if i == 3 {
			fmt.Fprintf(&b, "  ... and %d more\n", len(e.Sets)-3)
			break
		}
		fmt.Fprintf(&b, "  %-46s %s\n", s.String(), s.DownloadURL())
	}

	b.WriteString("\nThen import it:\n\n")
	b.WriteString("  askiso catalog add ~/Downloads/<downloaded>.zip\n")
	return b.String()
}

// resolveMessage finds a message in the installed catalogue, falling back to an
// actionable error built from the embedded registry.
//
// purpose completes the sentence "To <purpose>, download the message set" -- for
// example "read its schema" or "compare these versions".
func resolveMessage(query, purpose string) (catalog.Message, *catalog.Index, error) {
	idx, loadErr := loadCatalog()
	if loadErr == nil {
		if m, ok := idx.MessageMap[query]; ok {
			return m, idx, nil
		}
		if results := idx.Search(query); len(results) > 0 {
			return results[0], idx, nil
		}
	}
	return catalog.Message{}, idx, notInstalled(query, purpose, loadErr)
}

// notInstalled builds the guidance for a message AskISO cannot open.
func notInstalled(query, purpose string, reason error) error {
	e := &NotInstalledError{Query: query, Purpose: purpose, Reason: reason}

	reg, err := registry.Load()
	if err != nil {
		return e
	}

	msg, ok := reg.Lookup(query)
	if !ok {
		if results := reg.Search(query); len(results) > 0 {
			msg, ok = results[0], true
			e.Query = msg.ID
		}
	}
	if !ok {
		return e
	}

	e.Known = true
	e.Sets = reg.SetsFor(msg.ID)
	return e
}

// lightModeNotice is printed when a command produced a useful answer without a
// catalogue, so the reduced scope is never mistaken for the whole picture.
func lightModeNotice(what string) string {
	return fmt.Sprintf(
		"\n%s %s from AskISO's embedded index of the standard.\n"+
			"      Install schemas for full detail: askiso catalog add <zip>  (see: askiso catalog where)\n",
		subtleStyle.Render("light mode:"), what)
}

func firstToken(s string) string {
	if i := strings.IndexAny(s, " \t"); i > 0 {
		return s[:i]
	}
	return s
}
