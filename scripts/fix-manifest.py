#!/usr/bin/env python3
# SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
# SPDX-License-Identifier: Apache-2.0 OR MIT
"""Complete the web app manifest the generator leaves partly filled.

Two fields it does not populate:

theme_color was emitted as null. A manifest with a null there is not merely
incomplete — browsers reject the property and log "type string expected" to the
console on every page load, which is a console error on 2,859 pages and four
points off the Lighthouse best-practices score.

start_url was ".", which resolves against the manifest's own location. It works
by accident here and is wrong in principle: an installed app should open at the
site root regardless of which page it was installed from.

    python3 scripts/fix-manifest.py web/public/manifest.json
"""

from __future__ import annotations

import json
import sys
from pathlib import Path

# The light theme's ground. An installed app opens to it, and the pages already
# declare the same value in their theme-color meta for the light scheme.
THEME_COLOR = "#ffffff"


def main() -> int:
    path = Path(sys.argv[1] if len(sys.argv) > 1 else "web/public/manifest.json")
    if not path.is_file():
        print(f"no manifest at {path}", file=sys.stderr)
        return 1

    manifest = json.loads(path.read_text(encoding="utf-8"))
    changed = []

    if not isinstance(manifest.get("theme_color"), str):
        manifest["theme_color"] = THEME_COLOR
        changed.append("theme_color")

    if manifest.get("start_url") != "/":
        manifest["start_url"] = "/"
        changed.append("start_url")

    if changed:
        path.write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n",
                        encoding="utf-8")

    print(f"manifest: {', '.join(changed) if changed else 'already complete'}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
