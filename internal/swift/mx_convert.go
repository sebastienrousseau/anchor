// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package swift

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/sebastienrousseau/anchor/internal/converter"
)

// MT field widths. They are the reason this direction loses data: a reference
// that ISO 20022 allows 35 characters for has 16 here.
const (
	mtReferenceWidth = 16
	mtLineWidth      = 35
	mtPartyLines     = 4
	mtRemittanceLine = 4
)

// convertPacs008 turns a customer credit transfer into an MT103.
func convertPacs008(root *converter.Node, msgID string) (*Conversion, error) {
	b := &mtBuilder{}

	body, _, ok := find(root, "FIToFICstmrCdtTrf")
	if !ok {
		return nil, fmt.Errorf("%s carries no <FIToFICstmrCdtTrf>", msgID)
	}
	header, _ := child(body, "GrpHdr")

	transactions := childrenNamed(body, "CdtTrfTxInf")
	if len(transactions) == 0 {
		return nil, fmt.Errorf("%s carries no transaction", msgID)
	}
	tx := transactions[0]
	if len(transactions) > 1 {
		b.lost("/Document/FIToFICstmrCdtTrf/CdtTrfTxInf",
			fmt.Sprintf("MT103 carries one payment; %d were present and only the first was converted",
				len(transactions)), "")
	}

	// :20: is the sender's reference. The instruction identification is the
	// closest equivalent; the message identifier stands in when it is absent.
	reference := deepText(tx, "InstrId")
	referencePath := "/Document/FIToFICstmrCdtTrf/CdtTrfTxInf/PmtId/InstrId"
	if reference == "" {
		reference = text(header, "MsgId")
		referencePath = "/Document/FIToFICstmrCdtTrf/GrpHdr/MsgId"
	}
	b.set("20", referencePath, reference, mtReferenceWidth)

	// :23B: is mandatory in MT103 and has no ISO 20022 source.
	b.fields = append(b.fields, ":23B:CRED")
	b.derived("23B", "the bank operation code is mandatory in MT103 and has no source in the message")

	amount, ok := child(tx, "IntrBkSttlmAmt")
	if !ok {
		return nil, fmt.Errorf("%s carries no <IntrBkSttlmAmt>", msgID)
	}
	settlementDate := text(tx, "IntrBkSttlmDt")
	if settlementDate == "" {
		settlementDate = today()
		b.derived("32A", "the message carried no settlement date; today's date was used")
	}
	b.set("32A", "/Document/FIToFICstmrCdtTrf/CdtTrfTxInf/IntrBkSttlmAmt",
		isoDateToMT(settlementDate)+attr(amount, "Ccy")+mtAmount(amount.Text), 24)

	if instr, ok := child(tx, "InstrForNxtAgt"); ok {
		value := text(instr, "Cd")
		if info := text(instr, "InstrInf"); info != "" {
			if value == "" {
				value = info
			} else {
				value += "/" + info
			}
		}
		b.set("23E", "/Document/FIToFICstmrCdtTrf/CdtTrfTxInf/InstrForNxtAgt", value, 35)
	}

	if instructed, ok := child(tx, "InstdAmt"); ok {
		b.set("33B", "/Document/FIToFICstmrCdtTrf/CdtTrfTxInf/InstdAmt",
			attr(instructed, "Ccy")+mtAmount(instructed.Text), 18)
	}
	if rate := text(tx, "XchgRate"); rate != "" {
		b.set("36", "/Document/FIToFICstmrCdtTrf/CdtTrfTxInf/XchgRate",
			strings.Replace(rate, ".", ",", 1), 12)
	}

	debtor, hasDebtor := readParty(tx, "Dbtr")
	if !hasDebtor {
		return nil, fmt.Errorf("%s carries no <Dbtr>", msgID)
	}
	debtor.Account = accountID(tx, "DbtrAcct")
	b.setLines("50K", "/Document/FIToFICstmrCdtTrf/CdtTrfTxInf/Dbtr",
		partyLinesFor(debtor), mtPartyLines, mtLineWidth)
	b.reportPartyLosses(debtor, "/Document/FIToFICstmrCdtTrf/CdtTrfTxInf/Dbtr")

	debtorAgent := agentBIC(tx, "DbtrAgt")
	creditorAgent := agentBIC(tx, "CdtrAgt")
	b.set("52A", "/Document/FIToFICstmrCdtTrf/CdtTrfTxInf/DbtrAgt", debtorAgent, 11)
	b.set("57A", "/Document/FIToFICstmrCdtTrf/CdtTrfTxInf/CdtrAgt", creditorAgent, 11)

	creditor, hasCreditor := readParty(tx, "Cdtr")
	if !hasCreditor {
		return nil, fmt.Errorf("%s carries no <Cdtr>", msgID)
	}
	creditor.Account = accountID(tx, "CdtrAcct")
	b.setLines("59", "/Document/FIToFICstmrCdtTrf/CdtTrfTxInf/Cdtr",
		partyLinesFor(creditor), mtPartyLines, mtLineWidth)
	b.reportPartyLosses(creditor, "/Document/FIToFICstmrCdtTrf/CdtTrfTxInf/Cdtr")

	if rmt, ok := child(tx, "RmtInf"); ok {
		var lines []string
		for _, u := range childrenNamed(rmt, "Ustrd") {
			lines = append(lines, strings.TrimSpace(u.Text))
		}
		if len(lines) > 0 {
			b.setLines("70", "/Document/FIToFICstmrCdtTrf/CdtTrfTxInf/RmtInf/Ustrd",
				lines, mtRemittanceLine, mtLineWidth)
		}
		if strd, ok := child(rmt, "Strd"); ok {
			b.lost("/Document/FIToFICstmrCdtTrf/CdtTrfTxInf/RmtInf/Strd",
				"structured remittance information has no MT equivalent; it would have to be "+
					"flattened into the free text of field 70", firstLine(strd.Text))
		}
	}

	// :71A: is mandatory in MT103.
	bearer := text(tx, "ChrgBr")
	if mt, ok := reverseChargeBearer(bearer); ok {
		b.set("71A", "/Document/FIToFICstmrCdtTrf/CdtTrfTxInf/ChrgBr", mt, 3)
	} else {
		b.fields = append(b.fields, ":71A:SHA")
		b.derived("71A", "the charge bearer is mandatory in MT103; SHA was used")
	}

	for i, charge := range childrenNamed(tx, "ChrgsInf") {
		amt, ok := child(charge, "Amt")
		if !ok {
			continue
		}
		tag := "71F"
		if i > 0 {
			tag = "71G"
		}
		b.set(tag, "/Document/FIToFICstmrCdtTrf/CdtTrfTxInf/ChrgsInf",
			attr(amt, "Ccy")+mtAmount(amt.Text), 18)
	}

	reportUnconvertible(b, tx, "/Document/FIToFICstmrCdtTrf/CdtTrfTxInf")

	uetr := deepText(tx, "UETR")
	if uetr != "" {
		b.reports = append(b.reports, FieldReport{
			Tag:  "/Document/FIToFICstmrCdtTrf/CdtTrfTxInf/PmtId/UETR",
			Path: "{3:{121:}}", Fidelity: FidelityMapped, Value: uetr,
		})
	}

	return &Conversion{
		SourceType: msgID,
		TargetType: "MT103",
		XML:        b.message("103", debtorAgent, creditorAgent, uetr),
		Report:     sortReports(b.reports),
	}, nil
}

