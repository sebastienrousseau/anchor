---
name: "AskISO"
short_name: "AskISO"
title: "ISO 20022 MCP servers for AI agents"
description: "Thirteen open MCP servers that turn plain language into validated ISO 20022 messages: initiate, settle, read, reconcile, remediate, prove readiness."
keywords: "ISO 20022 MCP server, MCP payments, AI agent bank payments, pain.001 MCP, pacs.008 MCP, camt.053 MCP, agent payments protocol, embed ISO 20022"
author: "Sebastien Rousseau"
date: "2026-08-28"
news_publication_date: "2026-08-28"
layout: "page"
language: "en-GB"
schema: "page"
changefreq: "weekly"
copyright_year: "2026"
form_origin: "https://askiso.io"
nav_mcp: "true"
banner: "digital-constellation"
banner_alt: "A network of connected points of light against a dark background, suggesting messages moving between institutions."
eyebrow: "Innovation · agents"
headline: "Let an assistant do the ISO 20022 work"
lead: "Thirteen servers that turn a sentence into a validated payment message, read the statement that comes back, and prove where you stand against the scheme rules. Your agent calls them; nothing is uploaded."
---

## What this gives you

An AI assistant understands "pay this supplier EUR 4,200 and tell me when it
settles" perfectly well. It is poor at ISO 20022, because the standard comprises
2,845 message definitions and a scheme rulebook it has never read. These servers
close that gap: the assistant supplies the intention, and the server supplies the
message, already validated against the schema before it is handed back.

Four things become possible.

- **Pay from plain language.** A sentence becomes a validated `pain.001` or
  `pacs.008`, with the IBAN and the structure verified before the message is
  returned.
- **Read what the bank sent back.** A `camt.053` statement, an MT940 or a CSV
  becomes structured entries and balances your agent can reason about.
- **Match a statement to what you expected.** Reconciliation covers exact,
  short, split and batched settlements, attaching the reasoning to each match
  rather than a numerical score alone.
- **Prove where you stand.** Score a payload against a clearing profile, receive
  the compliant form, and seal the result into an evidence pack an auditor can
  independently verify.

## Install it

One line, no account, and nothing to configure before the first call:

```bash
uvx --from "iso20022-mcp[all]" iso20022-mcp
```

`pip install "iso20022-mcp[all]"` accomplishes the same thing permanently. The
gateway loads a light core and introduces each family as an extra, so an agent
concerned solely with payments never installs anything it will not call.

`[all]` currently covers the four message families — pain, pacs, camt and acmt.
The specialist servers below install on their own name, for example
`pip install structured-address-fix-mcp`, and the gateway finds them once they
are present.

## The thirteen servers

Grouped by the job rather than by the message family, because that is the
order in which the work actually arrives.

### Start here

