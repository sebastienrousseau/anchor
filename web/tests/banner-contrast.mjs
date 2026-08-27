// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Contrast of text set over a photograph, measured from the pixels.
//
// axe-core cannot judge this case. Where a banner's ground is an image behind a
// gradient scrim, axe reports "background color could not be determined due to
// a pseudo element" and files the result as incomplete — a deferral to a human,
// not a pass. The a11y suite treats those deferrals as one of the two classes
// it knows to be undecidable, and this is the one that then goes unmeasured.
// Three hundred of them accumulate on a build, so they are worth measuring
// rather than trusting.
//
// The method is the direct one: hide the banner's text, screenshot what remains
// exactly as the browser painted it — image, scrim and all — then for every
// line box compare the text colour against every pixel it covers and keep the
// worst. That is a lower bound on the true ratio, because the darkest pixel
// under a line box is at least as bad as the darkest pixel under a stroke.
//
// Two exclusions, both measurement artefacts rather than passes:
//
//   - an element painting its own opaque background, which is every filled
//     button. Hiding it removes the fill too, so what would be measured is the
//     photograph behind a button, a surface no glyph is ever set on. axe
//     resolves these correctly and does not defer on them.
//   - text that is not opaque, where what the eye sees is a composite rather
//     than the element's own colour.
//
//   make banner-contrast
//
// Needs puppeteer-core and a Chrome; the make target skips rather than fails
// when they are absent.

import puppeteer from 'puppeteer-core';
import zlib from 'node:zlib';

const BASE = process.env.ASKISO_BASE_URL || 'http://127.0.0.1:8899';
const CHROME = process.env.CHROME_PATH
  || '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome';

const PAGES = ['', 'solutions/', 'innovation/', 'news/', 'about/', 'vision/',
  'knowledge/', 'workspace/', 'playground/', 'messages/', 'deadline/', 'docs/',
  'faq/', 'conformance/', 'contact/', 'legal/', 'messages/pacs.008.001.13/'];

const COMBINATIONS = [
  ['light', null], ['dark', null],
  ['light', 'light'], ['light', 'dark'],
  ['dark', 'light'], ['dark', 'dark'],
];

// --- PNG, the subset Chrome emits: 8-bit truecolour, non-interlaced ---------

function decodePNG(buf) {
  let pos = 8;
  const idat = [];
  let ihdr = null;
  while (pos < buf.length) {
    const len = buf.readUInt32BE(pos);
    const type = buf.toString('ascii', pos + 4, pos + 8);
    if (type === 'IHDR') {
      ihdr = {
        w: buf.readUInt32BE(pos + 8), h: buf.readUInt32BE(pos + 12),
        depth: buf[pos + 16], colour: buf[pos + 17], interlace: buf[pos + 20],
      };
    } else if (type === 'IDAT') {
      idat.push(buf.subarray(pos + 8, pos + 8 + len));
    } else if (type === 'IEND') {
      break;
    }
    pos += 12 + len;
  }
  if (ihdr.depth !== 8 || ihdr.interlace !== 0) {
    throw new Error(`unsupported PNG: depth ${ihdr.depth}, interlace ${ihdr.interlace}`);
  }
  const nch = { 0: 1, 2: 3, 4: 2, 6: 4 }[ihdr.colour];
  const raw = zlib.inflateSync(Buffer.concat(idat));
  const { w, h } = ihdr;
  const stride = w * nch;
  const out = Buffer.alloc(h * stride);
  let prev = Buffer.alloc(stride);
  let p = 0;
  for (let y = 0; y < h; y++) {
    const ft = raw[p++];
    const line = Buffer.from(raw.subarray(p, p + stride));
    p += stride;
    for (let i = 0; i < stride; i++) {
      const a = i >= nch ? line[i - nch] : 0;
      const b = prev[i];
      const c = i >= nch ? prev[i - nch] : 0;
      if (ft === 1) {
        line[i] = (line[i] + a) & 255;
      } else if (ft === 2) {
        line[i] = (line[i] + b) & 255;
      } else if (ft === 3) {
        line[i] = (line[i] + ((a + b) >> 1)) & 255;
      } else if (ft === 4) {
        const pp = a + b - c;
        const pa = Math.abs(pp - a);
        const pb = Math.abs(pp - b);
        const pc = Math.abs(pp - c);
        line[i] = (line[i] + (pa <= pb && pa <= pc ? a : pb <= pc ? b : c)) & 255;
      }
    }
    line.copy(out, y * stride);
    prev = line;
  }
  return { w, h, nch, px: out };
}

const channel = (v) => {
  const n = v / 255;
  return n <= 0.04045 ? n / 12.92 : ((n + 0.055) / 1.055) ** 2.4;
};
const luminance = (r, g, b) => 0.2126 * channel(r) + 0.7152 * channel(g) + 0.0722 * channel(b);
const ratio = (a, b) => (Math.max(a, b) + 0.05) / (Math.min(a, b) + 0.05);
const rgba = s => (s.match(/[\d.]+/g) || []).map(Number);

// --- measure ---------------------------------------------------------------

const browser = await puppeteer.launch({
  executablePath: CHROME,
  headless: 'new',
  args: ['--no-sandbox', '--disable-gpu'],
});

const worst = new Map();
let measured = 0;