// convertPacs009 turns a financial institution credit transfer into an MT202.
func convertPacs009(root *converter.Node, msgID string) (*Conversion, error) {
	b := &mtBuilder{}

	body, _, ok := find(root, "FICdtTrf")
	if !ok {
		return nil, fmt.Errorf("%s carries no <FICdtTrf>", msgID)
	}
	header, _ := child(body, "GrpHdr")

	transactions := childrenNamed(body, "CdtTrfTxInf")
	if len(transactions) == 0 {
		return nil, fmt.Errorf("%s carries no transaction", msgID)
	}
	tx := transactions[0]
	if len(transactions) > 1 {
		b.lost("/Document/FICdtTrf/CdtTrfTxInf",
			fmt.Sprintf("MT202 carries one transfer; %d were present and only the first was converted",
				len(transactions)), "")
	}

	reference := deepText(tx, "InstrId")
	referencePath := "/Document/FICdtTrf/CdtTrfTxInf/PmtId/InstrId"
	if reference == "" {
		reference = text(header, "MsgId")
		referencePath = "/Document/FICdtTrf/GrpHdr/MsgId"
	}
	b.set("20", referencePath, reference, mtReferenceWidth)

	// :21: is mandatory in MT202: the related reference.
	related := deepText(tx, "EndToEndId")
	if related == "" {
		related = reference
		b.derived("21", "the message carried no end-to-end identification; the sender's reference was reused")
	}
	b.set("21", "/Document/FICdtTrf/CdtTrfTxInf/PmtId/EndToEndId", related, mtReferenceWidth)

	amount, ok := child(tx, "IntrBkSttlmAmt")
	if !ok {
		return nil, fmt.Errorf("%s carries no <IntrBkSttlmAmt>", msgID)
	}
	settlementDate := text(tx, "IntrBkSttlmDt")
	if settlementDate == "" {
		settlementDate = today()
		b.derived("32A", "the message carried no settlement date; today's date was used")
	}
	b.set("32A", "/Document/FICdtTrf/CdtTrfTxInf/IntrBkSttlmAmt",
		isoDateToMT(settlementDate)+attr(amount, "Ccy")+mtAmount(amount.Text), 24)

	debtorAgent := agentBIC(tx, "Dbtr")
	if debtorAgent == "" {
		debtorAgent = agentBIC(tx, "InstgAgt")
	}
	creditorAgent := agentBIC(tx, "Cdtr")

	b.set("52A", "/Document/FICdtTrf/CdtTrfTxInf/Dbtr", debtorAgent, 11)
	b.set("53A", "/Document/FICdtTrf/CdtTrfTxInf/InstgAgt", agentBIC(tx, "InstgAgt"), 11)
	b.set("57A", "/Document/FICdtTrf/CdtTrfTxInf/InstdAgt", agentBIC(tx, "InstdAgt"), 11)

	// :58a: is mandatory in MT202: the beneficiary institution.
	if creditorAgent == "" {
		return nil, fmt.Errorf("%s carries no creditor institution, which MT202 requires", msgID)
	}
	b.set("58A", "/Document/FICdtTrf/CdtTrfTxInf/Cdtr", creditorAgent, 11)

	reportUnconvertible(b, tx, "/Document/FICdtTrf/CdtTrfTxInf")

	uetr := deepText(tx, "UETR")
	if uetr != "" {
		b.reports = append(b.reports, FieldReport{
			Tag:  "/Document/FICdtTrf/CdtTrfTxInf/PmtId/UETR",
			Path: "{3:{121:}}", Fidelity: FidelityMapped, Value: uetr,
		})
	}

	return &Conversion{
		SourceType: msgID,
		TargetType: "MT202",
		XML:        b.message("202", debtorAgent, creditorAgent, uetr),
		Report:     sortReports(b.reports),
	}, nil
}

