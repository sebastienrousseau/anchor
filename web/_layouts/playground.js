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
  var statusEl = $("#status");
  var ready = false;

  window.askisoReady = function () {
    ready = true;
    statusEl.textContent = "Engine ready — light mode (embedded index of the standard; no schemas installed)";
    statusEl.className = "ready";
  };

  function requireEngine() {
    if (!ready) {
      statusEl.textContent = "Engine still loading — try again in a moment.";
      return false;
    }
    return true;
  }

  function call(fn) {
    var args = Array.prototype.slice.call(arguments, 1);
    try {
      return window.askiso[fn].apply(null, args);
    } catch (e) {
      return { ok: false, error: String(e && e.message ? e.message : e) };
    }
  }

  if (!WebAssembly || !WebAssembly.instantiateStreaming) {
    statusEl.textContent = "This browser does not support WebAssembly streaming.";
    statusEl.className = "error";
  } else {
    var go = new Go();
    WebAssembly.instantiateStreaming(fetch("/askiso.wasm"), go.importObject)
      .then(function (res) { go.run(res.instance); })
      .catch(function (err) {
        statusEl.textContent = "Could not load the engine: " + err;
        statusEl.className = "error";
      });
  }

  // ---- shared renderers -------------------------------------------------
  function showError(el, msg) {
    el.innerHTML = '<div class="verdict fail">' + esc(msg) + "</div>";
  }
  function showCode(el, text) {
    el.innerHTML = '<pre class="code">' + esc(text) + "</pre>";
  }

  // ---- lint -------------------------------------------------------------
  var SAMPLE_CACHE = null;
  function sampleXML(cb) {
    if (SAMPLE_CACHE) return cb(SAMPLE_CACHE);
    var r = call("generate", "pacs.008", "sepa", "25000.00", "EUR", false);
    if (r.ok) { SAMPLE_CACHE = r.data.xml; cb(SAMPLE_CACHE); }
  }

  $("#lint-fill").addEventListener("click", function () {
    if (!requireEngine()) return;
    sampleXML(function (xml) { $("#lint-in").value = xml; });
  });

  $("#lint-go").addEventListener("click", function () {
    if (!requireEngine()) return;
    var out = $("#lint-out");
    var r = call("lint", $("#lint-in").value, "message.xml");
    if (!r.ok) return showError(out, r.error);

    var d = r.data;
    var issues = d.issues || [];
    var cls = d.error_count > 0 ? "fail" : (d.warning_count > 0 ? "warn" : "pass");
    var verdict = d.error_count > 0
      ? d.error_count + " error" + (d.error_count === 1 ? "" : "s") + " found"
      : (d.warning_count > 0 ? d.warning_count + " warning(s)" : "All checks passed");

    var html = '<div class="verdict ' + cls + '">' + esc(verdict) +
      ' &middot; <span class="fw-normal">' + d.passed_count + " check(s) passed</span></div>";

    if (issues.length) {
      html += '<ul class="issues">';
      issues.forEach(function (i) {
        var chip = i.severity === "ERROR" ? "err" : (i.severity === "WARNING" ? "warnc" : "info");
        html += "<li>" +
          '<div class="issue-head"><span class="chip ' + chip + '">' + esc(i.severity) + "</span>" +
          '<span class="issue-rule">' + esc(i.rule) + "</span></div>" +
          '<div class="issue-msg">' + esc(i.message) + "</div>" +
          '<div class="issue-val">' + esc(i.field) + " = " + esc(i.value) + "</div></li>";
      });
      html += "</ul>";
    }
    out.innerHTML = html;
  });

  // ---- validate ---------------------------------------------------------
  var DEMO_XSD = [
    '<?xml version="1.0" encoding="UTF-8"?>',
    '<xs:schema xmlns="urn:demo" xmlns:xs="http://www.w3.org/2001/XMLSchema"',
    '           elementFormDefault="qualified" targetNamespace="urn:demo">',
    '  <xs:element name="Document" type="Document"/>',
    '  <xs:complexType name="Document">',
    '    <xs:sequence>',
    '      <xs:element name="MsgId" type="Max35Text"/>',
    '      <xs:element name="Amt" type="Amount"/>',
    '      <xs:element name="Sts" type="StatusCode" minOccurs="0"/>',
    '    </xs:sequence>',
    '  </xs:complexType>',
    '  <xs:complexType name="Amount">',
    '    <xs:simpleContent>',
    '      <xs:extension base="xs:decimal">',
    '        <xs:attribute name="Ccy" type="CurrencyCode" use="required"/>',
    '      </xs:extension>',
    '    </xs:simpleContent>',
    '  </xs:complexType>',
    '  <xs:simpleType name="CurrencyCode">',
    '    <xs:restriction base="xs:string"><xs:pattern value="[A-Z]{3,3}"/></xs:restriction>',
    '  </xs:simpleType>',
    '  <xs:simpleType name="Max35Text">',
    '    <xs:restriction base="xs:string">',
    '      <xs:minLength value="1"/><xs:maxLength value="35"/>',
    '    </xs:restriction>',
    '  </xs:simpleType>',
    '  <xs:simpleType name="StatusCode">',
    '    <xs:restriction base="xs:string">',
    '      <xs:enumeration value="ACCP"/><xs:enumeration value="RJCT"/>',
    '    </xs:restriction>',
    '  </xs:simpleType>',
    '</xs:schema>'
  ].join("\n");

  var DEMO_XML = [
    '<?xml version="1.0" encoding="UTF-8"?>',
    '<Document xmlns="urn:demo">',
    '  <MsgId>MSG-0001</MsgId>',
    '  <Amt Ccy="EURO">25000.00</Amt>',
    '  <Sts>MAYBE</Sts>',
    '</Document>'
  ].join("\n");

  $("#val-demo").addEventListener("click", function () {
    $("#val-xml").value = DEMO_XML;
    $("#val-xsd").value = DEMO_XSD;
  });

  $("#val-go").addEventListener("click", function () {
    if (!requireEngine()) return;
    var out = $("#val-out");
    var r = call("validate", $("#val-xml").value, $("#val-xsd").value);
    if (!r.ok) return showError(out, r.error);

    var d = r.data;
    if (d.valid) {
      out.innerHTML = '<div class="verdict pass">Valid — the message conforms to the schema</div>';
      return;
    }

    var errs = d.errors || [];
    var html = '<div class="verdict fail">' + errs.length +
      " schema error" + (errs.length === 1 ? "" : "s") + "</div>" +
      '<div class="tablewrap"><table><thead><tr>' +
      "<th>Line</th><th>Rule</th><th>Path</th><th>Expected</th><th>Found</th><th>Message</th>" +
      "</tr></thead><tbody>";
    errs.forEach(function (e) {
      html += "<tr>" +
        '<td class="mono">' + e.line + ":" + e.column + "</td>" +
        '<td><span class="chip err">' + esc(e.rule) + "</span></td>" +
        '<td class="mono wrap-normal">' + esc(e.path) + "</td>" +
        '<td class="mono">' + esc(e.expected || "\u2014") + "</td>" +
        '<td class="mono">' + esc(e.actual || "\u2014") + "</td>" +
        "<td>" + esc(e.message) + "</td></tr>";
    });
    out.innerHTML = html + "</tbody></table></div>";
  });

  // ---- November 2026 address readiness ----------------------------------
  var ADDR_BAD = [
    '<?xml version="1.0" encoding="UTF-8"?>',
    '<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">',
    '  <CdtTrfTxInf>',
    '    <Dbtr>',
    '      <Nm>Acme Ltd</Nm>',
    '      <PstlAdr>',
    '        <AdrLine>12 High Street</AdrLine>',
    '        <AdrLine>London</AdrLine>',
    '        <AdrLine>United Kingdom</AdrLine>',
    '      </PstlAdr>',
    '    </Dbtr>',
    '    <Cdtr>',
    '      <Nm>Globex SA</Nm>',
    '      <PstlAdr>',
    '        <StrtNm>Rue de la Paix</StrtNm>',
    '        <TwnNm>Paris</TwnNm>',
    '        <Ctry>France</Ctry>',
    '      </PstlAdr>',
    '    </Cdtr>',
    '  </CdtTrfTxInf>',
    '</Document>'
  ].join("\n");

  var ADDR_OK = [
    '<?xml version="1.0" encoding="UTF-8"?>',
    '<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">',
    '  <CdtTrfTxInf>',
    '    <Dbtr>',
    '      <Nm>Acme Ltd</Nm>',
    '      <PstlAdr>',
    '        <StrtNm>High Street</StrtNm>',
    '        <BldgNb>12</BldgNb>',
    '        <PstCd>EC1A 1BB</PstCd>',
    '        <TwnNm>London</TwnNm>',
    '        <Ctry>GB</Ctry>',
    '      </PstlAdr>',
    '    </Dbtr>',
    '    <Cdtr>',
    '      <Nm>Globex SA</Nm>',
    '      <PstlAdr>',
    '        <AdrLine>Rue de la Paix 5</AdrLine>',
    '        <TwnNm>Paris</TwnNm>',
    '        <Ctry>FR</Ctry>',
    '      </PstlAdr>',
    '    </Cdtr>',
    '  </CdtTrfTxInf>',
    '</Document>'
  ].join("\n");

  $("#addr-demo").addEventListener("click", function () { $("#addr-in").value = ADDR_BAD; });
  $("#addr-demo-ok").addEventListener("click", function () { $("#addr-in").value = ADDR_OK; });

  var SHAPE_CHIP = { structured: "ok", hybrid: "warnc", unstructured: "err", empty: "info" };

  $("#addr-go").addEventListener("click", function () {
    if (!requireEngine()) return;
    var out = $("#addr-out");
    var payload = $("#addr-in").value;

    var r = call("checkRules", payload, "cbpr-2026");
    if (!r.ok) return showError(out, r.error);
    var d = r.data;

    var html = "";
    if (d.rules_checked === 0) {
      html += '<div class="verdict pass">This message type is out of scope for the address requirement &mdash; ' +
        d.rules_skipped + " rule(s) skipped.</div>";
    } else if (d.error_count === 0) {
      html += '<div class="verdict pass">Ready &mdash; all ' + d.rules_checked +
        " address rule(s) passed.</div>";
    } else {
      html += '<div class="verdict fail">Not ready &mdash; ' + d.error_count +
        " blocking issue" + (d.error_count === 1 ? "" : "s") + " before 14 November 2026.</div>";
    }

    // Shape of every address in the message.
    var shapes = call("addresses", payload);
    if (shapes.ok && shapes.data.length) {
      html += '<div class="tablewrap mb-block"><table><thead><tr>' +
        "<th>Address</th><th>Shape</th><th>Status</th></tr></thead><tbody>";
      shapes.data.forEach(function (a) {
        var chip = SHAPE_CHIP[a.shape] || "info";
        var note = a.shape === "unstructured" ? "Rejected from Nov 2026"
          : a.shape === "hybrid" ? "Accepted, no end date"
          : a.shape === "structured" ? "Fully compliant" : "Nothing to check";
        html += "<tr>" +
          '<td class="mono wrap-normal">' + esc(a.path) + "</td>" +
          '<td><span class="chip ' + chip + '">' + esc(a.shape) + "</span></td>" +
          "<td>" + esc(note) + "</td></tr>";
      });
      html += "</tbody></table></div>";
    }

    var findings = d.findings || [];
    if (findings.length) {
      html += '<ul class="issues">';
      findings.forEach(function (f) {
        var chip = f.severity === "ERROR" ? "err" : (f.severity === "WARNING" ? "warnc" : "info");
        html += "<li>" +
          '<div class="issue-head"><span class="chip ' + chip + '">' + esc(f.severity) + "</span>" +
          '<span class="issue-rule">' + esc(f.rule_id) + " &middot; " + esc(f.rule) + "</span></div>" +
          '<div class="issue-msg">' + esc(f.message) + "</div>" +
          '<div class="issue-val">at ' + esc(f.path) + "</div>";
        if (f.expected) html += '<div class="issue-val">expected ' + esc(f.expected) + "</div>";
        if (f.remediation) html += '<div class="issue-msg mt-tight">' + esc(f.remediation) + "</div>";
        html += "</li>";
      });
      html += "</ul>";
    }
    out.innerHTML = html;
  });

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
        ? '<div class="verdict fail">Generated, but AskIso’s own linter reports ' + d.error_count + ' error(s) — a known defect for this preset.</div>'
        : '<div class="verdict pass">Generated and passes all ' + d.passed_count + " business-rule checks</div>";
    }
    out.innerHTML = banner + '<pre class="code">' + esc(r.data.xml) + "</pre>";
  });

  // ---- browse -----------------------------------------------------------
  function runBrowse() {
    if (!requireEngine()) return;
    var out = $("#browse-out");
    var r = call("search", $("#browse-q").value);
    if (!r.ok) return showError(out, r.error);

    var rows = r.data || [];
    if (!rows.length) {
      out.innerHTML = '<p class="empty">No message matches that query.</p>';
      return;
    }

    var html = '<p class="hint">' + rows.length + " result(s)</p>" +
      '<div class="tablewrap"><table><thead><tr>' +
      "<th>Identifier</th><th>Business area</th><th>Message set</th><th>Official download</th>" +
      "</tr></thead><tbody>";

    rows.slice(0, 60).forEach(function (m) {
      var set = (m.message_sets && m.message_sets[0]) || null;
      html += "<tr>" +
        '<td class="mono">' + esc(m.id) + "</td>" +
        "<td>" + esc(m.domain_name || m.domain) + "</td>" +
        "<td>" + (set ? esc(set.name + " " + set.version) : "&mdash;") + "</td>" +
        "<td>" + (set
          ? '<a href="' + esc(set.url) + '" rel="noopener">iso20022.org &#8599;</a>'
          : "&mdash;") + "</td></tr>";
    });
    html += "</tbody></table></div>";
    if (rows.length > 60) html += '<p class="hint mt-loose">Showing the first 60 of ' + rows.length + ".</p>";
    out.innerHTML = html;
  }

  $("#browse-go").addEventListener("click", runBrowse);
  $("#browse-q").addEventListener("keydown", function (e) { if (e.key === "Enter") runBrowse(); });

  // ---- convert ----------------------------------------------------------
  $("#conv-fill").addEventListener("click", function () {
    if (!requireEngine()) return;
    sampleXML(function (xml) { $("#conv-in").value = xml; });
  });
  $("#conv-go").addEventListener("click", function () {
    if (!requireEngine()) return;
    var out = $("#conv-out");
    var r = call("toJSON", $("#conv-in").value);
    if (!r.ok) return showError(out, r.error);
    showCode(out, r.data.json);
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

  // ---- translate --------------------------------------------------------
  function renderMapping(m) {
    var html = '<div class="verdict pass">' + esc(m.MTCode) + " &rarr; " + esc(m.MXCode) + "</div>" +
      '<p class="hint">' + esc(m.Description) + "</p>" +
      '<div class="tablewrap"><table><thead><tr><th>MT tag</th><th>MX element path</th><th>Notes</th></tr></thead><tbody>';
    (m.FieldMaps || []).forEach(function (f) {
      html += "<tr>" +
        '<td class="mono">' + esc(f.MTTag) + "</td>" +
        '<td class="mono">' + esc(f.MXPath) + "</td>" +
        "<td>" + esc(f.Comments) + "</td></tr>";
    });
    return html + "</tbody></table></div>";
  }

  $("#tr-go").addEventListener("click", function () {
    if (!requireEngine()) return;
    var out = $("#tr-out");
    var r = call("translate", $("#tr-q").value);
    if (!r.ok) return showError(out, r.error);
    out.innerHTML = renderMapping(r.data);
  });

  $("#tr-all").addEventListener("click", function () {
    if (!requireEngine()) return;
    var out = $("#tr-out");
    var r = call("translate", "");
    if (!r.ok) return showError(out, r.error);
    var html = '<div class="tablewrap"><table><thead><tr><th>SWIFT MT</th><th>ISO 20022 MX</th><th>Purpose</th></tr></thead><tbody>';
    (r.data || []).forEach(function (m) {
      html += "<tr>" +
        '<td class="mono">' + esc(m.MTCode) + "</td>" +
        '<td class="mono">' + esc(m.MXCode) + "</td>" +
        "<td>" + esc(m.MTTitle) + "</td></tr>";
    });
    out.innerHTML = html + "</tbody></table></div>";
  });

  // ---- MT to MX conversion ----------------------------------------------
  var SAMPLE_MT103 = [
    "{1:F01BANKGB2LAXXX0000000000}{2:I103BANKDEFFXXXXN}{3:{121:f3a1b2c4-5d6e-4f70-8a91-2b3c4d5e6f70}}{4:",
    ":20:REF20260824001",
    ":23B:CRED",
    ":32A:260824EUR25000,00",
    ":50K:/GB29NWBK60161331926819",
    "ACME TRADING LIMITED",
    "14 GRESHAM STREET",
    "LONDON EC2V 7NN",
    ":52A:BANKGB2LXXX",
    ":57A:BANKDEFFXXX",
    ":59:/DE89370400440532013000",
    "MUELLER GMBH",
    "HAUPTSTRASSE 12",
    "60311 FRANKFURT AM MAIN",
    ":70:INVOICE 2026-0815 CONSULTING SERVICES",
    ":71A:SHA",
    "-}{5:{CHK:123456789ABC}}"
  ].join("\n");

  $("#mt-fill").addEventListener("click", function () {
    $("#mt-in").value = SAMPLE_MT103;
  });

  $("#mt-go").addEventListener("click", function () {
    if (!requireEngine()) return;
    var out = $("#mt-out");
    var r = call("convertMT", $("#mt-in").value);
    if (!r.ok) return showError(out, r.error);

    var d = r.data;
    var cls = d.lossless ? "pass" : "warn";
    var verdict = "MT" + d.source_type + " \u2192 " + d.target_type +
      (d.lossless ? " \u00b7 every field carried intact" : " \u00b7 lossy conversion");

    var html = '<div class="verdict ' + cls + '">' + esc(verdict) + "</div>" +
      '<p class="hint">' + d.mapped + " mapped &middot; " + d.derived + " derived &middot; " +
      d.truncated + " truncated &middot; " + d.unmapped + " unmapped</p>" +
      '<div class="tablewrap"><table><thead><tr><th>MT field</th><th>Fidelity</th><th>Target</th><th>Note</th></tr></thead><tbody>';

    (d.report || []).forEach(function (f) {
      var chip = f.fidelity === "unmapped" ? "err"
        : (f.fidelity === "mapped" ? "info" : "warnc");
      html += "<tr>" +
        '<td class="mono">:' + esc(f.tag) + ":</td>" +
        '<td><span class="chip ' + chip + '">' + esc(f.fidelity) + "</span></td>" +
        '<td class="mono">' + esc(f.path || "") + "</td>" +
        "<td>" + esc(f.note || "") + "</td></tr>";
    });
    html += "</tbody></table></div>";

    if (!d.lossless) {
      html += '<p class="hint">MT addresses are unstructured. Paste the result into the ' +
        "<strong>Nov 2026</strong> tab to see exactly which elements CBPR+ will reject " +
        "from 14 November 2026.</p>";
    }

    html += '<pre class="code">' + esc(d.xml) + "</pre>";
    out.innerHTML = html;
  });

  // ---- field validators -------------------------------------------------
  $$("[data-check]").forEach(function (btn) {
    btn.addEventListener("click", function () {
      if (!requireEngine()) return;
      var kind = btn.dataset.check;
      var map = { iban: ["checkIBAN", "#chk-iban", "IBAN"], bic: ["checkBIC", "#chk-bic", "BIC"], uetr: ["checkUETR", "#chk-uetr", "UETR"] };
      var spec = map[kind];
      var value = $(spec[1]).value;
      var r = call(spec[0], value);
      var out = $("#chk-out");
      if (!r.ok) return showError(out, r.error);
      out.innerHTML = r.data.valid
        ? '<div class="verdict pass">' + esc(spec[2]) + " is valid &middot; <span class=\"fw-normal\">" + esc(value) + "</span></div>"
        : '<div class="verdict fail">' + esc(spec[2]) + " is invalid &mdash; " + esc(r.data.reason) + "</div>";
    });
  });

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
