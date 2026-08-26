---
name: "AskISO"
short_name: "AI"
title: "The 14 November 2026 structured address mandate — what it requires and how to check"
description: "From 14 November 2026, CBPR+ rejects unstructured postal addresses on cross-border payments. 44% of banks are not on track and 65% of messages are still non-compliant. Check your own messages with AskISO."
keywords: "CBPR+ structured address, November 2026 deadline, ISO 20022 structured address mandate, unstructured address rejected, hybrid address pacs.008, SWIFT 2026 address requirement, structured address migration"
author: "Sebastien Rousseau"
date: "2026-08-25"
news_publication_date: "2026-08-25"
layout: "deadline"
language: "en-GB"
schema: "page"
changefreq: "weekly"
copyright_year: "2026"
form_origin: "https://askiso.io"
nav_deadline: "true"
eyebrow: "CBPR+ · cross-border payments"
headline: "Unstructured addresses stop being accepted"
lead: "From 14 November 2026, a cross-border payment carrying an unstructured postal address is rejected on the network. Not flagged, not downgraded — rejected."
---

## What changes

From 14 November 2026, financial institutions and corporates sending cross-border
payments over SWIFT CBPR+ or the key payment market infrastructures must use
**hybrid or fully structured addresses** wherever an address is present.
Non-compliant messages are rejected outright.

A structured address puts each component in its own element — town, post code,
country, street, building number — instead of the free-text `AdrLine` lines that
MT carried and that early ISO 20022 messages allowed. A hybrid address keeps
some address lines but requires town name and country to be structured.

This is not the whole of ISO 20022's 2026 change, and the address rule is the
one with a hard rejection behind it.

## Where the industry actually is

| Measure | Figure |
| :--- | :--- |
| Banks not on track for the deadline | **44%** |
| Messages still non-compliant, mid-2026 | **~65%** |
| Customer address records still unstructured, on average | **32%** |
| Institutions reporting core banking gaps for structured addresses | **60%** |

Nearly one institution in ten reports that more than half of its address data is
still non-compliant. The gap is rarely in the payment engine — it is in the
customer data that feeds it, which is why it takes longer to close than it looks.

## What comes after

| Date | What it brings |
| :--- | :--- |
| 14 November 2026 | Structured address mandate. Unstructured addresses rejected. |
| November 2027 | Enhanced data: purpose codes, LEI, structured remittance move from optional to expected. |
| November 2028 | MT retirement. The coexistence period ends. |

Treating 2026 as a one-off project is the expensive path. The same data quality
work — structured parties, verifiable identifiers, coded reasons instead of
prose — is what 2027 and 2028 also require.

## Check your own messages

AskISO checks the address rules without needing a schema installed, so this
takes about a minute:

```bash
go install github.com/sebastienrousseau/askiso/cmd/askiso@latest
```

Then point it at a message. This is a real session, replayed — the output below
is what the binary writes, checked against it on every commit:

```console
$ askiso lint payment.xml --profile cbpr-2026
  LINTER   Semantic Business Rule Linter: payment.xml

  ✅ All 2 semantic checks passed with zero issues!
     • IBAN Modulo-97 Checksums : Verified
     • BIC / SWIFT Structure   : Verified
     • ISO 4217 Decimals       : Verified
     • RFC 4122 UUIDv4 UETR    : Verified

    PROFILE   cbpr-2026 — CBPR+ requirements effective 14 November 2026: postal addresses must be hybrid or fully structured.

  ❌ [CBPR-ADDR-002] the address is fully unstructured (2 address line(s), no structured element)
     at       /Document/FIToFICstmrCdtTrf/CdtTrfTxInf/Cdtr/PstlAdr
     expected hybrid or structured
     fix      Move the town into <TwnNm> and the country into <Ctry>; keep the remainder in at most two <AdrLine> elements.

  ❌ [CBPR-ADDR-001] the address has no country
     at       /Document/FIToFICstmrCdtTrf/CdtTrfTxInf/Cdtr/PstlAdr/Ctry
     expected a <Ctry> element
     fix      Populate <TwnNm> and <Ctry> inside <PstlAdr>.

  ❌ [CBPR-ADDR-001] the address has no town name
     at       /Document/FIToFICstmrCdtTrf/CdtTrfTxInf/Cdtr/PstlAdr/TwnNm
     expected a <TwnNm> element
     fix      Populate <TwnNm> and <Ctry> inside <PstlAdr>.

Error: profile cbpr-2026 found 3 error(s)
```

