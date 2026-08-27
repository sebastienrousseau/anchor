#!/usr/bin/env python3
# SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
# SPDX-License-Identifier: Apache-2.0 OR MIT
"""Mark the pages that exist to be arrived at, not to be found.

Two pages here are answers to something that already happened rather than
destinations in their own right: the 404, which answers an address that does not
resolve, and the confirmation after the contact form, which answers a message
already sent. Ranking either would put a visitor on a page whose entire content
is about an event that did not happen to them.

The generator writes its own robots meta and ignores a `robots:` key in the
front matter, so this is done to the built page rather than declared in the
source. Removing them from the sitemap is not done by name anywhere: a page
marked noindex should not be listed, so check-sitemap.py reads the mark back out
of the built HTML. The rule has one place to live and the two cannot drift.

    python3 scripts/noindex.py web/public
"""

from __future__ import annotations

import re
import sys
from pathlib import Path

# Directories under the output root, without the trailing index.html.
PATHS = ("404", "contact/sent")

ROBOTS = re.compile(r'<meta name="robots" content="[^"]*"')
REPLACEMENT = '<meta name="robots" content="noindex, follow"'


def main() -> int:
    out = Path(sys.argv[1] if len(sys.argv) > 1 else "web/public")

    marked = []
    for rel in PATHS:
        page = out / rel / "index.html"
        if not page.exists():
            print(f"noindex: {rel}/ did not build", file=sys.stderr)
            return 1

        html = page.read_text(encoding="utf-8")
        if "noindex" in html:
            marked.append(rel)
            continue

        html, count = ROBOTS.subn(REPLACEMENT, html, count=1)
        if count == 0:
            # No robots meta to rewrite: add one rather than silently skip.
            html = html.replace("</head>", f"    {REPLACEMENT}>\n  </head>", 1)
        page.write_text(html, encoding="utf-8")
        marked.append(rel)

    # The sitemap is rebuilt before this runs, so it lists them again.
    sitemap = out / "sitemap.xml"
    if sitemap.exists():
        xml = sitemap.read_text(encoding="utf-8")
        for rel in PATHS:
            xml = re.sub(
                r"\s*<url>(?:(?!</url>).)*?<loc>[^<]*/" + re.escape(rel) + r"/</loc>.*?</url>",
                "", xml, flags=re.S)
        sitemap.write_text(xml, encoding="utf-8")

    print(f"noindex: {', '.join('/' + m + '/' for m in marked)}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
