#!/usr/bin/env python3
# SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
# SPDX-License-Identifier: Apache-2.0 OR MIT
"""Build a Google News sitemap from actual news articles only."""

from __future__ import annotations

import argparse
import json
import re
import xml.etree.ElementTree as ET
from pathlib import Path

SITEMAP = "http://www.sitemaps.org/schemas/sitemap/0.9"
NEWS = "http://www.google.com/schemas/sitemap-news/0.9"


def frontmatter(path: Path) -> dict[str, str]:
    text = path.read_text(encoding="utf-8")
    match = re.match(r"^---\n(.*?)\n---\n", text, re.S)
    if not match:
        raise ValueError(f"{path}: missing front matter")
    values: dict[str, str] = {}
    for line in match.group(1).splitlines():
        key, separator, raw = line.partition(":")
        if not separator:
            continue
        raw = raw.strip()
        try:
            values[key.strip()] = json.loads(raw)
        except json.JSONDecodeError:
            values[key.strip()] = raw
    return values


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("content", nargs="?", default="web/content/news")
    parser.add_argument("output", nargs="?", default="web/public/news-sitemap.xml")
    parser.add_argument("--base-url", default="https://askiso.io")
    args = parser.parse_args()

    ET.register_namespace("", SITEMAP)
    ET.register_namespace("news", NEWS)
    root = ET.Element(f"{{{SITEMAP}}}urlset")
    count = 0
    for article in sorted(Path(args.content).glob("*.md")):
        data = frontmatter(article)
        date = data.get("news_publication_date", "")
        title = data.get("title", "")
        if not date or not title:
            raise ValueError(f"{article}: news_publication_date and title are required")
        url = ET.SubElement(root, f"{{{SITEMAP}}}url")
        ET.SubElement(url, f"{{{SITEMAP}}}loc").text = (
            f"{args.base_url.rstrip('/')}/news/{article.stem}/"
        )
        news = ET.SubElement(url, f"{{{NEWS}}}news")
        publication = ET.SubElement(news, f"{{{NEWS}}}publication")
        ET.SubElement(publication, f"{{{NEWS}}}name").text = "AskISO"
        ET.SubElement(publication, f"{{{NEWS}}}language").text = data.get("language", "en-GB")
        ET.SubElement(news, f"{{{NEWS}}}publication_date").text = date
        ET.SubElement(news, f"{{{NEWS}}}title").text = title
        if data.get("keywords"):
            ET.SubElement(news, f"{{{NEWS}}}keywords").text = data["keywords"]
        count += 1

    output = Path(args.output)
    output.parent.mkdir(parents=True, exist_ok=True)
    ET.ElementTree(root).write(output, encoding="utf-8", xml_declaration=True)
    print(f"news sitemap: {count} article(s)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