// convertCamt053 turns a bank to customer statement into an MT940.
func convertCamt053(root *converter.Node, msgID string) (*Conversion, error) {
	b := &mtBuilder{}

	body, _, ok := find(root, "BkToCstmrStmt")
	if !ok {
		return nil, fmt.Errorf("%s carries no <BkToCstmrStmt>", msgID)
	}
	header, _ := child(body, "GrpHdr")

	statements := childrenNamed(body, "Stmt")
	if len(statements) == 0 {
		return nil, fmt.Errorf("%s carries no statement", msgID)
	}
	stmt := statements[0]
	if len(statements) > 1 {
		b.lost("/Document/BkToCstmrStmt/Stmt",
			fmt.Sprintf("MT940 carries one statement; %d were present and only the first was converted",
				len(statements)), "")
	}

	reference := text(stmt, "Id")
	referencePath := "/Document/BkToCstmrStmt/Stmt/Id"
	if reference == "" {
		reference = text(header, "MsgId")
		referencePath = "/Document/BkToCstmrStmt/GrpHdr/MsgId"
	}
	b.set("20", referencePath, reference, mtReferenceWidth)

	account := accountID(stmt, "Acct")
	if account == "" {
		return nil, fmt.Errorf("%s names no account, which MT940 requires", msgID)
	}
	b.set("25", "/Document/BkToCstmrStmt/Stmt/Acct/Id", account, mtLineWidth)

	// :28C: is mandatory: the statement number.
	sequence := text(stmt, "ElctrncSeqNb")
	if sequence == "" {
		sequence = text(stmt, "LglSeqNb")
	}
	if sequence == "" {
		sequence = "1"
		b.derived("28C", "the statement carried no sequence number; 1 was used")
	}
	b.set("28C", "/Document/BkToCstmrStmt/Stmt/ElctrncSeqNb", sequence, 5)

	opening, closing := "", ""
	for _, bal := range childrenNamed(stmt, "Bal") {
		code := deepText(bal, "Cd")
		amt, ok := child(bal, "Amt")
		if !ok {
			continue
		}
		sign := "C"
		if text(bal, "CdtDbtInd") == "DBIT" {
			sign = "D"
		}
		date := balanceDate(bal)
		value := sign + isoDateToMT(date) + attr(amt, "Ccy") + mtAmount(amt.Text)

		switch code {
		case "OPBD", "PRCD":
			opening = value
		case "CLBD":
			closing = value
		}
	}
	if opening == "" || closing == "" {
		return nil, fmt.Errorf("%s carries no opening and closing balance, which MT940 requires", msgID)
	}
	b.set("60F", "/Document/BkToCstmrStmt/Stmt/Bal (OPBD)", opening, 25)

	// Statement entries sit between the two balances, which is where MT940
	// puts them and the order a reader expects.
	// A statement line must carry a value date. An entry that names none takes
	// the statement's closing date, which is the date the statement is about.
	statementDate := ""
	for _, bal := range childrenNamed(stmt, "Bal") {
		if deepText(bal, "Cd") == "CLBD" {
			statementDate = balanceDate(bal)
		}
	}

	for i, entry := range childrenNamed(stmt, "Ntry") {
		path := "/Document/BkToCstmrStmt/Stmt/Ntry"
		if i > 0 {
			path = fmt.Sprintf("%s[%d]", path, i+1)
		}
		b.statementEntry(entry, path, statementDate)
	}

	b.set("62F", "/Document/BkToCstmrStmt/Stmt/Bal (CLBD)", closing, 25)

	return &Conversion{
		SourceType: msgID,
		TargetType: "MT940",
		XML:        b.message("940", statementServicer(stmt), "", ""),
		Report:     sortReports(b.reports),
	}, nil
}

