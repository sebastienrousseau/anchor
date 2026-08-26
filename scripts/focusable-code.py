#!/usr/bin/env python3
# SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
# SPDX-License-Identifier: Apache-2.0 OR MIT
"""Make syntax-highlighted code blocks reachable by keyboard.

A fenced block cannot reflow — its lines have a fixed width — so it scrolls
inside its own box rather than widening the page. That makes it a scroll region,
and WCAG 2.1.1 requires a scroll region to be operable from a keyboard: without
a tab stop, a reader who does not use a mouse cannot see the right-hand end of
any line that overflows.

The generator emits these blocks, so the attribute is added to the built pages
rather than to a template. Blocks that terminal.js replaces are left alone: it
removes them, and the terminal it builds carries its own transcript.

    python3 scripts/focusable-code.py web/public
"""

from __future__ import annotations

import re
import sys
from pathlib import Path

# The languages terminal.js takes over. Their <pre> never survives to be seen.
REPLACED = ("language-bash", "language-console")

BLOCK = re.compile(r'<pre class="([^"]*\bhighlight\b[^"]*)"')


def main() -> int:
    root = Path(sys.argv[1] if len(sys.argv) > 1 else "web/public")
    if not root.is_dir():
        print(f"no such directory: {root}", file=sys.stderr)
        return 2

    def add(m: re.Match) -> str:
        classes = m.group(1)
        if any(lang in classes for lang in REPLACED):
            return m.group(0)
        # role and label so it is announced as something to move into, rather
        # than as an unexplained tab stop.
        return (f'<pre class="{classes}" tabindex="0" role="region" '
                f'aria-label="Code block, scrollable"')

    touched = blocks = 0
    for page in root.rglob("*.html"):
        html = page.read_text(encoding="utf-8")
        new_html, n = BLOCK.subn(add, html)
        if n and new_html != html:
            page.write_text(new_html, encoding="utf-8")
            touched += 1
            blocks += n

    print(f"code blocks: {blocks} made focusable across {touched} page(s)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
