#!/usr/bin/env python3
# SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
# SPDX-License-Identifier: Apache-2.0 OR MIT
"""Every page that should be in the sitemap is, and nothing else is.

A page missing from the sitemap is a page a crawler has to find by following a
link to it, and with 2,845 message pages that is exactly the tail that does not
get found. So the count is checked rather than trusted.

Counting alone was what this did, in the workflow rather than here, and counting
alone has a blind spot: it compares two totals and is satisfied when they agree,
so a page dropped and a page wrongly added cancel out. It also had no way to
express a page that is deliberately absent. The 404 is one — it answers for
addresses that do not exist and is marked noindex, so listing it would invite a
crawler to index a page whose entire content is "that page is not here" — and
adding it broke the check the first time it ran.

Comparing the two sets rather than their sizes says which page, in which
direction, and lets the one deliberate exclusion be named as such.

    python3 scripts/check-sitemap.py web/public
"""

from __future__ import annotations

import re
import sys
from pathlib import Path

# Which pages must not appear is not a list kept here. A page marked noindex is
# one a crawler is asked not to index, and listing it in a sitemap asks the
# opposite; so the mark is read back out of the built page. scripts/noindex.py
# owns which pages carry it, and the two cannot drift apart.
NOINDEX = re.compile(r'<meta name="robots" content="[^"]*noindex', re.I)

# Below this, something went wrong with the build rather than with the sitemap.
MINIMUM_PAGES = 2000


def main() -> int:
    out = Path(sys.argv[1] if len(sys.argv) > 1 else "web/public")

    sitemap = out / "sitemap.xml"
    if not sitemap.exists():
        print("check-sitemap: no sitemap.xml was built", file=sys.stderr)
        return 1

    listed = set()
    for loc in re.findall(r"<loc>([^<]+)</loc>", sitemap.read_text(encoding="utf-8")):
        path = re.sub(r"^https?://[^/]+", "", loc.strip())
        listed.add(path or "/")

    built = set()
    excluded = set()
    for page in out.rglob("index.html"):
        rel = page.relative_to(out).parent.as_posix()
        path = "/" if rel == "." else f"/{rel}/"
        built.add(path)
        if NOINDEX.search(page.read_text(encoding="utf-8", errors="replace")):
            excluded.add(path)

    if len(built) < MINIMUM_PAGES:
        print(f"check-sitemap: only {len(built)} page(s) built", file=sys.stderr)
        return 1

    should_be_listed = built - excluded
    missing = sorted(should_be_listed - listed)
    extra = sorted(listed - should_be_listed)

    for path in missing[:10]:
        print(f"check-sitemap: {path} was built but is not in the sitemap", file=sys.stderr)
    for path in extra[:10]:
        reason = "is marked noindex" if path in excluded else "is not a built page"
        print(f"check-sitemap: {path} is in the sitemap but {reason}", file=sys.stderr)

    if missing or extra:
        print(f"check-sitemap: {len(missing)} missing, {len(extra)} unexpected",
              file=sys.stderr)
        return 1

    print(f"sitemap: {len(listed)} entries for {len(built)} built page(s)"
          + (f", excluding {', '.join(sorted(excluded))} as noindex" if excluded else ""))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
