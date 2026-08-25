// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

// Turns shell code blocks into terminal windows that type themselves out.
//
// Progressive enhancement, deliberately: the commands are already in the HTML
// and stay there. This only wraps them in chrome and reveals them line by line.
// With scripting off, with a crawler, or with an assistant reading the page,
// the same text is present and complete — which matters, because these blocks
// are the part of the page somebody copies.
//
// The reveal clips the block rather than rewriting it. These blocks are
// syntax-highlighted into nested spans, and an earlier version that split the
// markup on newlines to animate each line cut straight through those spans —
// the browser repaired the unbalanced HTML by discarding it, so `go install
// github.com/...` rendered as `go`. Animating a clip-path touches nothing.
(function () {
  "use strict";

  var SELECTOR = "pre.language-bash, pre.language-console, pre.language-sh";

  function prefersReducedMotion() {
    return window.matchMedia &&
      window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  }

  function decorate(pre) {
    var code = pre.querySelector("code");
    if (!code || pre.closest(".terminal")) {
      return;
    }

    var lineCount = (code.textContent.replace(/\n+$/, "").match(/\n/g) || []).length + 1;

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

    pre.parentNode.insertBefore(shell, pre);
    shell.appendChild(bar);
    shell.appendChild(pre);

    // Duration scales with the block: a one-liner should not take as long as
    // an eight-line session. Capped so a long block never outstays the reader.
    var seconds = Math.min(0.28 * lineCount + 0.25, 2.4);
    shell.style.setProperty("--term-duration", seconds.toFixed(2) + "s");

    return shell;
  }

  function run() {
    var blocks = document.querySelectorAll(SELECTOR);
    if (!blocks.length) {
      return;
    }

    var shells = [];
    for (var i = 0; i < blocks.length; i++) {
      var shell = decorate(blocks[i]);
      if (shell) {
        shells.push(shell);
      }
    }

    // Reduced motion still gets the terminal chrome; it just arrives finished.
    if (prefersReducedMotion() || !("IntersectionObserver" in window)) {
      shells.forEach(function (s) {
        s.classList.add("is-done");
      });
      return;
    }

    var observer = new IntersectionObserver(
      function (entries) {
        entries.forEach(function (entry) {
          if (!entry.isIntersecting) {
            return;
          }
          // is-armed must come off: it holds the closed clip-path at the same
          // specificity as the animation, so leaving it on wins the cascade
          // and the block stays blank after the reveal finishes.
          entry.target.classList.remove("is-armed");
          entry.target.classList.add("is-typing");
          observer.unobserve(entry.target);
        });
      },
      { rootMargin: "0px 0px -12% 0px" }
    );

    shells.forEach(function (s) {
      s.classList.add("is-armed");
      observer.observe(s);
    });
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", run);
  } else {
    run();
  }
})();
