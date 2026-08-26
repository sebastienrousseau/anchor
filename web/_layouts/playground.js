// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>

// The distribution bars carry a width that depends on the data, and a
// style="width:…" attribute is exactly what the CSP blocks. Setting it through
// CSSOM after insertion is not covered by style-src, so the bars keep their
// dynamic width without loosening the policy.
function applyBarWidths(root) {
  var bars = (root || document).querySelectorAll(".bar[data-width]");
  for (var i = 0; i < bars.length; i++) {
    bars[i].style.width = bars[i].getAttribute("data-width") + "px";
    bars[i].removeAttribute("data-width");
  }
}
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Extracted from the standalone page. It has to be a file rather than an
// inline block: the site's CSP is script-src 'self', so an inline script
// on this page would simply not run.
(function () {
  "use strict";

  var $ = function (sel, root) { return (root || document).querySelector(sel); };
  var $$ = function (sel, root) { return Array.prototype.slice.call((root || document).querySelectorAll(sel)); };

  function esc(s) {
    return String(s == null ? "" : s).replace(/[&<>"']/g, function (c) {
      return { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c];
    });
  }

  // ---- tabs -------------------------------------------------------------
  $$(".tablist button").forEach(function (btn) {
    btn.addEventListener("click", function () {
      $$(".tablist button").forEach(function (b) { b.setAttribute("aria-selected", String(b === btn)); });
      $$(".panel").forEach(function (p) {
        p.setAttribute("data-active", String(p.dataset.panel === btn.dataset.panel));
      });
      if (btn.dataset.panel === "stats") renderStats();
    });
  });

  // ---- engine bootstrap -------------------------------------------------
  //
  // The engine is a 5.7 MB WebAssembly module, and instantiating it blocks the
  // main thread for a couple of hundred milliseconds. A visitor who came to
  // read about the tools rather than run one should not pay either cost, so
  // nothing is fetched until there is some sign they intend to use the page.
  //
  // First interaction of any kind starts it, which in practice means it is
  // already loading before anyone reaches a button. If they beat it there, the
  // action is queued rather than refused: "try again in a moment" is a worse
  // answer than simply doing what was asked, slightly later.
  var statusEl = $("#status");
  var ready = false;
  var starting = false;
  // Every action blocked while the engine loads, in the order it was asked
  // for. A single slot is not enough: pressing "Use a sample" and then "Lint"
  // before the engine arrives would replay only the second, leaving the sample
  // unfilled and the lint reporting an empty message.
  var replayQueue = [];

  // Recorded in the capture phase, so the button is known before the handler
  // that will be blocked has run.
  var lastAction = null;
  document.addEventListener("click", function (e) {
    var b = e.target && e.target.closest && e.target.closest(".playground button");
    if (b) {
      lastAction = b;
    }
  }, true);

  window.askisoReady = function () {
    ready = true;
    statusEl.textContent = "Engine ready — light mode (embedded index of the standard; no schemas installed)";
    statusEl.className = "ready";
    var pending = replayQueue;
    replayQueue = [];
    pending.forEach(function (b) { b.click(); });
  };

  function startEngine() {
    if (starting || ready) {
      return;
    }
    starting = true;

    if (!window.WebAssembly || !WebAssembly.instantiateStreaming) {
      statusEl.textContent = "This browser does not support WebAssembly streaming.";
      statusEl.className = "error";
      return;
    }

    statusEl.textContent = "Loading the engine…";
    statusEl.className = "";

    var go = new Go();
    WebAssembly.instantiateStreaming(fetch("/askiso.wasm"), go.importObject)
      .then(function (res) { go.run(res.instance); })
      .catch(function (err) {
        starting = false;
        statusEl.textContent = "Could not load the engine: " + err;
        statusEl.className = "error";
      });
  }

  ["pointerdown", "keydown", "focusin"].forEach(function (evt) {
    document.addEventListener(evt, startEngine, { once: true, passive: true });
  });

  function requireEngine() {
    if (ready) {
      return true;
    }
    startEngine();
    if (lastAction && replayQueue.indexOf(lastAction) === -1) {
      replayQueue.push(lastAction);
    }
    statusEl.textContent = "Loading the engine — this will run as soon as it is ready.";
    statusEl.className = "";
    return false;
  }

  function call(fn) {
    var args = Array.prototype.slice.call(arguments, 1);
    try {
      return window.askiso[fn].apply(null, args);
    } catch (e) {
      return { ok: false, error: String(e && e.message ? e.message : e) };
    }
  }

  // ---- shared renderers -------------------------------------------------
  function showError(el, msg) {
    el.innerHTML = '<div class="verdict fail">' + esc(msg) + "</div>";
  }

  // ---- generate ---------------------------------------------------------
  $("#gen-go").addEventListener("click", function () {
    if (!requireEngine()) return;
    var out = $("#gen-out");
    var r = call("generate", $("#gen-type").value, $("#gen-preset").value, $("#gen-amount").value, "", false);
    if (!r.ok) return showError(out, r.error);

    var lintRes = call("lint", r.data.xml, "generated.xml");
    var banner = "";
    if (lintRes.ok) {
      var d = lintRes.data;
      banner = d.error_count > 0
        ? '<div class="verdict fail">Generated, but AskISO’s own linter reports ' + d.error_count + ' error(s) — a known defect for this preset.</div>'
        : '<div class="verdict pass">Generated and passes all ' + d.passed_count + " business-rule checks</div>";
    }
    out.innerHTML = banner + '<pre class="code">' + esc(r.data.xml) + "</pre>";
  });

  // ---- codes ------------------------------------------------------------
  function runCodes() {
    if (!requireEngine()) return;
    var out = $("#codes-out");
    var r = call("codes", $("#codes-q").value);
    if (!r.ok) return showError(out, r.error);
    var rows = r.data || [];
    if (!rows.length) {
      out.innerHTML = '<p class="empty">No code matches that query.</p>';
      return;
    }
    var html = '<div class="tablewrap"><table><thead><tr>' +
      "<th>Code</th><th>Name</th><th>Category</th><th>Description</th><th>Applies to</th>" +
      "</tr></thead><tbody>";
    rows.forEach(function (c) {
      html += "<tr>" +
        '<td class="mono">' + esc(c.code) + "</td>" +
        "<td>" + esc(c.name) + "</td>" +
        "<td>" + esc(c.category) + "</td>" +
        "<td>" + esc(c.description) + "</td>" +
        '<td class="mono">' + esc(c.applies_to) + "</td></tr>";
    });
    out.innerHTML = html + "</tbody></table></div>";
  }
  $("#codes-go").addEventListener("click", runCodes);
  $("#codes-q").addEventListener("keydown", function (e) { if (e.key === "Enter") runCodes(); });

  // ---- stats ------------------------------------------------------------
  var statsRendered = false;
  function renderStats() {
    if (statsRendered || !ready) return;
    var out = $("#stats-out");
    var r = call("stats");
    if (!r.ok) return showError(out, r.error);
    var d = r.data;
    var max = d.domains.length ? d.domains[0].count : 1;

    var html = '<div class="verdict pass">' + d.total + " message definitions across " + d.messageSets + " published message sets</div>" +
      '<div class="tablewrap"><table><thead><tr><th>Domain</th><th>Business area</th><th>Messages</th><th>Share</th></tr></thead><tbody>';
    d.domains.forEach(function (row) {
      var pct = (row.count / d.total) * 100;
      html += "<tr>" +
        '<td class="mono">' + esc(row.domain) + "</td>" +
        "<td>" + esc(row.name) + "</td>" +
        '<td class="mono">' + row.count + "</td>" +
        '<td><span class="bar" data-width="' + Math.max(2, Math.round((row.count / max) * 140)) + '"></span> ' +
        '<span class="mono pct">' + pct.toFixed(1) + "%</span></td></tr>";
    });
    out.innerHTML = html + "</tbody></table></div>";
    applyBarWidths(out);
    statsRendered = true;
  }
})();
