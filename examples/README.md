<!-- SPDX-License-Identifier: Apache-2.0 OR MIT -->

# Examples

Runnable programs, one per flow. Each is small enough to read in a sitting and
complete enough to use as the starting point for a real integration.

These complement the [documentation examples](https://pkg.go.dev/github.com/sebastienrousseau/askiso/pkg/iso20022#pkg-examples),
which are snippets showing a single call. The programs here are end-to-end: they
take input, exit with a meaningful status, and print something you could act on.

Every one of them is built and exercised by `go test ./examples/...`, so none of
them can rot without the build noticing.

| Example | Question it answers | Needs a catalogue |
| --- | --- | --- |
| [`remediate`](remediate) | What is wrong with this message, and what do I change? | No |
| [`cbpr-2026`](cbpr-2026) | Will this still be accepted after 14 November 2026? | No |
| [`mt-to-mx`](mt-to-mx) | What does this SWIFT MT message look like as ISO 20022, and what was lost? | No |
| [`lookup`](lookup) | What is this message, and what converts into it? | No |
| [`ci-sarif`](ci-sarif) | How do I fail a build on bad messages? | No |
| [`batch-audit`](batch-audit) | Across everything we send, which rule do we fail most? | No |
| [`validate`](validate) | Is this message valid against its XML Schema? | **Yes** |

## Running them

```bash
go run ./examples/remediate  payment.xml
go run ./examples/cbpr-2026  payment.xml
go run ./examples/mt-to-mx   payment.mt103
go run ./examples/lookup     pacs.008
go run ./examples/ci-sarif   ./messages > askiso.sarif
go run ./examples/batch-audit ./outbound
go run ./examples/validate   payment.xml ~/iso20022-catalogue
```

## Why only one needs a catalogue

AskISO redistributes no ISO 20022 specification content. It embeds an index of
the standard — every identifier, every version, which message set publishes it,
and where the Registration Authority hosts it — so lookup, linting, the scheme
rule profiles and MT conversion all work with nothing on disk.

Schema validation is the exception, because it needs the schema text itself.
Download the message sets you need from [iso20022.org](https://www.iso20022.org/),
free of charge, and point `validate` at the folder.

`validate` reports a missing schema as missing rather than reporting the message
as valid. Those two outcomes look identical if a tool conflates them, and only
one of them means the message is correct.

## Exit codes

The programs that answer a yes/no question exit non-zero when the answer is no,
so they compose into scripts and pipelines without parsing their output:

```bash
if ! go run ./examples/cbpr-2026 payment.xml; then
  echo "not ready for November 2026"
fi
```