// statementServicer finds the BIC of the institution that produced a
// statement, which is what the MT header names as the sender.
func statementServicer(stmt *converter.Node) string {
	if acct, ok := child(stmt, "Acct"); ok {
		if svcr, ok := child(acct, "Svcr"); ok {
			if bic := deepText(svcr, "BICFI"); bic != "" {
				return bic
			}
		}
	}
	return deepText(stmt, "BICFI")
}

// choiceDate reads a date that ISO 20022 wraps in a date-or-date-time choice:
// <ReqdExctnDt><Dt>2026-08-26</Dt></ReqdExctnDt>. Taking the first descendant
// by name finds the wrapper and its empty text, which is why this exists.
func choiceDate(parent *converter.Node, name string) string {
	wrapper, ok := child(parent, name)
	if !ok {
		return ""
	}
	if v := text(wrapper, "Dt"); v != "" {
		return v
	}
	if v := text(wrapper, "DtTm"); v != "" {
		return v
	}
	return strings.TrimSpace(wrapper.Text)
}

// balanceDate reads a balance's date, which camt.053 wraps the same way.
func balanceDate(bal *converter.Node) string { return choiceDate(bal, "Dt") }

// reportUnconvertible names the elements MT has no field for. They are the ones
// a regulator added and a legacy system will never see.
func reportUnconvertible(b *mtBuilder, tx *converter.Node, path string) {
	for _, item := range []struct{ name, note string }{
		{"Purp", "MT has no purpose code field; the payment's purpose is lost"},
		{"RgltryRptg", "MT has no regulatory reporting field beyond free text in 77B"},
		{"Tax", "MT has no structured tax information"},
		{"UltmtDbtr", "MT103 carries an ultimate debtor only as free text in field 50, which this conversion does not use"},
		{"UltmtCdtr", "MT103 carries an ultimate creditor only as free text in field 59, which this conversion does not use"},
		{"MndtRltdInf", "MT has no mandate information field"},
		{"InstrForCdtrAgt", "an instruction to the creditor agent has no MT103 field of its own"},
	} {
		node, ok := child(tx, item.name)
		if !ok {
			continue
		}
		b.lost(path+"/"+item.name, item.note, firstLine(strings.TrimSpace(node.Text)))
	}
}

