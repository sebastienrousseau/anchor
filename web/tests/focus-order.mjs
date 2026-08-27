// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// WCAG 2.4.3 Focus Order: what looks first should not be reached last.
//
// This is the class of failure no automated checker reports, because focus
// order cannot be decided from markup alone — a sequence that reads oddly on
// one page is the only sensible one on another. axe therefore passes every page
// here, and passed the one that prompted this: the generator appends its search
// trigger near the end of the body and fixes it to the viewport, so it painted
// at the top right of the header and answered the keyboard at tab stop 53 of
// 53, after the whole footer. Reaching the search on a 2,861-page site meant
// tabbing through an entire page first.
//
// What is checkable is the disagreement between the two orders. An element
// painted in the top band of the first screen, which nothing reaches until the
// back half of the tab sequence, is visually early and operationally last —
// the shape of the failure, and specific enough not to fire on a page whose
// order is merely unusual.
//
// Deliberately narrow. It does not require the tab order to match reading order
// generally: a skip link belongs first though it is invisible, and a card's
// action sits below its heading in both orders anyway. It only objects when the
// two disagree at the extremes.
//
//   make focus-order
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

// Painted within this many pixels of the top of the first screen counts as
// visually early; the header and its controls sit well inside it.
const HEADER_BAND = 140;

// Reached after this share of the tab sequence counts as operationally late.
const LATE_FRACTION = 0.5;

const browser = await puppeteer.launch({
  executablePath: CHROME,
  headless: 'new',
  args: ['--no-sandbox', '--disable-gpu'],
});

let failures = 0;
let checked = 0;

for (const pg of PAGES) {
  const page = await browser.newPage();
  await page.setViewport({ width: 1280, height: 900 });
  await page.goto(`${BASE}/${pg}`, { waitUntil: 'networkidle0' });
  await new Promise(r => setTimeout(r, 500));

  // Walk the tab sequence once, stopping when it wraps to where it began.
  const sequence = [];
  for (let i = 0; i < 400; i++) {
    await page.keyboard.press('Tab');
    const stop = await page.evaluate(() => {
      const el = document.activeElement;
      if (!el || el === document.body) return null;
      const r = el.getBoundingClientRect();
      const label = (el.getAttribute('aria-label') || el.textContent || '')
        .trim().replace(/\s+/g, ' ').slice(0, 30);
      return {
        key: `${el.tagName}#${el.id}.${el.className}|${label}`,
        id: el.id, tag: el.tagName.toLowerCase(), label,
        top: Math.round(r.top + window.scrollY),
        // A control fixed to the viewport is painted where it sits now,
        // whatever the page has scrolled to.
        fixed: getComputedStyle(el).position === 'fixed',
        viewportTop: Math.round(r.top),
      };
    });
    if (stop === null) break;
    if (sequence.length && stop.key === sequence[0].key) break;
    sequence.push(stop);
  }

  checked++;
  if (sequence.length < 3) {
    console.log(`  FAIL  /${pg} — only ${sequence.length} tab stop(s); the page is not keyboard operable`);
    failures++;
    await page.close();
    continue;
  }

  const lateFrom = Math.ceil(sequence.length * LATE_FRACTION);
  const offenders = sequence
    .map((s, i) => ({ ...s, position: i + 1 }))
    .filter(s => s.position > lateFrom)
    .filter(s => (s.fixed ? s.viewportTop : s.top) <= HEADER_BAND);

  if (offenders.length) {
    failures++;
    console.log(`  FAIL  /${pg}`);
    for (const o of offenders) {
      console.log(`          ${o.tag}${o.id ? `#${o.id}` : ''} "${o.label}" is painted `
        + `${o.fixed ? o.viewportTop : o.top}px from the top but is tab stop `
        + `${o.position} of ${sequence.length}`);
    }
  } else {
    console.log(`  ok    /${pg} — ${sequence.length} tab stops, nothing visually first is reached last`);
  }
  await page.close();
}

await browser.close();
console.log(failures === 0
  ? `\n${checked} page(s) walked, focus order agrees with what is painted`
  : `\n${failures} of ${checked} page(s) disagree`);
process.exit(failures === 0 ? 0 : 1);
