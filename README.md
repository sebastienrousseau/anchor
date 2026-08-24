<!-- SPDX-License-Identifier: Apache-2.0 OR MIT -->

<p align="center">
  <img src="logo.svg" alt="Anchor" width="128" />
</p>

<h1 align="center">Anchor ⚓</h1>

<p align="center">
  <strong>The ISO 20022 command line.</strong><br>
  Search, inspect, validate, lint, and generate ISO 20022 messages from your terminal.
</p>

<p align="center">
  <a href="https://github.com/sebastienrousseau/anchor/actions"><img src="https://img.shields.io/github/actions/workflow/status/sebastienrousseau/anchor/ci.yml?style=for-the-badge&logo=github" alt="Build" /></a>
  <a href="https://www.iso20022.org"><img src="https://img.shields.io/badge/standard-ISO%2020022-blue.svg?style=for-the-badge" alt="ISO 20022" /></a>
  <a href="LICENSE-APACHE"><img src="https://img.shields.io/badge/license-Apache%202.0%20%2F%20MIT-orange.svg?style=for-the-badge" alt="License" /></a>
  <a href="https://scorecard.dev/viewer/?uri=github.com/sebastienrousseau/anchor"><img src="https://img.shields.io/ossf-scorecard/github.com/sebastienrousseau/anchor?style=for-the-badge&label=OpenSSF%20Scorecard&logo=openssf" alt="OpenSSF Scorecard" /></a>
</p>

---

## Try it without installing anything

