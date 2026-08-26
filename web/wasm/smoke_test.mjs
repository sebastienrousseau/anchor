// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Smoke test for the WebAssembly build.
//
// The website and the CLI share pkg/iso20022, so this checks that the Go/JS
// bridge actually carries that behaviour across intact -- an API renamed in Go
// or a field that stops serialising would break the site silently otherwise.
//
//   make web && node web/wasm/smoke_test.mjs
//
// Exits non-zero on the first failure.

import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const siteDir = path.join(path.dirname(fileURLToPath(import.meta.url)), "..", "public");

globalThis.performance ||= { now: () => Date.now() };

new Function(fs.readFileSync(path.join(siteDir, "wasm_exec.js"), "utf8"))();

const go = new globalThis.Go();
const { instance } = await WebAssembly.instantiate(
  fs.readFileSync(path.join(siteDir, "askiso.wasm")),
  go.importObject,
);

let readyFired = false;
globalThis.askisoReady = () => {
  readyFired = true;
};

go.run(instance);
await new Promise((r) => setTimeout(r, 200));

const A = globalThis.askiso;

let failures = 0;
function check(name, condition, detail) {
  if (condition) {
    console.log(`  ok    ${name}`);
  } else {
    failures++;
    console.error(`  FAIL  ${name}${detail ? " -- " + detail : ""}`);
  }
}

console.log("askiso wasm smoke test\n");

// --- bridge -----------------------------------------------------------------
check("ready callback fires", readyFired);
check("global askiso object exists", typeof A === "object");

const expected = [
  "checkBIC", "checkIBAN", "checkUETR", "codes", "diagram", "generate",
  "info", "lifecycle", "lint", "search", "sets", "stats", "toJSON",
  "addresses", "checkRules", "profiles", "sarif", "translate", "validate", "version",
];
const missing = expected.filter((k) => typeof A?.[k] !== "function");
check(`all ${expected.length} API functions exported`, missing.length === 0, `missing: ${missing}`);

// --- generate + lint --------------------------------------------------------
const gen = A.generate("pacs.008", "sepa", "25000.00", "", false);
check("generate returns a message", gen.ok, gen.error);
check(
  "generated message declares the pacs.008 namespace",
  gen.ok && gen.data.xml.includes("urn:iso:std:iso:20022:tech:xsd:pacs.008"),
);

const clean = A.lint(gen.data.xml, "generated.xml");
check("generated message lints clean", clean.ok && clean.data.error_count === 0,
  clean.ok ? JSON.stringify(clean.data.issues) : clean.error);
check("lint reports the checks it ran", clean.ok && clean.data.passed_count > 0);

// A deliberately broken checksum must be caught, or lint proves nothing.
const badIBAN = gen.data.xml.replace(/<IBAN>DE\d{2}/, "<IBAN>DE00");
const dirty = A.lint(badIBAN, "bad.xml");
check("lint catches a bad IBAN checksum",
  dirty.ok && dirty.data.error_count > 0 &&
    dirty.data.issues.some((i) => i.rule.includes("IBAN")),
  dirty.ok ? `errors=${dirty.data.error_count}` : dirty.error);

// --- schema validation ------------------------------------------------------
// A small self-contained schema, so the test needs no downloaded specification.
const SCHEMA = `<?xml version="1.0" encoding="UTF-8"?>
<xs:schema xmlns="urn:t" xmlns:xs="http://www.w3.org/2001/XMLSchema"
           elementFormDefault="qualified" targetNamespace="urn:t">
  <xs:element name="Document" type="Doc"/>
  <xs:complexType name="Doc">
    <xs:sequence><xs:element name="Ccy" type="Cur"/></xs:sequence>
  </xs:complexType>
  <xs:simpleType name="Cur">
    <xs:restriction base="xs:string"><xs:pattern value="[A-Z]{3,3}"/></xs:restriction>
  </xs:simpleType>
</xs:schema>`;

const good = A.validate(`<Document xmlns="urn:t"><Ccy>EUR</Ccy></Document>`, SCHEMA);
check("valid document passes schema validation", good.ok && good.data.valid,
  good.ok ? JSON.stringify(good.data.errors) : good.error);

const bad = A.validate(`<Document xmlns="urn:t"><Ccy>EURO</Ccy></Document>`, SCHEMA);
check("pattern violation is caught",
  bad.ok && !bad.data.valid && bad.data.errors[0].rule === "pattern",
  bad.ok ? JSON.stringify(bad.data) : bad.error);
check("schema errors carry a path and a line",
  bad.ok && bad.data.errors[0].path.includes("Ccy") && bad.data.errors[0].line > 0,
  bad.ok ? JSON.stringify(bad.data.errors[0]) : bad.error);

