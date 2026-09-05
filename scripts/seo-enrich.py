#!/usr/bin/env python3
# SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
# SPDX-License-Identifier: Apache-2.0 OR MIT
"""Repair the social descriptions and add breadcrumb and software markup.

Three things the generator gets wrong or leaves out, fixed after the build so
they cannot drift from the page:

  * og:description and twitter:description were the first 160 characters of
    the rendered body -- eyebrow, headline and lead run together -- so every
    share of every page carried a sentence nobody wrote. They now carry the
    meta description, which somebody did.
  * No page said where it sat in the site. A BreadcrumbList tells a search
    engine and an assistant that /mcp/setup/ belongs to the MCP servers, and
    that a message page belongs to the reference, which is how a result gets a
    path under its title rather than a bare URL.
  * Nothing said the site describes a piece of software. A SoftwareApplication
    node on the home page and the documentation names the licence, the price,
    the platforms and where the source is, which is what an assistant is asked
    when somebody wants "a free ISO 20022 validator".

Everything is read back out of the built page, so the markup agrees with the
visible text by construction.

    python3 scripts/seo-enrich.py web/public
"""

from __future__ import annotations

import html
import json
import re
import sys
from pathlib import Path

SITE = "https://askiso.io"

LD = re.compile(
    r'(<script type="application/ld\+json">\s*)(\{.*?\})(\s*</script>)', re.S
)
DESC = re.compile(r'<meta name="description" content="(.*?)"', re.S)
OG = re.compile(r'(<meta property="og:description" content=")(.*?)(")', re.S)
TW = re.compile(r'(<meta name="twitter:description" content=")(.*?)(")', re.S)
H1 = re.compile(r"<h1\b[^>]*>(.*?)</h1>", re.S | re.I)
TAGS = re.compile(r"<[^>]+>")
RELEASE = re.compile(r"AskISO (v\d+\.\d+\.\d+)\. Released under")

# Pages that get the SoftwareApplication node. Site-wide would be noise: the
# home page and the documentation are where somebody decides whether to install.
SOFTWARE = {"", "docs"}


def h1_text(markup: str) -> str:
    m = H1.search(markup)
    return html.unescape(TAGS.sub("", m.group(1))).strip() if m else ""


# A section's crumb is its name, not its headline: "News", not "What changed,
# and when". Anything not listed falls back to the page's own h1.
SECTIONS = {
    "messages": "Message reference",
    "mcp": "MCP servers",
    "news": "News",
    "docs": "Documentation",
    "contact": "Contact",
}


def crumbs(rel: str, root: Path, titles: dict[str, str]) -> list[tuple[str, str]]:
    """Home, then each ancestor directory that has a page, then this page."""
    parts = [p for p in rel.split("/") if p]
    out = [("Home", f"{SITE}/")]
    for depth in range(1, len(parts) + 1):
        path = "/".join(parts[:depth])
        page = root / path / "index.html"
        if not page.is_file():
            continue
        if path not in titles:
            titles[path] = h1_text(page.read_text(encoding="utf-8", errors="replace"))
        name = titles[path] if depth == len(parts) else SECTIONS.get(path, titles[path])
        out.append((name, f"{SITE}/{path}/"))
    return out


def breadcrumb_node(rel: str, items: list[tuple[str, str]]) -> dict:
    return {
        "@type": "BreadcrumbList",
        "@id": f"{SITE}/{rel}#breadcrumb" if rel else f"{SITE}/#breadcrumb",
        "itemListElement": [
            {"@type": "ListItem", "position": i + 1, "name": name, "item": url}
            for i, (name, url) in enumerate(items)
        ],
    }


def software_node(version: str) -> dict:
    node = {
        "@type": "SoftwareApplication",
        "@id": f"{SITE}/#software",
        "name": "AskISO",
        "url": f"{SITE}/",
        "applicationCategory": "DeveloperApplication",
        "applicationSubCategory": "ISO 20022 validation, linting and SWIFT MT conversion",
        "operatingSystem": "Linux, macOS, Windows, WebAssembly",
        "programmingLanguage": "Go",
        "isAccessibleForFree": True,
        "offers": {"@type": "Offer", "price": "0", "priceCurrency": "GBP"},
        "license": [
            "https://www.apache.org/licenses/LICENSE-2.0",
            "https://opensource.org/license/mit",
        ],
        "codeRepository": "https://github.com/sebastienrousseau/askiso",
        "downloadUrl": "https://github.com/sebastienrousseau/askiso/releases",
        "installUrl": f"{SITE}/docs/",
        "softwareHelp": {"@type": "CreativeWork", "url": f"{SITE}/docs/"},
        "author": {"@id": f"{SITE}/#organization"},
        "featureList": [
            "XML Schema validation of ISO 20022 messages",
            "Business rule linting with rule identifiers and remediation",
            "CBPR+ scheme rule profiles including structured address readiness",
            "SWIFT MT to ISO 20022 conversion with a fidelity report",
            "Offline index of 2,845 ISO 20022 message definitions",
            "Language server, GitHub Action and Model Context Protocol server",
        ],
    }
    if version:
        node["softwareVersion"] = version
    return node


def enrich(page: Path, root: Path, titles: dict[str, str]) -> bool:
    markup = page.read_text(encoding="utf-8", errors="replace")
    rel = page.parent.relative_to(root).as_posix()
    rel = "" if rel == "." else rel
    changed = False

    desc = DESC.search(markup)
    if desc:
        d = desc.group(1)
        for pat in (OG, TW):
            m = pat.search(markup)
            if m and m.group(2) != d:
                markup = markup[: m.start()] + m.group(1) + d + m.group(3) + markup[m.end():]
                changed = True

    block = LD.search(markup)
    if not block:
        if changed:
            page.write_text(markup, encoding="utf-8")
        return changed
    graph = json.loads(block.group(2))
    nodes = graph.setdefault("@graph", [])
    types = {n.get("@type") for n in nodes}

    if rel and "BreadcrumbList" not in types:
        items = crumbs(rel, root, titles)
        if len(items) > 1:
            node = breadcrumb_node(rel, items)
            nodes.append(node)
            for n in nodes:
                if n.get("@type") == "WebPage":
                    n["breadcrumb"] = {"@id": node["@id"]}
            changed = True

    if rel in SOFTWARE and "SoftwareApplication" not in types:
        m = RELEASE.search(markup)
        nodes.append(software_node(m.group(1) if m else ""))
        for n in nodes:
            if n.get("@type") == "WebPage":
                n["about"] = {"@id": f"{SITE}/#software"}
        changed = True

    if changed:
        rebuilt = block.group(1) + json.dumps(graph, indent=2) + block.group(3)
        markup = markup[: block.start()] + rebuilt + markup[block.end():]
        page.write_text(markup, encoding="utf-8")
    return changed


def main() -> int:
    root = Path(sys.argv[1] if len(sys.argv) > 1 else "web/public")
    if not root.is_dir():
        print(f"seo-enrich: no such directory: {root}", file=sys.stderr)
        return 1
    titles: dict[str, str] = {}
    done = sum(enrich(p, root, titles) for p in sorted(root.rglob("index.html")))

    robots = root / "robots.txt"
    if robots.is_file():
        text = robots.read_text(encoding="utf-8").rstrip("\n") + "\n"
        line = f"Sitemap: {SITE}/news-sitemap.xml"
        if line not in text:
            text += line + "\n"
        robots.write_text(text, encoding="utf-8")

    print(f"seo-enrich: {done} page(s) given social descriptions, breadcrumbs and software markup")
    return 0


if __name__ == "__main__":
    sys.exit(main())
