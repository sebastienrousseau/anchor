---
name: "AskISO"
short_name: "AskISO"
title: "ISO 20022 agent recipes"
description: "Four flows an agent runs end to end: migrate an MT103, reconcile a statement, recall a wrong payment, and turn a signed agent mandate into a wire."
keywords: "MT103 to pacs.008, camt.053 reconciliation, camt.056 payment recall, AP2 mandate pain.001, ISO 20022 agent workflow"
author: "Sebastien Rousseau"
date: "2026-08-28"
news_publication_date: "2026-08-28"
layout: "page"
language: "en-GB"
schema: "page"
changefreq: "monthly"
copyright_year: "2026"
form_origin: "https://askiso.io"
banner: "getty-images-dqHskSJDfe4"
banner_alt: "A trading floor screen showing rows of financial data."
eyebrow: "Agents · recipes"
headline: "Four flows, end to end"
lead: "Each one is a single conversation. The agent chooses the tools; you approve the steps. Nothing here moves money — every flow stops at a validated message."
---

## Migrate a legacy MT103

The difficulty most institutions still contend with: a correspondent transmits
MT, and whatever leaves must be ISO 20022 carrying structured addresses.

> Here is an MT103. Convert it to a pacs.008, then tell me what the conversion
> lost and whether the addresses pass CBPR+.

The agent converts through `pacs008-mcp`, which returns a fidelity report
identifying every field as mapped, derived, truncated or dropped, so nothing
disappears silently. It then passes the result to `structured-address-fix-mcp`,
because MT addresses are free text by construction and cannot satisfy the
structured address requirement without remediation.

What returns is the converted message, an enumeration of what the format could
not carry, and the compliant form of each address.

## Reconcile a statement

> Match this camt.053 against the payments I expected this week, and explain
> anything that did not match cleanly.

`camt053-mcp` parses the statement into entries and balances. `reconcile-mcp`
matches them against your expected payments and handles the cases that make
reconciliation tedious: a short payment, an overpayment, one invoice settled by
several credits, and several invoices settled by one.

Each match carries the reasoning behind it rather than a score alone, which is
precisely what makes the result reviewable by somebody who was not party to the
conversation. A sandbox mode exists, so an initial run requires no real data.

## Recall a payment, then close the case

Exceptions and investigations constitute the least-tooled corner of the standard,
and the portion that habitually costs an operations team its afternoon.

> We sent this payment twice. Raise a cancellation for the duplicate, then
> record the response when it comes back.

`camt-exceptions` generates a valid `camt.056` cancellation request, and later
the `camt.029` that resolves the investigation. Both are checked against the
schema before they are returned.

## Turn an agent mandate into a wire

The newest of the four, and the one that will matter most as agent payments
become ordinary.

> Here is a signed AP2 mandate. Produce the pain.001 it authorises, within its
> spending cap.

`ap2-iso20022` reads the Google AP2 or Coinbase x402 mandate, checks the
spending cap, the expiry and the authorisation, and produces a wire-valid
message. It transforms and validates only. Moving the money stays a separate,
human-guarded step, which is deliberate: an agent that can compose a payment is
useful, and one that can send it unsupervised is a different risk entirely.

## Compose your own

The four above are conversations, not scripts, so they combine. Ask for a
readiness score across a folder of messages, then remediation for the ones that
fail, then an evidence pack sealed over the result — that is
`iso20022-readiness-suite-mcp` and `iso20022-evidence-pack-mcp` doing in one
exchange what usually takes a project.

## Next

- [The thirteen servers](/mcp/) — what each one does
- [Setting it up](/mcp/setup/) — one configuration block
- [The command line](/docs/) — the same engine without an assistant