const missingSchema = A.validate(gen.data.xml, "");
check("missing schema points at the official download",
  !missingSchema.ok && missingSchema.error.includes("iso20022.org"), missingSchema.error);

// --- scheme rules (the November 2026 address requirement) --------------------
const profiles = A.profiles();
check("profiles are listed", profiles.ok && profiles.data.some((p) => p.name === "cbpr-2026"),
  profiles.error);

const UNSTRUCTURED = `<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">
  <Dbtr><PstlAdr><AdrLine>12 High Street</AdrLine><AdrLine>London</AdrLine></PstlAdr></Dbtr>
</Document>`;
const STRUCTURED = `<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">
  <Dbtr><PstlAdr><StrtNm>High St</StrtNm><TwnNm>London</TwnNm><Ctry>GB</Ctry></PstlAdr></Dbtr>
</Document>`;

const badAddr = A.checkRules(UNSTRUCTURED, "cbpr-2026");
check("unstructured address is rejected",
  badAddr.ok && badAddr.data.error_count > 0 &&
    badAddr.data.findings.some((f) => f.rule_id === "CBPR-ADDR-002"),
  badAddr.ok ? JSON.stringify(badAddr.data.findings?.map((f) => f.rule_id)) : badAddr.error);
check("address findings carry a path and a fix",
  badAddr.ok && badAddr.data.findings[0].path && badAddr.data.findings[0].remediation,
  badAddr.ok ? JSON.stringify(badAddr.data.findings[0]) : badAddr.error);

const goodAddr = A.checkRules(STRUCTURED, "cbpr-2026");
check("structured address passes", goodAddr.ok && goodAddr.data.error_count === 0,
  goodAddr.ok ? JSON.stringify(goodAddr.data.findings) : goodAddr.error);

// camt.053 is out of scope for the address requirement.
const exempt = A.checkRules(UNSTRUCTURED.replace(/pacs\.008\.001\.10/g, "camt.053.001.11"), "cbpr-2026");
check("exempt message types are skipped",
  exempt.ok && exempt.data.error_count === 0 && exempt.data.rules_skipped > 0,
  exempt.ok ? JSON.stringify(exempt.data) : exempt.error);

const shapes = A.addresses(UNSTRUCTURED);
check("addresses are classified",
  shapes.ok && shapes.data[0].shape === "unstructured",
  shapes.ok ? JSON.stringify(shapes.data) : shapes.error);

check("unknown profile is a friendly error", !A.checkRules(STRUCTURED, "nope").ok);

// --- registry ---------------------------------------------------------------
const search = A.search("pacs.008");
check("search finds pacs.008 versions", search.ok && search.data.length >= 10,
  search.ok ? `count=${search.data.length}` : search.error);
check("results report installed=false in the browser",
  search.ok && search.data.every((m) => m.installed === false));
check("results carry a downloadable message set",
  search.ok && search.data[0].message_sets?.[0]?.url?.startsWith("https://www.iso20022.org/"),
  JSON.stringify(search.ok ? search.data[0].message_sets?.[0] : null));

const info = A.info("camt.053.001.11");
check("info resolves a known identifier", info.ok && info.data.id === "camt.053.001.11", info.error);

const unknown = A.info("zzzz.999.999.99");
check("info rejects an unknown identifier", !unknown.ok);

const stats = A.stats();
check("stats covers the whole standard",
  stats.ok && stats.data.total > 2500 && stats.data.messageSets > 250,
  stats.ok ? `total=${stats.data.total} sets=${stats.data.messageSets}` : stats.error);

const sets = A.sets();
check("message sets list carries download URLs",
  sets.ok && sets.data.length > 250 && sets.data[0].url.includes("/message-set/"),
  sets.error);

// --- conversion, codes, translation ----------------------------------------
const json = A.toJSON(gen.data.xml);
check("XML converts to JSON", json.ok && JSON.parse(json.data.json).Document, json.error);

const codes = A.codes("AC04");
check("code lookup finds AC04", codes.ok && codes.data[0]?.code === "AC04", codes.error);

const tr = A.translate("MT103");
check("MT103 maps to pacs.008",
  tr.ok && tr.data.MXCode.startsWith("pacs.008") && tr.data.FieldMaps.length > 5, tr.error);

const trAll = A.translate("");
check("empty translate query returns the full matrix", trAll.ok && trAll.data.length > 1);

// --- MT to MX conversion ----------------------------------------------------
// The browser must produce byte-identical output to the CLI, so these assert on
// the actual generated document rather than on the fact that a call succeeded.
const MT103 = [
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
  ":70:INVOICE 2026-0815",
  ":71A:SHA",
  "-}"
].join("\n");

