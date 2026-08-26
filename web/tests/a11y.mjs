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

const PAGES = ['', 'solutions/', 'innovation/', 'news/', 'about/', 'workspace/',
  'playground/', 'messages/', 'deadline/', 'docs/', 'faq/', 'conformance/',
  'contact/', 'messages/pacs.008.001.13/'];

const browser = await puppeteer.launch({
  executablePath: CHROME, headless: 'new',
  args: ['--no-sandbox', '--disable-gpu'] });

let violations = 0;
let checked = 0;

console.log('axe-core, WCAG 2.2 AA — every system preference and theme choice\n');

for (const p of PAGES) {
  for (const [system, theme] of COMBINATIONS) {
    const page = await browser.newPage();
    await page.setViewport({ width: 1280, height: 900 });
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
        resultTypes: ['violations'],
      });
    });

    checked++;
    if (result.violations.length) {
      violations += result.violations.length;
      console.log(`  FAIL  /${p} (system ${system}, chose ${theme || 'nothing'})`);
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
      console.log(`  ok    /${p} (system ${system}, chose ${theme || 'nothing'})`);
    }
    await page.close();
  }
}

await browser.close();
console.log(violations === 0
  ? `\n${checked} page renders audited, no WCAG 2.2 AA violations`
  : `\n${violations} violation(s) across ${checked} page renders`);
process.exit(violations === 0 ? 0 : 1);