// reverseChargeBearer maps an ISO 20022 charge bearer back to its MT code.
func reverseChargeBearer(code string) (string, bool) {
	switch strings.ToUpper(strings.TrimSpace(code)) {
	case "DEBT":
		return "OUR", true
	case "CRED":
		return "BEN", true
	case "SHAR":
		return "SHA", true
	}
	return "", false
}

// isoDateToMT turns "2026-08-24" into "260824", which is how MT writes a date.
func isoDateToMT(iso string) string {
	v := strings.TrimSpace(iso)
	if len(v) < 10 {
		return ""
	}
	return v[2:4] + v[5:7] + v[8:10]
}

// mtAmount writes a number the way MT does: a comma for the decimal point, and
// a trailing comma when there are no decimals at all.
func mtAmount(value string) string {
	v := strings.TrimSpace(value)
	if v == "" {
		return ""
	}
	if !strings.Contains(v, ".") {
		return v + ","
	}
	return strings.Replace(v, ".", ",", 1)
}

// sortReports orders a report by source path, so two conversions of the same
// message read the same way.
func sortReports(reports []FieldReport) []FieldReport {
	out := append([]FieldReport(nil), reports...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Tag < out[j].Tag })
	return out
}

// today is the settlement date used when a message carries none.
func today() string { return time.Now().UTC().Format("2006-01-02") }

// ---------------------------------------------------------------------------
// Statement entries
// ---------------------------------------------------------------------------

// MT940 line widths.
const (
	mtEntryReference = 16
	mtInfoLine       = 65
	mtInfoLines      = 6
)

// mtTransactionType matches the transaction type identification code MT940
// carries: one of N, F or S, then three alphanumerics.
var mtTransactionType = regexp.MustCompile(`^[NFS][A-Z0-9]{3}$`)

// statementEntry writes one :61: statement line and its :86: information.
//
// The transaction type identification code is the part that cannot be derived.
// ISO 20022 describes a transaction with a structured bank transaction code --
// a domain, a family and a sub-family -- and MT940 wants a four-character code
// from a different vocabulary. Anchor uses the proprietary code when the
// statement carries one that already has MT's shape, which is how a bank that
// generated the camt.053 from an MT940 gets its own code back. Otherwise it
// uses NMSC, which is the designated value for a transaction with no more
// specific code, and says so.
func (b *mtBuilder) statementEntry(entry *converter.Node, path, statementDate string) {
	amt, ok := child(entry, "Amt")
	if !ok {
		b.lost(path, "the entry carries no amount, so there is no statement line to write", "")
		return
	}

	sign := "C"
	if text(entry, "CdtDbtInd") == "DBIT" {
		sign = "D"
	}
	// A reversal is marked RC or RD rather than C or D.
	if strings.EqualFold(text(entry, "RvslInd"), "true") {
		sign = "R" + sign
	}

	valueDate := choiceDate(entry, "ValDt")
	if valueDate == "" {
		valueDate = choiceDate(entry, "BookgDt")
	}
	if valueDate == "" {
		valueDate = statementDate
		b.derived("61", "the entry carries no value date; the statement's closing date was used")
	}
	if valueDate == "" {
		b.lost(path, "the entry carries no date and the statement names none, "+
			"so no statement line could be written", "")
		return
	}

	var line strings.Builder
	line.WriteString(isoDateToMT(valueDate))
	// The entry date is the booking date's month and day, written only when it
	// differs from the value date.
	if booking := choiceDate(entry, "BookgDt"); booking != "" && booking != valueDate {
		if mt := isoDateToMT(booking); len(mt) == 6 {
			line.WriteString(mt[2:])
		}
	}
	line.WriteString(sign)
	line.WriteString(mtAmount(amt.Text))
	line.WriteString(b.transactionType(entry, path))

	reference := entryReference(entry)
	trimmed, _ := truncate(reference, mtEntryReference)
	line.WriteString(trimmed)

	if servicer := text(entry, "AcctSvcrRef"); servicer != "" {
		short, _ := truncate(servicer, mtEntryReference)
		line.WriteString("//" + short)
	}

	b.fields = append(b.fields, ":61:"+line.String())
	b.reports = append(b.reports, FieldReport{
		Tag: path, Path: ":61:", Fidelity: FidelityMapped, Value: line.String(),
	})

	if info := entryInformation(entry); len(info) > 0 {
		b.setLines("86", path+"/AddtlNtryInf", info, mtInfoLines, mtInfoLine)
	}
	reportEntryLosses(b, entry, path)
}

