#!/usr/bin/env python3
# SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
# SPDX-License-Identifier: Apache-2.0 OR MIT

"""Write sitemap.xml from the pages that were actually built.

ssg emits a sitemap whose contents depend on its incremental cache: a clean
build produces an empty <urlset> and a per-page sitemap in every directory,
while a rebuild over an existing output aggregates them into the root. CI
always builds clean, so the deployed sitemap listed nothing at all — 2,845
pages published and none discoverable.

Deriving it from the built output instead makes it independent of cache state,
which is the property a sitemap needs: it should describe what was published,
and nothing else can tell you that as reliably as looking at what was
published.
"""

from __future__ import annotations

import os
import sys
from datetime import date
from xml.sax.saxutils import escape

BASE_URL = "https://askiso.io"


def urls(root: str) -> list[str]:
    """Every page in the output, as a site-absolute URL path."""
    found = []
    for dirpath, dirnames, filenames in os.walk(root):
        # Build bookkeeping and asset stores are not pages.
        dirnames[:] = [d for d in dirnames if not d.startswith(".")]
        if "index.html" not in filenames:
            continue
        rel = os.path.relpath(dirpath, root)
        found.append("/" if rel == "." else f"/{rel.replace(os.sep, '/')}/")
    return sorted(found)


def main() -> int:
    root = sys.argv[1] if len(sys.argv) > 1 else "web/public"
    if not os.path.isdir(root):
        print(f"gen-sitemap: {root} is not a directory", file=sys.stderr)
        return 1

    paths = urls(root)
    if not paths:
        print(f"gen-sitemap: no pages found under {root}", file=sys.stderr)
        return 1

    today = date.today().isoformat()
    out = ['<?xml version="1.0" encoding="UTF-8"?>',
           '<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">']
    for p in paths:
        # The home page is the entry point; a message page changes only when
        # the Registration Authority publishes a new version of the standard.
        priority, changefreq = ("1.0", "weekly") if p == "/" else ("0.6", "monthly")
        out.append("  <url>")
        out.append(f"    <loc>{escape(BASE_URL + p)}</loc>")
        out.append(f"    <lastmod>{today}</lastmod>")
        out.append(f"    <changefreq>{changefreq}</changefreq>")
        out.append(f"    <priority>{priority}</priority>")
        out.append("  </url>")
    out.append("</urlset>")

    with open(os.path.join(root, "sitemap.xml"), "w", encoding="utf-8") as fh:
        fh.write("\n".join(out) + "\n")

    print(f"sitemap: {len(paths)} URL(s)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
