// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package tui

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/sebastienrousseau/anchor/internal/catalog"
	"github.com/sebastienrousseau/anchor/internal/registry"
	"github.com/sebastienrousseau/anchor/pkg/iso20022"
)

// Browsing a catalogue tells you a message exists. What someone actually wants
// to know is whether the one in front of them is correct, and whether it will
// still be accepted after 14 November 2026. Both answers are one keystroke away
// here, from the same engine the CLI runs.

// checkMessage runs every check Anchor has against a message's sample and puts
// the report in the viewer.
func (m *Model) checkMessage(msg catalog.Message) {
	m.viewingTitle = msg.ID + " (check)"
	m.viewingContent = m.renderCheck(msg)
	m.viewport.SetContent(m.viewingContent)
	m.viewport.GotoTop()
}

func (m *Model) renderCheck(msg catalog.Message) string {
	var b strings.Builder

	fmt.Fprintf(&b, "%s\n%s\n\n", msg.ID, strings.Repeat("─", len(msg.ID)))

	if msg.XMLSamplePath == "" {
		fmt.Fprintf(&b, "No sample message is installed for %s, so there is nothing to check.\n\n", msg.ID)
		b.WriteString("Samples ship alongside the schemas in the Registration Authority's\n")
		b.WriteString("download. Install the set with:\n\n  anchor catalog fetch " + msg.BaseCode + "\n")
		return b.String()
	}

	instance, err := os.ReadFile(msg.XMLSamplePath)
	if err != nil {
		fmt.Fprintf(&b, "Could not read the sample: %v\n", err)
		return b.String()
	}

	fmt.Fprintf(&b, "sample   %s\n", msg.XMLSamplePath)
	if msg.XSDPath != "" {
		fmt.Fprintf(&b, "schema   %s\n", msg.XSDPath)
	}
	b.WriteString("\n")

	b.WriteString(m.renderLint(instance))
	b.WriteString(m.renderSchema(instance, msg))
	b.WriteString(m.renderProfile(instance))

	b.WriteString("\nRun the same checks outside the TUI:\n")
	fmt.Fprintf(&b, "  anchor lint %s --profile cbpr-2026\n", msg.XMLSamplePath)
	fmt.Fprintf(&b, "  anchor validate %s\n", msg.XMLSamplePath)
	return b.String()
}

func (m *Model) renderLint(instance []byte) string {
	var b strings.Builder
	b.WriteString("BUSINESS RULES\n")

	res, err := iso20022.Lint(instance, "")
	if err != nil {
		fmt.Fprintf(&b, "  could not lint: %v\n\n", err)
		return b.String()
	}
	if res.Errors == 0 && res.Warnings == 0 {
		fmt.Fprintf(&b, "  ✅ %d check(s) passed\n\n", res.Passed)
		return b.String()
	}

	for _, issue := range res.Issues {
		mark := "⚠️ "
		if issue.Severity == iso20022.SeverityError {
			mark = "❌"
		}
		fmt.Fprintf(&b, "  %s [%s] %s\n", mark, issue.Rule, issue.Message)
		if issue.Field != "" {
			fmt.Fprintf(&b, "       %s = %s\n", issue.Field, issue.Value)
		}
	}
	b.WriteString("\n")
	return b.String()
}

func (m *Model) renderSchema(instance []byte, msg catalog.Message) string {
	var b strings.Builder
	b.WriteString("SCHEMA\n")

	if msg.XSDPath == "" {
		b.WriteString("  the schema for this message is not installed\n\n")
		return b.String()
	}

	res, err := iso20022.ValidateFile(instance, msg.XSDPath)
	if err != nil {
		fmt.Fprintf(&b, "  could not validate: %v\n\n", err)
		return b.String()
	}
	if res.Valid {
		b.WriteString("  ✅ the sample validates against its schema\n\n")
		return b.String()
	}

	const shown = 20
	for i, e := range res.Errors {
		if i == shown {
			fmt.Fprintf(&b, "  ... and %d more\n", len(res.Errors)-shown)
			break
		}
		fmt.Fprintf(&b, "  ❌ %d:%d %s\n       %s\n", e.Line, e.Column, e.Path, e.Message)
	}
	b.WriteString("\n")
	return b.String()
}

