#!/usr/bin/env python3
# SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
# SPDX-License-Identifier: Apache-2.0 OR MIT
"""Score the readability of the built site.

Prose on a standards site drifts towards the standard's own register: long
sentences, nouns stacked on nouns, and a reader who gives up. This measures it
instead of arguing about it, using the two scores that have an agreed formula.

    Flesch Reading Ease  = 206.835 - 1.015 * (words/sentence) - 84.6 * (syllables/word)
    Flesch-Kincaid Grade = 0.39 * (words/sentence) + 11.8 * (syllables/word) - 15.59

What is scored, and what is not
-------------------------------
Each page is scored on its own prose: the hero lead and the body. Navigation,
the footer and the assurance band are identical on all 2,853 pages, so counting
them would measure the template rather than the page, and would drag every score
towards the same number.

Code blocks, inline code and tables are removed before scoring. `pacs.008.001.10`
is not a word, its full stops are not sentence endings, and leaving it in makes
the arithmetic meaningless.

Headings are scored as sentences of their own. They are read, so they count, but
they rarely end in a full stop and would otherwise be glued onto the sentence
that follows.

Usage
-----
    python3 scripts/readability.py web/public
    python3 scripts/readability.py web/public --min-ease 55 --max-ease 58 \
        --min-grade 8 --max-grade 9
"""

from __future__ import annotations

import argparse
import html
import json
import re
import sys
from pathlib import Path

# The generated message pages are reference cards, not prose: a heading, a table,
# four commands and four short answers. Flesch is defined over continuous prose,
# and on a page built mostly of fragments it measures the page's shape rather
# than its writing -- these average under nine words per sentence, where the two
# bands cannot both be satisfied at once whatever the vocabulary.
#
# They are reported rather than gated. Padding 2,845 near-identical pages with
# filler to move a number would make them worse to read and would risk being
# treated as thin duplicate content, which is the opposite of the intent.
REFERENCE = "messages/"

# Pages whose text is not prose a reader works through, so the scores would be
# noise. Each entry is a reason, not a convenience.
SKIP = {
    # The message index is a catalogue: 700 links and a table, by design.
    "messages/index.html",
}

# Abbreviations that end in a full stop without ending a sentence.
ABBREV = r"(?:e\.g|i\.e|etc|vs|Mr|Mrs|Ms|Dr|St|No|approx|Fig|cf|al)"


def strip_chrome(markup: str) -> str:
    """Reduce a page to the prose it is actually about."""
    # Everything outside <main> is site chrome. The layouts wrap page content in
    # <main>, so this is the page's own text and nothing repeated across the site.
    main = re.search(r"<main\b[^>]*>(.*?)</main>", markup, re.S | re.I)
    body = main.group(1) if main else markup

    # Remove what is not prose, in this order: script and style first, then the
    # structures whose text would be scored as sentences if left behind.
    for pattern in (
        r"<script\b.*?</script>",
        r"<style\b.*?</style>",
        r"<pre\b.*?</pre>",
        r"<code\b.*?</code>",
        r"<table\b.*?</table>",
        r"<nav\b.*?</nav>",
        r"<footer\b.*?</footer>",
        r"<svg\b.*?</svg>",
        # Skip-links and visually hidden text are for assistive technology, not
        # for reading order.
        r'<[^>]*class="[^"]*visually-hidden[^"]*"[^>]*>.*?</[a-z]+>',
    ):
        body = re.sub(pattern, " ", body, flags=re.S | re.I)

    # A heading is read as its own unit, so give it a terminator before the tags
    # are stripped and it merges into the paragraph beneath it.
    body = re.sub(r"</h[1-6]>", ". ", body, flags=re.I)
    # Same for list items and paragraphs that end without punctuation.
    body = re.sub(r"</li>", ". ", body, flags=re.I)
    body = re.sub(r"</p>", " ", body, flags=re.I)

    body = re.sub(r"<[^>]+>", " ", body)
    body = html.unescape(body)
    # Collapse whitespace and the doubled stops the substitutions above create.
    body = re.sub(r"\s+", " ", body)
    body = re.sub(r"\.\s*(?:\.\s*)+", ". ", body)
    return body.strip()


