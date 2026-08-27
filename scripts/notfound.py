#!/usr/bin/env python3
# SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
# SPDX-License-Identifier: Apache-2.0 OR MIT
"""Put the built 404 page where GitHub Pages looks for it.

Pages serves /404.html for any address it cannot match. Without one it serves
its own, which is titled "Page not found - GitHub Pages", carries GitHub's
branding rather than this site's, and offers no way back in. Every stale link
and every mistyped message identifier landed there — and since 2,845 of the
2,862 pages here describe one message, a mistyped identifier is easily the most
likely wrong address anyone types.

The page is written as ordinary content so it gets the same header, footer,
banner and theme as everything else. That leaves it at /404/index.html, and this
copies it to the name Pages wants. One file can answer for every address only
because every reference in it is absolute, which is checked here rather than
assumed: a relative `src` would resolve against whatever wrong path the visitor
happened to type, and 404 pages are exactly where nobody is watching.

    python3 scripts/notfound.py web/public
"""

from __future__ import annotations

import re
import sys
from pathlib import Path

# src/href values that would break when the page is served from another path.
# Anything absolute, a full URL, a data: or mailto: URI, or a fragment is fine.
RELATIVE = re.compile(r'(?:src|href)="(?!https?:|//|/|data:|mailto:|#)([^"]+)"')


def main() -> int:
    out = Path(sys.argv[1] if len(sys.argv) > 1 else "web/public")
    built = out / "404" / "index.html"
    if not built.exists():
        print("notfound: web/content/404.md did not build", file=sys.stderr)
        return 1

    html = built.read_text(encoding="utf-8")

    relative = sorted(set(RELATIVE.findall(html)))
    if relative:
        print("notfound: the 404 page references these relatively, and it is "
              "served from every address:", file=sys.stderr)
        for r in relative:
            print(f"  {r}", file=sys.stderr)
        return 1

    # The copy at /404/ returns 200 and would be indexed on its own merits,
    # which is not what a page saying "that page is not here" should rank for.
    # The one served as /404.html carries the status and needs no help, but it
    # costs nothing to mark both.
    marked = re.sub(r'<meta name="robots" content="[^"]*"',
                    '<meta name="robots" content="noindex, follow"', html, count=1)

    (out / "404.html").write_text(marked, encoding="utf-8")
    built.write_text(marked, encoding="utf-8")

    # A page that says nothing is here does not belong in the sitemap.
    sitemap = out / "sitemap.xml"
    if sitemap.exists():
        xml = sitemap.read_text(encoding="utf-8")
        pruned = re.sub(r"\s*<url>(?:(?!</url>).)*?<loc>[^<]*/404/</loc>.*?</url>",
                        "", xml, flags=re.S)
        if pruned != xml:
            sitemap.write_text(pruned, encoding="utf-8")

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
