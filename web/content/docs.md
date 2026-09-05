---
name: "AskISO"
short_name: "AskISO"
title: "AskISO documentation — commands, profiles and integrations"
description: "Every AskISO command, which ones need a catalogue, the scheme rule profiles, and how to wire it into your editor, your build or an AI assistant."
keywords: "askiso documentation, ISO 20022 CLI reference, askiso commands, ISO 20022 MCP server, ISO 20022 language server, askiso lint, askiso validate, askiso translate"
author: "Sebastien Rousseau"
date: "2026-08-25"
news_publication_date: "2026-08-25"
layout: "page"
language: "en-GB"
schema: "page"
changefreq: "weekly"
copyright_year: "2026"
form_origin: "https://askiso.io"
nav_docs: "true"
banner: "getty-images-dqHskSJDfe4"
banner_alt: "Lines of code and data rendered as a cyan grid."
eyebrow: "Reference"
headline: "Documentation"
lead: "Every command, what it needs, and how AskISO fits into your editor, your build and your AI assistant."
---

## Install

```bash
go install github.com/sebastienrousseau/askiso/cmd/askiso@latest
```

There is no tagged release yet, so `go install` is the way in. The Homebrew cask
and Scoop manifest publish on the first tag.

The two protocol servers are separate binaries, because a client launches each
one and takes over its standard input and output:

```bash
go install github.com/sebastienrousseau/askiso/cmd/askiso-mcp@latest
go install github.com/sebastienrousseau/askiso/cmd/askiso-lsp@latest
```

Building from source needs **Go 1.26.6 or newer** — that release carries the
standard library security fixes AskISO builds against.

## See it work

Every session here is replayed against the binary on each commit. So what you
read below is what AskISO actually prints, not a transcription of it.

Converting a SWIFT MT message, with the fidelity report that says what survived
the trip:

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
       MT addresses are unstructured; CBPR+ rejects those once the
       deferred structured address requirement takes effect. Populate
       TwnNm and Ctry.
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
       MT addresses are unstructured; CBPR+ rejects those once the
       deferred structured address requirement takes effect. Populate
       TwnNm and Ctry.
  ✅ :71A:  mapped
       → /Document/FIToFICstmrCdtTrf/CdtTrfTxInf/ChrgBr

  ⚠️  this conversion is lossy — review the entries above before relying on it
  → check the CBPR+ address rules:      askiso lint <file> --profile cbpr-2026
