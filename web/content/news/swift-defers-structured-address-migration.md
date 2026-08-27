---
name: "AskISO"
short_name: "AskISO"
title: "Swift defers the structured address migration"
description: "Swift has deferred every payments change in Standards Release 2026, the structured address requirement among them, and confirms new timing by December."
keywords: "Swift structured address deferred, Standards Release 2026 delay, CBPR+ structured address extension, ISO 20022 November 2026 postponed, structured address migration timing"
author: "Sebastien Rousseau"
date: "2026-08-27"
news_publication_date: "2026-08-27"
layout: "page"
language: "en-GB"
schema: "page"
changefreq: "weekly"
copyright_year: "2026"
form_origin: "https://askiso.io"
banner: "getty-images-f9bcOaV5zbU"
banner_alt: "Looking up between financial district towers against a clear blue sky."
eyebrow: "News · 27 August 2026"
headline: "Swift defers the structured address migration"
lead: "The November cutover is off. Swift has deferred every payments change in Standards Release 2026 and will confirm new timing by December. The requirement itself has not been withdrawn."
---

## What Swift announced

On 27 August 2026 Swift [accepted a community request](https://www.swift.com/news-events/news/swift-accepts-community-request-extend-structured-address-migration-iso-20022-payment-messages)
to extend the structured address migration for ISO 20022 payment messages. The
change applies to Standards Release 2026, the annual update to formats and rules
carried on the network.

The reason given is readiness. Progress across the industry remains uneven, with
substantial parts of it in every region still unable to satisfy the requirement,
and several communities formally approached Swift and their domestic payment
market infrastructures requesting additional time. Swift consulted those
infrastructures and agreed.

## What that means in practice

Three things follow, and they are worth separating because they carry different
dates.

**All payments changes are deferred.** That includes the structured address
requirement. Swift will spend the coming weeks consulting banks, central banks
and payment market infrastructures, together with market practice groups and
corporates, to settle the timing and the approach. An update is promised by
December at the latest, as part of the ordinary governance cycle. Other payments
changes planned for November will be phased separately.

**Securities and trade changes are decoupled and accelerated.** The rest of
Standards Release 2026 supports business and regulatory priorities that do not
depend on the address work, among them the move to T+1 settlement in some
markets. Those changes now go live in Q1 2027, with an exact date expected by
mid-September once the relevant stakeholders have been consulted.

**Nothing has been withdrawn.** There is no new deadline for structured
addresses yet, and that is the part worth reading carefully. The requirement was
agreed by the community in 2023 and still stands. What moved is when it bites,
not whether it does.

## What has not changed

Structured addresses already work. Swift is explicit that institutions which
have completed the work can benefit immediately, because structured addresses
flow across the network as matters currently stand. Swift also encourages domestic market
infrastructures and financial institutions to keep going, on the grounds that
domestic adoption is what makes cross-border progress possible.

The wider migration is largely settled. More than 98% of payment instructions
now travel as ISO 20022, following last year's transition away from MT. The
address requirement is the remaining piece of a change that has otherwise
happened.

Nor does the deferral repair anybody's data. The obstacle was rarely the payment
engine. It was the customer records feeding it, where a single free-text field
had held a whole address for decades. That work is unchanged in scope, and an
institution that stops now will meet the same problem later with less warning.

## What this means for you

If you were ready for November, you have lost nothing. Your messages keep
flowing, and you now hold an advantage over institutions that are not.

If you were not going to be ready, you have been given room rather than a
reprieve. The sensible use of it is to find out precisely where you stand, since
the December update may set a date that leaves less time than the one just
vacated.

Either way, the question is identical to the one it was last week: which of your
messages would be rejected, and for what reason?

## How AskISO helps

AskISO exists to answer that question, and none of what it does depends on a
date.

**Check any message against the rule as it stands.** Paste a message into the
[workspace](/workspace/) and it names every finding, the rule behind it and the
path in the document, so a finding can be checked rather than taken on trust.
Nothing you paste leaves your browser.

**Measure a whole portfolio, not a sample.** `askiso batch` runs the same engine
over a directory of messages and reports how many would fail and on what. That
turns "we are probably not ready" into a number you can put in front of a
steering committee, and re-run next quarter to show movement.

**Separate the address rule from everything else.** The CBPR+ profiles are
carried as named rule sets, so you can ask specifically what fails on structured
addresses and distinguish it from unrelated findings. When Swift confirms the new
timing, that is a profile update rather than a rebuild.

**Keep it running as the rules move.** The same engine runs as a command, in
your editor through the language server, in CI, and through the MCP server for
an assistant. A rule that changes changes once, everywhere.

**Nothing is uploaded, and no specification content is redistributed.** AskISO
indexes the standard and links to the Registration Authority, which publishes
the schemas and message definition reports free of charge. Your messages stay on
your own machine.

Start with the [structured address briefing](/deadline/) for what the
requirement actually says, or check a message now in the [workspace](/workspace/).

## What to watch next

Swift has said more will follow, with updates at Sibos and resources maintained
in the customer area of swift.com. The two dates worth holding are mid-September,
for the securities and trade go-live date, and December, for the payments
timing.

We will update the [briefing](/deadline/) when Swift confirms the new timing, and
say so in the [release notes](/news/).
