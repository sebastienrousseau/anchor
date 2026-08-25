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
// A block is a session when its first line starts with "$ ". Lines beginning
// "$ " are typed; every other line is output belonging to the command above it.
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
  var AFTER_COMMAND_MS = 260; // pause between the command and its output
  var BETWEEN_MS = 420; // pause before the next command starts

  function reduced() {
    return window.matchMedia &&
      window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  }

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

  function build(pre) {
    var text = pre.textContent;
    if (!/^\s*\$ /.test(text)) {
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
    transcript.className = "visually-hidden";
    transcript.textContent = text;

    parse(text).forEach(function (step) {
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

    return shell;
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
      var at = 0;

      node.classList.add("is-typing");

      (function tick() {
        if (at >= full.length) {
          node.classList.remove("is-typing");
          node.classList.add("is-done");
          window.setTimeout(next, AFTER_COMMAND_MS);
          return;
        }
        at += 1;
        target.textContent = full.slice(0, at);
        window.setTimeout(tick, CHAR_MS);
      })();
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

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", run);
  } else {
    run();
  }
})();