func (m *Model) renderProfile(instance []byte) string {
	var b strings.Builder
	b.WriteString("14 NOVEMBER 2026 (cbpr-2026)\n")

	res, err := iso20022.CheckProfile(instance, "cbpr-2026", "")
	if err != nil {
		fmt.Fprintf(&b, "  could not apply the profile: %v\n\n", err)
		return b.String()
	}
	if res.Checked == 0 {
		fmt.Fprintf(&b, "  exempt — this message type is out of scope: all %d rule(s) skipped\n\n", res.Skipped)
		return b.String()
	}
	if res.Errors == 0 && res.Warnings == 0 {
		fmt.Fprintf(&b, "  ✅ %d rule(s) passed\n\n", res.Checked)
		return b.String()
	}

	for _, f := range res.Findings {
		fmt.Fprintf(&b, "  ❌ [%s] %s\n       at %s\n", f.RuleID, f.Message, f.Path)
		if f.Remediation != "" {
			fmt.Fprintf(&b, "       %s\n", f.Remediation)
		}
	}
	b.WriteString("\n")
	return b.String()
}

// ---------------------------------------------------------------------------
// Catalogue manager
// ---------------------------------------------------------------------------

// renderCatalogue lists what is installed against the whole published standard,
// so someone can see at a glance what they are missing and where to get it.
func (m *Model) renderCatalogue() string {
	var b strings.Builder

	reg, err := registry.Load()
	if err != nil {
		fmt.Fprintf(&b, "Could not read the embedded registry: %v\n", err)
		return b.String()
	}

	installed := map[string]bool{}
	root := ""
	if m.idx != nil {
		root = m.idx.RootDir
		for _, msg := range m.idx.Messages {
			installed[msg.ID] = true
		}
	}

	// A set counts as installed when every message it publishes is present.
	type row struct {
		set     registry.Set
		have    int
		total   int
		partial bool
	}
	var rows []row
	for _, set := range reg.Sets {
		have, total := 0, 0
		for _, msg := range reg.Messages {
			if !setPublishes(msg, set.ID) {
				continue
			}
			total++
			if installed[msg.ID] {
				have++
			}
		}
		if total == 0 {
			continue
		}
		rows = append(rows, row{set: set, have: have, total: total, partial: have > 0 && have < total})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].set.Name != rows[j].set.Name {
			return rows[i].set.Name < rows[j].set.Name
		}
		return rows[i].set.Version < rows[j].set.Version
	})

	complete, partial := 0, 0
	for _, r := range rows {
		switch {
		case r.have == r.total:
			complete++
		case r.have > 0:
			partial++
		}
	}

	b.WriteString("CATALOGUE\n──────────\n\n")
	if root == "" {
		b.WriteString("Nothing is installed. Anchor still knows the whole standard from its\n")
		b.WriteString("embedded index, but reading schema text needs the files.\n\n")
	} else {
		fmt.Fprintf(&b, "root      %s\n", root)
	}
	fmt.Fprintf(&b, "sets      %d complete, %d partial, %d of %d published\n\n",
		complete, partial, complete+partial, len(rows))

	for _, r := range rows {
		mark := "  "
		switch {
		case r.have == r.total:
			mark = "✅"
		case r.have > 0:
			mark = "◐ "
		}
		fmt.Fprintf(&b, "%s %-46s %-6s %3d/%-3d\n", mark, truncate(r.set.Name, 46), r.set.Version, r.have, r.total)
		if r.have < r.total {
			fmt.Fprintf(&b, "     %s\n", r.set.DownloadURL())
		}
	}

	b.WriteString("\nInstall a set:  anchor catalog fetch <message-or-set>\n")
	b.WriteString("Import a file:  anchor catalog add <downloaded.zip>\n")
	return b.String()
}

// setPublishes reports whether a message belongs to a set.
func setPublishes(msg registry.Message, setID string) bool {
	for _, id := range msg.SetIDs {
		if id == setID {
			return true
		}
	}
	return false
}

// truncate shortens a name to fit a column, counting runes so an accented set
// name is not cut mid-character.
func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max <= 1 {
		return string(r[:max])
	}
	return string(r[:max-1]) + "…"
}
