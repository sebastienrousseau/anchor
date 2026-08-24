// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package codes

import (
	"fmt"
	"sort"
	"strings"
)

// Category represents the classification of an ISO 20022 external code set.
type Category string

const (
	CategoryReason  Category = "Return / Reject Reason"
	CategoryPurpose Category = "Payment Purpose"
	CategoryCharge  Category = "Charge Bearer"
	CategoryStatus  Category = "Payment Transaction Status"
	CategoryBalance Category = "Account Balance Type"
)

// CodeItem represents an individual standard code definition.
type CodeItem struct {
	Code        string   `json:"code"`
	Category    Category `json:"category"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	AppliesTo   string   `json:"applies_to"`
}

var standardCodes = []CodeItem{
	// Return / Reject Reason Codes (pacs.004, pacs.002, pain.002)
	{
		Code:        "AC01",
		Category:    CategoryReason,
		Name:        "Incorrect Account Number",
		Description: "Account identifier is invalid, does not exist, or check digits fail.",
		AppliesTo:   "pacs.004, pacs.002, pain.002",
	},
	{
		Code:        "AC04",
		Category:    CategoryReason,
		Name:        "Account Closed",
		Description: "Account number specified has been closed by the account owner or bank.",
		AppliesTo:   "pacs.004, pacs.002, pain.002",
	},
	{
		Code:        "AC06",
		Category:    CategoryReason,
		Name:        "Account Blocked",
		Description: "Account is blocked from receiving credit or debit transactions (e.g. legal arrest, compliance freeze).",
		AppliesTo:   "pacs.004, pacs.002, pain.002",
	},
	{
		Code:        "AG01",
		Category:    CategoryReason,
		Name:        "Transaction Forbidden",
		Description: "Payment transaction is not permitted on this account type or jurisdiction.",
		AppliesTo:   "pacs.004, pacs.002",
	},
	{
		Code:        "AM04",
		Category:    CategoryReason,
		Name:        "Insufficient Funds",
		Description: "Debtor account does not hold sufficient liquidity to settle the debit transaction.",
		AppliesTo:   "pain.002, pacs.004, pacs.002",
	},
	{
		Code:        "AM05",
		Category:    CategoryReason,
		Name:        "Duplication",
		Description: "Payment is a duplicate of a previously received or settled instruction.",
		AppliesTo:   "pacs.004, pacs.002, pain.002",
	},
	{
		Code:        "MD01",
		Category:    CategoryReason,
		Name:        "No Valid Mandate",
		Description: "Direct debit mandate is missing, expired, revoked, or invalid.",
		AppliesTo:   "pacs.004, pain.002",
	},
	{
		Code:        "RC01",
		Category:    CategoryReason,
		Name:        "Bank Identifier Incorrect",
		Description: "Routing BIC or clearing identifier does not exist or is inactive.",
		AppliesTo:   "pacs.004, pacs.002, pain.002",
	},
	{
		Code:        "RR04",
		Category:    CategoryReason,
		Name:        "Regulatory Reason",
		Description: "Payment rejected due to regulatory compliance, sanctions screening, or AML constraints.",
		AppliesTo:   "pacs.004, pacs.002",
	},
	{
		Code:        "TM01",
		Category:    CategoryReason,
		Name:        "Cut-off Time Exceeded",
		Description: "Payment arrived after the network or RTGS clearing cycle cut-off window.",
		AppliesTo:   "pacs.004, pacs.002",
	},
	{
		Code:        "ED05",
		Category:    CategoryReason,
		Name:        "Settlement Date in Past",
		Description: "Settlement date specified is earlier than the processing date.",
		AppliesTo:   "pacs.002, pain.002",
	},

	// Payment Purpose Codes (pain.001, pacs.008 Purp/Cd)
	{
		Code:        "SALA",
		Category:    CategoryPurpose,
		Name:        "Salary / Payroll Payment",
		Description: "Payment is for the settlement of salaries, wages, or employee compensation.",
		AppliesTo:   "pain.001, pacs.008",
	},
	{
		Code:        "PENS",
		Category:    CategoryPurpose,
		Name:        "Pension Payment",
		Description: "Payment is for the disbursement of retirement pension benefits.",
		AppliesTo:   "pain.001, pacs.008",
	},
	{
		Code:        "TAXS",
		Category:    CategoryPurpose,
		Name:        "Tax Payment",
		Description: "Payment is for government or municipal tax liabilities, VAT, or duties.",
		AppliesTo:   "pain.001, pacs.008",
	},
	{
		Code:        "DIVI",
		Category:    CategoryPurpose,
		Name:        "Dividend Payment",
		Description: "Payment is for corporate equity dividend distributions to shareholders.",
		AppliesTo:   "pain.001, pacs.008, seev.031",
	},
	{
		Code:        "SUPP",
		Category:    CategoryPurpose,
		Name:        "Supplier Payment",
		Description: "Commercial invoice settlement to a commercial vendor or supplier.",
		AppliesTo:   "pain.001, pacs.008",
	},
	{
		Code:        "TREA",
		Category:    CategoryPurpose,
		Name:        "Treasury Payment",
		Description: "Internal treasury operations, liquidity sweeps, or FX settlements.",
		AppliesTo:   "pacs.009, pain.001",
	},
	{
		Code:        "INTC",
		Category:    CategoryPurpose,
		Name:        "Intra-Company Payment",
		Description: "Fund transfers between subsidiaries or affiliated accounts of the same corporate group.",
		AppliesTo:   "pain.001, pacs.008",
	},
	{
		Code:        "LOAN",
		Category:    CategoryPurpose,
		Name:        "Loan Settlement / Disbursement",
		Description: "Principal disbursement or repayment installment for a credit facility or loan.",
		AppliesTo:   "pain.001, pacs.008",
	},
	{
		Code:        "SSBE",
		Category:    CategoryPurpose,
		Name:        "Social Security Benefit",
		Description: "Government social welfare and public assistance disbursements.",
		AppliesTo:   "pain.001, pacs.008",
	},

	// Charge Bearer Codes (ChrgBr)
	{
		Code:        "SHAR",
		Category:    CategoryCharge,
		Name:        "Shared Charges",
		Description: "Transaction charges on the ordering side are paid by debtor; charges on the beneficiary side are paid by creditor (Standard SEPA default).",
		AppliesTo:   "pacs.008, pain.001",
	},
	{
		Code:        "DEBT",
		Category:    CategoryCharge,
		Name:        "Borne by Debtor (OUR)",
		Description: "All interbank and routing charges are paid exclusively by the ordering debtor.",
		AppliesTo:   "pacs.008, pain.001",
	},
	{
		Code:        "CRED",
		Category:    CategoryCharge,
		Name:        "Borne by Creditor (BEN)",
		Description: "All transaction charges are deducted from the principal payment amount received by the creditor.",
		AppliesTo:   "pacs.008, pain.001",
	},
	{
		Code:        "SLEV",
		Category:    CategoryCharge,
		Name:        "Service Level Rules",
		Description: "Charges are applied following the overarching multilateral clearing service level agreement.",
		AppliesTo:   "pacs.008, pain.001",
	},

	// Payment Status Codes (TxSts / GrpSts)
	{
		Code:        "ACTC",
		Category:    CategoryStatus,
		Name:        "Accepted Technical Validation",
		Description: "Instruction has passed syntax, schema, and routing validation and is queued for processing.",
		AppliesTo:   "pacs.002, pain.002",
	},
	{
		Code:        "ACCP",
		Category:    CategoryStatus,
		Name:        "Accepted Customer Profile / Settlement Confirmed",
		Description: "Payment has been accepted and successfully booked / settled to the account.",
		AppliesTo:   "pacs.002, pain.002",
	},
	{
		Code:        "ACSP",
		Category:    CategoryStatus,
		Name:        "Accepted Settlement in Process",
		Description: "Payment instruction is accepted and currently awaiting clearing rail settlement finality.",
		AppliesTo:   "pacs.002, pain.002",
	},
	{
		Code:        "RJCT",
		Category:    CategoryStatus,
		Name:        "Rejected",
		Description: "Payment instruction has been rejected due to business rule validation or technical errors.",
		AppliesTo:   "pacs.002, pain.002",
	},
	{
		Code:        "PDNG",
		Category:    CategoryStatus,
		Name:        "Pending",
		Description: "Payment is held pending manual review, compliance clearance, or authorization.",
		AppliesTo:   "pacs.002, pain.002",
	},

	// Balance Type Codes (camt.053 Bal/Tp)
	{
		Code:        "OPBD",
		Category:    CategoryBalance,
		Name:        "Opening Booked Balance",
		Description: "Book balance at the start of the reconciliation reporting period.",
		AppliesTo:   "camt.053, camt.052",
	},
	{
		Code:        "CLBD",
		Category:    CategoryBalance,
		Name:        "Closing Booked Balance",
		Description: "Final ledger book balance at the close of the reporting statement period.",
		AppliesTo:   "camt.053, camt.052",
	},
	{
		Code:        "CLAV",
		Category:    CategoryBalance,
		Name:        "Closing Available Balance",
		Description: "Closing funds immediately available for withdrawal / transfer after float clearance.",
		AppliesTo:   "camt.053, camt.052",
	},
	{
		Code:        "ITBD",
		Category:    CategoryBalance,
		Name:        "Interim Booked Balance",
		Description: "Snapshot balance booked during the intra-day business cycle.",
		AppliesTo:   "camt.052",
	},
}

// GetAllCodes returns the full standard code dictionary.
func GetAllCodes() []CodeItem {
	return standardCodes
}

// Lookup finds matching codes by exact code, name, category, or keyword.
func Lookup(query string) []CodeItem {
	q := strings.ToUpper(strings.TrimSpace(query))
	if q == "" {
		return standardCodes
	}

	var exact []CodeItem
	var partial []CodeItem

	for _, item := range standardCodes {
		if strings.ToUpper(item.Code) == q {
			exact = append(exact, item)
			continue
		}
		if strings.Contains(strings.ToUpper(item.Code), q) ||
			strings.Contains(strings.ToUpper(item.Name), q) ||
			strings.Contains(strings.ToUpper(item.Description), q) ||
			strings.Contains(strings.ToUpper(string(item.Category)), q) {
			partial = append(partial, item)
		}
	}

	results := append(exact, partial...)
	sort.Slice(results, func(i, j int) bool {
		if results[i].Category != results[j].Category {
			return results[i].Category < results[j].Category
		}
		return results[i].Code < results[j].Code
	})
	return results
}

// FormatCode renders a styled terminal block for a code item.
func FormatCode(c CodeItem) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "• Code        : %s\n", c.Code)
	fmt.Fprintf(&sb, "• Category    : %s\n", c.Category)
	fmt.Fprintf(&sb, "• Name        : %s\n", c.Name)
	fmt.Fprintf(&sb, "• Description : %s\n", c.Description)
	fmt.Fprintf(&sb, "• Applies To  : %s\n", c.AppliesTo)
	return sb.String()
}