```

Conversion between MT and MX is lossy in both directions. The report names
every field and what became of it, so nothing is dropped without saying so.

## Commands

Commands marked ◆ read the actual XSD files and need a catalogue installed.
Everything else works standalone, against the index embedded in the binary.

| Command | What it does |
| :--- | :--- |
| `askiso` ◆ | Terminal UI: live search, message table, schema and sample viewers |
| `askiso <question>` | Put a question straight on the command line — no quotes needed |
| `askiso ask` | The assistant, as an explicit subcommand or an interactive REPL |
| `askiso search <query>` | Search by identifier, domain, code or keyword |
| `askiso info <msg-id>` | Metadata and schema paths; names the set to download if absent |
| `askiso validate <xml>` | Full XSD validation, pure Go, no external tools |
| `askiso lint <xml>` | Business rules and scheme profiles |
| `askiso batch <dir>` | Validate and lint many messages at once |
| `askiso translate <file>` | SWIFT MT to ISO 20022 and back, with a fidelity report |
| `askiso generate <type>` | Synthetic messages for testing |
| `askiso diff <from> <to>` ◆ | Schema comparison with breaking-change classification |
| `askiso convert <xml>` | ISO 20022 XML to structured JSON and back |
| `askiso format <xml>` | Pretty-print or minify a message |
| `askiso code <code>` | Look up external codes — reasons, purpose, charges, status |
| `askiso schema <msg-id>` ◆ | Syntax-highlighted XSD |
| `askiso sample <msg-id>` ◆ | Syntax-highlighted sample message |
| `askiso flow` | Generate a linked four-stage payment lifecycle |
| `askiso graph` | Sequence diagrams and flowcharts, Mermaid or ASCII |
| `askiso mock` | A local HTTP mock clearing rail for testing |
| `askiso stats` ◆ | Catalogue metrics and domain distribution |
| `askiso list` | The 56 message set categories |
| `askiso catalog` | Manage the local catalogue: `fetch`, `add`, `status`, `where` |
| `askiso cbpr-pack import` | Build a private release manifest and local conformance sample suite |
| `askiso cbpr-pack status` | Show present and missing CBPR+ Usage Guideline variants |
| `askiso cbpr-pack verify` | Verify pinned local samples, schemas and external codes |
| `askiso doctor` | Diagnostics: what was found, and where it looked |

Every command takes `--json`. Exit status is 0 on success and 1 on failure, so
any of them drops into a pipeline.

## Getting a catalogue

AskISO ships no ISO 20022 specification content. Download the message sets you
need from the Registration Authority and import them:

```bash
askiso catalog fetch pacs.008     # opens the right page, imports what lands
askiso catalog add ~/Downloads/*.zip
askiso catalog status             # what you have against all 285 published sets
askiso catalog where              # every location searched, and which won
```

The search order is `--catalog`, then `$ASKISO_CATALOG`, then
`$XDG_DATA_HOME/askiso/catalog`, the platform data directory, and finally the
working directory and its parents.

## Rule profiles

Schema validity says a message is well formed. It says nothing about whether a
clearing system will accept it. Profiles are the rules on top:

| Profile | What it checks |
| :--- | :--- |
| `base` | Structural sanity checks that apply everywhere |
| `cbpr-plus` | Live SR2025 message and Usage Identifier dispatch, BAH consistency, party/address rules, totals, UETRs, currencies and pacs.009 variants |
| `cbpr-2026` | Readiness for the deferred structured-address requirement; no replacement date is assumed ([briefing](/deadline/)) |
| `cbpr-2027` | Enhanced data: purpose codes, LEI, structured remittance |
| `investigations` | The camt exceptions and investigations family |
| `verification-of-payee` | VoP requirements |
| `all` | Everything above |

```bash
askiso lint payment.xml --profile cbpr-2026
askiso batch ./messages --profile all --format sarif > findings.sarif
```

SARIF output uploads straight into GitHub code scanning.

The embedded `cbpr-plus` rules cover the cross-message layer without a
catalogue. Exact per-message restrictions remain defined by the applicable
Swift MyStandards Usage Guidelines. A base ISO 20022 schema is not a CBPR+
Usage Guideline.

### Local CBPR+ packs

The CBPR+ Usage Guideline workflow targets **Standards Release 2025
(SR2025)**. The separate `cbpr-2026` profile is only a forward-looking
structured-address readiness check and is not an SR2025 Usage Guideline pack.

AskISO ships the engine, not Swift publications. An authorised user can point
the CLI at a private folder of Usage Guideline PDFs:

To obtain executable material, open **CBPR+ SR2025 (Combined)** in MyStandards,
select all 31 Usage Guidelines, add them to **My Selection**, and request the
**XML Schema Package** export. It contains one directory per selected Usage
Guideline, including payload and BAH XSDs. A prepared export downloads
immediately; a generated one appears under the **MyDownloads** icon at the top
right. If multi-selection export is unavailable, use each Usage Guideline's
**Documentation → Export** action with the XML Schema format. Keep specialised
variant folders such as STP, COV, ADV, MLP and COL intact.

```bash
askiso ask "Where is UETR mandatory?" --cbpr-pack /secure/CBPRPlus-SR2025
askiso lint payment.xml --cbpr-pack /secure/CBPRPlus-SR2025
askiso batch ./messages --cbpr-pack /secure/CBPRPlus-SR2025 --format sarif

askiso cbpr-pack import /secure/CBPRPlus-SR2025 \
  --workspace ~/.askiso-cbpr/sr2025 \
  --release SR2025 \
  --external-codes ~/Downloads/2Q2026_externalcodesets_v3.json \
  --generate-samples \
  --acknowledge-entitlement
askiso cbpr-pack status ~/.askiso-cbpr/sr2025
askiso cbpr-pack verify /secure/CBPRPlus-SR2025 \
  --workspace ~/.askiso-cbpr/sr2025
askiso cbpr-pack conformance /secure/CBPRPlus-SR2025 \
  --workspace ~/.askiso-cbpr/sr2025 --as-of 2026-09-05
askiso lint payment.xml --cbpr-workspace ~/.askiso-cbpr/sr2025
askiso batch ./messages --cbpr-workspace ~/.askiso-cbpr/sr2025 --schema
askiso validate payment.xml pacs.008.001.08.xsd \
  --external-codes ~/Downloads/2Q2026_externalcodesets_v3.json
```

The folder is compiled in memory with local `pdftotext`; it is not uploaded,
copied, or cached. For repeated runs, `askiso cbpr-pack compile <folder>
--output private.cbpr-pack.json` creates an owner-readable local pack. Pack
files are gitignored by default and must not be redistributed unless the user
has the necessary rights.

Local-only refers to AskISO's processing. A folder stored in iCloud Drive or
another synchronised filesystem may still be uploaded by that provider under
the user's operating-system settings.

`ask --cbpr-pack` performs local extractive search across PDF, MyStandards JSON,
XML/XSD and XLSX files. Legacy `.xls` files are inventoried but must be saved as
`.xlsx` to be searched. Results use relative local filenames and PDF pages where
applicable. The command bypasses all model-provider code, including OpenAI and
Ollama, even if provider credentials are configured.

Every result includes its pack fingerprint and coverage. PDF-derived checks
cover explicit hierarchy/cardinality tables and supported lexical types, while
clearly warning that narrative, conditional, external-code-set, and
diagram-only rules may require separate validation.

The workspace path adds a content-minimised manifest, exact source hashes and a
versioned sample suite without copying the user's inputs. Registration Authority
XLSX, record/group JSON and v3 JSON Schema code-set publications are supported;
their enumerated values are enforced when matching external simple types appear
in a local XSD. Suite expectations follow an explicit filename convention:
`.invalid.`, `-invalid` or `_invalid` expects rejection, while other paired XML
samples expect acceptance. This is a reproducible local baseline, not a Swift
Readiness Portal verdict.

Use `lint --cbpr-workspace` or `batch --cbpr-workspace` to load that pinned
baseline directly. Both commands verify its pack and external-code fingerprints;
`batch --schema` applies the pinned external-code values to matching schema types.
Workspace and ad-hoc pack flags are mutually exclusive.

Private-source discovery supports PDF, Excel (`.xlsx`/`.xls`), MyStandards
Usage Guideline JSON Schema, and XML. AskISO indexes the JSON export's release,
message and Usage Guideline variant metadata without storing its schema body in
the workspace. JSON Schema and XML Schema remain distinct: JSON exports count
as guideline JSON, while only local XSD/XML schemas can be paired with XML
samples. ZIP archives are intentionally ignored.

With `--generate-samples`, each entitled executable XSD produces an owner-only
schema-valid positive and a well-formed wrong-namespace negative inside the
private workspace. The suite hash-pins them, records `origin: generated` and
the negative mutation, and verifies them before use. These derived fixtures are
AskISO validator self-tests—not Swift-authored samples or certification.

Status reports overall guideline inventory separately from exact executable
message/Business Service coverage. User-provided samples are paired by embedded
`BizSvc` or filename variant markers; AskISO refuses ambiguous core/ADV/COV/STP
matches instead of silently selecting the first schema.

The strict conformance gate additionally checks private permissions,
entitlement acknowledgement, 31/31 executable variants, positive and negative
user cases, representative failure categories, the pinned external-code
publication quarter, FINplus BAH/payload bindings, and optional content-free
independent evidence. It never invokes an online validator itself.

AskISO is an independent project and is not affiliated with, endorsed by, or
certified by Swift. Swift and MyStandards are trademarks of S.W.I.F.T. SC. Do
not publish a compiled pack unless the source licence permits it; this project
documentation is not legal advice.

## In an editor

`askiso-lsp` is a language server, so diagnostics appear as you type. It syncs whole documents rather than single edits, and it also offers hover and
completion; both need a catalogue, and say so rather than guessing when there
is none.

```lua
-- Neovim
vim.lsp.start({ name = 'askiso', cmd = { 'askiso-lsp' } })
```

## In an AI assistant

`askiso-mcp` speaks the Model Context Protocol over stdio, exposing eleven
tools — lint, validate, translate, diff, generate, search, info, code, convert,
and the profile check. An assistant set up this way works against the same engine as the command line,
rather than leaning on its own memory of the standard.

```json
{
  "mcpServers": {
    "askiso": { "command": "askiso-mcp" }
  }
}
```

## In continuous integration

```yaml
- run: go install github.com/sebastienrousseau/askiso/cmd/askiso@latest
- run: askiso batch ./messages --profile cbpr-2026 --format sarif > askiso.sarif
- uses: github/codeql-action/upload-sarif@v3
  with:
    sarif_file: askiso.sarif
```

## Known limitations

The [repository README](https://github.com/sebastienrousseau/askiso#known-limitations)
lists every one in detail, stated plainly because a validation tool that
overstates itself is worse than no tool. In short, there are four of them. `translate` covers seven MT/MX pairs both
ways, and the exception family in one direction only. MT940 transaction types
fall back to `NMSC`, because no verified mapping exists for them. `diff` treats
any pattern change as breaking, since deciding otherwise is not possible. And a
message generated from a schema is minimal and synthetic rather than realistic.

---

*AskISO is a developer tool, not regulatory advice. The authoritative source for
ISO 20022 is [iso20022.org](https://www.iso20022.org/).*