def strip_markdown(source: str) -> str:
    """Reduce a content file to its prose, for scoring a draft before a build.

    Authoring against the score is only practical if it can be checked in a
    second. This mirrors strip_chrome: front matter, fenced code, inline code
    and tables come out, and headings and list items become their own sentences.
    """
    # Front matter.
    source = re.sub(r"\A---\n.*?\n---\n", "", source, flags=re.S)
    # The lead and headline in front matter are read, so score them: they are
    # pulled back in by the caller rather than dropped here.
    source = re.sub(r"```.*?```", " ", source, flags=re.S)
    source = re.sub(r"`[^`]*`", " ", source)
    # Tables: any line that is mostly pipes.
    source = "\n".join(l for l in source.split("\n") if not l.strip().startswith("|"))
    # Links keep their text, drop their target.
    source = re.sub(r"\[([^\]]*)\]\([^)]*\)", r"\1", source)
    source = re.sub(r"<[^>]+>", " ", source)
    source = re.sub(r"[*_#>]+", " ", source)

    # Markdown prose is hard-wrapped, so a line is not a sentence. Only a blank
    # line ends a paragraph; within one, the lines are joined. Getting this
    # wrong made every paragraph look like a string of eight-word sentences,
    # which is the opposite of the problem being measured.
    blocks, current = [], []
    for line in source.split("\n"):
        stripped = line.strip()
        if not stripped:
            if current:
                blocks.append(" ".join(current))
                current = []
            continue
        item = re.match(r"^[-+*]\s+(.*)$", stripped)
        if item:
            # A list item is its own unit, however it wraps.
            if current:
                blocks.append(" ".join(current))
            current = [item.group(1)]
            continue
        current.append(stripped)
    if current:
        blocks.append(" ".join(current))

    out = []
    for block in blocks:
        block = block.strip()
        if not block or not re.search(r"[A-Za-z]", block):
            continue
        # A block ending in a colon introduces a code sample or a list. The
        # sample is removed before scoring, so without a terminator the lead-in
        # is glued to the paragraph after it and reads as one long sentence.
        block = re.sub(r":$", ".", block)
        if not re.search(r"[.!?]$", block):
            block += "."
        out.append(block)
    return re.sub(r"\s+", " ", " ".join(out)).strip()


def sentences(text: str) -> list[str]:
    """Split into sentences, without breaking on abbreviations or decimals."""
    protected = re.sub(rf"\b{ABBREV}\.", lambda m: m.group(0).replace(".", "\0"), text)
    # A full stop between digits is a decimal, not an ending.
    protected = re.sub(r"(?<=\d)\.(?=\d)", "\0", protected)
    parts = re.split(r"(?<=[.!?])[\s ]+", protected)
    out = []
    for part in parts:
        part = part.replace("\0", ".").strip()
        # A fragment with no letters is a stray bullet or a number, not a sentence.
        if part and re.search(r"[A-Za-z]", part):
            out.append(part)
    return out


def words(text: str) -> list[str]:
    """Words, in the sense the formulas mean: runs of letters."""
    return re.findall(r"[A-Za-z]+(?:'[A-Za-z]+)?", text)


def count_syllables(word: str) -> int:
    """Estimate syllables with the standard vowel-group heuristic.

    Every implementation of these scores estimates syllables, and they differ at
    the margins. This is the widely used form: vowel groups, a silent trailing
    -e, and a floor of one.
    """
    word = word.lower()
    word = re.sub(r"[^a-z]", "", word)
    if not word:
        return 0
    if len(word) <= 3:
        return 1
    word = re.sub(r"(?:[^laeiouy]es|[^laeiouy]e)$", "", word)
    word = re.sub(r"^y", "", word)
    groups = re.findall(r"[aeiouy]+", word)
    return max(1, len(groups))


def score(text: str) -> dict:
    sents = sentences(text)
    ws = words(text)
    if not sents or not ws:
        return {"words": len(ws), "sentences": len(sents), "ease": None, "grade": None}
    syl = sum(count_syllables(w) for w in ws)
    wps = len(ws) / len(sents)
    spw = syl / len(ws)
    return {
        "words": len(ws),
        "sentences": len(sents),
        "syllables": syl,
        "words_per_sentence": round(wps, 2),
        "syllables_per_word": round(spw, 3),
        "ease": round(206.835 - 1.015 * wps - 84.6 * spw, 1),
        "grade": round(0.39 * wps + 11.8 * spw - 15.59, 1),
    }


