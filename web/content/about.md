---
name: "AskISO"
short_name: "AskISO"
title: "About AskISO — who builds it and why"
description: "AskISO is an independent open-source project for ISO 20022 messaging. Who maintains it, what it will and will not do, and how it is funded."
keywords: "about AskISO, ISO 20022 open source, independent ISO 20022 tooling, AskISO maintainer"
author: "Sebastien Rousseau"
date: "2026-08-26"
news_publication_date: "2026-08-26"
layout: "page"
language: "en-GB"
schema: "page"
changefreq: "monthly"
copyright_year: "2026"
form_origin: "https://askiso.io"
nav_about: "true"
banner: "francais-a-londres-b07tX5xgZ5U-unsplash"
banner_alt: "The Royal Exchange in the City of London, seen from the street."
eyebrow: "About us"
headline: "An independent project, deliberately"
lead: "No vendor, no licence server, and no commercial interest in what your messages contain."
---

## What AskISO is

An open-source toolchain for ISO 20022 messaging: validation, linting, scheme rule evaluation, and conversion between SWIFT MT and ISO 20022. It operates on your machine, in your pipeline, in your editor, or inside a browser tab.

It is maintained by [Sebastien Rousseau](https://sebastienrousseau.com/), who
has spent his career in payments engineering. Contributions arrive through the
public repository, and the discussion that produced any decision is visible
there alongside the code.

## Why it is independent

Payment message validation is a position of considerable trust. A tool declaring a message acceptable must have no reason to prefer that answer, and a tool that observes your messages must be one you can independently audit.

AskISO resolves both structurally rather than by promise. It is open source, so every verdict can be inspected. It executes locally, so no service of ours ever receives a payment instruction. And it redistributes no specification content,
so it competes with nobody selling access to the standard.

## Our mission

To make correctness in ISO 20022 checkable by anyone, without a licence, a
portal, or a support contract.

The standard is published free of charge. The knowledge required to use it
correctly is not, and that gap is what this project exists to close.
[Vision and mission](/vision/) sets out what we are working towards and how we
would know whether it worked.

## What we will not do

**We will not guess.** Where no verifiable mapping exists, AskISO reports that rather than inventing one. A confident incorrect answer about a payment costs considerably more than an honest gap.

**We will not host your messages.** There is no upload, no account, and no
telemetry. This is a structural decision, not a policy that could later change.

**We will not redistribute the standard.** The Registration Authority publishes
it free of charge, and mirroring it would put a copy of unknown age between you
and the source of truth.

## How it is funded

It is not funded. There is no company, no investment, and no paid tier. The costs amount to a domain registration and a maintainer's time.

That arrangement is deliberate rather than temporary. The immediate objective is adoption by the practitioners who handle these messages daily, and an invoice would obstruct precisely that.

That is worth stating plainly, because it sets expectations correctly. Replies
come from a person rather than a support desk, usually within a few days.
Roadmap decisions are made in public. Nothing here is underwritten by a
commercial guarantee, and the licence says so in the usual terms.

## What it is not

AskISO is a tool, not regulatory advice. A clean result is not an assurance
that a scheme or a correspondent will accept your message, and nothing here
substitutes for your own compliance process.

It is also unaffiliated with ISO and with SWIFT. ISO 20022 is a trade mark of
ISO; SWIFT is the Registration Authority for the standard. Neither endorses
this project.

## Getting in touch

The [issue tracker](https://github.com/sebastienrousseau/askiso/issues) is the
public record of reported problems. For anything else, including a message
AskISO handles incorrectly, use the [contact page](/contact/).
