---
name: "AskISO"
short_name: "AskISO"
title: "ISO 20022 questions, answered — AskISO FAQ"
description: "What changes in November 2026, whether your payment data is uploaded, what AskISO costs, and where to get ISO 20022 schemas. Straight answers, no jargon."
keywords: "ISO 20022 FAQ, CBPR+ November 2026, structured address deadline, ISO 20022 validation, MT to MX migration, ISO 20022 schema download, ISO 20022 free validator"
author: "Sebastien Rousseau"
date: "2026-08-26"
layout: "page"
language: "en-GB"
schema: "faq"
changefreq: "weekly"
copyright_year: "2026"
form_origin: "https://askiso.io"
news_publication_date: "2026-08-26"
nav_faq: "true"
banner: "corporate-finance"
banner_alt: "Financial reports and a tablet on a meeting-room table."
eyebrow: "Answers"
headline: "Questions we get asked"
lead: "Straight answers for payments operations, compliance teams, and the engineers who have to implement what they decide."
---

## The structured address change

### What actually changes, and when?

CBPR+ requires the postal addresses in cross-border payment messages to be
**structured or hybrid**. An address written entirely as free-text `<AdrLine>`
elements no longer passes.

The cutover was to be 14 November 2026. Swift deferred every payments change in
Standards Release 2026 on 27 August 2026 and will confirm replacement timing by
December at the latest. The requirement itself stands.

CBPR+ stands for *cross-border payments and reporting plus*: the market practice
guidelines that govern how banks exchange ISO 20022 payment messages with each
other. They sit on top of the standard — a message can be perfectly valid ISO
20022 and still breach them, which is why a schema check alone does not tell you
whether a payment will be accepted.

Structured means the town goes in `<TwnNm>` and the country in `<Ctry>`, as an
ISO 3166-1 alpha-2 code. Hybrid keeps at most two `<AdrLine>` elements alongside
those two mandatory elements. Fully unstructured is what stops being accepted.

### What happens if we are not ready?

Messages that carry a fully unstructured address are rejected by the receiving
institution rather than repaired. A rejection is a failed payment, an
investigation, and a customer asking why their money did not arrive — which is
why most institutions treat this as an operations risk rather than a technical
one.

### Is that the only deadline?

No. There are three, and they are separate:

| Date | What changes |
| --- | --- |
| Deferred, timing by December | Structured or hybrid addresses become mandatory for CBPR+ |
| November 2027 | Enhanced data requirements take effect |
| November 2028 | The SWIFT MT retirement date for cross-border payments |

The [November 2026 page](/deadline/) covers the address rules in detail, with
the rule identifiers AskISO reports so you can trace a finding to the rule that
produced it.

### How do we check whether our messages are affected?

Paste one into the [workspace](../workspace/). Every finding names the rule it
came from and the exact path in the document, so you can hand it to whoever owns
that field. Nothing is uploaded — see below.

## Privacy and security

### Is our payment data uploaded anywhere?

No. There is no AskISO-operated service that could receive a payment
instruction.

The command line runs on your machine. The website runs the same engine compiled
to WebAssembly **inside your browser tab** — the message you paste is processed
by code already downloaded to your device, and never sent anywhere. You can
verify this: open your browser's network panel, paste a message, and watch that
no request carries it.

### Can we run it somewhere with no internet access?

Yes. The command line embeds an index of the whole published standard, so
lookup, search, linting, conversion, generation and the scheme rule profiles all
work with no network and no catalogue on disk. Only schema validation needs
files, and those are ones you download once from the Registration Authority.

### Is it safe to use on production payment data?

That is your institution's call, but the properties that usually matter are: the
data does not leave the machine, the source is public and auditable, the
dependency tree is small, releases are built in public CI, and a software bill
of materials ships with each release. The [security
policy](https://github.com/sebastienrousseau/askiso/blob/main/SECURITY.md)
covers reporting and supported versions.

## Coverage and correctness

### How do we know the validation is right?

Because it is checked against an independent implementation and the result is
published. AskISO's schema validator is compared with **libxml2** across the
whole catalogue, and the two agree on every message tested. The [conformance
page](../conformance/) carries the current numbers and the method.

The point is not that a validator claims to be correct. It is that you can see
what was tested, against what, and what the result was.

### Which messages are supported?

All of them, for lookup and reference: **2,845 message definitions** across 30
business areas are indexed, and each has [its own page](../messages/). Linting,
the scheme rule profiles and generation cover the payment and cash management
messages most institutions need; SWIFT MT conversion covers the MT categories
with a defined ISO 20022 equivalent.

Where a conversion or a check is not supported, AskISO says so rather than
guessing. A tool that quietly invents a mapping is worse than one that declines.

### Why does it sometimes say it cannot answer?

Because it would have to guess. If a message references a schema you have not
downloaded, AskISO reports that the schema is absent instead of pretending the
message is valid. If a SWIFT MT type has no defined ISO 20022 equivalent, it
refuses the conversion instead of producing something plausible.

For a payment message, a confident wrong answer costs more than no answer.

## Schemas and licensing

### Why do I have to download the schemas myself?

The ISO 20022 message definitions and schemas are published by the Registration
Authority at [iso20022.org](https://www.iso20022.org/), free of charge, under
their terms. AskISO indexes the standard — identifiers, versions, message sets,
where each is published — but redistributes **none** of the specification
content itself.

That is a deliberate constraint, and the build enforces it: if a schema or a
message definition report ever reached the published site, the build fails.

### How do I get a catalogue?

Download the message sets you need from the Registration Authority, then point
AskISO at the folder. On the website, the [workspace](../workspace/) lets you
choose that folder in its Sources panel; nothing is uploaded there either, as
the browser reads the files locally.

### What does it cost?

Nothing, and that is a commitment rather than an introductory stage. There is no
paid tier, no account, no API key and no licence server, and the tool answers
before it asks for an email address, because it never asks for one. The code is
open source under Apache-2.0 or MIT, at your option, and remains so.

Should an institution eventually want something built for it alone, that would
be a separate commercial arrangement and would not alter anything described
here.

### Is AskISO affiliated with SWIFT or ISO?

No. It is an independent open-source project. ISO 20022 is a trade mark of ISO,
and SWIFT is the Registration Authority for the standard. AskISO is not endorsed
by, affiliated with, or connected to either.

## Using it in practice

### Can we run it in CI?

Yes, and that is where most of the value is: catching a malformed message before
it reaches a counterparty rather than after. There is a GitHub Action, and the
linter emits **SARIF 2.1.0**, so findings appear as annotations in code scanning
alongside everything else your pipeline reports. The [developer
documentation](../docs/) has the details.

### Does it work in an editor?

Yes. `askiso-lsp` is a language server, so a payment message open in your editor
gets the same findings inline that the command line would report.

### Can an AI assistant use it?

Yes. `askiso-mcp` is a Model Context Protocol server exposing the same engine as
tools, so an assistant can look up a message, lint one, or check a rule profile
and cite the rule identifier it used rather than improvising an answer.

### How do we report a problem or ask for something?

Use the [issue tracker](https://github.com/sebastienrousseau/askiso/issues) — it
is the public list of reported problems and requested changes. Opening one needs
a free GitHub account and takes a minute; everything is visible to everyone,
including the reply.

The most useful report is a message AskISO gets wrong: one it accepts that your
counterparty rejected, or one it rejects that is genuinely fine. Include the
message with any real account numbers, names and references replaced — the
structure is what matters, not the values.

If the problem is a security issue, do not open a public report. The [security
policy](https://github.com/sebastienrousseau/askiso/blob/main/SECURITY.md)
explains how to report it privately.
