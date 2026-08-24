// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package swift

import (
	"fmt"
	"strings"

	"github.com/sebastienrousseau/anchor/internal/converter"
)

// The initiation messages invert the interbank ones: a corporate instructs its
// bank rather than a bank instructing another bank, and one instruction can
// carry many transactions. MT101 and MT104 have the same two-sequence shape, so
// the conversion produces sequence A once and sequence B per transaction.
//
//	pain.001 -> MT101   request for transfer
//	pain.008 -> MT104   request for debit transfer
//	pacs.010 -> MT204   financial markets direct debit

// convertPain001 turns a customer credit transfer initiation into an MT101.
func convertPain001(root *converter.Node, msgID string) (*Conversion, error) {
	b := &mtBuilder{}

	body, _, ok := find(root, "CstmrCdtTrfInitn")
	if !ok {
		return nil, fmt.Errorf("%s carries no <CstmrCdtTrfInitn>", msgID)
	}
	header, _ := child(body, "GrpHdr")

	instructions := childrenNamed(body, "PmtInf")
	if len(instructions) == 0 {
		return nil, fmt.Errorf("%s carries no payment instruction", msgID)
	}

	b.set("20", "/Document/CstmrCdtTrfInitn/GrpHdr/MsgId",
		text(header, "MsgId"), mtReferenceWidth)

	// :28D: is mandatory in MT101: the message index and total. A single
	// message is 1/1, which is what a converted instruction always is.
	b.fields = append(b.fields, ":28D:1/1")
	b.derived("28D", "the message index and total are mandatory in MT101; a single message is 1/1")

	// Sequence A takes its execution date, ordering customer and account
	// servicing institution from the first instruction. MT101 allows a
	// transaction to override each, which is how the rest are carried.
	first := instructions[0]
	executionDate := choiceDate(first, "ReqdExctnDt")
	if executionDate == "" {
		executionDate = today()
		b.derived("30", "the instruction carried no execution date; today's date was used")
	}
	b.set("30", "/Document/CstmrCdtTrfInitn/PmtInf/ReqdExctnDt",
		isoDateToMT(executionDate), 6)

	if initiating, ok := readParty(header, "InitgPty"); ok && initiating.Name != "" {
		b.set("50L", "/Document/CstmrCdtTrfInitn/GrpHdr/InitgPty", initiating.Name, mtLineWidth)
	}

	debtor, hasDebtor := readParty(first, "Dbtr")
	if !hasDebtor {
		return nil, fmt.Errorf("%s carries no <Dbtr>", msgID)
	}
	debtor.Account = accountID(first, "DbtrAcct")
	b.setLines("50H", "/Document/CstmrCdtTrfInitn/PmtInf/Dbtr",
		partyLinesFor(debtor), mtPartyLines, mtLineWidth)
	b.reportPartyLosses(debtor, "/Document/CstmrCdtTrfInitn/PmtInf/Dbtr")

	debtorAgent := agentBIC(first, "DbtrAgt")
	b.set("52A", "/Document/CstmrCdtTrfInitn/PmtInf/DbtrAgt", debtorAgent, 11)

	var creditorAgent string
	for i, pmt := range instructions {
		path := fmt.Sprintf("/Document/CstmrCdtTrfInitn/PmtInf[%d]", i+1)
		if i == 0 {
			path = "/Document/CstmrCdtTrfInitn/PmtInf"
		}

		transactions := childrenNamed(pmt, "CdtTrfTxInf")
		if len(transactions) == 0 {
			continue
		}
		for j, tx := range transactions {
			txPath := path + "/CdtTrfTxInf"
			if j > 0 {
				txPath = fmt.Sprintf("%s/CdtTrfTxInf[%d]", path, j+1)
			}
			agent, err := b.transferSequenceB(tx, txPath, msgID)
			if err != nil {
				return nil, err
			}
			if agent != "" {
				creditorAgent = agent
			}
		}
		reportInstructionLosses(b, pmt, path)
	}

	return &Conversion{
		SourceType: msgID,
		TargetType: "MT101",
		XML:        b.message("101", debtorAgent, creditorAgent, ""),
		Report:     sortReports(b.reports),
	}, nil
}

