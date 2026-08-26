---
name: "AskISO"
short_name: "AI"
title: "Workspace — check an ISO 20022 message online"
description: "Check ISO 20022 and SWIFT MT messages, identifiers, IBANs, BICs and UETRs. Every finding names its rule and path. Runs in your browser; nothing is uploaded."
keywords: "ISO 20022 workspace, validate ISO 20022 online, MT to MX converter, IBAN checker, BIC checker, pacs.008 lint, ISO 20022 message lookup"
author: "Sebastien Rousseau"
date: "2026-08-26"
news_publication_date: "2026-08-26"
layout: "workspace"
language: "en-GB"
schema: "page"
changefreq: "weekly"
copyright_year: "2026"
form_origin: "https://askiso.io"
nav_workspace: "true"
eyebrow: "One input · nothing uploaded"
headline: "Paste it. AskISO works out what it is."
lead: "A message, an identifier, an IBAN or a question — a single box, and the tool determines what to do with it. Every finding names the rule it originated from and its path in the document, so you can verify it rather than accept it on trust."
---

## What you can paste

The workspace accepts several categories of input and identifies which one you
provided before doing anything else. It then says what it decided, so a wrong
reading shows up at once rather than being buried in the result.

- **An ISO 20022 message.** Any XML document declaring an ISO 20022 namespace. It is
linted, evaluated against the November 2026 requirements, and validated against
your schema whenever you have one installed.
- **A SWIFT MT message.** MT101, MT103, MT104, MT107, MT202, MT204 and MT940
convert into their ISO 20022 equivalents, accompanied by a fidelity report.
- **A message identifier.** Enter `pacs.008` for every version of that
definition, or `pacs.008.001.13` for one specific version.
- **An IBAN, a BIC or a UETR.** Each is checked against the standard that
defines it, and any failure explains itself.

## How to read a finding

Every finding carries the same four parts, because all four are needed before
anybody can act on it.

The **rule identifier**, such as `CBPR-ADDR-002`, is the reference you quote in
a ticket or an email. The **path** gives the precise location within the
document, written as an XPath, so nobody hunts through the message by hand. The
**message** explains the problem in ordinary language, without jargon. The
**remediation** specifies what to change.

Where a correct value can be worked out, the finding gives it. A failed IBAN
checksum, for example, reports the check digits that particular account number
actually requires. Where no correct value can be established, the finding says
so rather than guessing.

## Your message is not uploaded

The workspace runs the same engine as the command line, compiled to WebAssembly
and run entirely inside this browser tab. Your message is processed by code
already downloaded to your own device.

You can confirm this independently. Open your browser's network panel, paste a
message, and observe that no request transmits it anywhere.

## Schema validation and your catalogue

Linting and the scheme rules need nothing from you. Schema validation is
different, because it needs the schema text itself, and AskISO redistributes no
specification content.

Download the message sets you need from
[iso20022.org](https://www.iso20022.org/), free of charge, then select the
folder on the [advanced tools](../playground/) page. Your browser reads those
files locally, so they are never uploaded either.

Without a catalogue the workspace still lints and still applies the November
2026 requirements. It simply reports that schema validation did not run, rather
than implying your message satisfied one.

## Taking the result with you

The Studio panel exports whatever you have just executed. Findings are available
as JSON, as a SARIF 2.1.0 log for GitHub code scanning, and as an evidence pack
written for a human reader.

The evidence pack records the engine version, the rule profile applied, and
whether schema validation actually ran. That final detail matters whenever the
result is clean, because a clean result without a schema is a much narrower
claim than it looks.
