#!/usr/bin/env python3
# SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
# SPDX-License-Identifier: Apache-2.0 OR MIT
"""Check the built site for the search defects that are easy to ship and hard to notice.

Every rule here corresponds to something that was actually wrong on this site at
some point, not to a checklist copied from a blog post:

  * og:image pointed at a file that was never built, so every share of every one
    of 2,853 pages showed a broken image;
  * meta descriptions ran to 248 characters, which search results truncate;
  * titles ran to 83 characters, likewise;
  * the FAQ declared itself an FAQ but emitted no FAQPage markup.

    python3 scripts/seocheck.py web/public
"""

from __future__ import annotations

import argparse
import html
import re
import sys
import xml.etree.ElementTree as ET
from collections import Counter
from pathlib import Path

# Search results cut a title at roughly 60 characters and a description at
# roughly 160. Below a floor they are too thin to describe the page.
TITLE_MIN, TITLE_MAX = 15, 60
DESC_MIN, DESC_MAX = 70, 160


def attr(markup: str, pattern: str) -> str | None:
    m = re.search(pattern, markup, re.S | re.I)
    return html.unescape(m.group(1)).strip() if m else None


def check(path: Path, rel: str, referenced: set[str]) -> list[str]:
    page = path.read_text(encoding="utf-8", errors="replace")
    # Comments are not rendered, so nothing in them is a defect. Without this a
    # comment explaining why the banner <img> is preloaded was itself reported
    # as an <img> with no alt text.
    page = re.sub(r"<!--.*?-->", " ", page, flags=re.S)
    head = page[: page.find("</head>") if "</head>" in page else len(page)]
    problems = []

    title = attr(head, r"<title>(.*?)</title>")
    if not title:
        problems.append("no <title>")
    elif not (TITLE_MIN <= len(title) <= TITLE_MAX):
        problems.append(f"title is {len(title)} characters, want {TITLE_MIN}-{TITLE_MAX}")

    desc = attr(head, r'<meta name="description" content="(.*?)"')
    if not desc:
        problems.append("no meta description")
    elif not (DESC_MIN <= len(desc) <= DESC_MAX):
        problems.append(f"description is {len(desc)} characters, want {DESC_MIN}-{DESC_MAX}")

    if not attr(head, r'<link rel="canonical" href="(.*?)"'):
        problems.append("no canonical link")
    if not attr(page, r'<html[^>]*lang="(.*?)"'):
        problems.append("no lang on <html>")

    # Exactly one h1: zero leaves the page without a subject, more than one
    # leaves search engines guessing which is the subject.
    h1s = re.findall(r"<h1\b[^>]*>(.*?)</h1>", page, re.S | re.I)
    if len(h1s) != 1:
        problems.append(f"{len(h1s)} <h1> elements, want exactly 1")

    # A social image that does not exist is worse than none: it renders as a
    # broken card everywhere the link is shared.
    for prop, pattern in (
        ("og:image", r'property="og:image" content="(.*?)"'),
        ("twitter:image", r'name="twitter:image" content="(.*?)"'),
    ):
        src = attr(head, pattern)
        if not src:
            problems.append(f"no {prop}")
        else:
            local = re.sub(r"^https?://[^/]+/", "", src)
            if local not in referenced:
                problems.append(f"{prop} points at {local}, which is not in the build")

    # Every image needs alt text, which is an accessibility rule first and a
    # search rule second.
    for img in re.findall(r"<img\b[^>]*>", page, re.I):
        if not re.search(r'\balt\s*=', img, re.I):
            problems.append("an <img> has no alt attribute")
            break

    # A relative url() inside an inlined stylesheet resolves against the page,
    # not against the stylesheet it came from. `logo.svg` became /faq/logo.svg
    # and 404ed on every entry page.
    for style in re.findall(r"<style>(.*?)</style>", page, re.S):
        for ref in re.findall(r"url\(\s*['\"]?([^'\")]+)", style):
            ref = ref.strip()
            if ref.startswith(("/", "data:", "http://", "https://", "#")):
                continue
            problems.append(
                f"inlined CSS has a relative url({ref!r}), which resolves "
                f"against the page rather than the stylesheet")
            break

    if rel == "faq/index.html" and "FAQPage" not in page:
        problems.append("the FAQ carries no FAQPage structured data")

    # Links that only work with a trailing slash.
    #
    # GitHub Pages serves /messages as well as /messages/, and does not redirect
    # between them. A document-relative href on /messages/ resolves against
    # /messages/; the same href on /messages resolves against /, which is a
    # different page entirely. Every link on the message index 404ed this way
    # for anyone arriving without the slash, and the link checker did not see it
    # because it resolves relative to the directory, which is the case that works.
    depth = rel.count("/")
    if depth >= 1:
        for href in re.findall(r'href="([^"#?]+)"', page):
            if href.startswith(("/", "http://", "https://", "mailto:", "data:")):
                continue
            if href.startswith("../"):
                continue  # resolves the same either way; browsers clamp at root
            problems.append(
                f"link {href!r} is document-relative, so it breaks when this "
                f"page is served without a trailing slash")
            break

    return problems


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("root", nargs="?", default="web/public")
    ap.add_argument("--sample", type=int, default=0,
                    help="check this many generated message pages rather than all "
                         "of them; they share one template, so a sample proves it")
    args = ap.parse_args()

    root = Path(args.root)
    if not root.is_dir():
        print(f"no such directory: {root}", file=sys.stderr)
        return 2

    # Everything the build actually produced, for checking references resolve.
    present = {str(p.relative_to(root)) for p in root.rglob("*") if p.is_file()}

    pages = sorted(root.rglob("index.html"))
    editorial = [p for p in pages if "messages/" not in str(p.relative_to(root))
                 or str(p.relative_to(root)) == "messages/index.html"]
    generated = [p for p in pages if p not in editorial]
    if args.sample:
        generated = generated[: args.sample]

    failures = 0
    titles = Counter()
    for path in editorial + generated:
        rel = str(path.relative_to(root))
        problems = check(path, rel, present)
        t = attr(path.read_text(encoding="utf-8", errors="replace"),
                 r"<title>(.*?)</title>")
        if t:
            titles[t] += 1
        if problems:
            failures += 1
            print(f"/{rel[:-len('index.html')]}")
            for p in problems:
                print(f"  {p}")

    # Two pages sharing a title compete with each other in search results.
    duplicates = {t: n for t, n in titles.items() if n > 1}
    if duplicates:
        failures += len(duplicates)
        print("\nduplicate titles:")
        for t, n in sorted(duplicates.items(), key=lambda kv: -kv[1]):
            print(f"  {n} pages share {t!r}")

    news_path = root / "news-sitemap.xml"
    try:
        news_root = ET.parse(news_path).getroot()
        ns = {"s": "http://www.sitemaps.org/schemas/sitemap/0.9",
              "n": "http://www.google.com/schemas/sitemap-news/0.9"}
        news_urls = news_root.findall("s:url", ns)
        if not news_urls:
            raise ValueError("contains no articles")
        for item in news_urls:
            loc = item.findtext("s:loc", default="", namespaces=ns)
            date = item.findtext("n:news/n:publication_date", default="", namespaces=ns)
            title = item.findtext("n:news/n:title", default="", namespaces=ns)
            if "/news/" not in loc or not date or not title:
                raise ValueError(f"invalid news entry: loc={loc!r}, date={date!r}, title={title!r}")
    except (OSError, ET.ParseError, ValueError) as exc:
        failures += 1
        print(f"\nnews sitemap: {exc}")

    checked = len(editorial) + len(generated)
    if failures:
        print(f"\n{checked} page(s) checked, {failures} problem(s)")
        return 1
    print(f"{checked} page(s) checked, no problems: titles and descriptions in "
          f"range, canonical and language set, one h1 each, social images resolve")
    return 0


if __name__ == "__main__":
    sys.exit(main())
