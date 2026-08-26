---
name: "AskISO"
short_name: "AI"
title: "AskISO — ISO 20022 validator and CBPR+ 2026 checker"
description: "Validate, lint and convert ISO 20022 and SWIFT MT messages, and check them against the CBPR+ November 2026 address rules. Free, open source, nothing uploaded."
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
banner: "freeman-zhou-oV9hp8wXkPE"
banner_alt: "A financial district skyline at dawn, mirrored in still water."
hero_alt: "The AskISO mark: a question mark inside a circle, in cyan."
eyebrow: "ISO 20022 · 14 November 2026"
headline: "Know your ISO 20022 messages will be accepted"
lead: "Paste one. See which rule failed, in which field, and what to change. Nothing is uploaded."
---

## Start with a single message

Paste a payment message into the [workspace](/workspace/) and see what comes
back. It recognises the format on its own, then validates the message and
explains every result in plain language.

Each finding names three things: the rule that fired, the exact field it points
at, and what you should change. That final part matters most, because a finding
you cannot act on is really just a finding you have to ask somebody about.

Suppose a creditor IBAN fails its checksum. AskISO never stops at "invalid". It
reports that the check digits are incorrect and calculates the appropriate pair
for that account number. It also warns you the account number itself may be the
underlying mistake.

## Look up any message definition

The [message reference](/messages/) covers all 2,845 ISO 20022 definitions,
including every version of pacs.008, pain.001, camt.053 and the rest, arranged
by business area. Each page records which version superseded it and where the
Registration Authority publishes the schema.

There is no account to create and nothing to subscribe to.

## Take the evidence away with you

Findings rarely stay with whoever discovered them. Export yours as JSON, as
SARIF for code scanning, or as a written summary for a ticket or a review.

That pack also records what was **not** checked. Without a schema installed,
"clean" only means that nothing contradicted the message. Those are genuinely
different claims, and the difference deserves to be written down.

## Why the answers can be trusted

**Our validation is checked against somebody else's.** AskISO validates ISO
20022 messages with its own implementation of the schema rules. To demonstrate
that it agrees with the reference, we ran the entire catalogue through libxml2
as well. Both accept the same 1,035 documents and reject the same 3,711, which
is agreement on 4,746 of 4,746. A single command reproduces that result.

**Schema validity is only half the question.** A perfectly valid message can still be refused by a clearing system. AskISO
therefore applies the scheme rules that sit above the schema: CBPR+
requirements, the November 2026 address mandate, enhanced data, and LEI and UETR
correctness.

**Conversion reports its own losses.** MT101, MT103, MT104, MT107, MT202, MT204
and MT940 all convert into their ISO 20022 equivalents and back again. Every conversion includes a fidelity report describing what was mapped,
derived, shortened or lost. Conversion between MT and MX genuinely loses detail,
and concealing that would help nobody.

## Fits wherever you already work

Run it on your own machine as a command line tool. Install it in your editor and
it identifies problems while you type. Add it to your build pipeline and a
malformed message fails there, rather than reaching a correspondent. Connect it
to an AI assistant and that assistant can cite a rule identifier instead of
improvising. Or simply use this website, which runs the identical engine inside
your browser tab.

```bash
go install github.com/sebastienrousseau/askiso/cmd/askiso@latest
askiso lint payment.xml --profile cbpr-2026
```

No account, no API key, and nothing ever leaves your machine. That is the honest
answer to the first question your security team will ask.

## Why we publish no schemas ourselves

The Registration Authority publishes ISO 20022 at
[iso20022.org](https://www.iso20022.org/), free of charge, and we deliberately
do not mirror it. You download whichever message sets you need and point AskISO
at the folder.

That arrangement benefits you directly. Your schemas remain current, and you
validate against the authoritative source instead of a copy of uncertain age.
Search, linting, MT conversion, code lookup and message generation all operate
with no download whatsoever.

## Where AskISO deliberately stops

Some questions have no answer that can be verified, and when we meet one we say
so rather than improvise.

MT940 expects a transaction type code, but sometimes no verified mapping exists
from the ISO 20022 bank transaction code. AskISO writes `NMSC` and tells you
exactly what was lost. Similarly, camt.110 expects a coded investigation type
where MT carries only prose. AskISO uses the proprietary branch and identifies
its source, rather than inventing a code.

Guessing would be straightforward. It would also be wrong occasionally, and you
would have no reliable way to tell which occasions those were. For a payment
instruction, a confident wrong answer costs considerably more than an honest
gap.

The same caution applies to what AskISO itself is. It is a tool rather than
regulatory advice, so a clean result is never a guarantee that a scheme or a
correspondent will accept your message. And it is an independent open-source
project, with no affiliation to ISO or to SWIFT.

## Try it with a message of your own

The quickest way to judge any of this is to [check a message](/workspace/) and
read what comes back. Everything runs inside your browser, so nothing is
uploaded, and the whole exercise takes about a minute.
