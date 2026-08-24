// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package swift

import (
	"fmt"
	"strings"
)

// The direct debit messages retire in the 2028 milestone rather than 2026, which
// is why they come after the credit transfers. Their shape is the mirror image:
// the creditor sits at the instruction level and the debtor at the transaction
// level, because one creditor collects from many debtors.

// pain008Input is a customer direct debit initiation: one creditor, one
// collection date, and one or more debtors.
type pain008Input struct {
	MsgID        string
	CreatedAt    string
	InitiatingBy partyInfo
	Instructions []pain008Instruction
}

// pain008Instruction is one PmtInf: everything that applies to a creditor's
// collection on a given date.
type pain008Instruction struct {
	ID             string
	CollectionDate string
	Creditor       partyInfo
	CreditorAgent  string
	Charges        string
	ChargesAcct    string
	Transaction    pain008Transaction
}

// pain008Transaction is one DrctDbtTxInf: what is collected from one debtor.
type pain008Transaction struct {
	EndToEndID      string
	InstructionID   string
	Currency        string
	Amount          string
	MandateID       string
	DebtorAgent     string
	Debtor          partyInfo
	InstructionInfo string
	Remittance      string
}

func buildPain008(in pain008Input) string {
	var b strings.Builder

	fmt.Fprintf(&b, `<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pain.008.001.07">
  <CstmrDrctDbtInitn>
    <GrpHdr>
      <MsgId>%s</MsgId>
      <CreDtTm>%s</CreDtTm>
      <NbOfTxs>%d</NbOfTxs>
      <InitgPty>
%s
      </InitgPty>
    </GrpHdr>`,
		esc(in.MsgID), esc(in.CreatedAt), len(in.Instructions),
		partyBlock(in.InitiatingBy, "        "))

	for _, pmt := range in.Instructions {
		tx := pmt.Transaction

		// PaymentInstruction fixes the order: PmtInfId, PmtMtd, NbOfTxs,
		// ReqdColltnDt, Cdtr, CdtrAcct, CdtrAgt, ChrgBr, ChrgsAcct.
		fmt.Fprintf(&b, `
    <PmtInf>
      <PmtInfId>%s</PmtInfId>
      <PmtMtd>DD</PmtMtd>
      <NbOfTxs>1</NbOfTxs>
      <ReqdColltnDt>%s</ReqdColltnDt>
      <Cdtr>
%s
      </Cdtr>
      <CdtrAcct>%s
      </CdtrAcct>
      <CdtrAgt>
        <FinInstnId>
          <BICFI>%s</BICFI>
        </FinInstnId>
      </CdtrAgt>`,
			esc(pmt.ID), esc(pmt.CollectionDate),
			partyBlock(pmt.Creditor, "        "),
			accountBlockOrPlaceholder(pmt.Creditor.Account, "        "),
			esc(pmt.CreditorAgent))

		if pmt.Charges != "" {
			fmt.Fprintf(&b, "\n      <ChrgBr>%s</ChrgBr>", esc(pmt.Charges))
		}
		if pmt.ChargesAcct != "" {
			fmt.Fprintf(&b, "\n      <ChrgsAcct>%s\n      </ChrgsAcct>",
				accountBlock(pmt.ChargesAcct, "        "))
		}

		// DirectDebitTransactionInformation fixes the order: PmtId, InstdAmt,
		// ChrgBr, DrctDbtTx, DbtrAgt, Dbtr, DbtrAcct, InstrForCdtrAgt, RmtInf.
		fmt.Fprintf(&b, `
      <DrctDbtTxInf>
        <PmtId>%s
          <EndToEndId>%s</EndToEndId>
        </PmtId>
        <InstdAmt Ccy="%s">%s</InstdAmt>`,
			optionalIndented("InstrId", tx.InstructionID, "          "),
			esc(tx.EndToEndID), esc(tx.Currency), esc(tx.Amount))

		if tx.MandateID != "" {
			// MT104 carries the mandate reference but not the date it was
			// signed, so DtOfSgntr is left out rather than invented.
			fmt.Fprintf(&b, `
        <DrctDbtTx>
          <MndtRltdInf>
            <MndtId>%s</MndtId>
          </MndtRltdInf>
        </DrctDbtTx>`, esc(tx.MandateID))
		}

		fmt.Fprintf(&b, `
        <DbtrAgt>
          <FinInstnId>
            <BICFI>%s</BICFI>
          </FinInstnId>
        </DbtrAgt>
        <Dbtr>
%s
        </Dbtr>
        <DbtrAcct>%s
        </DbtrAcct>`,
			esc(tx.DebtorAgent),
			partyBlock(tx.Debtor, "          "),
			accountBlockOrPlaceholder(tx.Debtor.Account, "          "))

		if tx.InstructionInfo != "" {
			fmt.Fprintf(&b, "\n        <InstrForCdtrAgt>%s</InstrForCdtrAgt>", esc(tx.InstructionInfo))
		}
		if tx.Remittance != "" {
			fmt.Fprintf(&b, "\n        <RmtInf>\n          <Ustrd>%s</Ustrd>\n        </RmtInf>", esc(tx.Remittance))
		}

		b.WriteString("\n      </DrctDbtTxInf>\n    </PmtInf>")
	}

	b.WriteString("\n  </CstmrDrctDbtInitn>\n</Document>")
	return b.String()
}

