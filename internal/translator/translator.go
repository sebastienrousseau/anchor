// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package translator

import (
	"fmt"
	"sort"
	"strings"
)

// Mapping represents an MT <-> MX conversion definition.
type Mapping struct {
	MTCode      string
	MTTitle     string
	MXCode      string
	MXTitle     string
	Description string
	FieldMaps   []FieldMap
}

// FieldMap represents individual field-level correspondence.
type FieldMap struct {
	MTTag    string
	MXPath   string
	Comments string
}

var standardMappings = []Mapping{
	{
		MTCode:      "MT101",
		MTTitle:     "Request for Transfer",
		MXCode:      "pain.001.001.09",
		MXTitle:     "CustomerCreditTransferInitiation",
		Description: "A corporate instructs its bank to make one or more payments. MT101 retires with the rest of the MT payments suite; pain.001 replaces it.",
		FieldMaps: []FieldMap{
			{MTTag: ":20:", MXPath: "GrpHdr/MsgId", Comments: "Sender's Reference"},
			{MTTag: ":28D:", MXPath: "(none)", Comments: "Message index/total describes MT chaining; pain.001 has no equivalent"},
			{MTTag: ":30:", MXPath: "PmtInf/ReqdExctnDt/Dt", Comments: "Requested Execution Date"},
			{MTTag: ":50H/A:", MXPath: "PmtInf/Dbtr & DbtrAcct", Comments: "Ordering Customer; the sequence A party becomes InitgPty"},
			{MTTag: ":52A/D:", MXPath: "PmtInf/DbtrAgt", Comments: "Account Servicing Institution"},
			{MTTag: ":21:", MXPath: "CdtTrfTxInf/PmtId/EndToEndId", Comments: "Transaction Reference; starts each sequence B"},
			{MTTag: ":21F:", MXPath: "CdtTrfTxInf/PmtId/InstrId", Comments: "F/X Deal Reference"},
			{MTTag: ":23E:", MXPath: "CdtTrfTxInf/InstrForDbtrAgt", Comments: "Instruction Code; carried as free text"},
			{MTTag: ":32B:", MXPath: "CdtTrfTxInf/Amt/InstdAmt", Comments: "Currency & Transaction Amount"},
			{MTTag: ":36:", MXPath: "CdtTrfTxInf/XchgRateInf/XchgRate", Comments: "Exchange Rate"},
			{MTTag: ":57A/D:", MXPath: "CdtTrfTxInf/CdtrAgt", Comments: "Account With Institution"},
			{MTTag: ":59/59A:", MXPath: "CdtTrfTxInf/Cdtr & CdtrAcct", Comments: "Beneficiary Customer & IBAN"},
			{MTTag: ":70:", MXPath: "CdtTrfTxInf/RmtInf/Ustrd", Comments: "Remittance Information"},
			{MTTag: ":71A:", MXPath: "PmtInf/ChrgBr", Comments: "Details of Charges (BEN, OUR, SHA -> CRED, DEBT, SHAR)"},
			{MTTag: ":25A:", MXPath: "PmtInf/ChrgsAcct", Comments: "Charges Account"},
		},
	},
	{
		MTCode:      "MT104",
		MTTitle:     "Request for Debit Transfer",
		MXCode:      "pain.008.001.07",
		MXTitle:     "CustomerDirectDebitInitiation",
		Description: "A creditor instructs its bank to collect from one or more debtors. Retires in the 2028 milestone alongside MT107.",
		FieldMaps: []FieldMap{
			{MTTag: ":20:", MXPath: "GrpHdr/MsgId", Comments: "Sender's Reference"},
			{MTTag: ":30:", MXPath: "PmtInf/ReqdColltnDt", Comments: "Requested Execution Date"},
			{MTTag: ":50C/L:", MXPath: "GrpHdr/InitgPty", Comments: "Instructing Party; option letter separates it from the creditor"},
			{MTTag: ":50A/K:", MXPath: "PmtInf/Cdtr & CdtrAcct", Comments: "Creditor and its account"},
			{MTTag: ":52A/D:", MXPath: "PmtInf/CdtrAgt", Comments: "Creditor's Bank"},
			{MTTag: ":21:", MXPath: "DrctDbtTxInf/PmtId/EndToEndId", Comments: "Transaction Reference; starts each sequence B"},
			{MTTag: ":21C:", MXPath: "DrctDbtTx/MndtRltdInf/MndtId", Comments: "Mandate Reference"},
			{MTTag: ":21D:", MXPath: "(none)", Comments: "Direct Debit Reference has no equivalent"},
			{MTTag: ":32B:", MXPath: "DrctDbtTxInf/InstdAmt", Comments: "Currency & Transaction Amount"},
			{MTTag: ":57A/D:", MXPath: "DrctDbtTxInf/DbtrAgt", Comments: "Debtor's Bank"},
			{MTTag: ":59/59A:", MXPath: "DrctDbtTxInf/Dbtr & DbtrAcct", Comments: "Debtor and its account"},
			{MTTag: ":70:", MXPath: "DrctDbtTxInf/RmtInf/Ustrd", Comments: "Remittance Information"},
			{MTTag: ":71A:", MXPath: "PmtInf/ChrgBr", Comments: "Details of Charges"},
			{MTTag: ":26T:", MXPath: "(none)", Comments: "Transaction Type Code is scheme-specific"},
		},
	},
	{
		MTCode:      "MT107",
		MTTitle:     "General Direct Debit Message",
		MXCode:      "pain.008.001.07",
		MXTitle:     "CustomerDirectDebitInitiation",
		Description: "The same structure as MT104, used where no prior agreement between the banks exists.",
		FieldMaps: []FieldMap{
			{MTTag: ":20:", MXPath: "GrpHdr/MsgId", Comments: "Sender's Reference"},
			{MTTag: ":30:", MXPath: "PmtInf/ReqdColltnDt", Comments: "Requested Execution Date"},
			{MTTag: ":50A/K:", MXPath: "PmtInf/Cdtr & CdtrAcct", Comments: "Creditor and its account"},
			{MTTag: ":21:", MXPath: "DrctDbtTxInf/PmtId/EndToEndId", Comments: "Transaction Reference"},
			{MTTag: ":32B:", MXPath: "DrctDbtTxInf/InstdAmt", Comments: "Currency & Transaction Amount"},
			{MTTag: ":59/59A:", MXPath: "DrctDbtTxInf/Dbtr & DbtrAcct", Comments: "Debtor and its account"},
		},
	},
	{
		MTCode:      "MT204",
		MTTitle:     "Financial Markets Direct Debit Message",
		MXCode:      "pacs.010.001.06",
		MXTitle:     "FinancialInstitutionDirectDebit",
		Description: "One institution collects from others, typically for margin. Every party is an institution rather than a customer.",
		FieldMaps: []FieldMap{
			{MTTag: ":20:", MXPath: "GrpHdr/MsgId & CdtInstr/CdtId", Comments: "Sequence A reference"},
			{MTTag: ":19:", MXPath: "(none)", Comments: "Sum of Amounts is derivable from the transactions"},
			{MTTag: ":30:", MXPath: "CdtInstr/IntrBkSttlmDt", Comments: "Value Date"},
			{MTTag: ":58A:", MXPath: "CdtInstr/Cdtr", Comments: "Beneficiary Institution: the party collecting"},
			{MTTag: ":20: (seq B)", MXPath: "DrctDbtTxInf/PmtId/EndToEndId", Comments: "Transaction Reference; starts each sequence B"},
			{MTTag: ":21:", MXPath: "DrctDbtTxInf/PmtId/EndToEndId", Comments: "Related Reference, preferred when present"},
			{MTTag: ":32B:", MXPath: "DrctDbtTxInf/IntrBkSttlmAmt", Comments: "Currency & Transaction Amount"},
			{MTTag: ":53A:", MXPath: "DrctDbtTxInf/Dbtr", Comments: "Debit Institution: the party being collected from"},
			{MTTag: ":72:", MXPath: "DrctDbtTxInf/RmtInf/Ustrd", Comments: "Sender to Receiver Information, as free text"},
		},
	},
	{
		MTCode:      "MTn92",
		MTTitle:     "Request for Cancellation",
		MXCode:      "camt.056.001.11",
		MXTitle:     "FIToFIPaymentCancellationRequest",
		Description: "Asks the receiver to cancel a payment already sent. The category digit varies -- MT192 for a customer payment, MT292 for an institution one, and so on through the nine categories.",
		FieldMaps: []FieldMap{
			{MTTag: ":20:", MXPath: "Assgnmt/Id & Case/Id", Comments: "Transaction Reference Number"},
			{MTTag: ":21:", MXPath: "Undrlyg/TxInf/OrgnlEndToEndId", Comments: "Related Reference: the payment being cancelled"},
			{MTTag: ":11S:", MXPath: "Undrlyg/TxInf/OrgnlGrpInf", Comments: "MT and date of the original message"},
			{MTTag: ":32A:", MXPath: "OrgnlIntrBkSttlmAmt & OrgnlIntrBkSttlmDt", Comments: "Original value date, currency and amount"},
			{MTTag: ":79:", MXPath: "CxlRsnInf/AddtlInf", Comments: "Narrative reason, as free text"},
			{MTTag: "121 UETR", MXPath: "Undrlyg/TxInf/OrgnlUETR", Comments: "Tracking reference of the original payment"},
		},
	},
	{
		MTCode:      "MTn95",
		MTTitle:     "Queries",
		MXCode:      "camt.110.001.01",
		MXTitle:     "InvestigationRequest",
		Description: "Asks a question about a payment. camt.110 wants a coded investigation type and reason; an MT query carries prose, so the proprietary branch names the source message and the text becomes the narrative.",
		FieldMaps: []FieldMap{
			{MTTag: ":20:", MXPath: "InvstgtnReq/MsgId", Comments: "Transaction Reference Number"},
			{MTTag: ":21:", MXPath: "Undrlyg/IntrBk/OrgnlEndToEndId", Comments: "Related Reference"},
			{MTTag: ":11S:", MXPath: "Undrlyg/IntrBk/OrgnlGrpInf", Comments: "MT and date of the original message"},
			{MTTag: ":75:", MXPath: "InvstgtnData/AddtlReqData/ReqNrrtv", Comments: "Queries, numbered, joined into the narrative"},
			{MTTag: ":77A:", MXPath: "InvstgtnData/AddtlReqData/ReqNrrtv", Comments: "Narrative"},
			{MTTag: "(none)", MXPath: "InvstgtnTp/Prtry", Comments: "The type is derived: an MT query carries no code"},
		},
	},
	{
		MTCode:      "MTn96",
		MTTitle:     "Answers",
		MXCode:      "camt.111.001.02",
		MXTitle:     "InvestigationResponse",
		Description: "Answers a query. The status is mandatory in camt.111 and absent from MT, so CLSD is used: an answer closes the investigation.",
		FieldMaps: []FieldMap{
			{MTTag: ":20:", MXPath: "InvstgtnRspn/MsgId", Comments: "Transaction Reference Number"},
			{MTTag: ":21:", MXPath: "OrgnlInvstgtnReq/MsgId", Comments: "Related Reference: the query being answered"},
			{MTTag: ":76:", MXPath: "InvstgtnData/RspnData/RspnNrrtv", Comments: "Answers, numbered, joined into the narrative"},
			{MTTag: ":77A:", MXPath: "InvstgtnData/RspnData/RspnNrrtv", Comments: "Narrative"},
			{MTTag: "(none)", MXPath: "InvstgtnSts/Sts", Comments: "The status is derived: CLSD, because an answer closes the investigation"},
		},
	},
	{
		MTCode:      "MT103",
		MTTitle:     "Single Customer Credit Transfer",
		MXCode:      "pacs.008.001.10",
		MXTitle:     "FIToFICustomerCreditTransfer",
		Description: "Direct customer payment instruction across interbank payment rails.",
		FieldMaps: []FieldMap{
			{MTTag: ":20:", MXPath: "GrpHdr/MsgId or PmtId/EndToEndId", Comments: "Transaction Reference Number"},
			{MTTag: ":23B:", MXPath: "PmtTpInf/LclInstrm", Comments: "Bank Operation Code (e.g. CRED)"},
			{MTTag: ":32A:", MXPath: "IntrBkSttlmAmt & IntrBkSttlmDt", Comments: "Value Date, Currency, Amount"},
			{MTTag: ":33B:", MXPath: "InstdAmt", Comments: "Instructed Currency & Amount"},
			{MTTag: ":50K/A:", MXPath: "Dbtr & DbtrAcct", Comments: "Ordering Customer (Name, IBAN, Address)"},
			{MTTag: ":52A/D:", MXPath: "DbtrAgt", Comments: "Ordering Institution / Bank BIC"},
			{MTTag: ":57A/D:", MXPath: "CdtrAgt", Comments: "Account With Institution / Creditor Bank"},
			{MTTag: ":59/59A:", MXPath: "Cdtr & CdtrAcct", Comments: "Beneficiary Customer & IBAN"},
			{MTTag: ":70:", MXPath: "RmtInf/Ustrd", Comments: "Remittance Information"},
			{MTTag: ":71A:", MXPath: "ChrgBr", Comments: "Details of Charges (BEN, OUR, SHA -> DEBT, CRED, SHAR)"},
			{MTTag: "121 UETR", MXPath: "PmtId/UETR", Comments: "UUIDv4 End-to-End Tracking identifier"},
		},
	},
	{
		MTCode:      "MT202",
		MTTitle:     "General Financial Institution Transfer",
		MXCode:      "pacs.009.001.10 (CORE)",
		MXTitle:     "FinancialInstitutionCreditTransfer",
		Description: "Interbank treasury, FX settlement, and liquidity replenishment transfers.",
		FieldMaps: []FieldMap{
			{MTTag: ":20:", MXPath: "GrpHdr/MsgId", Comments: "Transaction Reference Number"},
			{MTTag: ":21:", MXPath: "PmtId/EndToEndId", Comments: "Related Reference"},
			{MTTag: ":32A:", MXPath: "IntrBkSttlmAmt & IntrBkSttlmDt", Comments: "Value Date, Currency, Settlement Amount"},
			{MTTag: ":52A:", MXPath: "InstgAgt", Comments: "Ordering Financial Institution"},
			{MTTag: ":57A:", MXPath: "InstdAgt", Comments: "Account With Institution"},
			{MTTag: ":58A:", MXPath: "Cdtr (FI)", Comments: "Beneficiary Institution"},
		},
	},
	{
		MTCode:      "MT202 COV",
		MTTitle:     "Financial Institution Transfer for Customer Covers",
		MXCode:      "pacs.009.001.10 (COVE)",
		MXTitle:     "FinancialInstitutionCreditTransfer (Cover)",
		Description: "Reimbursement leg settling liquidity through clearing for an underlying pacs.008.",
		FieldMaps: []FieldMap{
			{MTTag: ":20:", MXPath: "GrpHdr/MsgId", Comments: "Cover Message Identifier"},
			{MTTag: ":21:", MXPath: "PmtId/EndToEndId", Comments: "Must match pacs.008 EndToEndId"},
			{MTTag: "Seq B :50K:", MXPath: "UndrlygCstmrCdtTrf/Dbtr", Comments: "Underlying Ordering Customer"},
			{MTTag: "Seq B :59:", MXPath: "UndrlygCstmrCdtTrf/Cdtr", Comments: "Underlying Beneficiary Customer"},
			{MTTag: "121 UETR", MXPath: "PmtId/UETR", Comments: "Must match underlying pacs.008 UETR"},
		},
	},
	{
		MTCode:      "MT940",
		MTTitle:     "Customer Statement Message",
		MXCode:      "camt.053.001.11",
		MXTitle:     "BankToCustomerAccountStatement",
		Description: "End-of-day official accounting and reconciliation statement for account owners.",
		FieldMaps: []FieldMap{
			{MTTag: ":20:", MXPath: "GrpHdr/MsgId", Comments: "Statement Reference Number"},
			{MTTag: ":25:", MXPath: "Stmt/Acct/Id/IBAN", Comments: "Account Identification / IBAN"},
			{MTTag: ":28C:", MXPath: "Stmt/LglSeqNb", Comments: "Statement Number / Sequence Number"},
			{MTTag: ":60F/M:", MXPath: "Stmt/Bal (OPBD)", Comments: "Opening Booked Balance (Credit/Debit, Date, Amount)"},
			{MTTag: ":61:", MXPath: "Stmt/Ntry", Comments: "Statement Line / Transaction Entry details"},
			{MTTag: ":62F/M:", MXPath: "Stmt/Bal (CLBD)", Comments: "Closing Booked Balance"},
			{MTTag: ":64:", MXPath: "Stmt/Bal (CLAV)", Comments: "Closing Available Balance"},
			{MTTag: ":86:", MXPath: "Stmt/Ntry/NtryDtls/TxDtls/RmtInf", Comments: "Information to Account Owner"},
		},
	},
	{
		MTCode:      "MT942",
		MTTitle:     "Interim Transaction Report",
		MXCode:      "camt.052.001.11",
		MXTitle:     "BankToCustomerAccountReport",
		Description: "Intraday account report providing interim balance snapshots during the business day.",
		FieldMaps: []FieldMap{
			{MTTag: ":20:", MXPath: "GrpHdr/MsgId", Comments: "Report Reference Number"},
			{MTTag: ":25:", MXPath: "Rpt/Acct/Id", Comments: "Account Identification"},
			{MTTag: ":34F:", MXPath: "Rpt/Bal (ITBD)", Comments: "Floor Limit Indicator & Interim Balance"},
			{MTTag: ":61:", MXPath: "Rpt/Ntry", Comments: "Interim Booked & Pending Entries"},
			{MTTag: ":86:", MXPath: "Rpt/Ntry/RmtInf", Comments: "Transaction Information"},
		},
	},
}

