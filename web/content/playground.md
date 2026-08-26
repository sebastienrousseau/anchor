---
name: "AskISO"
short_name: "AI"
title: "Generate ISO 20022 samples and look up code sets"
description: "Generate a valid ISO 20022 sample message, look up an external code set, and see what the standard actually contains. Runs in your browser."
keywords: "generate ISO 20022 sample, pacs.008 sample XML, ISO 20022 external code sets, ISO 20022 message statistics, ISO 20022 tools browser"
author: "Sebastien Rousseau"
date: "2026-08-25"
news_publication_date: "2026-08-25"
layout: "playground"
language: "en-GB"
schema: "page"
changefreq: "weekly"
copyright_year: "2026"
form_origin: "https://askiso.io"
nav_playground: "true"
eyebrow: "Tools · nothing uploaded"
headline: "Build a message, look something up"
lead: "Generate a sample message to work from, look up a code set, or see what the standard contains. Already have a message to check? The workspace does that in one step."
---

## What these tools are for

These three cover what the [workspace](/workspace/) deliberately does not. The workspace answers questions about a message you already have. These help when you have not built one yet, or when the question is about the standard itself.

Everything runs inside your browser, on the same engine as the command line. Nothing you enter is ever uploaded.

## Generate a sample message

Generating a message is how most integrations begin. You need something structurally correct to send through a pipeline before genuine data exists, and composing one manually from a schema is slow and error-prone.

Select a message type and AskISO constructs one. Where a template exists the result is realistic. Where none exists, AskISO traverses the schema instead and produces something minimal and synthetic. It identifies which of the two you received, because a synthetic message is adequate for assembling a pipeline and misleading as an example of genuine traffic.

## Look up a code set

ISO 20022 refers to external code sets by name rather than listing their values
in the schema. That is efficient for the standard and awkward in practice: the schema establishes that a field holds a code, but never which particular codes are permitted.

This searches the code sets AskISO carries, so you can verify a value without opening a specification document. The origin of each code appears alongside it.

## See what the standard contains

The standard is larger than most people expect, and its shape is hard to picture from a list. This shows the distribution: how many definitions each business area publishes, and how many versions sit behind them.

It is genuinely useful for scoping an implementation. "We support ISO 20022" means something different for
payments, where a handful of messages cover most traffic, than for securities,
where the catalogue runs to hundreds.

## Checking a message you already have

Use the [workspace](/workspace/) instead. Paste the message and it identifies what you provided, then lints it, applies the November 2026 requirements, and validates it against your own schemas whenever you have selected a catalogue.

Some of that was possible here too, which meant two interfaces to learn and no clear way to tell which one to open. There is now one.
