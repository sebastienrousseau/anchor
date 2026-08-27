// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// The shell block and the terminal it becomes must have the same box.
//
// terminal.js replaces a `<pre>` with terminal chrome — a title bar and a
// screen — and anything the two boxes disagree about moves every paragraph
// below them at the moment the script runs. The `pre` therefore carries the
// bar's height as block-start padding, which is a number in one stylesheet
// standing for the rendered height of an element in another. It read 3.35rem
// against an actual 2.5625rem + 1.15rem, and every shell block on the page grew
// by the 5.8px difference; /docs/ measured a layout shift of 0.064.
//
// A total-height comparison would be the obvious check and the wrong one. The
// terminal wraps a long command where the `pre` scrolls it, so a block holding
// one can legitimately differ by whole lines, and a gate written against the
// total would either fail on that or be loosened until it caught nothing. What
// is actually being asserted is narrower and exact: the chrome around the text
// is identical, so the swap contributes nothing of its own.
//
//   make terminal-swap
//
// Needs puppeteer-core and a Chrome; the make target skips rather than fails
// when they are absent.

import puppeteer from 'puppeteer-core';

const BASE = process.env.ASKISO_BASE_URL || 'http://127.0.0.1:8899';
const CHROME = process.env.CHROME_PATH
  || '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome';

// Pages carrying a recorded session and a plain shell block respectively.
const PAGES = ['docs/', '', 'faq/', 'conformance/'];

const browser = await puppeteer.launch({
  executablePath: CHROME,
  headless: 'new',
  args: ['--no-sandbox', '--disable-gpu'],
});

const px = v => Math.round(parseFloat(v) * 100) / 100;
let failures = 0;
let compared = 0;

for (const pg of PAGES) {
  // Without the script: the boxes the markup alone produces.
  const bare = await browser.newPage();
  await bare.setViewport({ width: 1280, height: 900 });
  await bare.setRequestInterception(true);
  bare.on('request', r => (r.url().endsWith('/terminal.js') ? r.abort() : r.continue()));
  await bare.goto(`${BASE}/${pg}`, { waitUntil: 'networkidle0' });
  const pre = await bare.evaluate(() => {
    const el = [...document.querySelectorAll('pre')]
      .find(p => /language-(bash|sh|console|shell)/.test(p.className));
    if (!el) return null;
    const s = getComputedStyle(el);
    return {
      padTop: s.paddingTop, padBottom: s.paddingBottom,
      padLeft: s.paddingLeft, padRight: s.paddingRight,
      marginTop: s.marginTop, marginBottom: s.marginBottom,
      fontSize: s.fontSize, lineHeight: s.lineHeight,
    };
  });
  await bare.close();
  if (!pre) continue;

  // With it: the boxes the terminal produces.
  const live = await browser.newPage();
  await live.setViewport({ width: 1280, height: 900 });
  await live.goto(`${BASE}/${pg}`, { waitUntil: 'networkidle0' });
  await new Promise(r => setTimeout(r, 600));
  const term = await live.evaluate(() => {
    const el = document.querySelector('.terminal');
    if (!el) return null;
    const s = getComputedStyle(el);
    const screen = getComputedStyle(el.querySelector('.terminal-screen'));
    return {
      barHeight: el.querySelector('.terminal-bar').getBoundingClientRect().height,
      marginTop: s.marginTop, marginBottom: s.marginBottom,
      padBottom: screen.paddingBottom, padLeft: screen.paddingLeft,
      padRight: screen.paddingRight, padTop: screen.paddingTop,
      fontSize: screen.fontSize, lineHeight: screen.lineHeight,
    };
  });
  await live.close();
  if (!term) {
    console.log(`  FAIL  /${pg} — a shell block is present but no terminal was built`);
    failures++;
    continue;
  }

  // The text starts this far down in each: padding in the `pre`, bar plus the
  // screen's own padding in the terminal.
  const checks = [
    ['text starts at', px(pre.padTop), px(px(term.barHeight) + px(term.padTop))],
    ['text ends at', px(pre.padBottom), px(term.padBottom)],
    ['inset left', px(pre.padLeft), px(term.padLeft)],
    ['inset right', px(pre.padRight), px(term.padRight)],
    ['margin above', px(pre.marginTop), px(term.marginTop)],
    ['margin below', px(pre.marginBottom), px(term.marginBottom)],
    ['font size', px(pre.fontSize), px(term.fontSize)],
    ['line height', px(pre.lineHeight), px(term.lineHeight)],
  ];

  const bad = checks.filter(([, a, b]) => Math.abs(a - b) > 1);
  compared += checks.length;
  if (bad.length) {
    failures += bad.length;
    console.log(`  FAIL  /${pg}`);
    for (const [what, a, b] of bad) {
      console.log(`          ${what}: the block says ${a}px, the terminal ${b}px`
        + ` — the page moves ${px(Math.abs(a - b))}px when the script runs`);
    }
  } else {
    console.log(`  ok    /${pg} — the block and the terminal have the same box`);
  }
}

await browser.close();
console.log(failures === 0
  ? `\n${compared} box measurement(s) compared, the swap moves nothing`
  : `\n${failures} measurement(s) disagree`);
process.exit(failures === 0 ? 0 : 1);