// transferSequenceB writes one MT101 transaction, returning the creditor agent
// so the header can name it.
func (b *mtBuilder) transferSequenceB(tx *converter.Node, path, msgID string) (string, error) {
	reference := deepText(tx, "EndToEndId")
	if reference == "" {
		reference = deepText(tx, "InstrId")
	}
	if reference == "" {
		return "", fmt.Errorf("%s carries a transaction with no identification", msgID)
	}
	b.set("21", path+"/PmtId/EndToEndId", reference, mtReferenceWidth)

	if instrID := deepText(tx, "InstrId"); instrID != "" && instrID != reference {
		b.set("21F", path+"/PmtId/InstrId", instrID, mtReferenceWidth)
	}
	// MT fixes the order of its fields, and a receiver reads them in it. The
	// calls below follow the MT101 sequence B layout rather than the order the
	// ISO 20022 elements happen to appear in.
	if instr := text(tx, "InstrForDbtrAgt"); instr != "" {
		b.set("23E", path+"/InstrForDbtrAgt", instr, mtLineWidth)
	}

	// The amount sits inside a choice: an instructed amount or an equivalent.
	amt, ok := instructedAmount(tx)
	if !ok {
		return "", fmt.Errorf("%s carries a transaction with no amount", msgID)
	}
	b.set("32B", path+"/Amt/InstdAmt", attr(amt, "Ccy")+mtAmount(amt.Text), 18)

	creditorAgent := agentBIC(tx, "CdtrAgt")
	b.set("57A", path+"/CdtrAgt", creditorAgent, 11)

	creditor, ok := readParty(tx, "Cdtr")
	if !ok {
		return "", fmt.Errorf("%s carries a transaction with no creditor", msgID)
	}
	creditor.Account = accountID(tx, "CdtrAcct")
	b.setLines("59", path+"/Cdtr", partyLinesFor(creditor), mtPartyLines, mtLineWidth)
	b.reportPartyLosses(creditor, path+"/Cdtr")

	if rmt, ok := child(tx, "RmtInf"); ok {
		var lines []string
		for _, u := range childrenNamed(rmt, "Ustrd") {
			lines = append(lines, strings.TrimSpace(u.Text))
		}
		if len(lines) > 0 {
			b.setLines("70", path+"/RmtInf/Ustrd", lines, mtRemittanceLine, mtLineWidth)
		}
		if strd, ok := child(rmt, "Strd"); ok {
			b.lost(path+"/RmtInf/Strd",
				"structured remittance information has no MT equivalent", firstLine(strd.Text))
		}
	}

	// :36: closes sequence B, after the charge fields.
	if rate := deepText(tx, "XchgRate"); rate != "" {
		b.set("36", path+"/XchgRateInf/XchgRate", strings.Replace(rate, ".", ",", 1), 12)
	}

	reportUnconvertible(b, tx, path)
	return creditorAgent, nil
}

// instructedAmount reads the amount out of the AmountType choice.
func instructedAmount(tx *converter.Node) (*converter.Node, bool) {
	amt, ok := child(tx, "Amt")
	if !ok {
		return nil, false
	}
	if instd, ok := child(amt, "InstdAmt"); ok {
		return instd, true
	}
	if eqvt, ok := child(amt, "EqvtAmt"); ok {
		return child(eqvt, "Amt")
	}
	return nil, false
}

