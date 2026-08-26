// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

// A three-panel workspace: Sources, Ask, Studio.
//
// Sources is what an answer is grounded in — the catalogue the visitor pointed
// at and the message they are working on. Ask takes one input and works out
// what was meant. Studio turns the result into something they can hand to
// somebody else.
//
// "Ask" here is not a language model. It dispatches to the same engine the
// command line uses and answers from the embedded index, and every finding
// carries the XPath it came from. That is a deliberate choice rather than a
// limitation: a tool that tells a bank its message is fine had better be able
// to say exactly which rule it checked, and a model that paraphrases a
// specification it half-remembers cannot.
(function () {
  "use strict";

  var engineReady = false;

  function $(sel, root) {
    return (root || document).querySelector(sel);
  }

  function el(tag, cls, text) {
    var n = document.createElement(tag);
    if (cls) n.className = cls;
    if (text !== undefined) n.textContent = text;
    return n;
  }

  // Every engine function answers {ok: true, data: …} or {ok: false, error: …}.
  // call unwraps that once, here, so no handler has to remember the envelope —
  // forgetting it is silent rather than loud, and reads as "everything is
  // invalid" rather than as a bug.
  function call(fn) {
    var args = Array.prototype.slice.call(arguments, 1);
    try {
      var res = window.askiso[fn].apply(null, args);
      if (res && res.ok === false) {
        return { error: res.error || "The engine could not answer." };
      }
      return { data: res ? res.data : null };
    } catch (e) {
      return { error: String(e && e.message ? e.message : e) };
    }
  }

  // ---------------------------------------------------------------------
  // Intent
  //
  // One box, because asking somebody to classify their own input before the
  // tool will look at it is work the tool should be doing.
  // ---------------------------------------------------------------------

  var MX_NS = /urn:iso:std:iso:20022:tech:xsd:([a-z]+\.\d+\.\d+\.\d+)/i;
  var FULL_ID = /^[a-z]{4}\.\d{3}\.\d{3}\.\d{2}$/i;
  var BASE_CODE = /^[a-z]{4}\.\d{3}$/i;
  var IBAN = /^[A-Z]{2}\d{2}[A-Z0-9]{10,30}$/i;
  var BIC = /^[A-Z]{6}[A-Z0-9]{2}([A-Z0-9]{3})?$/i;
  var UETR = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

  function classify(input) {
    var s = input.trim();
    if (!s) return { kind: "empty" };

    // A SWIFT MT message: block structure, or the field tags that only appear
    // in one.
    if (/^\{1:/.test(s) || /^:\d{2}[A-Z]?:/m.test(s)) {
      return { kind: "mt", text: s };
    }
    if (s.charAt(0) === "<") {
      var m = MX_NS.exec(s);
      return { kind: "mx", text: s, messageID: m ? m[1] : "" };
    }
    if (FULL_ID.test(s)) return { kind: "message-id", id: s.toLowerCase() };
    // A base code names a family, not one definition, so it lists versions.
    if (BASE_CODE.test(s)) return { kind: "search", query: s.toLowerCase(), family: true };
    if (UETR.test(s)) return { kind: "uetr", value: s };
    if (IBAN.test(s) && /\d/.test(s)) return { kind: "iban", value: s.toUpperCase() };
    if (BIC.test(s)) return { kind: "bic", value: s.toUpperCase() };
    return { kind: "search", query: s };
  }

  // Plain-language description of what the tool decided to do, shown before
  // the answer. A dispatcher that guesses silently is a dispatcher nobody
  // trusts the second time it guesses wrong.
  function describe(intent) {
    switch (intent.kind) {
      case "mt": return "Read as a SWIFT MT message — converting to ISO 20022.";
      case "mx": return intent.messageID
        ? "Read as " + intent.messageID + " — linting, and validating if your catalogue has the schema."
        : "Read as ISO 20022 XML — linting. It declares no ISO 20022 namespace, so the schema cannot be resolved.";
      case "message-id": return "Read as a message identifier — looking it up.";
      case "iban": return "Read as an IBAN — checking the mod-97 checksum.";
      case "bic": return "Read as a BIC — checking the structure.";
      case "uetr": return "Read as a UETR — checking it is a version-4 UUID.";
      case "search": return intent.family
        ? "Read as a message family — listing its versions."
        : "Searching identifiers, domains and business areas.";
      default: return "";
    }
  }

  // ---------------------------------------------------------------------
  // Rendering
  // ---------------------------------------------------------------------

  var lastResult = null; // what Studio offers, and what Sources describes

  function verdict(ok, text) {
    return el("div", "ws-verdict " + (ok ? "is-pass" : "is-fail"), text);
  }

  // Every finding shows the rule and the path it came from. That pairing is
  // the citation: it is what lets a reader check the claim rather than take it.
  function findingList(items) {
    var list = el("ul", "ws-findings");
    items.forEach(function (f) {
      var li = el("li");
      // The identifier first: CBPR-ADDR-002 is what somebody puts in a ticket,
      // and the name is what they read. Showing only the name loses the handle.
      li.appendChild(el("span", "ws-rule", f.rule_id || f.rule || "rule"));
      li.appendChild(el("span", "ws-msg",
        f.message || f.rule || ""));
      if (f.path) li.appendChild(el("code", "ws-path", f.path));
      if (f.expected) {
        li.appendChild(el("span", "ws-expect", "expected " + f.expected +
          (f.actual ? ", found " + f.actual : "")));
      }
      if (f.remediation || f.fix) {
        li.appendChild(el("span", "ws-fix", f.remediation || f.fix));
      }
      list.appendChild(li);
    });
    return list;
  }

  function suggestions(items) {
    var row = el("div", "ws-suggest");
    items.forEach(function (s) {
      var b = el("button", "ws-chip", s.label);
      b.type = "button";
      b.addEventListener("click", function () {
        // A chip either runs something on what is already there, or replaces
        // the input and asks again. The first kind used to be written as the
        // second, which meant "Convert to MT" set the input to the message it
        // was already showing and re-ran the same lint: pressing it did
        // nothing visible, twice.
        if (typeof s.run === "function") {
          s.run();
          return;
        }
        $("#ws-input").value = s.input;
        submit();
      });
      row.appendChild(b);
    });
    return row;
  }

  function codeBlock(text) {
    var pre = el("pre", "ws-code");
    pre.textContent = text;
    return pre;
  }

  function answer(nodes) {
    var out = $("#ws-answer");
    out.innerHTML = "";
    nodes.forEach(function (n) { if (n) out.appendChild(n); });
  }

  // ---------------------------------------------------------------------
  // Handlers, one per intent
  // ---------------------------------------------------------------------

  function handleMX(intent) {
    var nodes = [];
    var lint = call("lint", intent.text);

    if (lint.error) {
      nodes.push(verdict(false, lint.error));
      answer(nodes);
      return;
    }

    var issues = (lint.data && (lint.data.issues || lint.data.findings)) || [];

    // Scheme rules sit on top of lint: a message can pass every checksum and
    // still be rejected by a clearing system, so both have to be run before
    // anything is said about the message as a whole.
    var rules = call("checkRules", intent.text, "cbpr-2026");
    var ruleFindings = (rules.data && rules.data.findings) || [];

    var total = issues.length + ruleFindings.length;
    nodes.push(verdict(total === 0,
      total === 0
        ? "Clean — checksums, structure, formats and the November 2026 rules all pass."
        : total + " finding(s): " + issues.length + " lint, " +
          ruleFindings.length + " from the November 2026 rules."));

    if (issues.length) {
      nodes.push(el("h3", null, "Lint"));
      nodes.push(findingList(issues));
    }
    if (ruleFindings.length) {
      nodes.push(el("h3", null, "November 2026 rules"));
      nodes.push(findingList(ruleFindings));
    }

    lastResult = {
      kind: "mx",
      input: intent.text,
      messageID: intent.messageID || (rules.data && rules.data.file) || "",
      issues: issues,
      rules: ruleFindings,
      profile: (rules.data && rules.data.profile) || "cbpr-2026",
      checked: rules.data ? rules.data.rules_checked : 0,
      skipped: rules.data ? rules.data.rules_skipped : 0,
    };

    nodes.push(suggestions([
      { label: "Convert to SWIFT MT", run: function () { showConversion(intent); } },
      { label: "Show as JSON", run: function () { showJSON(intent); } },
    ]));
    answer(nodes);
    renderStudio();
    renderSources(intent);

    validateAgainstCatalogue(intent);
  }

  // Schema validation, when the visitor has the schema.
  //
  // The page said it would validate "if your catalogue has the schema" and then
  // never did: it linted, applied the rules, and stopped. Saying a check ran
  // when it did not is the one failure a validator cannot afford, so it now
  // runs, and says plainly when it could not.
  //
  // Resolving a schema reads files from disk, which is asynchronous, so this
  // appends to the answer already on screen rather than holding it back.
  function validateAgainstCatalogue(intent) {
    var cat = window.askisoCatalogue;
    var out = $("#ws-answer");
    if (!out) return;

    var slot = el("div", "ws-schema");
    out.appendChild(slot);

    if (!cat || typeof cat.count !== "function" || cat.count() === 0) {
      slot.appendChild(el("p", "ws-note",
        "Schema validation did not run: no catalogue is open. Choose the folder " +
        "you downloaded from iso20022.org in Sources, and this message will be " +
        "checked against its schema too."));
      return;
    }

    slot.appendChild(el("p", "ws-note", "Checking against the schema in your catalogue…"));

    Promise.resolve(cat.schemaFor(intent.text)).then(function (xsd) {
      slot.innerHTML = "";

      if (!xsd) {
        var id = cat.messageIDFrom(intent.text) || "this message";
        slot.appendChild(el("p", "ws-note",
          "Your catalogue has no schema for " + id + ", so schema validation " +
          "did not run. Everything above still applies."));
        return;
      }

      var res = call("validate", intent.text, xsd);
      if (res.error) {
        slot.appendChild(el("p", "ws-note", res.error));
        return;
      }

      var d = res.data || {};
      var errors = d.errors || [];
      slot.appendChild(el("h3", null, "Schema"));
      slot.appendChild(d.valid
        ? verdict(true, "Valid against the schema in your catalogue.")
        : verdict(false, errors.length + " schema error(s)."));
      if (errors.length) {
        slot.appendChild(findingList(errors));
      }

      lastResult.schema = { valid: d.valid, errors: errors };
      renderStudio();
      renderSources(intent);
    }).catch(function (e) {
      slot.innerHTML = "";
      slot.appendChild(el("p", "ws-note",
        "The schema could not be read: " + (e && e.message ? e.message : e)));
    });
  }

  // Convert whatever is in the box to the other format, in whichever direction
  // that is. The engine decides from the payload; the page does not need to.
  function showConversion(intent) {
    var res = call("convertMT", intent.text);
    var nodes = [];

    if (res.error) {
      nodes.push(verdict(false, res.error));
      nodes.push(suggestions([
        { label: "Back to the findings", run: function () { handleMX(intent); } },
      ]));
      answer(nodes);
      return;
    }

    var d = res.data || {};
    var target = d.target_type || "the other format";
    nodes.push(verdict(true, "Converted " +
      (d.source_type ? d.source_type + " to " : "to ") + target + "."));
    nodes.push(codeBlock(d.xml || ""));

    if (d.report && d.report.length) {
      nodes.push(el("h3", null, "What survived the conversion"));
      nodes.push(fidelityList(d.report));
    }

    lastResult = { kind: "mt", input: intent.text, xml: d.xml, report: d.report };
    nodes.push(suggestions([
      { label: "Back to the findings", run: function () { handleMX(intent); } },
    ]));
    answer(nodes);
    renderStudio();
    renderSources(intent);
  }

  function showJSON(intent) {
    var res = call("toJSON", intent.text);
    var nodes = [];

    if (res.error) {
      nodes.push(verdict(false, res.error));
      nodes.push(suggestions([
        { label: "Back to the findings", run: function () { handleMX(intent); } },
      ]));
      answer(nodes);
      return;
    }

    var json = (res.data && res.data.json) || "";
    nodes.push(verdict(true, "The same message, as JSON."));
    nodes.push(codeBlock(json));

    lastResult = { kind: "json", input: intent.text, json: json };
    nodes.push(suggestions([
      { label: "Back to the findings", run: function () { handleMX(intent); } },
    ]));
    answer(nodes);
    renderStudio();
    renderSources(intent);
  }

  // The fidelity report, rendered the same way wherever a conversion is shown.
  function fidelityList(report) {
    return findingList(report.map(function (r) {
      return {
        rule: r.fidelity || r.status,
        message: r.tag + (r.note ? " — " + r.note : ""),
        path: r.path,
      };
    }));
  }

  function handleMT(intent) {
    var res = call("convertMT", intent.text);
    var nodes = [];

    if (res.error) {
      nodes.push(verdict(false, res.error));
      answer(nodes);
      return;
    }
    var d = res.data || {};

    // target_type is what the engine actually returns. This read d.target and
    // d.message_id, neither of which exists, so every conversion claimed to
    // have produced "ISO 20022" whichever direction it went.
    nodes.push(verdict(true, "Converted " +
      (d.source_type ? d.source_type + " to " : "to ") +
      (d.target_type || "ISO 20022") + "."));
    nodes.push(codeBlock(d.xml || ""));

    if (d.report && d.report.length) {
      nodes.push(el("h3", null, "What survived the conversion"));
      nodes.push(fidelityList(d.report));
    }

    lastResult = { kind: "mt", input: intent.text, xml: d.xml, report: d.report };
    answer(nodes);
    renderStudio();
    renderSources(intent);
  }

  function handleMessageID(intent) {
    var res = call("info", intent.id);
    var nodes = [];

    if (res.error) {
      nodes.push(verdict(false, res.error));
      answer(nodes);
      return;
    }
    var res2 = res.data || {};

    nodes.push(verdict(true, res2.id + " — " + (res2.domain_name || res2.domain)));

    var dl = el("dl", "ws-facts");
    [["Business area", res2.domain_name], ["Base code", res2.base_code],
     ["Version", res2.version],
     ["Schema installed", res2.installed ? "yes" : "no"],
     ["Published in", (res2.message_sets || []).map(function (m) { return m.name; }).join(", ")]]
      .forEach(function (pair) {
        if (!pair[1]) return;
        dl.appendChild(el("dt", null, pair[0]));
        dl.appendChild(el("dd", null, String(pair[1])));
      });
    nodes.push(dl);

    nodes.push(el("p", "ws-note",
      "AskISO holds the index, not the specification. The message definition " +
      "and schema are published by the Registration Authority."));

    lastResult = { kind: "info", info: res2 };
    answer(nodes);
    renderStudio();
    renderSources(intent);
  }

  function handleCheck(intent) {
    var fn = { iban: "checkIBAN", bic: "checkBIC", uetr: "checkUETR" }[intent.kind];
    var res = call(fn, intent.value);
    var d = res.data || {};
    var ok = !!d.valid;

    answer([
      verdict(ok, intent.value + (ok ? " is valid." : " is not valid.")),
      d.reason ? el("p", "ws-note", d.reason) : null,
      res.error ? el("p", "ws-note", res.error) : null,
    ]);
    lastResult = { kind: intent.kind, value: intent.value, result: d };
    renderStudio();
  }

  function handleSearch(intent) {
    var res = call("search", intent.query);
    var hits = res.data || [];
    var nodes = [];

    if (!hits.length) {
      nodes.push(verdict(false, "Nothing matches “" + intent.query + "”."));
      // Being specific about what search covers is more useful than a shrug:
      // it matches identifiers, domains and business areas, not free text, so
      // "structured address" finds nothing however reasonable the question is.
      nodes.push(el("p", "ws-note",
        "This box matches message identifiers, domains and business areas — it " +
        "does not answer questions in words. Try a code:"));
      nodes.push(suggestions([
        { label: "pacs.008", input: "pacs.008" },
        { label: "camt.053", input: "camt.053" },
        { label: "pain.001", input: "pain.001" },
      ]));
      // A question asked in words is a reasonable thing to do, so it lands
      // somewhere useful rather than at a dead end.
      var more = el("p", "ws-note");
      more.appendChild(document.createTextNode("Asking a question in words? The "));
      var faq = el("a", null, "frequently asked questions");
      faq.href = "../faq/";
      more.appendChild(faq);
      more.appendChild(document.createTextNode(" cover the November 2026 change, privacy and schemas, and the "));
      var ref = el("a", null, "message reference");
      ref.href = "../messages/";
      more.appendChild(ref);
      more.appendChild(document.createTextNode(" lists every definition by business area."));
      nodes.push(more);
      answer(nodes);
      return;
    }

    nodes.push(verdict(true, hits.length + " message definition(s) match."));
    var list = el("ul", "ws-hits");
    hits.slice(0, 25).forEach(function (h) {
      var li = el("li");
      var b = el("button", "ws-link", h.id);
      b.type = "button";
      b.addEventListener("click", function () {
        $("#ws-input").value = h.id;
        submit();
      });
      li.appendChild(b);
      li.appendChild(el("span", "ws-msg", h.domain_name || h.domain || ""));
      list.appendChild(li);
    });
    nodes.push(list);
    if (hits.length > 25) {
      nodes.push(el("p", "ws-note", "Showing the first 25 of " + hits.length + "."));
    }

    lastResult = { kind: "search", query: intent.query, hits: hits };
    answer(nodes);
    renderStudio();
  }

  // ---------------------------------------------------------------------
  // Sources and Studio
  // ---------------------------------------------------------------------

  function renderSources(intent) {
    var box = $("#ws-sources");
    if (!box) return;
    box.innerHTML = "";

    var cat = window.askisoCatalogue;
    var count = cat ? cat.count() : 0;

    var item = el("div", "ws-source");
    item.appendChild(el("span", "ws-source-kind", "Catalogue"));
    item.appendChild(el("span", "ws-source-name",
      count ? count + " schemas from " + cat.name() : "None selected"));
    item.appendChild(el("span", "ws-source-note",
      count ? "Read from your disk. Never uploaded."
            : "Full schema validation needs the schemas you downloaded."));
    box.appendChild(item);

    if (intent && (intent.kind === "mx" || intent.kind === "mt")) {
      var doc = el("div", "ws-source");
      doc.appendChild(el("span", "ws-source-kind",
        intent.kind === "mt" ? "MT message" : "ISO 20022 message"));
      doc.appendChild(el("span", "ws-source-name",
        intent.messageID || (intent.kind === "mt" ? "SWIFT MT" : "unknown message")));
      doc.appendChild(el("span", "ws-source-note",
        (intent.text || "").length.toLocaleString() + " characters, in this tab only"));
      box.appendChild(doc);
    }
  }

  // The pack itself lives in evidence.js: it is a pure function of the run, and
  // keeping it that way is what lets the smoke test check its wording.
  function evidencePack() {
    if (!window.askisoEvidence) {
      return "The evidence pack could not be built: evidence.js did not load.";
    }
    var v = call("version");
    var version = (v.data && (v.data.version || v.data)) || "unknown version";
    var cat = window.askisoCatalogue;
    var hasCatalogue = !!(cat && typeof cat.count === "function" && cat.count() > 0);
    return window.askisoEvidence.pack(lastResult, version, hasCatalogue);
  }

  // Studio turns a result into something that leaves the page: a report to
  // attach to a ticket, a file for a pipeline. Nothing here is generated
  // server-side, so what is offered depends entirely on what was just run.
  function renderStudio() {
    var box = $("#ws-studio");
    if (!box) return;
    box.innerHTML = "";

    if (!lastResult) {
      box.appendChild(el("p", "ws-note", "Run something and the outputs appear here."));
      return;
    }

    var offers = [];

    if (lastResult.kind === "mx") {
      var findings = (lastResult.issues || []).concat(lastResult.rules || []);
      offers.push({
        label: "Findings as JSON",
        note: findings.length + " finding(s)",
        name: "askiso-findings.json",
        type: "application/json",
        body: function () { return JSON.stringify(findings, null, 2); },
      });

      // SARIF is what a code-scanning pipeline reads. It is rendered by the
      // same engine the CLI uses -- the report downloaded here matches what
      // `askiso lint --format sarif` writes for the same message, which is
      // worth claiming only because it is checked.
      offers.push({
        label: "SARIF 2.1.0",
        note: "for GitHub code scanning or any SARIF pipeline",
        name: "askiso.sarif",
        type: "application/sarif+json",
        body: function () {
          var res = call("sarif", lastResult.input, lastResult.profile);
          return res.error ? JSON.stringify({ error: res.error }, null, 2) : res.data;
        },
      });

      // The evidence pack is for a human: a ticket, a review, an email to the
      // bank asking why a payment was returned. It restates the finding in the
      // order somebody would need to act on it, and names what was NOT checked,
      // because an absent schema is the difference between "clean" and
      // "nothing contradicted it".
      offers.push({
        label: "Evidence pack",
        note: "Markdown, for a ticket or a review",
        name: "askiso-evidence.md",
        type: "text/markdown",
        body: function () { return evidencePack(); },
      });
    }

    if (lastResult.kind === "mt" && lastResult.xml) {
      offers.push({
        label: "Converted message",
        note: "ISO 20022 XML",
        name: "converted.xml",
        type: "application/xml",
        body: function () { return lastResult.xml; },
      });
      offers.push({
        label: "Fidelity report",
        note: "what was mapped, derived, truncated or lost",
        name: "fidelity.json",
        type: "application/json",
        body: function () { return JSON.stringify(lastResult.report || [], null, 2); },
      });
    }

    if (lastResult.kind === "json" && lastResult.json) {
      offers.push({
        label: "The message as JSON",
        note: "the same content, in a shape most tooling reads more easily",
        name: "message.json",
        type: "application/json",
        body: function () { return lastResult.json; },
      });
    }

    if (lastResult.kind === "info") {
      offers.push({
        label: "Message metadata",
        note: "JSON",
        name: lastResult.info.id + ".json",
        type: "application/json",
        body: function () { return JSON.stringify(lastResult.info, null, 2); },
      });
    }

    if (!offers.length) {
      box.appendChild(el("p", "ws-note", "Nothing to export from that result."));
      return;
    }

    offers.forEach(function (o) {
      var card = el("div", "ws-offer");
      card.appendChild(el("span", "ws-offer-label", o.label));
      card.appendChild(el("span", "ws-offer-note", o.note));

      var row = el("div", "ws-offer-actions");
      var copy = el("button", "ws-chip", "Copy");
      copy.type = "button";
      copy.addEventListener("click", function () {
        navigator.clipboard.writeText(o.body()).then(function () {
          copy.textContent = "Copied";
          window.setTimeout(function () { copy.textContent = "Copy"; }, 1400);
        });
      });
      row.appendChild(copy);

      var dl = el("a", "ws-chip", "Download");
      dl.href = URL.createObjectURL(new Blob([o.body()], { type: o.type }));
      dl.download = o.name;
      row.appendChild(dl);

      card.appendChild(row);
      box.appendChild(card);
    });
  }

  // ---------------------------------------------------------------------

  // Set when somebody asks before the engine has finished loading, so the
  // request runs the moment it can rather than being dropped on the floor.
  var pendingSubmit = false;

  function submit() {
    if (!engineReady) {
      pendingSubmit = true;
      if (typeof window.askisoStartEngine === "function") {
        window.askisoStartEngine();
      }
      var loading = $("#ws-intent");
      if (loading) {
        loading.textContent = "Loading the engine — this will run as soon as it is ready.";
      }
      return;
    }

    var intent = classify($("#ws-input").value);
    var hint = $("#ws-intent");

    if (intent.kind === "empty") {
      hint.textContent = "";
      answer([el("p", "ws-note", "Paste a message, or type a message identifier.")]);
      return;
    }
    hint.textContent = describe(intent);

    switch (intent.kind) {
      case "mx": handleMX(intent); break;
      case "mt": handleMT(intent); break;
      case "message-id": handleMessageID(intent); break;
      case "iban":
      case "bic":
      case "uetr": handleCheck(intent); break;
      default: handleSearch(intent);
    }
  }

  function init() {
    var input = $("#ws-input");
    if (!input) return;

    $("#ws-go").addEventListener("click", submit);

    // Enter submits; Shift+Enter is a newline, because the input is usually a
    // pasted multi-line message.
    input.addEventListener("keydown", function (e) {
      if (e.key === "Enter" && !e.shiftKey) {
        e.preventDefault();
        submit();
      }
    });

    // The intent line updates as they type, so the dispatch is never a
    // surprise at the moment they press the button.
    input.addEventListener("input", function () {
      $("#ws-intent").textContent = describe(classify(input.value));
    });

    document.querySelectorAll("[data-ws-example]").forEach(function (b) {
      b.addEventListener("click", function () {
        input.value = b.getAttribute("data-ws-example");
        submit();
      });
    });

    renderSources(null);
    renderStudio();
  }

  window.askisoReady = function () {
    engineReady = true;
    var s = $("#ws-status");
    if (s) {
      s.textContent = "Engine ready — running locally in this tab.";
      s.className = "ws-status is-ready";
    }
    renderSources(null);

    if (pendingSubmit) {
      pendingSubmit = false;
      submit();
    }
  };

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})();