const conv = A.convertMT(MT103);
check("MT103 converts to pacs.008",
  conv.ok && conv.data.target_type === "pacs.008.001.10" &&
  conv.data.xml.includes("<IntrBkSttlmAmt Ccy=\"EUR\">25000.00</IntrBkSttlmAmt>"), conv.error);

check("the conversion reports every source field",
  conv.ok && conv.data.report.length >= 10 &&
  conv.data.report.some(f => f.tag === "23B" && f.fidelity === "unmapped"), conv.error);

check("an MT-derived address is flagged against the 2026 deadline",
  conv.ok && !conv.data.lossless &&
  conv.data.report.some(f => (f.note || "").includes("14 November 2026")), conv.error);

check("the fidelity counts add up",
  conv.ok &&
  conv.data.mapped + conv.data.derived + conv.data.truncated + conv.data.unmapped ===
    conv.data.report.length, conv.error);

const conv940 = A.convertMT([
  "{1:F01BANKGB2LAXXX0000000000}{2:I940BANKDEFFXXXXN}{4:",
  ":20:STMT1",
  ":25:GB29NWBK60161331926819",
  ":60F:C260823EUR100000,00",
  ":62F:C260824EUR125000,00",
  "-}"
].join("\n"));
check("MT940 converts to camt.053",
  conv940.ok && conv940.data.target_type === "camt.053.001.11" &&
  conv940.data.xml.includes("<Cd>CLBD</Cd>"), conv940.error);

// The reverse direction: the same call, an XML message instead.
const back = A.convertMT(conv.data.xml);
check("a converted pacs.008 converts back to MT103",
  back.ok && back.data.target_type === "MT103" &&
  back.data.xml.includes(":32A:260824EUR25000,00"), back.error);

// MT103 requires a bank operation code that ISO 20022 has no field for, so the
// reverse direction always has at least one thing to explain.
check("the reverse direction reports what it had to invent",
  back.ok && back.data.report.some(f => f.path === ":23B:" && f.fidelity === "derived"),
  back.error);

const convBad = A.convertMT("not a SWIFT message");
check("a non-MT input is a friendly error", !convBad.ok && convBad.error.length > 0);

const convEmpty = A.convertMT("");
check("an empty input names both directions",
  !convEmpty.ok && convEmpty.error.includes("MT103") && convEmpty.error.includes("pacs.008"));

const convUnsupported = A.convertMT(
  "{1:F01BANKGB2LAXXX0000000000}{2:I700BANKDEFFXXXXN}{4:\n:20:REF1\n-}");
check("an unsupported MT type is refused rather than guessed",
  !convUnsupported.ok && convUnsupported.error.includes("MT700"));

const diagram = A.diagram("pacs.008", "sepa", "mermaid");
check("mermaid diagram renders", diagram.ok && diagram.data.diagram.includes("sequenceDiagram"), diagram.error);

const chain = A.lifecycle("sepa");
check("lifecycle builds a linked chain", chain.ok, chain.error);

// --- field validators -------------------------------------------------------
check("valid IBAN accepted", A.checkIBAN("DE89370400440532013000").data.valid);
check("invalid IBAN rejected", !A.checkIBAN("DE00370400440532013000").data.valid);
check("valid BIC accepted", A.checkBIC("DEUTDEDDXXX").data.valid);
check("invalid BIC rejected", !A.checkBIC("NOTABIC").data.valid);
check("valid UETR accepted", A.checkUETR("e1b2c3d4-5678-4abc-8def-1234567890ab").data.valid);
check("non-v4 UUID rejected", !A.checkUETR("e1b2c3d4-5678-1abc-8def-1234567890ab").data.valid);

// --- error handling ---------------------------------------------------------
check("empty search is a friendly error", !A.search("").ok);
check("empty lint is a friendly error", !A.lint("", "x.xml").ok);
check("malformed XML is a friendly error", !A.lint("<not xml", "x.xml").ok);

// The version is stamped by the Makefile. An unstamped build still works, but
// every report it produces is unattributable, so the smoke test catches a
// wasm target that has quietly lost its -ldflags.
const ver = A.version();
check("version reports a build version", ver.ok && typeof ver.data.version === "string" && ver.data.version !== "",
  JSON.stringify(ver.data));
check("version is stamped, not the 'dev' default", ver.data?.version !== "dev", ver.data?.version);
check("version still states nothing is uploaded", (ver.data?.note || "").includes("Nothing is uploaded"));

