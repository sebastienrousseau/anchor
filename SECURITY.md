<!-- SPDX-License-Identifier: Apache-2.0 OR MIT -->

# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 0.1.x   | :white_check_mark: |

---

## Security Model & Hardening

Anchor is engineered specifically for mission-critical financial messaging environments:

### 1. XML External Entity (XXE) Protection
* Anchor executes XML schema validation in isolated sandboxes using pure-Go XML parsers and `xmllint` with `--nonet` network isolation enabled, preventing SSRF and file disclosure attacks.

### 2. Cryptographic Randomness
* All generated transaction identifiers (`UETR`, `EndToEndId`) use cryptographically secure random number generators (`crypto/rand`), satisfying RFC 4122 Version 4 standards.

### 3. No Bundled Specifications
* Anchor reads ISO 20022 schemas that you obtain from the Registration Authority. It ships no specification content, so a compromised release cannot substitute altered schemas for the ones you downloaded.

### 4. Untrusted Input Handling
* The catalogue scanner, XML parser, and linter treat all input as untrusted. Anchor's pure-Go parser rejects DTD entity references before any external tool is invoked, so `xmllint` never receives hostile input.

---

## Reporting a Vulnerability

If you discover a security vulnerability in Anchor, please report it responsibly:

* **Email**: `sebastian.rousseau@gmail.com`
* Please include steps to reproduce, sample payloads, and expected vs actual behavior.
* Security reports will receive an initial response within 24 hours.
