// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Loads every page and fails on anything the browser complains about.
//
// Errors, warnings, failed requests and any response of 400 or above. Two real
// defects reached production because nothing watched this: a relative url() in
// an inlined stylesheet that 404ed the brand mark, and a null theme_color in the
// manifest that browsers reject on every page load. The first was an error; the
// second only a warning, which is why both are watched now.
//
//   make web-console

import puppeteer from 'puppeteer-core';

const BASE = process.env.ASKISO_BASE_URL || 'http://127.0.0.1:8899';
const CHROME = process.env.CHROME_PATH
  || '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome';
const browser = await puppeteer.launch({
  executablePath: CHROME,
  headless: 'new', args: ['--no-sandbox', '--disable-gpu'] });
let fail = 0;
for (const p of ['', 'solutions/', 'innovation/', 'news/', 'about/', 'faq/', 'docs/',
                 'deadline/', 'conformance/', 'contact/', 'workspace/', 'playground/',
                 'messages/', 'messages/pacs.008.001.13/']) {
  const page = await browser.newPage();
  const errs = [];
  // Warnings too. A null theme_color in the manifest is logged as a warning,
  // not an error, and went unnoticed on 2,859 pages because this only watched
  // for errors.
  page.on('console', m => {
    if (m.type() === 'error' || m.type() === 'warning') errs.push(`[${m.type()}] ` + m.text().slice(0, 110));
  });
  page.on('pageerror', e => errs.push('pageerror: ' + String(e).slice(0, 110)));
  page.on('requestfailed', r => errs.push('failed: ' + r.url().slice(-60)));
  page.on('response', r => { if (r.status() >= 400) errs.push(`${r.status()} ${r.url().slice(-60)}`); });
  await page.goto(`${BASE}/${p}`, { waitUntil: 'networkidle0' });
  await new Promise(r => setTimeout(r, 300));
  if (errs.length) { fail++; console.log(`  FAIL  /${p}`); errs.slice(0,3).forEach(e => console.log('          ' + e)); }
  else console.log(`  ok    /${p}`);
  await page.close();
}
await browser.close();
console.log(fail === 0 ? '\nno console errors or failed requests on any page' : `\n${fail} page(s) with errors`);
process.exit(fail === 0 ? 0 : 1);