// transactionType picks the four-character code MT940 needs.
func (b *mtBuilder) transactionType(entry *converter.Node, path string) string {
	code, ok := child(entry, "BkTxCd")
	if !ok {
		b.derived("61", "the entry carries no bank transaction code; NMSC was used")
		return "NMSC"
	}

	// A proprietary code already shaped like an MT transaction type is the
	// original code coming back, not a guess.
	if prtry, ok := child(code, "Prtry"); ok {
		candidate := strings.ToUpper(text(prtry, "Cd"))
		if mtTransactionType.MatchString(candidate) {
			b.reports = append(b.reports, FieldReport{
				Tag: path + "/BkTxCd/Prtry/Cd", Path: ":61:", Fidelity: FidelityMapped,
				Value: candidate,
			})
			return candidate
		}
	}

	// A structured code describes the transaction in a different vocabulary,
	// and no mapping to MT's is published that Anchor can verify. NMSC is the
	// designated code for a transaction with no more specific one.
	structured := describeBankTransactionCode(code)
	b.reports = append(b.reports, FieldReport{
		Tag: path + "/BkTxCd", Path: ":61:", Fidelity: FidelityTruncated,
		Note: "MT940 wants a four-character transaction type from its own vocabulary, and " +
			"no verifiable mapping from the ISO 20022 bank transaction code exists; NMSC was " +
			"used and the structured code is lost",
		Value: structured,
	})
	return "NMSC"
}

// describeBankTransactionCode renders a structured code for the report.
func describeBankTransactionCode(code *converter.Node) string {
	domain, ok := child(code, "Domn")
	if !ok {
		return ""
	}
	parts := []string{text(domain, "Cd")}
	if family, ok := child(domain, "Fmly"); ok {
		parts = append(parts, text(family, "Cd"), text(family, "SubFmlyCd"))
	}

	var out []string
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, "/")
}

// entryReference finds the reference for the account owner. NONREF is what MT
// carries when there is none, and is not a placeholder Anchor invented.
func entryReference(entry *converter.Node) string {
	if details, ok := child(entry, "NtryDtls"); ok {
		for _, tx := range childrenNamed(details, "TxDtls") {
			if refs, ok := child(tx, "Refs"); ok {
				for _, name := range []string{"EndToEndId", "InstrId", "TxId"} {
					if v := text(refs, name); v != "" {
						return v
					}
				}
			}
		}
	}
	if v := text(entry, "NtryRef"); v != "" {
		return v
	}
	return "NONREF"
}

// entryInformation gathers what MT940 carries in field 86.
func entryInformation(entry *converter.Node) []string {
	var lines []string
	if v := text(entry, "AddtlNtryInf"); v != "" {
		lines = append(lines, v)
	}

	details, ok := child(entry, "NtryDtls")
	if !ok {
		return lines
	}
	for _, tx := range childrenNamed(details, "TxDtls") {
		if rmt, ok := child(tx, "RmtInf"); ok {
			for _, u := range childrenNamed(rmt, "Ustrd") {
				if v := strings.TrimSpace(u.Text); v != "" {
					lines = append(lines, v)
				}
			}
		}
		if v := text(tx, "AddtlTxInf"); v != "" {
			lines = append(lines, v)
		}
	}
	return lines
}

// reportEntryLosses names the entry detail MT940 has no field for.
func reportEntryLosses(b *mtBuilder, entry *converter.Node, path string) {
	details, ok := child(entry, "NtryDtls")
	if !ok {
		return
	}
	for _, tx := range childrenNamed(details, "TxDtls") {
		for _, item := range []struct{ name, note string }{
			{"RltdPties", "MT940 carries the counterparty only as free text in field 86"},
			{"RltdAgts", "MT940 has no field for the agents involved in an entry"},
			{"Chrgs", "MT940 has no per-entry charges field"},
			{"RtrInf", "MT940 has no structured return reason"},
			{"Purp", "MT940 has no purpose code field"},
		} {
			if _, ok := child(tx, item.name); !ok {
				continue
			}
			b.lost(path+"/NtryDtls/TxDtls/"+item.name, item.note, "")
		}
	}
}