// GetAllMappings returns the complete MT <-> MX cross-reference catalog.
func GetAllMappings() []Mapping {
	// Sorted by MT code, because the table is read as a migration checklist and
	// declaration order is an accident of when each mapping was written.
	out := append([]Mapping(nil), standardMappings...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].MTCode < out[j].MTCode })
	return out
}

// Lookup finds a mapping by MT code or MX code.
func Lookup(query string) (Mapping, bool) {
	q := strings.ToUpper(strings.TrimSpace(query))
	q = strings.ReplaceAll(q, " ", "")
	q = strings.ReplaceAll(q, ".", "")
	q = strings.ReplaceAll(q, "_", "")

	for _, m := range standardMappings {
		mtClean := strings.ReplaceAll(strings.ToUpper(m.MTCode), " ", "")
		mxClean := strings.ReplaceAll(strings.ReplaceAll(strings.ToUpper(m.MXCode), ".", ""), " ", "")
		if strings.Contains(mtClean, q) || strings.Contains(mxClean, q) {
			return m, true
		}
	}
	return Mapping{}, false
}

// FormatMapping renders a detailed terminal report of an MT <-> MX mapping.
func FormatMapping(m Mapping) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "=== SWIFT %s ⇄ ISO 20022 %s ===\n\n", m.MTCode, m.MXCode)
	fmt.Fprintf(&sb, "• MT Name : %s\n", m.MTTitle)
	fmt.Fprintf(&sb, "• MX Name : %s\n", m.MXTitle)
	fmt.Fprintf(&sb, "• Scope   : %s\n\n", m.Description)
	sb.WriteString("Field-Level Migration Matrix:\n")
	fmt.Fprintf(&sb, "  %-14s %-32s %s\n", "SWIFT MT Tag", "ISO 20022 XML Element Path", "Description / Notes")
	fmt.Fprintf(&sb, "  %-14s %-32s %s\n", "────────────", "──────────────────────────", "───────────────────")
	for _, f := range m.FieldMaps {
		fmt.Fprintf(&sb, "  %-14s %-32s %s\n", f.MTTag, f.MXPath, f.Comments)
	}
	return sb.String()
}
