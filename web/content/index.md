---
name: "AskISO"
short_name: "AI"
title: "AskISO — validate ISO 20022 messages and get ready for November 2026"
description: "Validate, lint and convert ISO 20022 and SWIFT MT messages, and check them against the CBPR+ November 2026 structured-address rules. Every finding cites the rule and the field. Nothing is uploaded. Open source, free, ships no specification content."
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
hero_alt: "The AskISO mark: a question mark inside a circle, in cyan."
eyebrow: "Open source · Apache-2.0 or MIT"
headline: "Know your ISO 20022 messages will be accepted"
lead: "Check a payment message against the schema, the scheme rules and the November 2026 requirements — and get told which rule failed, in which field, and what to change. Nothing is uploaded: the engine runs on your machine, or inside your browser tab."
---

## What you can do right now

**Check a message and understand the answer.** Paste an ISO 20022 or SWIFT MT
message into the [workspace](workspace/). It works out what you gave it, runs the
lint checks and the November 2026 rules, and reports each finding with the rule
identifier, the exact path in the document, what it expected, and what to change.
A finding you cannot trace to a rule is a finding you cannot act on.

**Look up any message.** All [2,845 message definitions](messages/) across 30
business areas, every version of each, what replaced it, and where to get the
schema from the Registration Authority.

**Take the evidence with you.** Findings as JSON, as SARIF 2.1.0 for code
scanning, or as an evidence pack written for a ticket — including a statement of
what was *not* checked, because "clean" without a schema means "nothing
contradicted it".

## Why the answers can be trusted

**Validation checked against an independent implementation.** A pure-Go
implementation of the XML Schema subset ISO 20022 uses — element order,
cardinality, choices, wildcards, patterns, enumerations, length and numeric
facets. Across the whole catalogue AskISO and libxml2 agree on **4,746 of 4,746
documents**, accepting the same 1,035 and rejecting the same 3,711. That figure
is reproduced by a command, not asserted in a brochure.

**Scheme rules, not just schema validity.** A message can be schema-valid and
still be rejected by a clearing system. AskISO checks the rules that sit on top:
CBPR+ requirements, the November 2026 structured-address mandate, enhanced-data
expectations, LEI and UETR correctness.

**MT and MX, both directions, honestly.** MT101, MT103, MT104, MT107, MT202,
MT204 and MT940 convert to pain.001, pacs.008, pain.008, pacs.009, pacs.010 and
camt.053, and each converts back. Every conversion carries a fidelity report
naming what was mapped, derived, truncated or lost — because conversion is lossy
by nature and a tool that hides that is not doing you a favour.

## Where it runs

On your machine as a command line and a terminal interface; in your editor as a
language server, with findings inline as you type; in your pipeline as a GitHub
Action emitting SARIF; alongside an AI assistant as an MCP server, so the
assistant cites a rule identifier instead of improvising. And in your browser,
on this site, with the identical engine compiled to WebAssembly.

```bash
go install github.com/sebastienrousseau/askiso/cmd/askiso@latest
askiso lint payment.xml --profile cbpr-2026
```

No account, no API key, no upload. There is no AskISO-operated service that
could see a payment instruction, which is the honest answer to the question your
security team will ask first.

## Ships no specification content

The Registration Authority publishes ISO 20022 free of charge at
[iso20022.org](https://www.iso20022.org/). AskISO does not mirror it. You
download the message sets you need and point AskISO at them, which keeps your
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

For a payment message, a confident wrong answer costs more than no answer.

AskISO is a tool, not regulatory advice. A clean result is not an assurance that
a scheme or correspondent will accept the message. It is an independent
open-source project, not affiliated with ISO or SWIFT.
