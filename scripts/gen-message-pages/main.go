// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

// Command gen-message-pages writes one content page per ISO 20022 message
// definition, for askiso.io.
//
// There are 2,845 of them, and each one is a query somebody types: a developer
// searching "pacs.008.001.10", an analyst asking what camt.053 is for. The
// pages exist so those searches land somewhere that answers rather than on a
// PDF behind a portal.
//
// Every fact on a generated page is derived from the embedded registry or from
// this codebase: which business area the message belongs to, which message sets
// publish it, which other versions exist, where the Registration Authority
// hosts the download, and what AskIso itself can do with it. Nothing describes
// what the message *means* — that is specification content, AskIso does not
// redistribute it, and inventing a plausible-sounding summary for 2,845
// messages would be the single fastest way to make the project untrustworthy.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sebastienrousseau/askiso/internal/generator"
	"github.com/sebastienrousseau/askiso/internal/registry"
	"github.com/sebastienrousseau/askiso/internal/swift"
	"github.com/sebastienrousseau/askiso/internal/translator"
	"github.com/sebastienrousseau/askiso/pkg/iso20022"
)

func main() {
	out := flag.String("out", "web/content/messages", "directory to write pages into")
	date := flag.String("date", "2026-08-25", "publication date for the front matter")
	flag.Parse()

	if err := run(*out, *date); err != nil {
		fmt.Fprintf(os.Stderr, "gen-message-pages: %v\n", err)
		os.Exit(1)
	}
}