// convertPain008 turns a customer direct debit initiation into an MT104.
func convertPain008(root *converter.Node, msgID string) (*Conversion, error) {
	b := &mtBuilder{}

	body, _, ok := find(root, "CstmrDrctDbtInitn")
	if !ok {
		return nil, fmt.Errorf("%s carries no <CstmrDrctDbtInitn>", msgID)
	}
	header, _ := child(body, "GrpHdr")

	instructions := childrenNamed(body, "PmtInf")
	if len(instructions) == 0 {
		return nil, fmt.Errorf("%s carries no payment instruction", msgID)
	}

	b.set("20", "/Document/CstmrDrctDbtInitn/GrpHdr/MsgId",
		text(header, "MsgId"), mtReferenceWidth)

	first := instructions[0]
	collectionDate := choiceDate(first, "ReqdColltnDt")
	if collectionDate == "" {
		collectionDate = text(first, "ReqdColltnDt")
	}
	if collectionDate == "" {
		collectionDate = today()
		b.derived("30", "the instruction carried no collection date; today's date was used")
	}
	b.set("30", "/Document/CstmrDrctDbtInitn/PmtInf/ReqdColltnDt",
		isoDateToMT(collectionDate), 6)

	if initiating, ok := readParty(header, "InitgPty"); ok && initiating.Name != "" {
		b.set("50C", "/Document/CstmrDrctDbtInitn/GrpHdr/InitgPty", initiating.Name, mtLineWidth)
	}

	creditor, hasCreditor := readParty(first, "Cdtr")
	if !hasCreditor {
		return nil, fmt.Errorf("%s carries no <Cdtr>", msgID)
	}
	creditor.Account = accountID(first, "CdtrAcct")
	b.setLines("50K", "/Document/CstmrDrctDbtInitn/PmtInf/Cdtr",
		partyLinesFor(creditor), mtPartyLines, mtLineWidth)
	b.reportPartyLosses(creditor, "/Document/CstmrDrctDbtInitn/PmtInf/Cdtr")

	creditorAgent := agentBIC(first, "CdtrAgt")
	b.set("52A", "/Document/CstmrDrctDbtInitn/PmtInf/CdtrAgt", creditorAgent, 11)

	if bearer := text(first, "ChrgBr"); bearer != "" {
		if mt, ok := reverseChargeBearer(bearer); ok {
			b.set("71A", "/Document/CstmrDrctDbtInitn/PmtInf/ChrgBr", mt, 3)
		}
	}

	var debtorAgent string
	for i, pmt := range instructions {
		path := fmt.Sprintf("/Document/CstmrDrctDbtInitn/PmtInf[%d]", i+1)
		if i == 0 {
			path = "/Document/CstmrDrctDbtInitn/PmtInf"
		}
		for j, tx := range childrenNamed(pmt, "DrctDbtTxInf") {
			txPath := path + "/DrctDbtTxInf"
			if j > 0 {
				txPath = fmt.Sprintf("%s/DrctDbtTxInf[%d]", path, j+1)
			}
			agent, err := b.debitSequenceB(tx, txPath, msgID)
			if err != nil {
				return nil, err
			}
			if agent != "" {
				debtorAgent = agent
			}
		}
		reportInstructionLosses(b, pmt, path)
	}

	return &Conversion{
		SourceType: msgID,
		TargetType: "MT104",
		XML:        b.message("104", creditorAgent, debtorAgent, ""),
		Report:     sortReports(b.reports),
	}, nil
}

// debitSequenceB writes one MT104 transaction, returning the debtor agent.
func (b *mtBuilder) debitSequenceB(tx *converter.Node, path, msgID string) (string, error) {
	reference := deepText(tx, "EndToEndId")
	if reference == "" {
		return "", fmt.Errorf("%s carries a transaction with no identification", msgID)
	}
	b.set("21", path+"/PmtId/EndToEndId", reference, mtReferenceWidth)

	// MT104 sequence B is laid out :21: :23E: :21C: :32B: :57a: :59a: :70:,
	// and a receiver reads the fields in that order.
	if instr := text(tx, "InstrForCdtrAgt"); instr != "" {
		b.set("23E", path+"/InstrForCdtrAgt", instr, mtLineWidth)
	}
	if mandate := deepText(tx, "MndtId"); mandate != "" {
		b.set("21C", path+"/DrctDbtTx/MndtRltdInf/MndtId", mandate, mtReferenceWidth)
	}

	amt, ok := child(tx, "InstdAmt")
	if !ok {
		return "", fmt.Errorf("%s carries a transaction with no amount", msgID)
	}
	b.set("32B", path+"/InstdAmt", attr(amt, "Ccy")+mtAmount(amt.Text), 18)

	debtorAgent := agentBIC(tx, "DbtrAgt")
	b.set("57A", path+"/DbtrAgt", debtorAgent, 11)

	debtor, ok := readParty(tx, "Dbtr")
	if !ok {
		return "", fmt.Errorf("%s carries a transaction with no debtor", msgID)
	}
	debtor.Account = accountID(tx, "DbtrAcct")
	b.setLines("59", path+"/Dbtr", partyLinesFor(debtor), mtPartyLines, mtLineWidth)
	b.reportPartyLosses(debtor, path+"/Dbtr")

	if rmt, ok := child(tx, "RmtInf"); ok {
		var lines []string
		for _, u := range childrenNamed(rmt, "Ustrd") {
			lines = append(lines, strings.TrimSpace(u.Text))
		}
		if len(lines) > 0 {
			b.setLines("70", path+"/RmtInf/Ustrd", lines, mtRemittanceLine, mtLineWidth)
		}
	}

	reportUnconvertible(b, tx, path)
	return debtorAgent, nil
}

