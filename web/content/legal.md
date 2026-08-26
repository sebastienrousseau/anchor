---
name: "AskISO"
short_name: "AskISO"
title: "Legal, privacy and trade marks"
description: "What AskISO collects, what it does not, the licence it is published under, and its relationship to ISO and SWIFT. No analytics, no cookies, no accounts."
keywords: "AskISO privacy, AskISO legal, ISO 20022 tool privacy policy, AskISO licence, ISO 20022 trade mark"
author: "Sebastien Rousseau"
date: "2026-08-26"
news_publication_date: "2026-08-26"
layout: "page"
language: "en-GB"
schema: "page"
changefreq: "yearly"
copyright_year: "2026"
form_origin: "https://askiso.io"
nav_legal: "true"
banner: "corporate-finance"
banner_alt: "Financial reports and a tablet on a meeting-room table."
eyebrow: "Legal"
headline: "What we collect, and what we do not"
lead: "Short, because there is little to say. No analytics, no cookies, no accounts, and no record of the messages you check."
---

## Privacy

### The messages you check

Nothing you validate, lint or convert is transmitted anywhere whatsoever. The engine is WebAssembly executing inside your browser tab, or a binary running on your own machine. There is no AskISO-operated service capable of receiving a payment instruction, and that is a structural property of how the tool is built rather than a policy that could subsequently be revised.

You can confirm this independently. Open your browser's network panel, paste a message, and observe that no request carries it anywhere.

### Schemas you select

Selecting a catalogue folder uses the browser's File System Access API. Your browser reads those files directly and passes their contents to the engine within the same tab. They are never uploaded, and the selection is forgotten when you close the tab unless you explicitly request otherwise.

### The contact form

The one place this site sends anything. When you submit it, the name, email
address, organisation and message you entered are transmitted to
[Formspree](https://formspree.io/legal/privacy-policy/), which forwards them by
email to the maintainer.

That information is used to answer you and for absolutely nothing else. It is never added to a mailing list, sold, or shared with anybody. Ask at any time for correspondence to be deleted
and it will be. Please avoid pasting real account numbers or customer names into it, because the structure of a message is what makes a report useful rather than the values themselves.

### What we do not do

No analytics whatsoever. No tracking pixels, no advertising, and no cookies of any description. No account, no login, and no visitor profile.

The single thing stored in your browser is your light or dark theme choice, kept
in local storage so the site does not flash the wrong colour on your next visit.
It never leaves your device, and clearing site data removes it.

### Hosting

The site is served by GitHub Pages. Like any web server, its infrastructure necessarily processes the IP address of requests in order to deliver pages. AskISO has no
access to those logs and derives nothing from them.

## Licence

AskISO is published under the Apache License 2.0 or the MIT License, at your
option. Both permit commercial use, modification and redistribution without restriction. The
[full text](https://github.com/sebastienrousseau/askiso/blob/main/LICENSE-APACHE)
sits in the repository.

The software is provided without warranty of any kind, as those licences state.
It is a tool rather than regulatory advice, and a clean result is not an
assurance that a scheme or a correspondent will accept your message. Your own
compliance process remains yours.

## Trade marks

ISO 20022 is a trade mark of the International Organization for Standardization.
SWIFT is a trade mark of the Society for Worldwide Interbank Financial
Telecommunication, which acts as the Registration Authority for the standard.

AskISO is an independent open-source project. It is not endorsed by, affiliated
with, or connected to ISO or SWIFT, and it does not redistribute any ISO 20022
specification content. Those names appear here only to describe what the tool
works with, which is nominative use.

## Reporting a security issue

Not in public. The
[security policy](https://github.com/sebastienrousseau/askiso/blob/main/SECURITY.md)
explains how to report one privately and what happens next.

## Contact

Questions about anything on this page go to the [contact page](/contact/), or
directly to the [issue tracker](https://github.com/sebastienrousseau/askiso/issues)
if they are technical.