func run(outDir, date string) error {
	reg, err := registry.Load()
	if err != nil {
		return fmt.Errorf("loading registry: %w", err)
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	// Version lineage: every message sharing a base code is a version of the
	// same definition, and "which version replaced this one" is one of the
	// questions the pages exist to answer.
	byBase := map[string][]string{}
	for _, m := range reg.Messages {
		byBase[m.BaseCode] = append(byBase[m.BaseCode], m.ID)
	}
	for k := range byBase {
		sort.Strings(byBase[k])
	}

	mtFor := mtSources()
	mxSupported := map[string]bool{}
	for _, id := range swift.SupportedMX() {
		mxSupported[id] = true
	}

	for _, m := range reg.Messages {
		page := buildPage(reg, m, byBase[m.BaseCode], mtFor[m.BaseCode],
			mxSupported[m.BaseCode], generator.HasTemplate(m.BaseCode), date)
		path := filepath.Join(outDir, m.ID+".md")
		if err := os.WriteFile(path, []byte(page), 0o644); err != nil {
			return err
		}
	}

	fmt.Printf("messages: %d page(s) written to %s\n", len(reg.Messages), outDir)
	return nil
}

// mtSources reports, for each MX base code, the MT messages that convert into
// it. It is inverted from the translator's own mapping table rather than
// hand-maintained here, so a conversion added there appears on these pages
// without anyone remembering to update a second list.
func mtSources() map[string][]translator.Mapping {
	out := map[string][]translator.Mapping{}
	for _, m := range translator.GetAllMappings() {
		base := baseCode(m.MXCode)
		if base == "" {
			continue
		}
		out[base] = append(out[base], m)
	}
	for k := range out {
		sort.Slice(out[k], func(i, j int) bool { return out[k][i].MTCode < out[k][j].MTCode })
	}
	return out
}

// baseCode reduces "pacs.008.001.10" to "pacs.008".
func baseCode(id string) string {
	parts := strings.Split(id, ".")
	if len(parts) < 2 {
		return ""
	}
	return parts[0] + "." + parts[1]
}

func buildPage(reg *registry.Registry, m registry.Message, versions []string,
	mtSrc []translator.Mapping, mxToMT, hasTemplate bool, date string) string {

	domain := iso20022.DomainName(m.Domain)
	sets := reg.SetsFor(m.ID)

	title := fmt.Sprintf("%s — ISO 20022 %s message", m.ID, domain)
	desc := fmt.Sprintf(
		"%s is an ISO 20022 message definition in the %s business area. "+
			"Validate, lint and generate it with AskIso, and download the schema from the Registration Authority.",
		m.ID, domain)

	var b strings.Builder
	fmt.Fprintf(&b, "---\n")
	fmt.Fprintf(&b, "name: %q\n", "AskIso")
	fmt.Fprintf(&b, "short_name: %q\n", "AI")
	fmt.Fprintf(&b, "title: %q\n", title)
	fmt.Fprintf(&b, "description: %q\n", desc)
	fmt.Fprintf(&b, "keywords: %q\n", strings.Join([]string{
		m.ID, m.BaseCode, m.Domain, "ISO 20022", domain,
		m.BaseCode + " schema", m.BaseCode + " xsd", "validate " + m.BaseCode,
	}, ", "))
	fmt.Fprintf(&b, "author: %q\n", "Sebastien Rousseau")
	fmt.Fprintf(&b, "date: %q\n", date)
	fmt.Fprintf(&b, "layout: %q\n", "page")
	fmt.Fprintf(&b, "language: %q\n", "en-GB")
	fmt.Fprintf(&b, "schema: %q\n", "page")
	fmt.Fprintf(&b, "changefreq: %q\n", "monthly")
	fmt.Fprintf(&b, "copyright_year: %q\n", "2026")
	fmt.Fprintf(&b, "form_origin: %q\n", "https://askiso.io")
	// Without this the news-sitemap generator warns once per page and falls
	// back to the build time, which would date all 2,845 pages to whenever CI
	// last ran rather than to when they were published.
	fmt.Fprintf(&b, "news_publication_date: %q\n", date)
	fmt.Fprintf(&b, "eyebrow: %q\n", domain)
	fmt.Fprintf(&b, "headline: %q\n", m.ID)
	fmt.Fprintf(&b, "lead: %q\n", fmt.Sprintf(
		"Version %s of %s, in the %s business area.", versionOf(m.ID), m.BaseCode, domain))
	fmt.Fprintf(&b, "---\n\n")

	// --- what is it -------------------------------------------------------
	fmt.Fprintf(&b, "## What %s is\n\n", m.ID)
	fmt.Fprintf(&b, "`%s` is an ISO 20022 message definition. Its business area is "+
		"**%s** (`%s`), and `%s` is the definition it versions.\n\n",
		m.ID, domain, m.Domain, m.BaseCode)
	fmt.Fprintf(&b, "AskIso does not reproduce the specification. The message definition "+
		"report and schema are published by the Registration Authority, free of charge, "+
		"and the links below go there.\n\n")

	// --- versions ---------------------------------------------------------
	if len(versions) > 1 {
		fmt.Fprintf(&b, "## Versions of %s\n\n", m.BaseCode)
		fmt.Fprintf(&b, "%d versions of this definition are published. "+
			"A newer version is not automatically a replacement — which one you send "+
			"is decided by the scheme or market infrastructure you are sending to.\n\n",
			len(versions))
		for _, v := range versions {
			marker := ""
			if v == m.ID {
				marker = " — this page"
			}
			fmt.Fprintf(&b, "- [`%s`](/messages/%s/)%s\n", v, v, marker)
		}
		fmt.Fprintf(&b, "\n")
	}

	// --- where to get it --------------------------------------------------
	fmt.Fprintf(&b, "## Where to get the schema\n\n")
	if len(sets) == 0 {
		fmt.Fprintf(&b, "The registry records no message set publishing this definition.\n\n")
	} else {
		fmt.Fprintf(&b, "`%s` is published in %d %s. Download from the Registration Authority, "+
			"then import:\n\n", m.ID, len(sets),
			plural(len(sets), "message set", "message sets"))
		fmt.Fprintf(&b, "```bash\naskiso catalog fetch %s\n```\n\n", m.BaseCode)
		fmt.Fprintf(&b, "| Message set | Version | Download |\n| :--- | :--- | :--- |\n")
		for _, s := range sets {
			fmt.Fprintf(&b, "| %s | %s | [iso20022.org](%s) |\n", s.Name, s.Version, s.URL)
		}
		fmt.Fprintf(&b, "\n")
	}

	// --- what askiso does with it ----------------------------------------
	fmt.Fprintf(&b, "## What AskIso does with %s\n\n", m.ID)
	fmt.Fprintf(&b, "```bash\n")
	// The comment column is aligned on the longest command so the block reads
	// as a table rather than as ragged output.
	cmds := [][2]string{
		{"askiso info " + m.ID, "metadata and schema paths"},
		{"askiso validate message.xml", "full XSD validation, needs the schema"},
		{"askiso lint message.xml", "business rules, needs no schema"},
	}
	if hasTemplate {
		cmds = append(cmds, [2]string{
			"askiso generate " + m.BaseCode, "from a template, needs no schema"})
	} else {
		cmds = append(cmds, [2]string{
			"askiso generate " + m.ID + " --from-schema", "walks the schema"})
	}
	width := 0
	for _, c := range cmds {
		if len(c[0]) > width {
			width = len(c[0])
		}
	}
	for _, c := range cmds {
		fmt.Fprintf(&b, "%-*s  # %s\n", width, c[0], c[1])
	}
	fmt.Fprintf(&b, "```\n\n")

	if hasTemplate {
		fmt.Fprintf(&b, "This message has a hand-written template with rail-aware defaults, "+
			"so a sample can be generated with no catalogue installed.\n\n")
	}

	// --- MT relationship --------------------------------------------------
	if len(mtSrc) > 0 || mxToMT {
		fmt.Fprintf(&b, "## SWIFT MT equivalence\n\n")
		if len(mtSrc) > 0 {
			fmt.Fprintf(&b, "%s converts into `%s`:\n\n",
				plural(len(mtSrc), "One MT message", "These MT messages"), m.BaseCode)
			for _, mt := range mtSrc {
				fmt.Fprintf(&b, "- **%s** — %s. %s\n", mt.MTCode, mt.MTTitle, mt.Description)
			}
			fmt.Fprintf(&b, "\n```bash\naskiso translate payment.mt%s\n```\n\n",
				strings.TrimPrefix(mtSrc[0].MTCode, "MT"))
		}
		if mxToMT {
			fmt.Fprintf(&b, "`%s` also converts back to MT. Both directions carry a fidelity "+
				"report naming every field that was mapped, derived, truncated or lost, "+
				"because conversion between the two is lossy in both directions.\n\n",
				m.BaseCode)
		}
	}

	// --- FAQ --------------------------------------------------------------
	// Answer-shaped, because that is the shape a search engine or an assistant
	// lifts into a result. Each answer is a fact this repository can stand
	// behind, not a paraphrase of a specification nobody here is licensed to
	// paraphrase.
	fmt.Fprintf(&b, "## Questions\n\n")
	fmt.Fprintf(&b, "### What business area does %s belong to?\n\n", m.ID)
	fmt.Fprintf(&b, "%s (`%s`).\n\n", domain, m.Domain)

	fmt.Fprintf(&b, "### How do I validate a %s message?\n\n", m.BaseCode)
	fmt.Fprintf(&b, "`askiso validate message.xml`. The schema is resolved from the "+
		"document's own namespace, so you do not pass it. Validation needs the schema "+
		"installed; `askiso lint` checks business rules without one.\n\n")

	fmt.Fprintf(&b, "### Does AskIso include the %s schema?\n\n", m.ID)
	fmt.Fprintf(&b, "No. AskIso redistributes no ISO 20022 specification content. "+
		"You download the message set from the Registration Authority and import it with "+
		"`askiso catalog add`. What ships in the binary is the index of what exists and "+
		"where to get it.\n\n")

	if len(versions) > 1 {
		fmt.Fprintf(&b, "### Which version of %s should I send?\n\n", m.BaseCode)
		fmt.Fprintf(&b, "That is decided by the scheme or market infrastructure you are "+
			"sending to, not by the standard. %d versions are published; "+
			"`askiso diff <from> <to>` classifies every structural difference between two "+
			"of them as breaking or compatible.\n\n", len(versions))
	}

	fmt.Fprintf(&b, "---\n\n")
	fmt.Fprintf(&b, "*This page is generated from AskIso's embedded index of the standard. "+
		"The authoritative source for ISO 20022 is "+
		"[iso20022.org](https://www.iso20022.org/).*\n")

	return b.String()
}

func versionOf(id string) string {
	if i := strings.LastIndex(id, "."); i >= 0 && i+1 < len(id) {
		return id[i+1:]
	}
	return id
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