// pacs010Input is a financial institution direct debit: one institution
// collecting from others.
type pacs010Input struct {
	MsgID          string
	CreatedAt      string
	CreditID       string
	Creditor       string
	SettlementDate string
	Instructing    string
	Transactions   []pacs010Transaction
}

// pacs010Transaction is one collection from one institution.
type pacs010Transaction struct {
	EndToEndID string
	Currency   string
	Amount     string
	Debtor     string
	Remittance string
}

func buildPacs010(in pacs010Input) string {
	var b strings.Builder

	fmt.Fprintf(&b, `<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.010.001.06">
  <FIDrctDbt>
    <GrpHdr>
      <MsgId>%s</MsgId>
      <CreDtTm>%s</CreDtTm>
      <NbOfTxs>%d</NbOfTxs>%s
    </GrpHdr>
    <CdtInstr>
      <CdtId>%s</CdtId>%s
      <Cdtr>
        <FinInstnId>
          <BICFI>%s</BICFI>
        </FinInstnId>
      </Cdtr>`,
		esc(in.MsgID), esc(in.CreatedAt), len(in.Transactions),
		agentElement("InstgAgt", in.Instructing, "      "),
		esc(in.CreditID),
		optionalIndented("IntrBkSttlmDt", in.SettlementDate, "      "),
		esc(in.Creditor))

	for _, tx := range in.Transactions {
		fmt.Fprintf(&b, `
      <DrctDbtTxInf>
        <PmtId>
          <EndToEndId>%s</EndToEndId>
        </PmtId>
        <IntrBkSttlmAmt Ccy="%s">%s</IntrBkSttlmAmt>
        <Dbtr>
          <FinInstnId>
            <BICFI>%s</BICFI>
          </FinInstnId>
        </Dbtr>`,
			esc(tx.EndToEndID), esc(tx.Currency), esc(tx.Amount), esc(tx.Debtor))

		if tx.Remittance != "" {
			fmt.Fprintf(&b, "\n        <RmtInf>\n          <Ustrd>%s</Ustrd>\n        </RmtInf>", esc(tx.Remittance))
		}
		b.WriteString("\n      </DrctDbtTxInf>")
	}

	b.WriteString("\n    </CdtInstr>\n  </FIDrctDbt>\n</Document>")
	return b.String()
}

// agentElement renders an optional agent identified by BIC.
func agentElement(name, bic, indent string) string {
	if bic == "" {
		return ""
	}
	return fmt.Sprintf("\n%s<%s>\n%s  <FinInstnId>\n%s    <BICFI>%s</BICFI>\n%s  </FinInstnId>\n%s</%s>",
		indent, name, indent, indent, esc(bic), indent, indent, name)
}
