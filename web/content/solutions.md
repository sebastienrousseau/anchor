---
name: "AskISO"
short_name: "AskISO"
title: "ISO 20022 solutions for banks, corporates and fintechs"
description: "Check a message before it is sent, migrate from SWIFT MT, meet the November 2026 address mandate, and embed validation in your pipeline, editor or AI assistant."
keywords: "ISO 20022 solutions, payment message validation, SWIFT MT migration, CBPR+ compliance, ISO 20022 CI pipeline, ISO 20022 API"
author: "Sebastien Rousseau"
date: "2026-08-26"
news_publication_date: "2026-08-26"
layout: "page"
language: "en-GB"
schema: "page"
changefreq: "monthly"
copyright_year: "2026"
form_origin: "https://askiso.io"
nav_solutions: "true"
banner: "getty-images-f9bcOaV5zbU"
banner_alt: "Looking up between financial district towers against a clear sky."
eyebrow: "Solutions"
headline: "Five problems, one engine"
lead: "Whatever route you take to it, the answer cites the same rule and the same field."
---

## Know whether a message will be accepted

This is the question behind almost every other one. A message can be perfectly valid ISO 20022 and still be refused by a clearing system, because the scheme rules sit above the standard itself.

AskISO checks all three layers: the schema, the lint rules, and whichever scheme profile applies to you. Each finding names the rule that produced it and
the exact path in the document, so it can be handed to whoever owns that field
rather than interpreted first.

**Start here:** [check a message](/workspace/). It runs in your browser and
nothing is uploaded.

## Meet the structured address mandate

A cross-border payment carrying an unstructured postal address is rejected. Not
flagged, not downgraded — rejected. Swift deferred the November 2026 cutover on
27 August and will confirm replacement timing by December.

The gap is rarely located in the payment engine. It sits in the customer data feeding that engine, which is precisely why remediation takes considerably longer than institutions anticipate.
AskISO reports which addresses are unstructured, where they are, and what each
one needs.

**Start here:** [what the deferred structured-address requirement changes](/deadline/).

## Migrate from SWIFT MT

MT101, MT103, MT104, MT107, MT202, MT204 and MT940 convert to their ISO 20022
equivalents, and each converts back. Every conversion carries a fidelity report
naming what was mapped, derived, shortened or lost.

That report matters more than the conversion itself. Translation between MT and MX loses detail inherently, and a tool returning only the output is asking you to assume nothing disappeared.

**Start here:** paste an MT message into the [workspace](/workspace/).

## Catch it in the pipeline, not at the counterparty

A malformed message caught during a build costs you a build. The same message caught by a correspondent costs an investigation, a repair, and a customer asking where their money went.

AskISO runs as a command line tool, as a GitHub Action, and as a language
server that marks problems in your editor while you type. Findings come out as
SARIF, so they appear in code scanning beside everything else your pipeline
reports.

**Start here:** the [developer documentation](/docs/).

## Give an AI assistant something it can cite

An assistant questioned about ISO 20022 answers from memory, confidently, and occasionally incorrectly. AskISO ships a Model Context Protocol server exposing the
same engine as ten tools, so the assistant looks the answer up and cites the
rule identifier it used.

Beyond looking things up, thirteen further [MCP servers](/mcp/) let an assistant
do the work: compose a payment, read the statement that returns, reconcile it,
remediate an address, and seal the outcome into an evidence pack. Each validates
its own output before returning it, and none of them moves money.

**Start here:** the [MCP servers](/mcp/), or the [integration guide](/docs/).

## What it costs

Nothing. It is open source under Apache-2.0 or MIT, with no account, no API key
and no licence server. The only thing you supply is the schemas, downloaded
free from the Registration Authority, and only when you want full schema
validation.

## Where to start reading

The [knowledge centre](/knowledge/) arranges everything published here by the
question you arrived with, rather than by what each page is called.
