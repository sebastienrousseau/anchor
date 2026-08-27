// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// WCAG 1.4.10 Reflow: no page scrolls sideways at 320 CSS pixels.
//
// The axe suite runs every page at 1280x900, which is where a desktop layout is
// at its most comfortable and where nothing to do with reflow can possibly go
// wrong. Three of the AA criteria only bite at a narrow width — 1.4.10 Reflow,
// 1.4.4 Resize text, 2.5.8 Target Size — so a suite that never leaves 1280
// reports a clean pass on a page a phone cannot read. That is how a `select`
// sized to "pacs.008 — FI to FI Customer Credit Transfer" and an unbreakable
// install command shipped: 353px and 486px of content in a 280px column, and
// the playground scrolled sideways on every phone.
//
// 320px is the figure the criterion names: 1280 CSS pixels at 400% zoom.
//
// What counts as a failure is the document scrolling, not an element being
// wider than the screen. A table or a fenced code block inside its own
// `overflow-x: auto` box is the correct way to present wide content — it
// scrolls in place and the page does not move. So the check is on the document,
// and the elements it then names as culprits exclude anything sitting inside a
// scroll container, which would otherwise be reported on every page that
// handles wide content properly.
//
//   make reflow
//
// Needs puppeteer-core and a Chrome; the make target skips rather than fails
// when they are absent.

import puppeteer from 'puppeteer-core';

const BASE = process.env.ASKISO_BASE_URL || 'http://127.0.0.1:8899';
const CHROME = process.env.CHROME_PATH
  || '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome';

const PAGES = ['', 'solutions/', 'innovation/', 'news/', 'about/', 'vision/',
  'knowledge/', 'workspace/', 'playground/', 'messages/', 'deadline/', 'docs/',
  'faq/', 'conformance/', 'contact/', 'legal/', '404/', 'messages/pacs.008.001.13/'];

// 320 is the criterion. 360 and 412 are the two widths most phones actually
// report, and a layout can pass at 320 by collapsing to a single column and
// still break at 412 where it has not collapsed yet.
const WIDTHS = [320, 360, 412];

const browser = await puppeteer.launch({
  executablePath: CHROME,
  headless: 'new',
  args: ['--no-sandbox', '--disable-gpu'],
});

let failures = 0;
let checked = 0;

for (const pg of PAGES) {
  for (const width of WIDTHS) {
    const page = await browser.newPage();
    await page.setViewport({ width, height: 720, isMobile: true, hasTouch: true });
    await page.goto(`${BASE}/${pg}`, { waitUntil: 'networkidle0' });
    // Let the terminal replay and the countdown settle: both replace text, and
    // a measurement taken first would miss what the replacement does.
    await new Promise(r => setTimeout(r, 600));

    const result = await page.evaluate(() => {
      const de = document.documentElement;
      // One pixel of slack: a fractional layout width rounds up and reports a
      // scrollWidth a pixel past the client width on a page nothing overflows.
      const overflow = de.scrollWidth - de.clientWidth;
      if (overflow <= 1) return { overflow };

      const scrolls = (el) => {
        for (let e = el.parentElement; e; e = e.parentElement) {
          const o = getComputedStyle(e).overflowX;
          if (o === 'auto' || o === 'scroll' || o === 'hidden' || o === 'clip') return true;
        }
        return false;
      };

      const culprits = [];
      for (const el of document.querySelectorAll('body *')) {
        const r = el.getBoundingClientRect();
        if (r.width <= 0 || r.right <= de.clientWidth + 1) continue;
        if (getComputedStyle(el).position === 'fixed') continue;
        if (scrolls(el)) continue;   // wide content held in its own scroller
        const cls = typeof el.className === 'string' && el.className
          ? `.${el.className.trim().split(/\s+/).join('.')}` : '';
        culprits.push(`${el.tagName.toLowerCase()}${el.id ? `#${el.id}` : ''}${cls}`
          + ` right=${Math.round(r.right)} "${(el.textContent || '').trim().replace(/\s+/g, ' ').slice(0, 34)}"`);
      }
      return { overflow, culprits: [...new Set(culprits)].slice(0, 5) };
    });

    checked++;
    if (result.overflow > 1) {
      failures++;
      console.log(`  FAIL  /${pg} at ${width}px — the page scrolls ${result.overflow}px sideways`);
      for (const c of result.culprits) console.log(`          ${c}`);
    } else {
      console.log(`  ok    /${pg} at ${width}px`);
    }
    await page.close();
  }
}

await browser.close();
console.log(failures === 0
  ? `\n${checked} renders at 320, 360 and 412px, no page scrolls sideways`
  : `\n${failures} of ${checked} renders scroll sideways`);
process.exit(failures === 0 ? 0 : 1);
