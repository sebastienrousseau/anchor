---
name: "AskIso"
short_name: "AI"
title: "AskIso — the developer's ISO 20022 toolchain"
description: "Validate, convert, generate and remediate ISO 20022 financial messages from your terminal, your editor, your CI pipeline or your AI assistant. Open source, ships no specification content, runs entirely on your machine."
keywords: "ISO 20022, pacs.008, pain.001, camt.053, CBPR+, structured address, SWIFT MT to MX, ISO 20022 validator, ISO 20022 CLI, MT103 converter, November 2026 deadline"
author: "Sebastien Rousseau"
date: "2026-08-25"
layout: "index"
language: "en-GB"
schema: "page"
changefreq: "weekly"
copyright_year: "2026"
form_origin: "https://askiso.io"
news_publication_date: "2026-08-25"
nav_home: "true"
hero_alt: "The AskIso mark: a question mark inside a circle, in cyan."
eyebrow: "Open source · Apache-2.0 or MIT"
headline: "ISO 20022, from the terminal you already live in"
lead: "Validate, lint, convert and generate financial messages without a portal, a licence key, or an upload. 2,845 message definitions indexed offline. Your payment data never leaves your machine."
---

## One command, then you are working

```bash
go install github.com/sebastienrousseau/askiso/cmd/askiso@latest
askiso validate payment.xml
```

No account, no API key, no upload. AskIso runs locally: the CLI on your machine,
the browser build inside your tab. There is no AskIso-operated service that could
see a payment instruction, which is the honest answer to the question your
security team will ask first.

## What it does

**Validation that agrees with the reference implementation.** A pure-Go
implementation of the XML Schema subset ISO 20022 uses — element order,
cardinality, choices, wildcards, patterns, enumerations, length and numeric
facets. Across the whole catalogue AskIso and libxml2 agree on **4,746 of 4,746
documents**, accepting the same 1,035 and rejecting the same 3,711. That figure
is reproduced by a command, not asserted in a brochure.

**Diagnostics that tell you how to fix it.** Where the reference says the
document is invalid, AskIso says which rule, at which XPath, what it expected
and what it found.

**MT and MX, both directions.** MT101, MT103, MT104, MT107, MT202, MT204 and
MT940 convert to pain.001, pacs.008, pain.008, pacs.009, pacs.010 and camt.053,
and each converts back. Every conversion carries a fidelity report naming what
was mapped, derived, truncated or lost — because conversion is lossy by nature
and a tool that hides that is not doing you a favour.

**Scheme rules, not just schema validity.** A message can be schema-valid and
still be rejected by a clearing system. AskIso checks the rules that sit on top:
CBPR+ requirements, the November 2026 structured-address mandate, enhanced-data
expectations, LEI and UETR correctness.

**Wherever you work.** A CLI, a terminal UI, a language server for editor
diagnostics as you type, and an MCP server so an AI assistant can use the same
engine.

## Ships no specification content

The Registration Authority publishes ISO 20022 free of charge at
[iso20022.org](https://www.iso20022.org/). AskIso does not mirror it. You
download the message sets you need and point AskIso at them, which keeps your
schemas current and means what you validate against comes from the source of
truth rather than a copy of unknown age.

Search, lint, MT conversion, code lookup, identifier checking and template
generation all work with no download at all.

## What it will not do

It will not invent a mapping it cannot verify against a published source. Where
MT940 wants a transaction-type code and no verifiable mapping from the ISO 20022
bank transaction code exists, it emits `NMSC` and tells you what was lost. Where
camt.110 wants a coded investigation type that MT carries as prose, it uses the
proprietary branch and names the source message rather than guessing.

AskIso is a developer tool, not regulatory advice. A clean result is not an
assurance that a scheme or correspondent will accept the message.
