---
name: "AskISO"
short_name: "AI"
title: "AskISO documentation — commands, profiles and integrations"
description: "Every AskISO command, which need a catalogue and which do not, the scheme rule profiles, and how to wire the MCP server into an AI assistant or the language server into an editor."
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
eyebrow: "Reference"
headline: "Documentation"
lead: "Every command, what it needs, and how AskISO fits into an editor, a pipeline or an AI assistant."
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

Every session on this site is replayed against the binary on each commit, so
what follows is what AskISO actually prints — not a transcription of it.

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
| `base` | Rules that apply everywhere |
| `cbpr-plus` | CBPR+ requirements in force today |
| `cbpr-2026` | The 14 November 2026 structured address mandate |
| `cbpr-2027` | Enhanced data: purpose codes, LEI, structured remittance |
| `investigations` | The camt exceptions and investigations family |
| `verification-of-payee` | VoP requirements |
| `all` | Everything above |

```bash
askiso lint payment.xml --profile cbpr-2026
askiso batch ./messages --profile all --format sarif > findings.sarif
```

SARIF output uploads straight into GitHub code scanning.

## In an editor

`askiso-lsp` is a language server, so diagnostics appear as you type. It
synchronises whole documents rather than incremental edits, and offers hover and
completion; both need a catalogue, and say so rather than guessing when there
is none.

```lua
-- Neovim
vim.lsp.start({ name = 'askiso', cmd = { 'askiso-lsp' } })
```

## In an AI assistant

`askiso-mcp` speaks the Model Context Protocol over stdio, exposing eleven
tools — lint, validate, translate, diff, generate, search, info, code, convert,
and the profile check. An assistant configured with it works against the same
engine as the CLI rather than against its own recollection of the standard.

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
overstates itself is worse than no tool. The short version: `translate` covers
seven MT/MX pairs both ways and the exception family one way only; MT940
transaction types fall back to `NMSC` because no verifiable mapping exists;
`diff` reports any pattern change as breaking because deciding otherwise is not
decidable; and a schema-generated message is minimal and synthetic rather than
realistic.

---

*AskISO is a developer tool, not regulatory advice. The authoritative source for
ISO 20022 is [iso20022.org](https://www.iso20022.org/).*
