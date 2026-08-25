---
name: "AskIso"
short_name: "AI"
title: "Conformance evidence — what AskIso is measured against"
description: "Differential agreement with libxml2 across all 4,746 schemas, streaming and buffered validation agreement, coverage enforced at 95% on a catalogue-free runner, and zero known vulnerabilities. Every figure names the command that reproduces it."
keywords: "ISO 20022 validator conformance, libxml2 differential testing, XML schema validator correctness, askiso conformance, ISO 20022 validation evidence"
author: "Sebastien Rousseau"
date: "2026-08-25"
news_publication_date: "2026-08-25"
layout: "page"
language: "en-GB"
schema: "page"
changefreq: "weekly"
copyright_year: "2026"
form_origin: "https://askiso.io"
nav_conformance: "true"
eyebrow: "Evidence"
headline: "Measured, not asserted"
lead: "A validator is only worth what it can prove. Every figure here names the command that reproduces it, and says when it was last run."
---

## Why this page exists

Anyone can claim their validator is correct. The question a bank's integration
architect actually asks is *how do you know*, and most tools in this space have
no answer beyond a test suite nobody outside can run.

Each measure below names the command that produces it. Run it yourself: the
commands are in the repository, and three of the four run on every commit.

## Correctness

### Differential agreement with libxml2

Across every schema in the ISO 20022 catalogue, AskIso and libxml2 return the
same verdict on **4,746 of 4,746 documents** — accepting the same 1,035 and
rejecting the same 3,711.

This is the measure that matters most for a validator written from scratch.
libxml2 is the reference implementation the industry already trusts; agreement
with it across the whole catalogue is a stronger statement than any number of
hand-written test cases.

```bash
make differential
```

Needs a catalogue and `xmllint`. It is not run in CI, because a clean runner has
neither — it runs before a release.

### Streaming and buffered agreement

A file of 8 MiB or more is validated as it is read, at roughly 120 bytes per
transaction rather than holding the whole document. A 39 MB statement costs
about 2.4 MB of memory.

The streaming path and the buffered path return **identical verdicts on 400 of
400** sample messages. A fast path that disagreed with the slow one would be
worse than no fast path.

```bash
make conformance
```

### Generated messages validate and lint clean

Every message AskIso can generate from a schema — all **4,746** — is generated,
validated against the schema it came from, and linted. All pass.

```bash
make conformance
```

## Engineering

### Test coverage

**95.2%**, enforced at a 95% floor, and measured on a runner with **no
catalogue installed**. That last part is the honest bit: measuring on a
developer machine credits code that only runs when a catalogue happens to be
present, which flatters the number by roughly a point.

```bash
make cover
```

Runs on every commit. The build fails below the floor.

### Known vulnerabilities

**Zero.** `govulncheck` runs against the full dependency graph and the standard
library on every commit and fails the build on a finding.

```bash
make vuln
```

Verified on every commit, on Linux, macOS and Windows.

### Fuzzing

The parsers all take input nobody vetted: a schema the user downloaded, a
message that arrived over a wire, an MT file from another bank's system. They
are fuzzed for the one property that matters — that they always return.

It has already found three real defects: an off-by-one slice that crashed the
CLI, an attribute-escaping bug that produced invalid XML, and a class of element
names Go's lenient decoder accepts but that could not be re-emitted.

```bash
make fuzz
```

### Accessibility

This site passes **WCAG 2.2 with zero issues across all 2,847 pages**, checked
on every deploy. It fails the deploy rather than the claim going stale.

## Coverage of the standard

| Measure | Figure |
| :--- | :--- |
| Message definitions indexed offline | 2,845 |
| Message sets | 285 |
| Business areas | 56 |
| Schemas generated, validated and linted | 4,746 |
| MT types converting to MX | 10 |
| MX families converting back to MT | 6 |
| Specification content redistributed | 0 bytes |

## What is not measured here

The figures above cover correctness against schemas and rules. They say nothing
about whether a given scheme, correspondent or market infrastructure will accept
a message — that depends on agreements AskIso has no visibility of.

Where a mapping cannot be verified against a published source, AskIso reports
the gap rather than guessing. MT940 transaction types fall back to `NMSC` and
name what was lost; camt.110 investigation types use the proprietary branch
rather than inventing a code. The
[known limitations](https://github.com/sebastienrousseau/askiso#known-limitations)
list every such case.

---

*Figures were last verified on 25 August 2026. The coverage, vulnerability and
accessibility numbers are enforced on every commit; the differential and
conformance suites need a catalogue and run before a release.*