for (const pg of PAGES) {
  for (const [system, theme] of COMBINATIONS) {
    const page = await browser.newPage();
    await page.setViewport({ width: 1280, height: 900, deviceScaleFactor: 1 });
    await page.emulateMediaFeatures([{ name: 'prefers-color-scheme', value: system }]);
    await page.goto(`${BASE}/${pg}`, { waitUntil: 'networkidle0' });
    await page.evaluate(t => (t
      ? document.documentElement.setAttribute('data-theme', t)
      : document.documentElement.removeAttribute('data-theme')), theme);
    await new Promise(r => setTimeout(r, 600));

    const banners = await page.evaluate(() => {
      // Only elements owning a text node directly. A wrapper whose text is its
      // children's concatenation has a box spanning all of them, and the
      // photograph under that box says nothing about any glyph.
      const owns = e => [...e.childNodes].some(n => n.nodeType === 3 && n.textContent.trim());
      const opaque = (e) => {
        const parts = (getComputedStyle(e).backgroundColor.match(/[\d.]+/g) || []).map(Number);
        return parts.length < 4 || parts[3] >= 0.99;
      };
      const out = [];
      document.querySelectorAll('.page-banner, .home-banner').forEach((hero, idx) => {
        const r = hero.getBoundingClientRect();
        if (r.height < 10) return;
        const texts = [...hero.querySelectorAll('*')]
          .filter(e => owns(e) && e.getClientRects().length && !opaque(e))
          .map((e) => {
            const s = getComputedStyle(e);
            // Glyph boxes, not the element box. A block-level eyebrow spans the
            // full width of its container however short its label is, and most
            // of that box is bare photograph the text never touches — on a
            // banner whose scrim fades to the right, measuring it reports a
            // failure against pixels no letter is set on. A Range over each
            // text node gives the boxes the glyphs actually occupy.
            const rects = [];
            for (const n of e.childNodes) {
              if (n.nodeType !== 3 || !n.textContent.trim()) continue;
              const range = document.createRange();
              range.selectNodeContents(n);
              for (const b of range.getClientRects()) {
                if (b.width >= 1 && b.height >= 1) {
                  rects.push({ x: b.x, y: b.y, w: b.width, h: b.height });
                }
              }
            }
            return {
              rects,
              text: e.textContent.trim().replace(/\s+/g, ' ').slice(0, 34),
              colour: s.color,
              size: parseFloat(s.fontSize),
              weight: s.fontWeight,
              sel: (typeof e.className === 'string' && e.className) || e.tagName.toLowerCase(),
            };
          })
          .filter(t => t.rects.length);
        if (texts.length) out.push({ idx, rect: { x: r.x, y: r.y, w: r.width, h: r.height }, texts });
      });
      return out;
    });
    if (!banners.length) {
      await page.close();
      continue;
    }

    await page.evaluate(() => {
      const owns = e => [...e.childNodes].some(n => n.nodeType === 3 && n.textContent.trim());
      document.querySelectorAll('.page-banner, .home-banner').forEach(h =>
        [...h.querySelectorAll('*')].filter(owns).forEach((e) => { e.style.visibility = 'hidden'; }));
    });
    await new Promise(r => setTimeout(r, 200));

    for (const hero of banners) {
      const cx = Math.max(0, hero.rect.x);
      const cy = Math.max(0, hero.rect.y);
      const shot = await page.screenshot({
        captureBeyondViewport: true,
        clip: { x: cx, y: cy, width: Math.min(1280 - cx, hero.rect.w), height: hero.rect.h },
      });
      const { w, h, nch, px } = decodePNG(Buffer.from(shot));

      for (const t of hero.texts) {
        const [r, g, b, alpha = 1] = rgba(t.colour);
        if (alpha < 0.99) continue;
        const tl = luminance(r, g, b);
        // WCAG 1.4.3: large text is 24px, or 18.66px at weight 700 or above.
        const large = t.size >= 24 || (t.size >= 18.66 && parseInt(t.weight, 10) >= 700);
        const need = large ? 3 : 4.5;

        let lo = Infinity;
        for (const rc of t.rects) {
          const x0 = Math.max(0, Math.round(rc.x - cx));
          const y0 = Math.max(0, Math.round(rc.y - cy));
          const x1 = Math.min(w, Math.round(rc.x - cx + rc.w));
          const y1 = Math.min(h, Math.round(rc.y - cy + rc.h));
          for (let yy = y0; yy < y1; yy++) {
            for (let xx = x0; xx < x1; xx++) {
              const o = yy * w * nch + xx * nch;
              const cr = ratio(tl, luminance(px[o], px[o + 1], px[o + 2]));
              if (cr < lo) lo = cr;
            }
          }
        }
        if (lo === Infinity) continue;
        measured++;
        const key = `${pg || '/'} ${t.sel} ${t.text}`;
        const rec = { page: pg || '/', sel: t.sel, text: t.text, lo, need, system, theme };
        if (!worst.has(key) || worst.get(key).lo > lo) worst.set(key, rec);
      }
    }
    await page.close();
  }
}
await browser.close();

const rows = [...worst.values()].sort((a, b) => a.lo - b.lo);
const fails = rows.filter(r => r.lo < r.need);

console.log('Banner text over photography, measured from the rendered pixels\n');
for (const r of rows) {
  const mark = r.lo < r.need ? 'FAIL' : 'ok  ';
  const where = r.lo < r.need ? `   (system ${r.system}, chose ${r.theme || 'nothing'})` : '';
  console.log(`  ${mark} ${r.lo.toFixed(2).padStart(6)} need ${r.need.toFixed(1)}  `
    + `${r.page.padEnd(22)} ${r.sel.slice(0, 18).padEnd(18)} ${r.text}${where}`);
}
console.log(fails.length === 0
  ? `\n${worst.size} banner text element(s) across ${measured} measurements, all at or above AA`
  : `\n${fails.length} of ${worst.size} banner text element(s) below AA`);
process.exit(fails.length === 0 ? 0 : 1);
