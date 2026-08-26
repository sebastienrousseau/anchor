// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

// Replays shell sessions the way they actually happen: the command is typed a
// character at a time, then its output arrives at once.
//
// The distinction is the point. A person types a command; a program prints its
// output. Revealing both at the same rate shows nobody anything true about
// using the tool — it is a wipe with a cursor on it. Commands type. Output
// appears.
//
// Two kinds of block get the treatment.
//
// A recorded session starts with "$ ": lines beginning "$ " are typed, every
// other line is output belonging to the command above it, and the whole thing
// is verified against the binary by scripts/sessions.
//
// A plain shell block — bash, sh, console — has no recorded output, so every
// non-empty line is a command and is typed. It animates but claims nothing,
// which is the right treatment for `go install` or a snippet the reader is
// meant to adapt.
//
// Nothing else animates. A lua or yaml block is configuration to copy, not a
// session, and typing it out would be theatre.
//
// Progressive enhancement throughout: the whole session is in the HTML, and
// that is what a crawler, an assistant, or a reader without scripting gets.
// The script only replays it. Because the replay shows half-typed text while
// it runs, the animated region is hidden from assistive technology and the
// complete transcript is exposed alongside — a screen reader should get the
// finished session, not a stutter.
(function () {
  "use strict";

  var CHAR_MS = 20; // per character of a typed command
  // The pauses are dead time: nothing is being typed and nothing is being
  // read, but the page is still visually settling, which is what Speed Index
  // measures. Trimmed to the shortest that still reads as a beat between
  // commands rather than as one continuous dump. The typing rate itself is
  // unchanged, because that is the part anyone actually watches.
  var AFTER_COMMAND_MS = 150; // pause between the command and its output
  var BETWEEN_MS = 240; // pause before the next command starts

  function reduced() {
    return window.matchMedia &&
      window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  }

  var SHELL = /\blanguage-(bash|sh|console|shell)\b/;

  // A session is a list of steps: a command, and the output it produced.
  function parse(text) {
    var steps = [];

    text.replace(/\n+$/, "").split("\n").forEach(function (line) {
      if (/^\$ /.test(line)) {
        steps.push({ command: line.slice(2), output: [] });
      } else if (steps.length) {
        steps[steps.length - 1].output.push(line);
      } else {
        steps.push({ command: null, output: [line] });
      }
    });

    return steps;
  }

  // A plain shell block: every line is something the reader would type. A blank
  // line is spacing rather than a command, so it does not become an empty
  // prompt sitting there waiting for input that never comes.
  function parseCommands(text) {
    return text.replace(/\n+$/, "").split("\n").map(function (line) {
      return line.trim() === ""
        ? { command: null, output: [""] }
        : { command: line, output: [] };
    });
  }

  function build(pre) {
    var text = pre.textContent;
    var session = /^\s*\$ /.test(text);

    if (!session && !SHELL.test(pre.className)) {
      return null;
    }

    var shell = document.createElement("div");
    shell.className = "terminal";

    var bar = document.createElement("div");
    bar.className = "terminal-bar";
    bar.setAttribute("aria-hidden", "true");
    bar.innerHTML =
      '<span class="terminal-dot"></span>' +
      '<span class="terminal-dot"></span>' +
      '<span class="terminal-dot"></span>' +
      '<span class="terminal-title">askiso</span>';

    var screen = document.createElement("div");
    screen.className = "terminal-screen";
    screen.setAttribute("aria-hidden", "true");

    var transcript = document.createElement("pre");
    // Its own class as well as the utility one. The utility lives in a layer,
    // and something unlayered was winning the box back: the transcript kept a
    // pre's padding and overflow, which made it a scrollable region with no
    // keyboard access — a WCAG 2.2 AA failure axe reports as serious, on every
    // page carrying a session.
    transcript.className = "visually-hidden terminal-transcript";
    transcript.textContent = text;

    var steps = session ? parse(text) : parseCommands(text);

    steps.forEach(function (step) {
      if (step.command !== null) {
        var cmd = document.createElement("div");
        cmd.className = "term-cmd";
        cmd.innerHTML =
          '<span class="term-prompt">$</span> <span class="term-typed"></span>';
        cmd.dataset.text = step.command;
        screen.appendChild(cmd);
      }
      if (step.output.length) {
        var out = document.createElement("div");
        out.className = "term-out";
        out.textContent = step.output.join("\n");
        screen.appendChild(out);
      }
    });

    pre.parentNode.insertBefore(shell, pre);
    shell.appendChild(bar);
    shell.appendChild(screen);
    shell.appendChild(transcript);
    pre.remove();

    reserveHeight(screen);

    return shell;
  }

  // Hold the height the finished session will occupy, before any of it is
  // typed.
  //
  // Commands appear a character at a time, so a command long enough to wrap
  // starts one line tall and becomes two partway through. The screen grows with
  // it and everything below moves down the page -- while somebody is reading
  // it, which is the worst moment for the paragraph they are on to jump. This
  // was the largest layout shift on the site.
  //
  // Measuring is exact where guessing is not: the text is written in, the
  // height is read, and the text is taken out again, all within one frame so
  // nothing is ever painted in the finished state.
  function reserveHeight(screen) {
    var cmds = screen.querySelectorAll(".term-cmd");
    if (!cmds.length) {
      return;
    }

    Array.prototype.forEach.call(cmds, function (cmd) {
      cmd.querySelector(".term-typed").textContent = cmd.dataset.text;
    });

    var height = screen.getBoundingClientRect().height;

    Array.prototype.forEach.call(cmds, function (cmd) {
      cmd.querySelector(".term-typed").textContent = "";
    });

    if (height > 0) {
      // Set through the CSSOM rather than a style attribute: the site's
      // style-src is 'self', which blocks the attribute but not this.
      screen.style.minHeight = height + "px";
    }
  }

  // Everything typed, everything printed. Reduced motion starts here.
  function settle(shell) {
    Array.prototype.forEach.call(
      shell.querySelectorAll(".term-cmd"),
      function (cmd) {
        cmd.querySelector(".term-typed").textContent = cmd.dataset.text;
        cmd.classList.add("is-done");
      }
    );
    Array.prototype.forEach.call(
      shell.querySelectorAll(".term-out"),
      function (out) {
        out.classList.add("is-shown");
      }
    );
    shell.classList.add("is-settled");
  }

  function play(shell) {
    var nodes = Array.prototype.slice.call(
      shell.querySelectorAll(".term-cmd, .term-out")
    );
    var index = 0;

    function next() {
      if (index >= nodes.length) {
        shell.classList.add("is-settled");
        return;
      }

      var node = nodes[index++];

      // Output is printed, not typed.
      if (node.classList.contains("term-out")) {
        node.classList.add("is-shown");
        window.setTimeout(next, BETWEEN_MS);
        return;
      }

      var target = node.querySelector(".term-typed");
      var full = node.dataset.text;
      var started = null;

      node.classList.add("is-typing");

      // Driven by elapsed time under requestAnimationFrame rather than a
      // setTimeout per character. A background tab throttles timers to about
      // one a second, which turned a one-second command into a minute of
      // watching it crawl; rAF simply does not run until the tab is visible
      // again, and deriving the position from the clock means the replay is
      // the same length however the browser schedules it.
      function tick(now) {
        if (started === null) {
          started = now;
        }
        var shown = Math.min(full.length, Math.floor((now - started) / CHAR_MS));

        if (target.textContent.length !== shown) {
          target.textContent = full.slice(0, shown);
        }

        if (shown >= full.length) {
          node.classList.remove("is-typing");
          node.classList.add("is-done");
          window.setTimeout(next, AFTER_COMMAND_MS);
          return;
        }
        window.requestAnimationFrame(tick);
      }

      window.requestAnimationFrame(tick);
    }

    next();
  }

  function run() {
    var shells = [];

    Array.prototype.forEach.call(
      document.querySelectorAll("pre"),
      function (pre) {
        var shell = build(pre);
        if (shell) {
          shells.push(shell);
        }
      }
    );

    if (!shells.length) {
      return;
    }

    if (reduced() || !("IntersectionObserver" in window)) {
      shells.forEach(settle);
      return;
    }

    var observer = new IntersectionObserver(
      function (entries) {
        entries.forEach(function (entry) {
          if (!entry.isIntersecting) {
            return;
          }
          observer.unobserve(entry.target);
          play(entry.target);
        });
      },
      { rootMargin: "0px 0px -15% 0px" }
    );

    shells.forEach(function (s) {
      observer.observe(s);
    });
  }

  // Run now, not on DOMContentLoaded.
  //
  // This script is the last thing in <body>, so every <pre> it transforms has
  // already been parsed. Waiting meant the swap happened after the first paint:
  // the browser drew the plain transcript, then replaced it with the taller
  // terminal, and everything below jumped down the page while somebody was
  // reading it. That was the largest layout shift on the site.
  run();
})();