// convertPacs010 turns a financial institution direct debit into an MT204.
func convertPacs010(root *converter.Node, msgID string) (*Conversion, error) {
	b := &mtBuilder{}

	body, _, ok := find(root, "FIDrctDbt")
	if !ok {
		return nil, fmt.Errorf("%s carries no <FIDrctDbt>", msgID)
	}
	header, _ := child(body, "GrpHdr")

	instructions := childrenNamed(body, "CdtInstr")
	if len(instructions) == 0 {
		return nil, fmt.Errorf("%s carries no credit instruction", msgID)
	}
	instruction := instructions[0]
	if len(instructions) > 1 {
		b.lost("/Document/FIDrctDbt/CdtInstr",
			fmt.Sprintf("MT204 carries one collection; %d were present and only the first was converted",
				len(instructions)), "")
	}

	reference := text(instruction, "CdtId")
	referencePath := "/Document/FIDrctDbt/CdtInstr/CdtId"
	if reference == "" {
		reference = text(header, "MsgId")
		referencePath = "/Document/FIDrctDbt/GrpHdr/MsgId"
	}
	b.set("20", referencePath, reference, mtReferenceWidth)

	if sum := text(instruction, "TtlIntrBkSttlmAmt"); sum != "" {
		b.set("19", "/Document/FIDrctDbt/CdtInstr/TtlIntrBkSttlmAmt", mtAmount(sum), 17)
	}
	if date := text(instruction, "IntrBkSttlmDt"); date != "" {
		b.set("30", "/Document/FIDrctDbt/CdtInstr/IntrBkSttlmDt", isoDateToMT(date), 6)
	}

	creditor := agentBIC(instruction, "Cdtr")
	if creditor == "" {
		return nil, fmt.Errorf("%s names no creditor institution, which MT204 requires", msgID)
	}
	b.set("58A", "/Document/FIDrctDbt/CdtInstr/Cdtr", creditor, 11)

	transactions := childrenNamed(instruction, "DrctDbtTxInf")
	if len(transactions) == 0 {
		return nil, fmt.Errorf("%s carries no transaction", msgID)
	}

	var lastDebtor string
	for i, tx := range transactions {
		path := "/Document/FIDrctDbt/CdtInstr/DrctDbtTxInf"
		if i > 0 {
			path = fmt.Sprintf("%s[%d]", path, i+1)
		}

		reference := deepText(tx, "EndToEndId")
		if reference == "" {
			return nil, fmt.Errorf("%s carries a transaction with no identification", msgID)
		}
		// Sequence B opens with :20:, its own transaction reference.
		b.set("20", path+"/PmtId/EndToEndId", reference, mtReferenceWidth)

		amt, ok := child(tx, "IntrBkSttlmAmt")
		if !ok {
			return nil, fmt.Errorf("%s carries a transaction with no amount", msgID)
		}
		b.set("32B", path+"/IntrBkSttlmAmt", attr(amt, "Ccy")+mtAmount(amt.Text), 18)

		debtor := agentBIC(tx, "Dbtr")
		if debtor == "" {
			return nil, fmt.Errorf("%s carries a transaction with no debtor institution", msgID)
		}
		b.set("53A", path+"/Dbtr", debtor, 11)
		lastDebtor = debtor

		if rmt, ok := child(tx, "RmtInf"); ok {
			var lines []string
			for _, u := range childrenNamed(rmt, "Ustrd") {
				lines = append(lines, strings.TrimSpace(u.Text))
			}
			if len(lines) > 0 {
				b.setLines("72", path+"/RmtInf/Ustrd", lines, 6, mtLineWidth)
			}
		}
	}

	return &Conversion{
		SourceType: msgID,
		TargetType: "MT204",
		XML:        b.message("204", lastDebtor, creditor, ""),
		Report:     sortReports(b.reports),
	}, nil
}

// reportInstructionLosses names what an instruction carries that MT cannot.
func reportInstructionLosses(b *mtBuilder, pmt *converter.Node, path string) {
	for _, item := range []struct{ name, note string }{
		{"PmtTpInf", "payment type information is scheme-specific and has no MT field"},
		{"UltmtDbtr", "MT carries an ultimate debtor only as free text, which this conversion does not use"},
		{"UltmtCdtr", "MT carries an ultimate creditor only as free text, which this conversion does not use"},
		{"CdtrSchmeId", "the creditor scheme identification has no MT field"},
	} {
		node, ok := child(pmt, item.name)
		if !ok {
			continue
		}
		b.lost(path+"/"+item.name, item.note, firstLine(strings.TrimSpace(node.Text)))
	}
}
