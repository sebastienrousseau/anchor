#!/usr/bin/env python3
# SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
# SPDX-License-Identifier: Apache-2.0 OR MIT
"""Give news articles a byline and mark them as NewsArticle.

The generator emits WebSite, Organization and WebPage for every page, which says
a page exists but not what kind of thing it is, who stands behind it, or what it
is based on. For a news article those three are the whole point.

An assistant answering "did Swift move the structured address deadline" has to
decide which of several pages to quote and whom to credit. A WebPage node gives
it a title and a date. A NewsArticle node gives it a headline, a named author, a
publisher and — through `isBasedOn` and `citation` — the primary source the piece
is reporting on, so the claim can be traced past this site to Swift's own
announcement. That is the difference between being read and being cited.

Everything is read back out of the built page. Structured data that disagrees
with the visible text is worse than none: it is grounds for a manual penalty, and
a second hand-maintained copy of a headline drifts the moment somebody edits one.

The byline is the visible half of the same claim. An article whose authorship
exists only in metadata asks a reader to take it on trust and gives an assistant
nothing to corroborate; the Person node carries the same name with links to where
that person can be checked, so the expertise can be validated from more than one
source rather than asserted here.

    python3 scripts/article-schema.py web/public
"""

from __future__ import annotations

import html
import json
import re
import sys
from pathlib import Path

SITE = "https://askiso.io"

# Where the author can be corroborated. Only places that genuinely identify the
# same person belong here: a sameAs an assistant cannot verify is worse than
# none, because it invites it to conflate two people.
AUTHOR_SAME_AS = (
    "https://sebastienrousseau.com/",
    "https://github.com/sebastienrousseau",
)

# Where the news articles live. The index at /news/ is a hub, not an article.
NEWS_DIR = "news"

LD = re.compile(r'(<script type="application/ld\+json">)(.*?)(</script>)', re.S)


def meta(markup: str, name: str) -> str:
    """A meta tag's content, by name or by property."""
    for pattern in (rf'<meta name="{name}" content="([^"]*)"',
                    rf'<meta property="{name}" content="([^"]*)"'):
        found = re.search(pattern, markup)
        if found:
            return html.unescape(found.group(1))
    return ""


def first_external_link(markup: str) -> str:
    """The first link out of this site in the article body.

    A news piece reporting an announcement links to it, and by the site's own
    convention that link comes in the opening sentence. Taking it from the page
    rather than from a list beside it keeps the citation and the prose the same
    claim.
    """
    body = markup
    start = body.find('<main')
    if start >= 0:
        body = body[start:]
    for href in re.findall(r'<a\b[^>]*href="(https?://[^"]+)"', body):
        if "askiso.io" in href or "github.com/sebastienrousseau" in href:
            continue
        return html.unescape(href)
    return ""


def article_node(page: Path, out: Path, markup: str) -> dict | None:
    heading = re.search(r"<h1[^>]*>(.*?)</h1>", markup, re.S)
    if not heading:
        return None

    rel = page.relative_to(out).parent.as_posix()
    url = f"{SITE}/{rel}/"
    published = meta(markup, "article:published_time") or meta(markup, "date")
    if not published:
        node = re.search(r'"datePublished":\s*"([^"]+)"', markup)
        published = node.group(1) if node else ""

    article: dict = {
        "@type": "NewsArticle",
        "@id": f"{url}#article",
        "headline": re.sub(r"<[^>]+>", "", heading.group(1)).strip(),
        "description": meta(markup, "description"),
        "url": url,
        "mainEntityOfPage": {"@id": f"{url}#webpage"},
        "inLanguage": "en-GB",
        "publisher": {"@id": f"{SITE}/#organization"},
        "isAccessibleForFree": True,
    }

    author = meta(markup, "author")
    if author:
        article["author"] = {
            "@type": "Person",
            "@id": f"{SITE}/#author",
            "name": author,
            "url": f"{SITE}/about/",
            "sameAs": list(AUTHOR_SAME_AS),
        }
    if published:
        article["datePublished"] = published
        article["dateModified"] = published

    image = meta(markup, "og:image")
    if image:
        article["image"] = image

    source = first_external_link(markup)
    if source:
        # isBasedOn is what the piece reports on; citation is the same URL as a
        # reference. Both are stated because consumers read one or the other.
        article["isBasedOn"] = source
        article["citation"] = source

    return article


def add_byline(markup: str, node: dict) -> str:
    """Put the author and the date at the head of the article body.

    Placed inside the content wrapper rather than the banner so it reads as part
    of the piece, and marked with rel="author" so the link and the Person node
    say the same thing to a parser that only reads one of them.
    """
    author = (node.get("author") or {}).get("name")
    published = node.get("datePublished")
    if not author or not published:
        return markup

    anchor = '<div lang="en">'
    if anchor not in markup or 'class="byline"' in markup:
        return markup

    pretty = published
    try:
        from datetime import date
        y, m, d = (int(part) for part in published.split("-")[:3])
        pretty = date(y, m, d).strftime("%-d %B %Y")
    except (ValueError, TypeError):
        pass

    html_byline = (
        '<p class="byline">By <a rel="author" href="/about/">'
        f'{author}</a> · <time datetime="{published}">{pretty}</time></p>'
    )
    return markup.replace(anchor, anchor + html_byline, 1)


def main() -> int:
    out = Path(sys.argv[1] if len(sys.argv) > 1 else "web/public")
    news = out / NEWS_DIR
    if not news.is_dir():
        print("article-schema: no news directory", file=sys.stderr)
        return 1

    marked = 0
    for page in sorted(news.glob("*/index.html")):
        markup = page.read_text(encoding="utf-8")
        block = LD.search(markup)
        if not block:
            print(f"article-schema: {page} carries no JSON-LD to extend", file=sys.stderr)
            return 1

        graph = json.loads(block.group(2))
        if any(n.get("@type") == "NewsArticle" for n in graph.get("@graph", [])):
            continue

        node = article_node(page, out, markup)
        if node is None:
            print(f"article-schema: {page} has no h1 to take a headline from",
                  file=sys.stderr)
            return 1

        graph["@graph"].append(node)
        rebuilt = block.group(1) + json.dumps(graph, indent=2) + block.group(3)
        markup = markup[:block.start()] + rebuilt + markup[block.end():]
        markup = add_byline(markup, node)
        page.write_text(markup, encoding="utf-8")
        marked += 1

    print(f"article schema: {marked} news article(s) marked as NewsArticle")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