def self_test() -> None:
    """Check the arithmetic against a case small enough to work out by hand.

    Ten one-syllable words in two sentences: 5 words per sentence, 1 syllable per
    word. Reading ease 206.835 - 5.075 - 84.6 = 117.16; grade 1.95 + 11.8 -
    15.59 = -1.84. If either drifts, the formulas have been mistyped.
    """
    s = score("The cat sat on the mat. The dog ran fast.")
    assert s["words"] == 10, s
    assert s["sentences"] == 2, s
    assert s["syllables"] == 10, s
    assert abs(s["ease"] - 117.2) < 0.15, s
    assert abs(s["grade"] - (-1.8)) < 0.15, s

    # Abbreviations must not split a sentence.
    assert len(sentences("Use pacs.008, e.g. version 13, in CBPR+ flows.")) == 1
    # Decimals must not split one either.
    assert len(sentences("The amount was 1000.50 in total.")) == 1
    # Two real sentences still split.
    assert len(sentences("First one here. Second one here.")) == 2


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("root", nargs="?", default="web/public")
    ap.add_argument("--min-ease", type=float, default=55.0)
    ap.add_argument("--max-ease", type=float, default=58.0)
    ap.add_argument("--min-grade", type=float, default=8.0)
    ap.add_argument("--max-grade", type=float, default=9.0)
    ap.add_argument("--min-words", type=int, default=120,
                    help="pages shorter than this are reported but not gated: "
                         "the scores are unstable on very little text")
    ap.add_argument("--only", action="append", default=None,
                    help="score just these paths, relative to root")
    ap.add_argument("--json", action="store_true")
    ap.add_argument("--explain", action="store_true",
                    help="with --markdown, list the sentences and words pulling "
                         "a page away from the band, so an edit can be aimed")
    ap.add_argument("--markdown", action="append", default=None,
                    help="score content source files instead of built pages, "
                         "so a draft can be checked without a site build")
    args = ap.parse_args()

    self_test()

    root = Path(args.root)
    if not root.is_dir():
        print(f"no such directory: {root}", file=sys.stderr)
        return 2

    if args.markdown:
        for src in args.markdown:
            path = Path(src)
            raw = path.read_text(encoding="utf-8", errors="replace")
            # The headline and lead sit in front matter but are the first thing
            # read on the page, so they are scored with the body.
            front = re.match(r"\A---\n(.*?)\n---\n", raw, re.S)
            intro = ""
            if front:
                for key in ("headline", "lead"):
                    m = re.search(rf'^{key}:\s*"(.*)"\s*$', front.group(1), re.M)
                    if m:
                        intro += m.group(1).rstrip(".") + ". "
            text = intro + strip_markdown(raw)
            s = score(text)
            s["page"] = src
            flag = "" if args.min_ease <= s["ease"] <= args.max_ease else "  <-- outside"
            print(f"{src:<40}ease {s['ease']:>6}  grade {s['grade']:>5}  "
                  f"w/sent {s['words_per_sentence']:>6}  words {s['words']:>5}{flag}")

            if args.explain:
                sents = sentences(text)
                longest = sorted(sents, key=lambda x: -len(words(x)))[:6]
                print("  longest sentences:")
                for sent in longest:
                    print(f"    {len(words(sent)):>3}w  {sent[:96]}")
                heavy = {}
                for w in words(text):
                    n = count_syllables(w)
                    if n >= 4:
                        heavy[w.lower()] = n
                if heavy:
                    top = sorted(heavy.items(), key=lambda kv: -kv[1])[:12]
                    print("  longest words: " + ", ".join(f"{w}({n})" for w, n in top))
                print()
        return 0

    if args.only:
        targets = [root / p for p in args.only]
    else:
        targets = sorted(root.rglob("index.html"))

    rows, failures, short, reference = [], [], [], []
    for path in targets:
        rel = str(path.relative_to(root))
        if rel in SKIP:
            continue
        text = strip_chrome(path.read_text(encoding="utf-8", errors="replace"))
        s = score(text)
        s["page"] = "/" + rel[: -len("index.html")]
        rows.append(s)

        if rel.startswith(REFERENCE):
            reference.append(s)
            continue

        if s["ease"] is None or s["words"] < args.min_words:
            short.append(s)
            continue
        if not (args.min_ease <= s["ease"] <= args.max_ease):
            failures.append((s, f"reading ease {s['ease']} outside {args.min_ease}-{args.max_ease}"))
        elif not (args.min_grade <= s["grade"] <= args.max_grade):
            failures.append((s, f"grade {s['grade']} outside {args.min_grade}-{args.max_grade}"))

    if args.json:
        print(json.dumps(rows, indent=1))
        return 1 if failures else 0

    gated = len(rows) - len(short) - len(reference)
    print(f"{len(rows)} page(s) scored, {gated} gated, {len(short)} too short "
          f"to gate, {len(reference)} reference page(s) reported only\n")
    print(f"{'page':<44}{'ease':>7}{'grade':>7}{'w/sent':>8}{'words':>8}")
    for s in sorted(rows, key=lambda r: (r["ease"] is None, r["ease"] or 0)):
        if s["words"] < args.min_words or s in reference:
            continue
        print(f"{s['page']:<44}{s['ease']:>7}{s['grade']:>7}"
              f"{s['words_per_sentence']:>8}{s['words']:>8}")

    if reference:
        eases = sorted(r["ease"] for r in reference if r["ease"] is not None)
        grades = sorted(r["grade"] for r in reference if r["grade"] is not None)
        if eases:
            mid = len(eases) // 2
            print(f"\n{len(reference)} reference page(s), reported not gated: "
                  f"ease {eases[0]}-{eases[-1]} (median {eases[mid]}), "
                  f"grade {grades[0]}-{grades[-1]} (median {grades[mid]})")

    if failures:
        print(f"\n{len(failures)} page(s) outside the band:")
        for s, why in failures:
            print(f"  {s['page']}: {why}")
        return 1

    print("\nevery gated page is inside the band")
    return 0


if __name__ == "__main__":
    sys.exit(main())