Run it across a directory to get a count rather than a verdict on one file:

```console
$ askiso batch ./messages --profile cbpr-2026
  BATCH   4 file(s)

  ✅ clean.xml
  ❌ pay-1.xml  pacs.008.001.10
      [CBPR-ADDR-002] the address is fully unstructured (2 address line(s), no structured element)
         at /Document/FIToFICstmrCdtTrf/CdtTrfTxInf/Cdtr/PstlAdr
      [CBPR-ADDR-001] the address has no country
         at /Document/FIToFICstmrCdtTrf/CdtTrfTxInf/Cdtr/PstlAdr/Ctry
      [CBPR-ADDR-001] the address has no town name
         at /Document/FIToFICstmrCdtTrf/CdtTrfTxInf/Cdtr/PstlAdr/TwnNm
      [CBPR-ADDR-002] the address is fully unstructured (3 address line(s), no structured element)
         at /Document/FIToFICstmrCdtTrf/CdtTrfTxInf/Dbtr/PstlAdr
      [CBPR-ADDR-004] 3 address lines; CBPR+ permits at most 2
         at /Document/FIToFICstmrCdtTrf/CdtTrfTxInf/Dbtr/PstlAdr/AdrLine
      [CBPR-ADDR-001] the address has no country
         at /Document/FIToFICstmrCdtTrf/CdtTrfTxInf/Dbtr/PstlAdr/Ctry
      [CBPR-ADDR-001] the address has no town name
         at /Document/FIToFICstmrCdtTrf/CdtTrfTxInf/Dbtr/PstlAdr/TwnNm

  ❌ pay-2.xml  pacs.008.001.10
      [CBPR-ADDR-002] the address is fully unstructured (2 address line(s), no structured element)
         at /Document/FIToFICstmrCdtTrf/CdtTrfTxInf/Cdtr/PstlAdr
      [CBPR-ADDR-001] the address has no country
         at /Document/FIToFICstmrCdtTrf/CdtTrfTxInf/Cdtr/PstlAdr/Ctry
      [CBPR-ADDR-001] the address has no town name
         at /Document/FIToFICstmrCdtTrf/CdtTrfTxInf/Cdtr/PstlAdr/TwnNm
      [CBPR-ADDR-002] the address is fully unstructured (3 address line(s), no structured element)
         at /Document/FIToFICstmrCdtTrf/CdtTrfTxInf/Dbtr/PstlAdr
      [CBPR-ADDR-004] 3 address lines; CBPR+ permits at most 2
         at /Document/FIToFICstmrCdtTrf/CdtTrfTxInf/Dbtr/PstlAdr/AdrLine
      [CBPR-ADDR-001] the address has no country
         at /Document/FIToFICstmrCdtTrf/CdtTrfTxInf/Dbtr/PstlAdr/Ctry
      [CBPR-ADDR-001] the address has no town name
         at /Document/FIToFICstmrCdtTrf/CdtTrfTxInf/Dbtr/PstlAdr/TwnNm

  ❌ pay-3.xml  pacs.008.001.10
      [CBPR-ADDR-002] the address is fully unstructured (2 address line(s), no structured element)
         at /Document/FIToFICstmrCdtTrf/CdtTrfTxInf/Cdtr/PstlAdr
      [CBPR-ADDR-001] the address has no country
         at /Document/FIToFICstmrCdtTrf/CdtTrfTxInf/Cdtr/PstlAdr/Ctry
      [CBPR-ADDR-001] the address has no town name
         at /Document/FIToFICstmrCdtTrf/CdtTrfTxInf/Cdtr/PstlAdr/TwnNm
      [CBPR-ADDR-002] the address is fully unstructured (3 address line(s), no structured element)
         at /Document/FIToFICstmrCdtTrf/CdtTrfTxInf/Dbtr/PstlAdr
      [CBPR-ADDR-004] 3 address lines; CBPR+ permits at most 2
         at /Document/FIToFICstmrCdtTrf/CdtTrfTxInf/Dbtr/PstlAdr/AdrLine
      [CBPR-ADDR-001] the address has no country
         at /Document/FIToFICstmrCdtTrf/CdtTrfTxInf/Dbtr/PstlAdr/Ctry
      [CBPR-ADDR-001] the address has no town name
         at /Document/FIToFICstmrCdtTrf/CdtTrfTxInf/Dbtr/PstlAdr/TwnNm

  1 passed, 3 failed, 21 error(s) total
$ askiso batch ./messages --profile cbpr-2026 --format sarif > findings.sarif
```

