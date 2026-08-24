// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package graph

import (
	"fmt"
	"strings"
)

// GenerateMermaid builds a Mermaid sequence diagram for an ISO 20022 payment flow.
func GenerateMermaid(msgType, preset string) string {
	presetUpper := strings.ToUpper(preset)
	if presetUpper == "" {
		presetUpper = "SEPA"
	}

	clearing := "Clearing System (" + presetUpper + ")"
	switch presetUpper {
	case "FEDNOW":
		clearing = "Federal Reserve (FedNow)"
	case "TARGET2":
		clearing = "Eurosystem RTGS (TARGET2)"
	case "CHAPS":
		clearing = "Bank of England (CHAPS)"
	}

	return fmt.Sprintf(`sequenceDiagram
    autonumber
    actor Debtor as Ordering Customer (Debtor)
    participant DB as Debtor Bank (FI-A)
    participant CS as %s
    participant CB as Creditor Bank (FI-B)
    actor Creditor as Beneficiary Customer (Creditor)

    Note over Debtor,Creditor: ISO 20022 End-to-End Payment Lifecycle [%s]

    Debtor->>DB: pain.001 (Customer Credit Transfer Initiation)
    Note right of DB: Validate IBAN mod-97, limits & AML

    DB->>CS: pacs.008 (Interbank Customer Credit Transfer)
    Note right of CS: Liquidity reservation & net/gross settlement

    CS-->>DB: pacs.002 (Payment Status Report - ACCP / Settled)
    CS->>CB: pacs.008 (Interbank Credit Transfer Delivery)

    CB->>Creditor: camt.054 / camt.053 (Credit Notification / Statement)
    Note over Debtor,Creditor: Transaction Finality Confirmed (UETR match)
`, clearing, presetUpper)
}

// GenerateASCII builds a clean terminal ASCII sequence diagram.
func GenerateASCII(msgType, preset string) string {
	presetUpper := strings.ToUpper(preset)
	if presetUpper == "" {
		presetUpper = "SEPA"
	}

	return fmt.Sprintf(`
  ISO 20022 Payment Lifecycle Sequence Diagram [%s]
  ========================================================================================
  [Debtor]            [Debtor Bank]          [Clearing Rail]         [Creditor Bank]       [Creditor]
     |                      |                       |                       |                  |
     |--- pain.001.001.11 ->|                       |                       |                  | (1. Customer Init)
     |    (Credit Transfer) |                       |                       |                  |
     |                      |--- pacs.008.001.10 -->|                       |                  | (2. Interbank Settle)
     |                      |    (CLRG: %-7s)    |                       |                  |
     |                      |<-- pacs.002.001.12 ---|                       |                  | (3. Settlement ACK)
     |                      |    (Status: ACCP)     |--- pacs.008.001.10 -->|                  | (4. Inbound Credit)
     |                      |                       |                       |--- camt.053 ---->| (5. Reconciliation)
     |                      |                       |                       |    (Statement)   |
  ========================================================================================
  • Guaranteed UETR tracking across all 4 hops (RFC 4122 UUIDv4)
`, presetUpper, presetUpper)
}
