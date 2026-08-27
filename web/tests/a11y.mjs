// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// WCAG 2.2 AA audit with axe-core, in a real browser.
//
// The generator ships an accessibility checker and it runs on every build, but
// it reads the markup. A whole class of AA failure only exists once the page is
// rendered: colour contrast depends on what the cascade actually resolved, focus
// order depends on layout, and a control hidden by CSS is a different thing from
// one hidden by an attribute. This drives the built pages in Chrome and asks
// axe-core, which is what WAVE and the other auditors are built on.
//
// Every combination of what the system asks for and what the visitor chose.
// Those are independent: a stylesheet that switches on prefers-color-scheme
// alone is correct until somebody on a light system chooses dark, at which
// point its colours and the site's disagree. That combination shipped a
// contrast ratio of 1.04 on code blocks, and this test missed it because it
// only ever set the theme attribute — inheriting whatever the machine running
// it happened to prefer.
//
//   make a11y-axe
//
// Needs puppeteer-core, axe-core and a Chrome; the make target skips rather
// than fails when they are absent.

import puppeteer from 'puppeteer-core';
import fs from 'node:fs';
import path from 'node:path';

const BASE = process.env.ASKISO_BASE_URL || 'http://127.0.0.1:8899';
const CHROME = process.env.CHROME_PATH
  || '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome';

const AXE = fs.readFileSync(
  path.join(process.cwd(), 'node_modules/axe-core/axe.min.js'), 'utf8');

// Every hand-written page, plus one generated message page as a proxy for the
// 2,845 that share its template.
// system preference, explicit choice (null means the visitor chose nothing).
const COMBINATIONS = [
  ['light', null], ['dark', null],
  ['light', 'light'], ['light', 'dark'],
  ['dark', 'light'], ['dark', 'dark'],
];

// Contrast does not care what size the window is, but several AA rules do —
// 2.5.8 Target Size above all, where two controls comfortably 24px apart on a
// desktop can end up adjacent once a row wraps. Running every theme at both
// sizes would double a suite that already takes minutes for no contrast
// benefit, so the phone width runs the two system preferences and no explicit
// choice, which is what the overwhelming majority of visitors are in.
const VIEWPORTS = [
  { label: 'desktop', width: 1280, height: 900, mobile: false, combinations: COMBINATIONS },
  { label: 'phone', width: 390, height: 780, mobile: true, combinations: COMBINATIONS.slice(0, 2) },
];

const PAGES = ['', 'solutions/', 'innovation/', 'news/', 'about/', 'vision/',
  'knowledge/', 'workspace/', 'playground/', 'messages/', 'deadline/', 'docs/',
  'faq/', 'conformance/', 'contact/', 'legal/', '404/', 'contact/sent/', 'messages/pacs.008.001.13/'];

// axe reports three outcomes, not two, and the third is where a real failure
// hid. A contrast ratio of exactly 1:1 is filed as "incomplete" rather than as
// a violation, because at 1:1 axe cannot tell text deliberately hidden behind
// its own background from a button whose label has vanished. This suite read
// only `violations`, so it called a nav CTA with an invisible label green.
//
// So incomplete results are failures too, with two exceptions that axe cannot
// decide and no cascade bug can hide behind:
//
//   - a background it could not resolve because a pseudo element paints it,
//     which is every heading over a banner scrim;
//   - an element whose content is only non-text characters, which is a table
//     cell holding an em dash.
//
// Anything else — above all a message quoting a ratio axe measured and then
// declined to rule on — fails. An unrecognised message fails as well, so a new
// class of deferral has to be looked at rather than passing unseen.
const DEFERRABLE = [
  /background color could not be determined due to a pseudo element/,
  /content contains only non-text characters/,
];

const undecidable = (node) => {
  const messages = (node.any || []).concat(node.all || [], node.none || [])
    .map(check => check.message || '')
    .filter(Boolean);
  // No message at all is not a reason to let it through.
  return messages.length > 0
    && messages.every(m => DEFERRABLE.some(rx => rx.test(m)));
};

const browser = await puppeteer.launch({
  executablePath: CHROME, headless: 'new',
  args: ['--no-sandbox', '--disable-gpu'] });

let violations = 0;
let checked = 0;
let deferred = 0;

console.log('axe-core, WCAG 2.2 AA — every system preference and theme choice, desktop and phone\n');

for (const p of PAGES) {
 for (const vp of VIEWPORTS) {
  for (const [system, theme] of vp.combinations) {
    const page = await browser.newPage();
    await page.setViewport({ width: vp.width, height: vp.height,
      isMobile: vp.mobile, hasTouch: vp.mobile });
    await page.emulateMediaFeatures([
      { name: 'prefers-color-scheme', value: system },
    ]);
    await page.goto(`${BASE}/${p}`, { waitUntil: 'networkidle0' });
    if (theme) {
      await page.evaluate(t => document.documentElement.setAttribute('data-theme', t), theme);
    } else {
      await page.evaluate(() => document.documentElement.removeAttribute('data-theme'));
    }
    // Let the theme transition finish: contrast is measured from resolved
    // colours, and a value read mid-transition is neither theme's.
    await new Promise(r => setTimeout(r, 400));

    await page.evaluate(AXE);
    const result = await page.evaluate(async () => {
      return await window.axe.run(document, {
        runOnly: { type: 'tag', values: ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa'] },
        resultTypes: ['violations', 'incomplete'],
      });
    });

    checked++;

    // An incomplete axe could not decide is reported as a violation of its own,
    // carrying the rule that raised it and the node it names.
    const undecided = [];
    for (const inc of result.incomplete) {
      const bad = inc.nodes.filter(n => !undecidable(n));
      if (bad.length) {
        undecided.push({ ...inc, nodes: bad, help: `${inc.help} (axe could not rule)` });
      } else {
        deferred += inc.nodes.length;
      }
    }
    result.violations.push(...undecided);

    if (result.violations.length) {
      violations += result.violations.length;
      console.log(`  FAIL  /${p} ${vp.label} (system ${system}, chose ${theme || 'nothing'})`);
      for (const v of result.violations) {
        console.log(`          ${v.id} [${v.impact}] ${v.help}`);
        for (const n of v.nodes.slice(0, 3)) {
          console.log(`            ${n.target.join(' ')}`);
          if (n.failureSummary) {
            console.log(`            ${n.failureSummary.split('\n').join(' ').slice(0, 150)}`);
          }
        }
      }
    } else {
      console.log(`  ok    /${p} ${vp.label} (system ${system}, chose ${theme || 'nothing'})`);
    }
    await page.close();
  }
 }
}

await browser.close();
console.log(violations === 0
  ? `\n${checked} page renders audited, no WCAG 2.2 AA violations`
    + `\n${deferred} contrast check(s) axe left to a human: text over a banner`
    + ` scrim, and table cells holding only an em dash`
  : `\n${violations} violation(s) across ${checked} page renders`);
process.exit(violations === 0 ? 0 : 1);
