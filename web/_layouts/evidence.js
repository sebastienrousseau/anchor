// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

// The evidence pack: one run, written down.
//
// A finding is only useful if somebody else can act on it, and the person who
// has to act is usually not the person who ran the check. This renders a run as
// Markdown that can be pasted into a ticket or an email to a correspondent bank
// without losing the two things that make a finding checkable -- the rule that
// produced it and the path it points at.
//
// It lives in its own file, and takes everything it needs as arguments, because
// a report that quietly reads global state is one that cannot be tested. The
// browser calls it from the workspace; web/wasm/smoke_test.mjs calls it
// directly.
(function (root) {
  "use strict";

  // pack renders a completed run.
  //
  //   result       -- {messageID, profile, checked, skipped, issues, rules}
  //   version      -- the engine version string, for reproducing the run
  //   hasCatalogue -- whether schema validation actually ran
  function pack(result, version, hasCatalogue) {
    var r = result || {};
    var findings = (r.issues || []).concat(r.rules || []);
    var out = [];

    out.push("# AskIso findings");
    out.push("");
    if (r.messageID) {
      out.push("- Message: `" + r.messageID + "`");
    }
    out.push("- Rule profile: `" + (r.profile || "unknown") + "`");
    if (r.checked) {
      out.push("- Rules applied: " + r.checked +
        (r.skipped ? " (" + r.skipped + " not applicable to this message type)" : ""));
    }
    out.push("- Engine: askiso " + (version || "unknown version") +
      ", run in the browser; the message was not uploaded.");

    // Naming what did not run matters more than naming what did. Without a
    // catalogue there is no schema check, and "clean" then means "nothing
    // contradicted it" rather than "it is valid" -- a difference somebody
    // reading this in a ticket cannot otherwise know about.
    out.push("- Schema validation: " + (hasCatalogue
      ? "run against the catalogue open in the browser tab."
      : "NOT run. No catalogue was open, so this covers lint and scheme rules only."));
    out.push("");

    if (!findings.length) {
      out.push("No findings. Every check that ran passed.");
      out.push("");
      return out.join("\n");
    }

    out.push("## " + findings.length + " finding(s)");
    out.push("");

    findings.forEach(function (f, i) {
      // The rule identifier heads the section: it is the handle somebody quotes
      // back, and a heading that reads only as prose cannot be quoted.
      out.push("### " + (i + 1) + ". " + (f.rule_id || f.rule || "finding"));
      out.push("");
      if (f.message) {
        out.push(f.message);
        out.push("");
      }
      if (f.path) {
        out.push("- Path: `" + f.path + "`");
      }
      if (f.severity) {
        out.push("- Severity: " + f.severity);
      }
      if (f.expected) {
        out.push("- Expected: " + f.expected + (f.actual ? " — found: " + f.actual : ""));
      }
      if (f.remediation || f.fix) {
        out.push("- Fix: " + (f.remediation || f.fix));
      }
      out.push("");
    });

    return out.join("\n");
  }

  root.askisoEvidence = { pack: pack };
})(typeof globalThis !== "undefined" ? globalThis : window);