// --- SARIF -------------------------------------------------------------------
//
// The website offers this file as a download and says it is the report a
// pipeline can ingest. That is only true if it is actually SARIF, so the shape
// is checked rather than assumed.
const ADDR_MSG = `<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">
  <FIToFICstmrCdtTrf><GrpHdr><MsgId>MSG-1</MsgId></GrpHdr>
    <CdtTrfTxInf><IntrBkSttlmAmt Ccy="EUR">1000.00</IntrBkSttlmAmt>
      <Cdtr><Nm>Beispiel GmbH</Nm>
        <PstlAdr><AdrLine>Musterstrasse 1</AdrLine><AdrLine>Berlin</AdrLine></PstlAdr>
      </Cdtr></CdtTrfTxInf></FIToFICstmrCdtTrf></Document>`;

const sarifRes = A.sarif(ADDR_MSG, "cbpr-2026");
check("sarif returns a report", sarifRes.ok, sarifRes.error);

let sarifDoc = null;
try {
  sarifDoc = JSON.parse(sarifRes.data);
} catch (e) {
  check("sarif output is JSON", false, e.message);
}

if (sarifDoc) {
  check("sarif declares version 2.1.0", sarifDoc.version === "2.1.0", sarifDoc.version);
  check("sarif names askiso as the driver",
    sarifDoc.runs?.[0]?.tool?.driver?.name === "askiso");

  const sarifResults = sarifDoc.runs?.[0]?.results ?? [];
  check("sarif carries the findings", sarifResults.length > 0);

  // A ruleId with no matching rule in the driver renders as a bare string in
  // every viewer that reads these files, so the two must agree.
  const described = new Set((sarifDoc.runs?.[0]?.tool?.driver?.rules ?? []).map((r) => r.id));
  check("every sarif result cites a described rule",
    sarifResults.every((r) => r.ruleId && described.has(r.ruleId)),
    sarifResults.map((r) => r.ruleId).join(", "));

  // The XPath is the citation. Losing it would leave a finding that names a
  // rule but not the place it fired -- unactionable, and not obviously broken.
  check("sarif keeps the path to each finding",
    sarifResults.every((r) =>
      r.locations?.[0]?.logicalLocations?.[0]?.fullyQualifiedName?.startsWith("/Document")),
    JSON.stringify(sarifResults[0]?.locations ?? null));

  // The finding count must match what checkRules reports. A report that drops
  // one is worse than no report.
  const viaRules = A.checkRules(ADDR_MSG, "cbpr-2026");
  check("sarif result count matches checkRules",
    viaRules.ok && viaRules.data.findings.length === sarifResults.length,
    `${viaRules.data?.findings?.length} findings vs ${sarifResults.length} results`);
}

check("sarif on an empty message is a friendly error", !A.sarif("").ok);

// --- evidence pack -----------------------------------------------------------
//
// The pack is what somebody pastes into a ticket, so the wording is the
// feature. In particular it has to say when schema validation did NOT run:
// "clean" without a catalogue means "nothing contradicted it".
new Function(
  fs.readFileSync(
    path.join(path.dirname(fileURLToPath(import.meta.url)), "..", "_layouts", "evidence.js"),
    "utf8",
  ),
)();

const E = globalThis.askisoEvidence;
check("evidence module loads", typeof E?.pack === "function");

if (typeof E?.pack === "function") {
  const rulesRes = A.checkRules(ADDR_MSG, "cbpr-2026");
  const run = {
    messageID: "pacs.008.001.10",
    profile: rulesRes.data.profile,
    checked: rulesRes.data.rules_checked,
    skipped: rulesRes.data.rules_skipped,
    issues: [],
    rules: rulesRes.data.findings,
  };

  const md = E.pack(run, "0.0.1", false);
  check("pack names the message", md.includes("pacs.008.001.10"));
  check("pack names the profile", md.includes(rulesRes.data.profile));
  check("pack states the message was not uploaded", md.includes("not uploaded"));
  check("pack warns that schema validation did not run",
    md.includes("Schema validation: NOT run"));
  check("pack cites every rule identifier",
    rulesRes.data.findings.every((f) => md.includes(f.rule_id)),
    md.slice(0, 400));
  check("pack cites every path",
    rulesRes.data.findings.every((f) => !f.path || md.includes(f.path)));
  check("pack carries the remediation",
    rulesRes.data.findings.every((f) => !f.remediation || md.includes(f.remediation)));

  const withCat = E.pack(run, "0.0.1", true);
  check("pack reports schema validation when a catalogue is open",
    withCat.includes("run against the catalogue") && !withCat.includes("NOT run"));

  const clean = E.pack(
    { messageID: "pacs.008.001.10", profile: "cbpr-2026", issues: [], rules: [] },
    "0.0.1",
    true,
  );
  check("a clean run still produces a pack", clean.includes("No findings"));
  check("a clean pack does not claim findings", !clean.includes("finding(s)"));
}

console.log(failures === 0 ? "\nall checks passed" : `\n${failures} check(s) failed`);
process.exit(failures === 0 ? 0 : 1);