The SARIF output drops into GitHub code scanning or any tool that reads it, so
the check can sit in a pipeline rather than in someone's terminal.

Nothing is uploaded. AskISO runs locally, which matters when the input is a real
payment instruction.

## The rules behind it

The `cbpr-2026` profile carries five address rules, `CBPR-ADDR-001` through
`CBPR-ADDR-005`. Every diagnostic names the rule, the path, what was expected
and what to do about it, so it can be handed to whoever owns the data rather
than interpreted first.

`--profile cbpr-2027` checks the enhanced-data expectations that follow a year
later, and `--profile all` runs everything.

## If you are converting from MT

MT addresses are unstructured by nature — that is what fields 50K and 59 carry.
A message converted from MT will therefore **not** satisfy the mandate until the
addresses are enriched from another source. AskISO says so rather than handing
you a message that looks compliant:

```console
$ askiso translate payment.mt103 --report
  TRANSLATE    MT103 → pacs.008.001.10

  source   payment.mt103
  fields   5 mapped, 3 derived, 2 truncated, 1 unmapped

  ⚠️  :121:  derived
       → /Document/FIToFICstmrCdtTrf/CdtTrfTxInf/PmtId/UETR
       the source carried no UETR; a new one was generated
  ✅ :20:  mapped
       → /Document/FIToFICstmrCdtTrf/GrpHdr/MsgId
  ❌ :23B:  unmapped
       bank operation code has no direct equivalent; carry it as a local
       instrument if the rail defines one
  ✅ :32A:  mapped
       → /Document/FIToFICstmrCdtTrf/CdtTrfTxInf/IntrBkSttlmAmt
  ✅ :50K:  mapped
       → /Document/FIToFICstmrCdtTrf/CdtTrfTxInf/Dbtr
  ⚠️  :50K (address):  truncated
       → /Document/FIToFICstmrCdtTrf/CdtTrfTxInf/Dbtr/PstlAdr/AdrLine
       MT addresses are unstructured; CBPR+ rejects those from 14
       November 2026. Populate TwnNm and Ctry before then.
  ⚠️  :52:  derived
       → /Document/FIToFICstmrCdtTrf/CdtTrfTxInf/DbtrAgt
       taken from the message header
  ⚠️  :57:  derived
       → /Document/FIToFICstmrCdtTrf/CdtTrfTxInf/CdtrAgt
       taken from the message header
  ✅ :59:  mapped
       → /Document/FIToFICstmrCdtTrf/CdtTrfTxInf/Cdtr
  ⚠️  :59 (address):  truncated
       → /Document/FIToFICstmrCdtTrf/CdtTrfTxInf/Cdtr/PstlAdr/AdrLine
       MT addresses are unstructured; CBPR+ rejects those from 14
       November 2026. Populate TwnNm and Ctry before then.
  ✅ :71A:  mapped
       → /Document/FIToFICstmrCdtTrf/CdtTrfTxInf/ChrgBr

  ⚠️  this conversion is lossy — review the entries above before relying on it
  → check the 14 Nov 2026 address rules:  askiso lint <file> --profile cbpr-2026
```

The fidelity report names every field in the source and what became of it —
mapped, derived, truncated or unmapped. Nothing is dropped silently, which
matters most in exactly this case: the conversion succeeds, the XML is
schema-valid, and it would still be rejected in November.

## Remediating at scale

For bulk address remediation there is a separate tool,
[structured-address-fix](https://github.com/sebastienrousseau/structured-address-fix),
which analyses and proposes structured forms for existing records. Remediation is
a data problem rather than a message problem, and it is worth separating the two:
validating tells you the size of the gap, remediation closes it.

---

*AskISO is a developer tool, not regulatory advice. A clean result is not an
assurance that a scheme or correspondent will accept a message. The authoritative
sources are [iso20022.org](https://www.iso20022.org/), your scheme operator, and
Swift's own CBPR+ documentation.*

*Figures above are drawn from published industry readiness reporting in mid-2026;
they describe the industry, not any individual institution.*