**[anchor.dev — the web version](https://sebastienrousseau.github.io/anchor/)** runs the same
engine compiled to WebAssembly. Lint, generate, browse, convert, look up codes and check
IBAN/BIC/UETR values in the browser.

Your messages never leave the tab — there is no server to send them to. That matters when
the payload is a real payment instruction.

---

## What Anchor is

Anchor is a single Go binary for working with ISO 20022 financial messages. It gives you
fuzzy search across the whole message catalogue, a Bubble Tea TUI, schema and sample
viewers, a semantic business-rule linter, synthetic message generation, MT ⇄ MX
cross-references, and a mock clearing rail — without leaving the terminal.

**Anchor does not redistribute ISO 20022 specifications.** The Registration Authority
publishes them free of charge at [iso20022.org](https://www.iso20022.org/); you download
what you need and point Anchor at it. That keeps the binary small, keeps your schemas
current, and means the specification content you validate against comes from the source
of truth rather than a mirror of unknown age.

> This is not the official ISO 20022 site. The sole source of up-to-date ISO 20022
> material is <https://www.iso20022.org/>

---

## Install

```bash
go install github.com/sebastienrousseau/anchor/cmd/anchor@latest
```

Or build from source:

```bash
git clone https://github.com/sebastienrousseau/anchor
cd anchor && make build
```

No external dependencies. Validation is pure Go, so there is no libxml2 or cgo to
install and results are identical on every platform. `xmllint`, if you have it, can be
used as a cross-check with `--engine libxml2`.

---

## Getting a catalogue

Anchor knows about all **2,845 message definitions across 285 published message sets**
out of the box — that index is embedded, so `search`, `info`, `code`, `translate`,
`lint` and `generate` work the moment you install it, with no download and no network.
That includes converting a real SWIFT MT message: `translate payment.mt103` needs no
schemas, because the target document is built rather than read.

Commands that read the actual XSD files — `schema`, `sample`, `diff`, `stats`,
`code --sets`, and full `validate` — need a catalogue. Download the message sets you want from the
[ISO 20022 catalogue of messages](https://www.iso20022.org/iso-20022-message-definitions)
and import them:

```bash
anchor catalog fetch pacs.008        # opens the right page, imports what lands
anchor code --import ~/Downloads/ExternalCodeSets.xlsx   # the external code sets
anchor catalog add ~/Downloads/PaymentsClearingAndSettlement_v11.zip
anchor catalog add ~/Downloads/*.zip
anchor catalog add ~/Downloads --dry-run     # see what would happen first
```

`catalog add` unpacks nested archives (the RA ships zips inside zips), sorts every file
into the right place, and matches the archive against the official set names so
`PaymentsClearingAndSettlement_v11.zip` and `payments-clearing-and-settlement.zip` both
land in one canonical directory.

Not sure what you have? `anchor catalog status` compares your install against the full
published standard; `anchor catalog where` shows every location Anchor searches.

The layout it produces, which you can also create by hand:

```text
<catalogue root>/
└── Payments Clearing and Settlement/
    ├── Version 11.0/
    │   ├── Schemas/                    ← required
    │   │   ├── pacs.008.001.10.xsd
    │   │   └── pacs.009.001.10.xsd
    │   ├── Sample Messages/            ← optional
    │   └── Message Definition Reports/ ← optional
    └── Version 12.0/
```

Anchor looks for the catalogue in this order:

| # | Location |
| :-- | :--- |
| 1 | `--catalog <path>` |
| 2 | `$ANCHOR_CATALOG` |
| 3 | `$XDG_DATA_HOME/anchor/catalog`, when that variable is set |
| 4 | `~/Library/Application Support/anchor/catalog` (macOS) |
| 5 | `~/.local/share/anchor/catalog` |
| 6 | The working directory and its parents |

`catalog add` writes to whichever of those already holds a catalogue, so importing more
message sets extends your existing one rather than quietly starting a second.

```bash
anchor doctor    # confirms what Anchor found, and fails if it found nothing
```

If no catalogue is present, commands that need one tell you where Anchor looked and exit
non-zero. It never reports an empty catalogue as a healthy one.

> **Keep the catalogue out of cloud-synced folders.** iCloud Drive, Dropbox and OneDrive
> evict cold files and leave placeholders behind. Anchor detects iCloud placeholders and
> refuses to parse them, but the local data directory is the reliable home.

---

## Commands

Commands marked ◆ need a catalogue; the rest work standalone.

| Command | Description |
| :--- | :--- |
| `anchor catalog fetch <msg\|set>` | Open the right download page, then import the archive when it lands |
| `anchor catalog add <zip\|dir>...` | Import message sets downloaded from iso20022.org (`--dry-run`, `--to`) |
| `anchor catalog status` | Compare what you have installed against all 285 published sets (`--all`) |
| `anchor catalog where` | Show every location Anchor searches, and which one it picked |
| `anchor` ◆ | Interactive TUI: live search, message table, schema and sample viewers |
| `anchor search <query>` | Search by ID, domain, code, or keyword (`--json`); uses the embedded registry when no catalogue is installed |
| `anchor info <msg-id>` | Metadata and schema paths (`--json`); without a catalogue, names the message set to download |
| `anchor schema <msg-id>` ◆ | Syntax-highlighted XSD (`--copy`, `--raw`) |
| `anchor sample <msg-id>` ◆ | Syntax-highlighted sample XML (`--copy`, `--raw`) |
| `anchor stats` ◆ | Catalogue metrics and domain distribution (`--json`) |
| `anchor diff <from> <to>` ◆ | Path-level schema comparison with breaking-change classification (`--breaking`, `--strict`, `--json`) |
| `anchor validate <xml> [xsd]` | Full XSD validation, pure Go, no external tools (`--json`, `--stream`, `--engine libxml2`) |
| `anchor lint <xml>` | Business rules plus scheme profiles (`--profile all`, `--strict`, `--json`, `--format sarif`) |
| `anchor generate <type>` | Any of the 2,845 messages: templates with rail presets for four, schema-driven for the rest (`--from-schema`, `--optional`) |
| `anchor convert <file>` | ISO 20022 XML ⇄ JSON (`--to-json`, `--to-xml`) |
| `anchor format <xml>` | Pretty-print or minify (`--minify`, `--copy`) |
| `anchor code [query]` | Look up codes: curated, from your schemas, and from an imported publication (`--sets`, `--set`, `--import`) |
| `anchor translate <code>` | SWIFT MT ⇄ ISO 20022 MX field-mapping reference (`--matrix`) |
| `anchor translate <file>` | Convert a real message either way, with a fidelity report (`--out`, `--report`, `--format json`) |
| `anchor translate --matrix` | The full MT ⇄ MX cross-reference, field by field |
| `anchor batch <dir\|glob>` | Lint, validate and profile many messages at once (`--format sarif`, `--workers`) |
| `anchor flow [type]` | Simulate a `pain.001` → `pacs.008` → `pacs.002` → `camt.053` lifecycle |
| `anchor graph [type]` | Sequence diagrams (`--format mermaid/ascii`) |
| `anchor mock` | Local HTTP mock clearing rail (`--port`) |
| `anchor doctor` | Diagnostics: catalogue, toolchain, AI connectivity |
| `anchor completion <shell>` | Shell completions for zsh, bash, fish, powershell |
| `anchor version` | Build version and metadata |

### Using Anchor from an AI assistant

`anchor-mcp` serves the same engine over the [Model Context Protocol](https://modelcontextprotocol.io),
so an assistant can check the specification instead of recalling it. Ten tools:
search, info, lint, profile check, validate, generate, MT translation, code
lookup, schema diff, and XML/JSON conversion.

```json
{
  "mcpServers": {
    "anchor": { "command": "anchor-mcp" }
  }
}
```

It speaks newline-delimited JSON-RPC 2.0 on stdin and stdout, writes nothing
else to stdout, and needs no catalogue for the seven tools that work in light
mode. `anchor-mcp --tools` lists what it exposes.

### Using Anchor from an editor

`anchor-lsp` is a language server for ISO 20022 XML. It publishes diagnostics as
you type — business rules, schema validation against your own downloaded XSDs,
and the CBPR+ rules that take effect on 14 November 2026 — and adds hover,
completion and a document outline driven by the schema.

Neovim:

```lua
vim.api.nvim_create_autocmd('FileType', {
  pattern = 'xml',
  callback = function()
    vim.lsp.start({ name = 'anchor', cmd = { 'anchor-lsp' }, root_dir = vim.fn.getcwd() })
  end,
})
```

Completion lists the elements the schema allows at the cursor, **in schema
order** — ISO 20022 content models are ordered sequences, so an alphabetical
list would suggest invalid documents. Inside an enumerated element it offers the
code set instead. Without a catalogue it offers nothing rather than guessing.

`--profile` selects the rule profile (default `cbpr-2026`); an empty value turns
scheme rules off. A client can change it at runtime by sending
`workspace/didChangeConfiguration` with `{"anchor": {"profile": "cbpr-plus"}}`.

### Rule profiles

| Profile | What it checks |
| :--- | :--- |
| `base` | Structural sanity, applicable to any message |
| `cbpr-plus` | CBPR+ requirements in force today |
| `cbpr-2026` | Postal addresses must be hybrid or structured from **14 November 2026** |
| `cbpr-2027` | The 2026 rules plus enhanced data: purpose codes, structured remittance, LEIs, UETRs |
| `investigations` | camt.110 / camt.111 — every investigation identifies its payment, every response quotes its request |
| `verification-of-payee` | acmt.023 / acmt.024 — a request says what to check, a failed report says why |
| `all` | Everything. Rules whose date has not arrived report as warnings, so this reads as a readiness report |

Dates that have not arrived produce **warnings**, not errors, so `--profile
cbpr-2027` tells you what to fix without failing a build for something that is
not yet required. The exception is `ENH-LEI-001`: a legal entity identifier that
fails its own ISO 7064 checksum is not a field awaiting a deadline, it is a
wrong one, and it is reported as an error today.

### In continuous integration

The repository ships a composite action, so a pull request that introduces a
non-compliant message is annotated rather than merged silently:

```yaml
permissions:
  contents: read
  security-events: write

jobs:
  iso20022:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v5
      - uses: actions/setup-go@v6
      - uses: sebastienrousseau/anchor@v1
        with:
          path: ./messages
          profile: cbpr-2026
```

Findings are uploaded as SARIF, so they appear on the diff. Nothing is
downloaded from iso20022.org: linting and the rule profiles run from the
embedded index.

### TUI keys

Type any text to filter the catalogue — **every plain letter belongs to the
filter**, so the shortcuts are modified. `Enter` opens the sample, `ctrl+s` the
schema, `ctrl+k` checks the message (business rules, schema, and the November
2026 address rules in one pane), `ctrl+y` copies, `ctrl+a` opens the assistant,
`?` shows help, `q` quits.

`/catalog` shows what you have installed against the whole published standard,
with a download link beside anything missing. `/check` is the same as `ctrl+k`.

---

## The website

`web/site` is a static page that loads `pkg/iso20022` compiled to WebAssembly, so the
browser runs byte-for-byte the same logic as the CLI. A message linted on the website
gets the identical verdict to one linted in a terminal or a CI pipeline.

```bash
make web         # build the WebAssembly bundle
make web-test    # 30-check smoke test of the Go/JS bridge (needs node)
make web-serve   # http://127.0.0.1:8765
```

The site is light mode only: it carries the embedded index of the standard, never any
schema content. Anything needing the XSD text links to the official download and points
at the CLI.

---

## Go SDK

```go
import "github.com/sebastienrousseau/anchor/pkg/iso20022"

xml, _ := iso20022.Generate(iso20022.GeneratorOptions{
    MsgType: "pacs.008",
    Preset:  "sepa",
    Amount:  "25000.00",
})

res, _ := iso20022.Lint([]byte(xml), "transfer.xml")
fmt.Printf("passed: %d  errors: %d\n", res.Passed, res.Errors)

jsonBytes, _ := iso20022.XMLToJSON([]byte(xml))
```

`pkg/iso20022` is the shared core. The CLI, the website's WebAssembly build, and any Go
service that imports it all run the same code.

Everything above works with no catalogue. To read schema text, open one:

```go
cat, err := iso20022.OpenCatalogue("")     // searches the conventional locations
info, _ := cat.Lookup("pacs.008.001.10")   // works with a nil *Catalogue too

if !info.Installed {
    // info.Sets names the message sets that publish it, each with a DownloadURL()
    for _, s := range info.Sets {
        fmt.Printf("download %s from %s\n", s, s.DownloadURL())
    }
}
```

The API reports `Installed: false` rather than failing, so callers can always tell the
user what to download.

---

## The November 2026 address requirement

From **14 November 2026** CBPR+ rejects fully unstructured postal addresses outright,
with no contingency. Anchor checks readiness:

```bash
anchor lint payment.xml --profile cbpr-2026
```

```
  ❌ [CBPR-ADDR-002] the address is fully unstructured (3 address line(s), no structured element)
     at       /Document/CdtTrfTxInf/Dbtr/PstlAdr
     expected hybrid or structured
     fix      Move the town into <TwnNm> and the country into <Ctry>; keep the
              remainder in at most two <AdrLine> elements.

  ❌ [CBPR-ADDR-003] "France" is not an ISO 3166-1 alpha-2 country code
     at       /Document/CdtTrfTxInf/Cdtr/PstlAdr/Ctry
     expected "FR" (the code for France)
```

The pack enforces town and country presence, the ISO 3166-1 alpha-2 code format, the
two-line/70-character hybrid limit, and flags hybrid addresses as workable but less
durable. The exempt message types — `camt.052`, `camt.053`, `camt.054`, `camt.060`,
`camt.025`, `admi.024` — are skipped and reported as out of scope rather than passing.

This matters because schema validation cannot catch it: ISO 20022 constrains `<Ctry>` to
`[A-Z]{2,2}` and nothing more, so `XX` validates. Anchor checks against the 249 assigned
ISO 3166-1 codes.

Available as `--profile cbpr-2026` in the CLI and as the **Nov 2026** tab on the website.
Neither needs a catalogue: the rules are embedded.

---

## Validation

`anchor validate` implements the XML Schema subset ISO 20022 uses — element order,
cardinality, choices, wildcards, patterns, enumerations, length and numeric facets — in
pure Go.

It is checked against the reference implementation: over the whole catalogue Anchor and
libxml2 agree on **4,746 of 4,746** documents, accepting the same 1,035 and rejecting the
same 3,711. `make differential` reproduces that.

Diagnostics say more than the reference does:

```
  payment.xml:20:7
    [pattern]  "EURO" does not match the required format
    at       /Document/FIToFICstmrCdtTrf/CdtTrfTxInf/IntrBkSttlmAmt/@Ccy
    expected [A-Z]{3,3}
    found    EURO
```

The schema is resolved from the document's own namespace, so the second argument is
optional. Exit status is 0 when valid, 1 when not, so it drops straight into CI.

---

## Known limitations

Stated plainly, because a validation tool that overstates itself is worse than no tool.

- **`generate` covers every message, two ways.** `pacs.008`, `pacs.009`, `pain.001` and
  `camt.053` come from templates with rail-aware defaults and need no catalogue. Every
  other message is built by walking its schema, which needs one installed. All 4,746
  installed schemas were generated, validated against themselves, and linted clean —
  that is asserted by `make conformance`, not claimed. A schema-built message is
  minimal and synthetic: it shows the shape, not a realistic payment.
- **`translate` covers seven pairs, both ways.** MT101, MT103, MT104, MT107, MT202, MT204
  and MT940 become `pain.001`, `pacs.008`, `pain.008`, `pacs.009`, `pacs.010` and
  `camt.053`, and each of those converts back. Statements carry their entries in both
  directions. Everything produced validates against its schema and lints clean, and every
  pair round-trips. The rest of the MT suite is a reference table only.
- **The exception family converts one way only.** MT n92, n95 and n96 become `camt.056`,
  `camt.110` and `camt.111` for every category (MT192, MT292, MT592 and the rest). The
  new messages want coded investigation types and reasons where MT carries prose, and
  Anchor will not invent a code it cannot verify: the proprietary branch of the choice
  names the source message and the prose becomes the narrative. Converting back is not
  implemented.
- **The two directions lose different things, and both say so.** MT to MX produces
  unstructured addresses, which CBPR+ stops accepting on 14 November 2026. MX to MT loses
  purpose codes, legal entity identifiers and structured remittance outright, flattens
  structured addresses into free text, and cuts a 35-character reference to the 16 an MT
  field allows. A statement entry keeps its amount, dates and references, but MT940 wants
  a four-character transaction type from its own vocabulary and no verifiable mapping from
  the ISO 20022 bank transaction code exists — so a structured code becomes `NMSC` and the
  report names what was lost. A proprietary code already shaped like an MT type is passed
  through exactly, which is how a statement generated from an MT940 gets its own codes
  back.
- **Conversion is lossy, by nature and by design.** MT addresses are unstructured, so a
  converted message will not satisfy CBPR+ from 14 November 2026 until the addresses are
  enriched. Every source field appears in the fidelity report; nothing is dropped
  silently.
- **`diff` compares patterns conservatively.** Deciding whether one regular expression
  accepts everything another does is not decidable in general, so any pattern change is
  reported as breaking.
- **`code` searches three sources.** A curated dictionary of 33 codes that needs nothing
  installed; every code set enumerated in the schemas you downloaded; and the Registration
  Authority's external code set publication once you import it with
  `anchor code --import <ExternalCodeSets.xlsx>`. Anchor ships none of the last two —
  they are your download, stored beside your catalogue.
- **`anchor-lsp` synchronises whole documents**, not incremental edits, and offers no
  code actions or formatting. Completion and hover need an installed catalogue; without
  one they say so rather than guessing.
- **Streaming validation releases transaction subtrees, not everything.** A file of
  8 MiB or more is validated as it is read, at roughly 120 bytes per transaction rather
  than the whole document — a 39 MB statement costs about 2.4 MB. The verdict is identical
  to the buffered path, asserted against every sample message in the catalogue. Positions
  beyond the last 16,384 lines are reported as byte offsets rather than line numbers.
- **`catalog fetch` guides a download; it does not perform one.** It finds the right
  message set, opens the Registration Authority's page, and imports the archive when it
  appears in your downloads folder. It never accepts the RA's terms on your behalf, and it
  will not, because those terms are yours to accept.
- **`convert` refuses names that are not valid XML.** Go's XML decoder is more lenient
  than the specification, and a name Anchor accepted on the way in would be one it could
  not emit on the way back. Found by fuzzing, along with an attribute-escaping bug.
- **Non-adjacent repeated elements cannot be converted to JSON.** A JSON object cannot
  express that ordering, so `convert` reports it rather than silently reordering the
  document. No message in the catalogue hits this.

---

## What is built, and what is next

Everything on the original roadmap is built. What follows is what it would take
next, listed so the gaps are visible rather than implied.

| Next | Why it is not there yet |
| :--- | :--- |
| A published bank-transaction-code mapping | MT940 field 61 wants MT's own four-character vocabulary. A mapping exists in scheme documentation; Anchor will carry one when it can be verified against a source, not before |
| Realistic schema-driven output | A schema walk produces a minimal, synthetic message with plausible identifiers. Making one read like a real trade — consistent parties, matching references across a lifecycle — needs domain data Anchor does not carry |

---

## Installing

```bash
# macOS and Linux
brew install sebastienrousseau/tap/anchor

# Windows
scoop bucket add sebastienrousseau https://github.com/sebastienrousseau/scoop-bucket
scoop install anchor

# Debian, Ubuntu, Fedora, RHEL, Alpine, Arch — see the release page for packages
# Or from source:
go install github.com/sebastienrousseau/anchor/cmd/anchor@latest
go install github.com/sebastienrousseau/anchor/cmd/anchor-mcp@latest
go install github.com/sebastienrousseau/anchor/cmd/anchor-lsp@latest
```

Every release archive carries all three binaries, an SBOM, and a Sigstore
signature over the checksums. The signing certificate records the workflow and
the commit that produced the artifact, so it can be verified without trusting a
key anyone had to store:

```bash
cosign verify-blob checksums.txt \
  --signature checksums.txt.sig \
  --certificate checksums.txt.pem \
  --certificate-identity-regexp 'https://github.com/sebastienrousseau/anchor/.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

---

## Development

```bash
make build         # build the binary
make test          # unit tests
make cover         # tests with a coverage floor
make ci            # the full local gate: fmt, vet, lint, test, cover, vuln, build
make conformance   # validate generated output against real schemas (needs a catalogue)
make conformance   # generate, convert and validate against your own catalogue
make differential  # agreement with libxml2 across the whole catalogue
make fuzz          # fuzz the parsers (FUZZTIME=5m for a longer run)
make web           # build the WebAssembly bundle for the website
make web-test      # smoke-test the Go/JS bridge
```

`make conformance` is not run in CI, because CI has no catalogue. Run it before tagging a
release. It records known defects explicitly, so fixing one turns the suite red until the
expectation is removed.

See [CONTRIBUTING.md](CONTRIBUTING.md) and [SECURITY.md](SECURITY.md).

---

## License

Dual-licensed at your option:

- **Apache License 2.0** ([LICENSE-APACHE](LICENSE-APACHE))
- **MIT License** ([LICENSE-MIT](LICENSE-MIT))

ISO 20022 is a registered standard of the International Organization for Standardization.
Anchor bundles no ISO 20022 specification content — see [NOTICE](NOTICE).
