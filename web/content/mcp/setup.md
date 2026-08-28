---
name: "AskISO"
short_name: "AskISO"
title: "Connecting the ISO 20022 MCP servers"
description: "Add the ISO 20022 MCP servers to Claude Code, Claude Desktop or any MCP client in one step, then make your first validated message from a prompt."
keywords: "claude mcp add, Claude Desktop MCP config, ISO 20022 MCP setup, uvx iso20022-mcp, MCP stdio server"
author: "Sebastien Rousseau"
date: "2026-08-28"
news_publication_date: "2026-08-28"
layout: "page"
language: "en-GB"
schema: "page"
changefreq: "monthly"
copyright_year: "2026"
form_origin: "https://askiso.io"
banner: "digital-constellation"
banner_alt: "A network of connected points of light against a dark background."
eyebrow: "Agents · setup"
headline: "One configuration block"
lead: "Every MCP client wants the same three things: a command, its arguments, and stdio. Here they are for the clients people actually use, and the first prompt worth trying."
---

## Before you start

Python 3.10 or later, and [uv](https://docs.astral.sh/uv/), which on macOS is
installed with `brew install uv`. Once uv is present, `uvx` executes the gateway
without installing anything permanently, which is the quickest way to establish
whether the suite is worth adopting.

## Claude Code

One command:

```bash
claude mcp add iso20022 -- uvx --from "iso20022-mcp[all]" iso20022-mcp
```

## Claude Desktop

Add this to `claude_desktop_config.json` and restart:

```json
{
  "mcpServers": {
    "iso20022": {
      "command": "uvx",
      "args": ["--from", "iso20022-mcp[all]", "iso20022-mcp"]
    }
  }
}
```

The tools appear beneath the tools icon after restarting, and Claude requests
confirmation before each call, so every individual step remains approved by you.

## Any other client

Command `uvx`, arguments `--from "iso20022-mcp[all]" iso20022-mcp`, transport
stdio. That constitutes the entire contract, and it is everything any MCP client
requires.

If you installed with `pip install "iso20022-mcp[all]"` rather than running it
through uvx, the command is simply `iso20022-mcp` with no arguments.

## Installing less than everything

The `[all]` extra installs every family. A more economical installation names
only the families you actually require:

```bash
pip install "iso20022-mcp[pain,pacs]"
```

The gateway maintains a light core and loads families as extras, so an agent
concerned only with initiating payments never carries the statement or securities
machinery alongside it.

## The first prompt

Begin with something you can verify by inspection:

> Generate a pain.001 paying ACME Ltd 4,200 EUR from my account
> GB29NWBK60161331926819, then validate it.

The assistant identifies the appropriate message, generates it, and validates it
against the schema before displaying anything. Where the IBAN is incorrect it
says so, naming the rule responsible rather than merely the field.

A second prompt worth attempting, because it addresses the requirement most
institutions actually have:

> Check whether the addresses in this pacs.008 meet the CBPR+ structured
> address rule, and show me the compliant version.

## What runs where

Everything executes on your own machine. The servers read the files you direct
them towards and return their answers over stdio; there is no intermediary
AskISO service and no account to create.

Schema validation requires the schemas themselves, which are not redistributed
here. Download the message sets you need from
[iso20022.org](https://www.iso20022.org/), free of charge, and direct the tools
at the folder containing them.

## Next

- [The thirteen servers](/mcp/) — what each one does, grouped by job
- [Recipes](/mcp/recipes/) — four flows an agent runs end to end
- [Frequently asked questions](/faq/) — the short answers