**[iso20022-mcp](https://github.com/sebastienrousseau/iso20022-mcp)** — the
gateway. Seven meta-tools (`search`, `list_families`, `list_servers`,
`describe`, `validate`, `generate`, `parse`) route to whichever server owns the
answer, so an assistant sees seven verbs instead of more than a hundred. Start
here unless you know exactly which family you need.

### Make a payment

**[pain001-mcp](https://github.com/sebastienrousseau/pain001-mcp)** — customer
credit transfer initiation. Schema discovery, IBAN and BIC checking, XSD
validation, cross-version migration, and an MT101 converter. Seventeen tools.

**[pacs008-mcp](https://github.com/sebastienrousseau/pacs008-mcp)** — FI-to-FI
credit transfers, returns (`pacs.004`) and status reports (`pacs.002`), with
the structured-address toolkit and an MT103 converter. Fifteen tools.

### Read what came back

**[camt053-mcp](https://github.com/sebastienrousseau/camt053-mcp)** —
bank-to-customer statements. Parses entries and balances into structured data,
validates them, and converts legacy MT94x. Twenty-two tools, the largest
surface in the suite.

**[bankstatementparser-mcp](https://github.com/sebastienrousseau/bankstatementparser-mcp)**
— the same job for everything that is not ISO 20022 yet: MT940, MT942, BAI2,
CSV and OFX. Detects the format, then returns the same shape of answer.

**[reconcile-mcp](https://github.com/sebastienrousseau/reconcile-mcp)** — match
statement entries against the payments you expected. Exact, short, over,
one-to-many and many-to-one, each with the reason it matched. Ships a sandbox
mode, so the first run needs no real data.

### Fix what is wrong

**[structured-address-fix-mcp](https://github.com/sebastienrousseau/structured-address-fix-mcp)**
— classify an address, assess it against a scheme policy, and remediate a whole
message into the structured form. Thirteen tools, and `get_cutover_date`
reports honestly that Swift has [deferred the
timing](/news/swift-defers-structured-address-migration/).

**[camt-exceptions](https://github.com/sebastienrousseau/camt-exceptions)** —
exceptions and investigations, the least-tooled corner of the standard.
Generates valid `camt.056` cancellation requests and `camt.029` resolutions.

### Prove you are ready

These four are why an institution rather than a developer would care.

**[iso20022-readiness-suite-mcp](https://github.com/sebastienrousseau/iso20022-readiness-suite-mcp)**
— the orchestration gateway. Scores a payload against a clearing profile,
proposes the compliant form, and simulates the response a bank would send.

**[iso20022-bank-profile-mcp](https://github.com/sebastienrousseau/iso20022-bank-profile-mcp)**
— the market-practice rules that sit beyond XSD validation, as versioned
profiles an agent can call. Profiles carry a tier, so a house rulebook can sit
beside the public ones without being published.

**[iso20022-evidence-pack-mcp](https://github.com/sebastienrousseau/iso20022-evidence-pack-mcp)**
— compiles findings, remediation diffs and simulated responses into a sealed
pack. The seal is a deterministic SHA-256 over the pack's canonical JSON, so
re-sealing identical content gives an identical digest and any change breaks
verification. A pack can also be signed with an Ed25519 key you hold, and the
detached signature verified by anyone you hand the public key to.

**[acmt001-mcp](https://github.com/sebastienrousseau/acmt001-mcp)** — account
management messaging: open, maintain and verify accounts from plain data.

### Bridge agent payments

**[ap2-iso20022](https://github.com/sebastienrousseau/ap2-iso20022)** — turns a
signed Google AP2 or Coinbase x402 mandate into a wire-valid `pain.001` or
`pacs.008`, with spending-cap, expiry and authorisation guardrails. It
transforms and validates. It never moves money.

## Embedding it in your own platform

The suite comprises ordinary Python packages speaking a documented protocol,
which makes it something you can position behind your own product rather than
merely something you operate yourself.

Three patterns, in the order most teams reach for them.

1. **Behind your assistant.** Direct an existing MCP client at the gateway and
   your product acquires ISO 20022 without your writing any of it — one
   configuration block, and no code whatsoever.
2. **Behind your API.** Import the underlying libraries directly and dispense
   with the protocol. Every server is a thin typed wrapper over a library that
   operates independently, so identical validation runs inside your service.
3. **Behind your brand.** The readiness gateway composes the others, so a
   platform can expose readiness scoring and remediation to its own customers as
   its own feature. Profiles carry tiers, which is where a house rulebook remains
   private.

Setting it up takes a single block of JSON, described in the
[setup guide](/mcp/setup/).

## Safe by construction

- **Validated before it is returned.** Every generator checks its output against
  the bundled XSD before handing it back, so a malformed message never leaves
  the tool.
- **It never moves money.** Producing a message is deliberately separate from
  sending one. The agent bridge transforms and validates, and stops there.
- **Read-only where it counts.** Reconciliation is pure matching, and the tools
  are marked read-only, idempotent and closed-world so a client can reason about
  what is safe to retry.
- **Nothing is uploaded.** The servers run on your machine, against your files.
  No specification content is redistributed: schemas come from your own download
  from the Registration Authority.

## Where to go next

- [Setting it up](/mcp/setup/) — one configuration block, and the first prompt to try
- [Recipes](/mcp/recipes/) — four flows an agent runs end to end
- [The command line](/docs/) — the same checks without an assistant
- [Structured addresses](/deadline/) — the rule that prompted most of this work
