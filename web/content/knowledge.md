---
name: "AskISO"
short_name: "AskISO"
title: "ISO 20022 knowledge centre"
description: "Everything AskISO publishes about ISO 20022 in one place: the CBPR+ SR2025 message guidance, 2026 readiness rules, documentation and evidence."
keywords: "ISO 20022 knowledge centre, ISO 20022 learning, pacs.008 explained, CBPR+ guidance, ISO 20022 documentation, ISO 20022 reference"
author: "Sebastien Rousseau"
date: "2026-08-26"
news_publication_date: "2026-08-26"
layout: "page"
language: "en-GB"
schema: "page"
changefreq: "weekly"
copyright_year: "2026"
form_origin: "https://askiso.io"
nav_knowledge: "true"
banner: "getty-images-dqHskSJDfe4"
banner_alt: "Lines of code and data rendered as a cyan grid."
eyebrow: "Knowledge centre"
headline: "Start wherever your question is"
lead: "Everything published here, organised by whatever you are trying to establish rather than by what it happens to be called."
---

## What ISO 20022 is

ISO 20022 is the international standard for financial messaging. It is
maintained by ISO Technical Committee 68, and Swift acts as its Registration
Authority. The standard defines XML message definitions, such as `pacs.008` for
a bank-to-bank credit transfer, `pain.001` for a payment initiation and
`camt.053` for a statement. Swift's CBPR+ guidelines, SEPA and the major
payment market infrastructures all build on it.

## I have a message and I do not know what is wrong with it

Paste it into the [workspace](/workspace/). It identifies the format automatically and lints it. It applies the November 2026 rules, and validates against your schemas when you have selected a catalogue. Every finding names the rule, the exact field, and what to change.

If a finding still makes no sense, the rule identifier it cites is the reference to quote when enquiring.

## I need to know what a message type is for

The [message reference](/messages/) covers all 2,845 definitions across 30 business areas. Each page records what the message is and which business area publishes it. It also lists every version, which one superseded it, and where the Registration Authority hosts the schema.

Three definitions arrive considerably more often than the remainder. [pacs.008](/messages/pacs.008.001.13/) is the customer credit transfer that moves money between banks. [pain.001](/messages/pain.001.001.12/) is the payment initiation a corporate sends to its bank. [camt.053](/messages/camt.053.001.13/) is the statement that comes back.

## I need to prepare for November 2026

Start with [what changes](/deadline/). It explains the structured address requirement, and what qualifies as structured or hybrid. It also identifies the specific rules AskISO applies. Remediation usually turns out to be a customer data problem rather than a payments one.

Then check your own messages in the [workspace](/workspace/). To see which rule you fail most often, run the whole outbound set through the [batch audit example](https://github.com/sebastienrousseau/askiso/tree/main/examples/batch-audit).

## I am integrating this into something

The [developer documentation](/docs/) covers every command, and which ones need a catalogue. It also covers the scheme rule profiles. And it explains how to wire the language server into an editor, or the MCP server into an AI assistant.

The [examples folder](https://github.com/sebastienrousseau/askiso/tree/main/examples) contains one runnable program per workflow, each sufficiently small to read in a single sitting.

## I want to know whether to trust the answers

The [conformance evidence](/conformance/) publishes what was tested, against what, and precisely what the result was. Every figure names the command that reproduces
it, because a validator that asks to be trusted has misunderstood its job.

## I have a question that is not here

The [frequently asked questions](/faq/) cover the November 2026 change and whether anything is uploaded. They also cover why schemas are not shipped, what it costs, and what happens when AskISO cannot answer.

If yours is not among them, the [contact page](/contact/) reaches a person directly. Questions that prove common eventually reach the FAQ, which is the most useful destination available to one.

## Where the standard itself lives

AskISO redistributes no specification content. The Registration Authority publishes the schemas, the message definition reports and the code sets at [iso20022.org](https://www.iso20022.org/), free of charge. That is the source of truth, rather than anything on this site.
