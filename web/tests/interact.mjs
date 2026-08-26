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
  // The wording is free to change; what it must not do is claim the engine is
  // ready before it has been fetched. Asserting the exact sentence made this
  // fail on a copy edit that was perfectly correct.
  check('workspace: status says the engine loads on use, not that it is ready',
    /engine loads/i.test(status) && !/ready/i.test(status), status);

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

  // Pressed before the engine exists, so it has to be queued and replayed.
  await page.evaluate(() => document.getElementById('gen-go').click());
  await page.waitForFunction(
    () => (document.querySelector('#gen-out')?.textContent || '').trim().length > 0,
    { timeout: 30000 });
  const generated = await page.$eval('#gen-out', e => e.textContent.trim());
  check('playground: an action taken before the engine is ready still runs',
    /<Document|xmlns/.test(generated), generated.slice(0, 90));
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
  // The tools the workspace already does were removed rather than duplicated.
  check('playground: no tab duplicates the workspace',
    !tabs.some(t => /^(Lint|Validate|Convert|Validators|Browse|MT)/i.test(t)),
    tabs.join(', '));
  await page.close();
}

// --- workspace: the follow-up chips must do something ----------------------
//
// They used to set the input to the message already on screen and re-submit,
// which re-ran the same lint. Pressing "Convert to MT" did nothing visible.
{
  const page = await browser.newPage();
  const errs = [];
  page.on('pageerror', e => errs.push(String(e)));
  await page.goto(`${BASE}/workspace/`, { waitUntil: 'networkidle0' });

const MSG = `<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">
  <FIToFICstmrCdtTrf><GrpHdr><MsgId>MSG-1</MsgId><CreDtTm>2026-08-26T09:00:00Z</CreDtTm><NbOfTxs>1</NbOfTxs></GrpHdr>
    <CdtTrfTxInf><PmtId><EndToEndId>E2E-1</EndToEndId></PmtId>
      <IntrBkSttlmAmt Ccy="EUR">1000.00</IntrBkSttlmAmt>
      <Dbtr><Nm>Acme</Nm></Dbtr>
      <DbtrAcct><Id><IBAN>DE89370400440532013000</IBAN></Id></DbtrAcct>
      <DbtrAgt><FinInstnId><BICFI>DEUTDEDDXXX</BICFI></FinInstnId></DbtrAgt>
      <CdtrAgt><FinInstnId><BICFI>BNPAFRPPXXX</BICFI></FinInstnId></CdtrAgt>
      <Cdtr><Nm>Global</Nm></Cdtr>
      <CdtrAcct><Id><IBAN>FR7630006000011234567890189</IBAN></Id></CdtrAcct>
    </CdtTrfTxInf></FIToFICstmrCdtTrf></Document>`;

async function run() {
  await page.evaluate(m => { document.getElementById('ws-input').value = m;
    document.getElementById('ws-go').click(); }, MSG);
  await page.waitForFunction(() => document.querySelector('.ws-verdict'), { timeout: 30000 });
}
const chips = async () => page.$$eval('.ws-suggest .ws-chip', e => e.map(x => x.textContent.trim()));
const clickChip = async label => page.evaluate(l => {
  [...document.querySelectorAll('.ws-suggest .ws-chip')].find(c => c.textContent.trim() === l).click();
}, label);




await run();
check('lint offers both chips', (await chips()).join('|').includes('Convert to SWIFT MT'), (await chips()).join('|'));

await clickChip('Convert to SWIFT MT');
await new Promise(r => setTimeout(r, 600));
let v = await page.$eval('.ws-verdict', e => e.textContent.trim());
let code = await page.$$eval('.ws-code', e => e.map(x => x.textContent).join('').trim());
check('convert changes the answer', /Converted/.test(v), v);
check('convert shows MT output', /^\{1:|:20:/m.test(code), code.slice(0, 90));
check('convert names a real target, not a placeholder', !/the other format/.test(v) && !/^Converted to ISO 20022\.$/.test(v), v);
let studio = await page.$$eval('#ws-studio .ws-offer-label', e => e.map(x => x.textContent.trim()));
check('studio offers the conversion', studio.some(l => /Converted|Fidelity/.test(l)), studio.join(', '));

await clickChip('Back to the findings');
await new Promise(r => setTimeout(r, 500));
v = await page.$eval('.ws-verdict', e => e.textContent.trim());
check('back returns to the findings', /finding|Clean/.test(v), v);

await clickChip('Show as JSON');
await new Promise(r => setTimeout(r, 600));
v = await page.$eval('.ws-verdict', e => e.textContent.trim());
code = await page.$$eval('.ws-code', e => e.map(x => x.textContent).join('').trim());
check('json changes the answer', /JSON/.test(v), v);
check('json output parses', (() => { try { JSON.parse(code); return true; } catch { return false; } })(), code.slice(0, 90));
studio = await page.$$eval('#ws-studio .ws-offer-label', e => e.map(x => x.textContent.trim()));
check('studio offers the JSON', studio.some(l => /JSON/.test(l)), studio.join(', '));

  check('workspace: no page errors from the chips', errs.length === 0, errs.join('; '));
  await page.close();
}

await browser.close();
console.log(failures === 0 ? '\nall interaction checks passed' : `\n${failures} failed`);
process.exit(failures === 0 ? 0 : 1);
