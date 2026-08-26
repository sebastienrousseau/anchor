// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Drives the two interactive pages the way a visitor does.
//
// The WebAssembly smoke test proves the engine works; this proves the pages
// around it do. It exists because the engine now loads on first interaction
// rather than on page load, and that change has a failure mode no static check
// can see: an action taken while the engine is still arriving has to be queued
// and replayed, and if the queue holds only the most recent one, pressing two
// buttons quickly silently drops the first.
//
// That is not hypothetical. It shipped, briefly, and the screenshot of an empty
// textarea beside a "paste a message" error is what found it.
//
//   make web-interact
//
// Needs puppeteer-core and a Chrome on the machine; the make target skips
// rather than fails when neither is present, so a contributor without them is
// not blocked.

import puppeteer from 'puppeteer-core';

const BASE = process.env.ASKISO_BASE_URL || 'http://127.0.0.1:8899';
const CHROME = process.env.CHROME_PATH
  || '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome';

const browser = await puppeteer.launch({
  executablePath: CHROME,
  headless: 'new',
  args: ['--no-sandbox', '--disable-gpu'],
});

let failures = 0;
const check = (name, ok, detail = '') => {
  console.log(`  ${ok ? 'ok  ' : 'FAIL'}  ${name}${ok ? '' : ' -- ' + detail}`);
  if (!ok) failures++;
};

console.log('askiso page interaction test\n');
// --- workspace: engine must not load until the visitor acts ---------------
{
  const page = await browser.newPage();
  const wasmRequests = [];
  page.on('request', r => { if (r.url().endsWith('.wasm')) wasmRequests.push(r.url()); });
  await page.goto(`${BASE}/workspace/`, { waitUntil: 'networkidle0' });

  check('workspace: engine not fetched on load', wasmRequests.length === 0,
    `${wasmRequests.length} wasm request(s)`);
  const status = await page.$eval('#ws-status', e => e.textContent.trim());
  check('workspace: status explains the deferred load',
    /loads when you start/i.test(status), status);

  // Act, exactly as a visitor would.
  await page.click('#ws-input');
  await page.type('#ws-input', 'pacs.008');
  await page.click('#ws-go');
  await page.waitForFunction(
    () => document.querySelector('.ws-verdict') !== null, { timeout: 30000 });
  const verdict = await page.$eval('.ws-verdict', e => e.textContent.trim());
  check('workspace: engine loads and answers on interaction',
    /message definition\(s\) match/.test(verdict), verdict);
  check('workspace: engine fetched exactly once', wasmRequests.length === 1,
    `${wasmRequests.length}`);
  await page.close();
}

// --- workspace: a click before the engine is ready must still run ----------
{
  const page = await browser.newPage();
  await page.goto(`${BASE}/workspace/`, { waitUntil: 'domcontentloaded' });
  // Type and submit immediately, racing the engine on purpose.
  await page.evaluate(() => {
    document.getElementById('ws-input').value = 'GB29NWBK60161331926819';
    document.getElementById('ws-go').click();
  });
  await page.waitForFunction(
    () => document.querySelector('.ws-verdict') !== null, { timeout: 30000 });
  const verdict = await page.$eval('.ws-verdict', e => e.textContent.trim());
  check('workspace: a request made before the engine is ready still runs',
    /is valid/.test(verdict), verdict);
  await page.close();
}

// --- playground: same contract --------------------------------------------
{
  const page = await browser.newPage();
  const wasmRequests = [];
  page.on('request', r => { if (r.url().endsWith('.wasm')) wasmRequests.push(r.url()); });
  const errors = [];
  page.on('pageerror', e => errors.push(String(e)));
  await page.goto(`${BASE}/playground/`, { waitUntil: 'networkidle0' });
  check('playground: engine not fetched on load', wasmRequests.length === 0,
    `${wasmRequests.length} wasm request(s)`);

  // Both clicks in one task, so the engine cannot possibly load between them
  // and both are genuinely blocked. Clicking them separately is timing
  // dependent: on a fast connection the engine arrives in the gap and the race
  // this is meant to exercise never happens.
  await page.evaluate(() => {
    document.getElementById('lint-fill').click();
    document.getElementById('lint-go').click();
  });
  await page.waitForFunction(
    () => document.querySelector('#lint-out .verdict') !== null, { timeout: 30000 });
  const verdict = await page.$eval('#lint-out .verdict', e => e.textContent.trim());
  // Both blocked actions must replay, in order: the sample fills the textarea
  // and the lint then runs against it. Asserting only that some verdict exists
  // passes on the failure case, because "paste a message to lint" is a verdict.
  const filled = await page.$eval('#lint-in', e => e.value.length);
  check('playground: the sample was filled by the replayed first action', filled > 100,
    `${filled} characters in the textarea`);
  check('playground: both blocked actions replayed and the lint passed',
    /passed/i.test(verdict) && !/paste an ISO/i.test(verdict), verdict);
  check('playground: no page errors', errors.length === 0, errors.join('; '));

  // Every tool tab must still switch.
  const tabs = await page.$$eval('[role="tab"]', els => els.map(e => e.textContent.trim()));
  let switched = 0;
  for (const name of tabs) {
    await page.evaluate(n => {
      [...document.querySelectorAll('[role="tab"]')]
        .find(t => t.textContent.trim() === n).click();
    }, name);
    const visible = await page.$$eval('.panel[data-active="true"]', e => e.length);
    if (visible === 1) switched++;
  }
  check(`playground: all ${tabs.length} tool tabs switch`, switched === tabs.length,
    `${switched}/${tabs.length}`);
  await page.close();
}

await browser.close();
console.log(failures === 0 ? '\nall interaction checks passed' : `\n${failures} failed`);
process.exit(failures === 0 ? 0 : 1);
