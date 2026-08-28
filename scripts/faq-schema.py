#!/usr/bin/env python3
# SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
# SPDX-License-Identifier: Apache-2.0 OR MIT
"""Add FAQPage structured data to every page that asks and answers questions.

Search engines show a question-and-answer page as an expandable result only when
the questions are marked up. The FAQ declared `schema: "faq"` in its front
matter, but nothing acted on it, so the markup was never emitted.

The questions are read back out of the built page rather than kept in a second
list. A hand-maintained copy of the same questions would drift from the page the
moment somebody edited one, and structured data that disagrees with the visible
text is worse than none: it is grounds for a manual penalty.

The 2,845 message pages each carry a Questions section — what business area this
message belongs to, how to validate one, whether the schema ships with the tool —
and those answers are exactly what an assistant lifts when somebody asks about a
message identifier. Unmarked, they were prose an engine had to guess at; marked,
they are a question with an accepted answer attached.

    python3 scripts/faq-schema.py web/public/faq/index.html
    python3 scripts/faq-schema.py web/public/messages          # every page under it
"""

from __future__ import annotations

import html
import json
import re
import sys
from pathlib import Path


def text_of(markup: str) -> str:
    """Visible text of an HTML fragment, collapsed to one line."""
    markup = re.sub(r"<(script|style)\b.*?</\1>", " ", markup, flags=re.S | re.I)
    markup = re.sub(r"<[^>]+>", " ", markup)
    return re.sub(r"\s+", " ", html.unescape(markup)).strip()


def extract(page: str) -> list[tuple[str, str]]:
    """Pair each question heading with the content up to the next heading."""
    pairs = []
    # Questions are h3; the h2s above them are section titles, not questions.
    for m in re.finditer(r"<h3\b[^>]*>(.*?)</h3>(.*?)(?=<h[23]\b|</main>)",
                         page, re.S | re.I):
        question = text_of(m.group(1))
        answer = text_of(m.group(2))
        if question and answer:
            pairs.append((question, answer))
    return pairs


def mark_one(path: Path) -> int:
    """Mark a single page, returning the number of questions written."""
    page = path.read_text(encoding="utf-8")
    if "FAQPage" in page or "</head>" not in page:
        return 0
    pairs = extract(page)
    if not pairs:
        return 0
    payload = {
        "@context": "https://schema.org",
        "@type": "FAQPage",
        "mainEntity": [
            {
                "@type": "Question",
                "name": q,
                "acceptedAnswer": {"@type": "Answer", "text": a},
            }
            for q, a in pairs
        ],
    }
    block = ('<script type="application/ld+json">'
             + json.dumps(payload, ensure_ascii=False)
             + "</script>")
    path.write_text(page.replace("</head>", block + "</head>", 1), encoding="utf-8")
    return len(pairs)


def mark_directory(root: Path) -> int:
    """Mark every page beneath a directory that has questions to mark.

    A page without them is skipped rather than failing the build: the index at
    /messages/ is a list, not a question, and there is nothing wrong with that.
    """
    marked = questions = 0
    for page in sorted(root.rglob("index.html")):
        n = mark_one(page)
        if n:
            marked += 1
            questions += n
    if marked == 0:
        print(f"faq-schema: no page under {root} carried questions", file=sys.stderr)
        return 1
    print(f"FAQPage markup added: {marked} page(s), {questions} question(s)")
    return 0


def main() -> int:
    if len(sys.argv) < 2:
        print("usage: faq-schema.py <built-faq-page.html>", file=sys.stderr)
        return 2

    target = Path(sys.argv[1])
    if target.is_dir():
        return mark_directory(target)

    if not target.is_file():
        print(f"no such file: {target}", file=sys.stderr)
        return 2
    path = target

    page = path.read_text(encoding="utf-8")
    pairs = extract(page)
    if not pairs:
        print("no questions found; refusing to emit empty FAQPage markup",
              file=sys.stderr)
        return 1

    payload = {
        "@context": "https://schema.org",
        "@type": "FAQPage",
        "mainEntity": [
            {
                "@type": "Question",
                "name": q,
                "acceptedAnswer": {"@type": "Answer", "text": a},
            }
            for q, a in pairs
        ],
    }

    block = ('<script type="application/ld+json">'
             + json.dumps(payload, ensure_ascii=False)
             + "</script>")

    if "FAQPage" in page:
        print("FAQPage markup is already present; nothing to do")
        return 0

    if "</head>" not in page:
        print("no </head> in the page", file=sys.stderr)
        return 1

    path.write_text(page.replace("</head>", block + "</head>", 1), encoding="utf-8")
    print(f"FAQPage markup added: {len(pairs)} question(s)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
