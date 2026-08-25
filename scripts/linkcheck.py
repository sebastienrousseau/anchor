#!/usr/bin/env python3
"""Check every internal link in the built site resolves to a real file.

A static site has no runtime to notice a dead link, and with 2,845 generated
pages a single bad reference in a shared layout is a bad reference 2,845 times.
This found exactly that: four leftover theme footer links, a favicon that was
referenced but never shipped, and a stylesheet ssg fingerprints while emitting
the page that asks for the bare name — six targets, every page, invisible.

External links are not fetched: this is a build gate, not a crawler, and a
network round trip per link would make it too slow to run on every commit.
"""
import os, re, sys
from html.parser import HTMLParser
from urllib.parse import urldefrag, urlparse

ROOT = sys.argv[1] if len(sys.argv) > 1 else "web/public"

class Links(HTMLParser):
    def __init__(self):
        super().__init__(); self.refs = []
    def handle_starttag(self, tag, attrs):
        a = dict(attrs)
        for key in ("href", "src"):
            if key in a and a[key]:
                self.refs.append(a[key])

def resolves(target, page_dir):
    t = urldefrag(target)[0]
    if not t or t.startswith(("http://", "https://", "mailto:", "data:", "tel:", "#", "//")):
        return True
    path = os.path.join(ROOT, t.lstrip("/")) if t.startswith("/") else os.path.normpath(os.path.join(page_dir, t))
    if os.path.isfile(path):
        return True
    if os.path.isdir(path) and os.path.isfile(os.path.join(path, "index.html")):
        return True
    return os.path.isfile(path + ".html")

broken, checked, pages = {}, 0, 0
for dirpath, _, files in os.walk(ROOT):
    for f in files:
        if not f.endswith(".html"):
            continue
        pages += 1
        full = os.path.join(dirpath, f)
        p = Links()
        try:
            p.feed(open(full, encoding="utf-8", errors="replace").read())
        except Exception as e:
            broken.setdefault(f"PARSE {full}", set()).add(str(e)); continue
        for ref in p.refs:
            checked += 1
            if not resolves(ref, dirpath):
                broken.setdefault(ref, set()).add(os.path.relpath(full, ROOT))

print(f"pages {pages}, references {checked}, broken targets {len(broken)}")
for target, where in sorted(broken.items())[:25]:
    ex = sorted(where)[:2]
    print(f"  {target}   ({len(where)} page(s), e.g. {', '.join(ex)})")
sys.exit(1 if broken else 0)
