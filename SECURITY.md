<!-- SPDX-License-Identifier: Apache-2.0 OR MIT -->

# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 0.1.x   | :white_check_mark: |

---

## Security Model & Hardening

AskIso is engineered specifically for mission-critical financial messaging environments:

### 1. XML External Entity (XXE) Protection
* AskIso executes XML schema validation in isolated sandboxes using pure-Go XML parsers and `xmllint` with `--nonet` network isolation enabled, preventing SSRF and file disclosure attacks.

### 2. Cryptographic Randomness
* All generated transaction identifiers (`UETR`, `EndToEndId`) use cryptographically secure random number generators (`crypto/rand`), satisfying RFC 4122 Version 4 standards.

### 3. No Bundled Specifications
* AskIso reads ISO 20022 schemas that you obtain from the Registration Authority. It ships no specification content, so a compromised release cannot substitute altered schemas for the ones you downloaded.

### 4. Untrusted Input Handling
* The catalogue scanner, XML parser, and linter treat all input as untrusted. AskIso's pure-Go parser rejects DTD entity references before any external tool is invoked, so `xmllint` never receives hostile input.

---

## Reporting a Vulnerability

If you discover a security vulnerability in AskIso, please report it privately:

* **Preferred**: [open a draft advisory](https://github.com/sebastienrousseau/askiso/security/advisories/new)
  through GitHub Security Advisories. It keeps the report private until a fix ships
  and produces a CVE without extra work.
* **Email**: `sebastian.rousseau@gmail.com`
* Please include steps to reproduce, sample payloads, and expected vs actual behaviour.

Please do not open a public issue for a vulnerability, and please give the timeline
below a chance to run before disclosing.

### Response commitments

These are the timescales a report can rely on. They are stated because a financial
institution assessing AskIso as a third-party component needs them in writing, not
because a volume of reports is expected.

| Stage | Commitment |
| :--- | :--- |
| Acknowledgement | 24 hours |
| Triage and severity assessment (CVSS v3.1) | 5 business days |
| Fix or documented mitigation — critical / high | 14 days |
| Fix or documented mitigation — medium / low | 90 days |
| Coordinated public disclosure | On fix release, or 90 days from the report, whichever is sooner |

If a deadline is going to be missed, the reporter is told before it passes rather
than after.

### Safe harbour

Research conducted in good faith under this policy is authorised, and no legal
action will be pursued for it. Good faith means: only your own data or data you are
authorised to test, no service degradation, no privacy violation, and no disclosure
of a vulnerability before the timeline above has run.

Note that AskIso processes data locally. The CLI runs on your machine, the website
is static, and the browser build runs in the tab — there is no AskIso-operated
service to test against, so the scope of this policy is the software itself.
