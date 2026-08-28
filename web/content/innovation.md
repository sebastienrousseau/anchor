---
name: "AskISO"
short_name: "AskISO"
title: "What we are building — the AskISO roadmap"
description: "The message families AskISO covers today, what is being built next, and the constraints that decide what gets built at all."
keywords: "AskISO roadmap, ISO 20022 tooling roadmap, pacs.008 support, CBPR+ rule profiles, ISO 20022 open source development"
author: "Sebastien Rousseau"
date: "2026-08-26"
news_publication_date: "2026-08-26"
layout: "page"
language: "en-GB"
schema: "page"
changefreq: "weekly"
copyright_year: "2026"
form_origin: "https://askiso.io"
nav_innovation: "true"
banner: "getty-images-dqHskSJDfe4"
banner_alt: "Lines of code and data rendered as a cyan grid."
eyebrow: "Innovation"
headline: "What is built, and what is next"
lead: "Published in the open, including the parts that are not finished."
---

## The suite as it stands

**Validation.** A pure-Go implementation of the XML Schema subset ISO 20022
uses. Across the whole catalogue it agrees with libxml2 on 4,746 of 4,746
documents, accepting the same 1,035 and rejecting the same 3,711.

**Linting.** Checks that need no schema at all: IBAN checksums, BIC structure, currency precision, UETR format and date order. Every finding names the rule,
the path, what was expected and what to change.

**Scheme rules.** Profiles that sit above the schema, including the CBPR+
requirements and the November 2026 structured address mandate.

**Conversion.** Seven SWIFT MT types convert to ISO 20022 and back, each with a
fidelity report describing what was mapped, derived, shortened or lost.

**Generation.** A valid sample message for any definition, constructed from a template where one exists and from the schema where none does.

**Reference.** All 2,845 message definitions indexed offline, with every
version and where to obtain the schema.

**Agents.** Thirteen [MCP servers](/mcp/) that expose the same work to an AI
assistant: initiate, settle, read a statement, reconcile it, remediate an
address, and seal the result into an evidence pack an auditor can verify.

## Where it runs

The same engine compiled five ways: a command line tool, a terminal interface,
a language server for editors, a Model Context Protocol server for AI
assistants, and WebAssembly for this website. One engine means one verdict —
a message linted here gets the answer it would get in a terminal.

## What is being worked on

**More scheme profiles.** CBPR+ 2027 enhanced data is partially covered, and
the market infrastructure profiles differ enough from one another to be worth
separating.

**Deeper remediation.** Findings currently explain what to change. Where a
correction can be derived without guessing, as with IBAN check digits, AskISO
now provides it. Extending that safely is the interesting problem.

**Broader conversion.** Seven MT types are supported in both directions. The
constraint is verification rather than effort: a mapping nobody can check
against a published source is one this project will not ship.

## How priorities are decided

By what fails in production. A message AskISO handles incorrectly moves straight to the front of the queue, because a validator that is wrong is worse than one that is merely incomplete.

Everything else is discussed in the open. The
[issue tracker](https://github.com/sebastienrousseau/askiso/issues) is the
roadmap, and the reasoning behind a decision sits beside it.

## What will not be built

Anything that requires guesswork. Rewriting an unstructured address means deciding which line is the town and which the country, and a tool that decides wrongly on a payment has caused real harm. Where correctness cannot be
established, AskISO reports the gap and stops.

## Why any of this

[Vision and mission](/vision/) sets out what this project is working towards,
and the circumstances that would make it unnecessary.
