// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package swift

import (
	"bytes"
	"crypto/rand"
	"encoding/xml"
	"fmt"
	"strings"
)

// The target documents are assembled here rather than in the conversion logic,
// so the mapping reads as a mapping and the XML shape lives in one place. Every
// value is escaped: MT fields are free text and routinely contain ampersands.

// esc escapes a value for XML character data. Writing to a bytes.Buffer cannot
// fail, so the error EscapeText returns is the writer's and never occurs here.
func esc(s string) string {
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, []byte(s))
	return buf.String()
}

// generateUETR produces an RFC 4122 version 4 UUID, which is what a UETR is.
func generateUETR() string {
	var u [16]byte
	_, _ = rand.Read(u[:])
	u[6] = (u[6] & 0x0f) | 0x40
	u[8] = (u[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", u[0:4], u[4:6], u[6:8], u[8:10], u[10:])
}

// partyBlock renders a party. An MT address is unstructured, so it becomes
// AdrLine -- which is exactly what CBPR+ stops accepting once the deferred
// structured address requirement takes effect,
// and what the conversion report flags.
func partyBlock(p partyInfo, indent string) string {
	var b strings.Builder

	// PartyIdentification fixes the order: Nm, PstlAdr, Id.
	name := p.Name
	if name == "" && p.BIC == "" {
		name = "NOT PROVIDED"
	}
	if name != "" {
		fmt.Fprintf(&b, "%s<Nm>%s</Nm>\n", indent, esc(name))
	}

	if len(p.Address) > 0 {
		fmt.Fprintf(&b, "%s<PstlAdr>\n", indent)
		// PostalAddress permits at most seven address lines.
		for i, line := range p.Address {
			if i >= 7 {
				break
			}
			fmt.Fprintf(&b, "%s  <AdrLine>%s</AdrLine>\n", indent, esc(line))
		}
		fmt.Fprintf(&b, "%s</PstlAdr>\n", indent)
	}

	// Option A parties identify by BIC rather than by name.
	if p.BIC != "" {
		fmt.Fprintf(&b, "%s<Id>\n%s  <OrgId>\n%s    <AnyBIC>%s</AnyBIC>\n%s  </OrgId>\n%s</Id>\n",
			indent, indent, indent, esc(p.BIC), indent, indent)
	}
	return strings.TrimRight(b.String(), "\n")
}

// accountBlock renders an account identifier, choosing IBAN or Othr by shape.
func accountBlock(account, indent string) string {
	if account == "" {
		return ""
	}
	if looksLikeIBAN(account) {
		return fmt.Sprintf("\n%s<Id>\n%s  <IBAN>%s</IBAN>\n%s</Id>", indent, indent, esc(account), indent)
	}
	return fmt.Sprintf("\n%s<Id>\n%s  <Othr>\n%s    <Id>%s</Id>\n%s  </Othr>\n%s</Id>",
		indent, indent, indent, esc(account), indent, indent)
}

// looksLikeIBAN applies the structural test: two letters, two digits, then
// alphanumerics. The checksum is the linter's business, not the converter's.
func looksLikeIBAN(s string) bool {
	v := strings.ToUpper(strings.ReplaceAll(s, " ", ""))
	if len(v) < 15 || len(v) > 34 {
		return false
	}
	for i, r := range v {
		switch {
		case i < 2:
			if r < 'A' || r > 'Z' {
				return false
			}
		case i < 4:
			if r < '0' || r > '9' {
				return false
			}
		default:
			if (r < 'A' || r > 'Z') && (r < '0' || r > '9') {
				return false
			}
		}
	}
	return true
}

type pacs008Input struct {
	MsgID           string
	CreatedAt       string
	UETR            string
	EndToEndID      string
	Currency        string
	Amount          string
	SettlementDay   string
	Instructed      *CurrencyAmount
	ExchangeRate    string
	Charges         string
	SenderCharges   *CurrencyAmount
	ReceiverCharges *CurrencyAmount
	Debtor          partyInfo
	Creditor        partyInfo
	DebtorAgent     string
	CreditorAgent   string
	InstructionCode string
	InstructionInfo string
	Remittance      string
}

func buildPacs008(in pacs008Input) string {
	var b strings.Builder

	fmt.Fprintf(&b, `<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">
  <FIToFICstmrCdtTrf>
    <GrpHdr>
      <MsgId>%s</MsgId>
      <CreDtTm>%s</CreDtTm>
      <NbOfTxs>1</NbOfTxs>
      <SttlmInf>
        <SttlmMtd>CLRG</SttlmMtd>
      </SttlmInf>
    </GrpHdr>
    <CdtTrfTxInf>
      <PmtId>
        <EndToEndId>%s</EndToEndId>
        <UETR>%s</UETR>
      </PmtId>
      <IntrBkSttlmAmt Ccy="%s">%s</IntrBkSttlmAmt>
      <IntrBkSttlmDt>%s</IntrBkSttlmDt>%s%s
      <ChrgBr>%s</ChrgBr>%s%s
      <Dbtr>
%s
      </Dbtr>`,
		esc(in.MsgID), esc(in.CreatedAt), esc(in.EndToEndID), esc(in.UETR),
		esc(in.Currency), esc(in.Amount), esc(in.SettlementDay),
		amountElement("InstdAmt", in.Instructed),
		optionalElement("XchgRate", in.ExchangeRate),
		esc(in.Charges),
		chargesBlock(in.SenderCharges, in.DebtorAgent),
		chargesBlock(in.ReceiverCharges, in.CreditorAgent),
		partyBlock(in.Debtor, "        "))

	if acct := accountBlock(in.Debtor.Account, "        "); acct != "" {
		fmt.Fprintf(&b, "\n      <DbtrAcct>%s\n      </DbtrAcct>", acct)
	}

	fmt.Fprintf(&b, `
      <DbtrAgt>
        <FinInstnId>
          <BICFI>%s</BICFI>
        </FinInstnId>
      </DbtrAgt>
      <CdtrAgt>
        <FinInstnId>
          <BICFI>%s</BICFI>
        </FinInstnId>
      </CdtrAgt>
      <Cdtr>
%s
      </Cdtr>`, esc(in.DebtorAgent), esc(in.CreditorAgent), partyBlock(in.Creditor, "        "))

	if acct := accountBlock(in.Creditor.Account, "        "); acct != "" {
		fmt.Fprintf(&b, "\n      <CdtrAcct>%s\n      </CdtrAcct>", acct)
	}

	if in.InstructionCode != "" || in.InstructionInfo != "" {
		b.WriteString("\n      <InstrForNxtAgt>")
		if in.InstructionCode != "" {
			fmt.Fprintf(&b, "\n        <Cd>%s</Cd>", esc(in.InstructionCode))
		}
		if in.InstructionInfo != "" {
			fmt.Fprintf(&b, "\n        <InstrInf>%s</InstrInf>", esc(in.InstructionInfo))
		}
		b.WriteString("\n      </InstrForNxtAgt>")
	}

	if in.Remittance != "" {
		fmt.Fprintf(&b, "\n      <RmtInf>\n        <Ustrd>%s</Ustrd>\n      </RmtInf>", esc(in.Remittance))
	}

	b.WriteString(`
    </CdtTrfTxInf>
  </FIToFICstmrCdtTrf>
</Document>`)
	return b.String()
}

type pacs009Input struct {
	MsgID         string
	CreatedAt     string
	UETR          string
	EndToEndID    string
	Currency      string
	Amount        string
	SettlementDay string
	Debtor        string
	Creditor      string
	Instructing   string
	Instructed    string
}

func buildPacs009(in pacs009Input) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.009.001.10">
  <FICdtTrf>
    <GrpHdr>
      <MsgId>%s</MsgId>
      <CreDtTm>%s</CreDtTm>
      <NbOfTxs>1</NbOfTxs>
      <SttlmInf>
        <SttlmMtd>CLRG</SttlmMtd>
      </SttlmInf>
    </GrpHdr>
    <CdtTrfTxInf>
      <PmtId>
        <EndToEndId>%s</EndToEndId>
        <UETR>%s</UETR>
      </PmtId>
      <IntrBkSttlmAmt Ccy="%s">%s</IntrBkSttlmAmt>
      <IntrBkSttlmDt>%s</IntrBkSttlmDt>
      <InstgAgt>
        <FinInstnId>
          <BICFI>%s</BICFI>
        </FinInstnId>
      </InstgAgt>
      <InstdAgt>
        <FinInstnId>
          <BICFI>%s</BICFI>
        </FinInstnId>
      </InstdAgt>
      <Dbtr>
        <FinInstnId>
          <BICFI>%s</BICFI>
        </FinInstnId>
      </Dbtr>
      <Cdtr>
        <FinInstnId>
          <BICFI>%s</BICFI>
        </FinInstnId>
      </Cdtr>
    </CdtTrfTxInf>
  </FICdtTrf>
</Document>`,
		esc(in.MsgID), esc(in.CreatedAt), esc(in.EndToEndID), esc(in.UETR),
		esc(in.Currency), esc(in.Amount), esc(in.SettlementDay),
		esc(in.Instructing), esc(in.Instructed), esc(in.Debtor), esc(in.Creditor))
}

type camt053Input struct {
	MsgID       string
	CreatedAt   string
	StatementNo string
	Account     string
	Opening     balanceInfo
	Closing     balanceInfo
	Entries     []entryInfo
}

// entryInfo is one statement entry.
type entryInfo struct {
	Reference       string
	ServicerRef     string
	Currency        string
	Amount          string
	Sign            string
	Reversal        bool
	BookingDate     string
	ValueDate       string
	TransactionType string
	Information     []string
}

func buildCamt053(in camt053Input) string {
	var b strings.Builder

	fmt.Fprintf(&b, `<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:camt.053.001.11">
  <BkToCstmrStmt>
    <GrpHdr>
      <MsgId>%s</MsgId>
      <CreDtTm>%s</CreDtTm>
    </GrpHdr>
    <Stmt>
      <Id>%s</Id>%s
      <CreDtTm>%s</CreDtTm>
      <Acct>%s
      </Acct>`,
		esc(in.MsgID), esc(in.CreatedAt), esc(in.MsgID), sequenceNumber(in.StatementNo),
		esc(in.CreatedAt), accountBlock(in.Account, "        "))

	for _, bal := range []balanceInfo{in.Opening, in.Closing} {
		fmt.Fprintf(&b, `
      <Bal>
        <Tp>
          <CdOrPrtry>
            <Cd>%s</Cd>
          </CdOrPrtry>
        </Tp>
        <Amt Ccy="%s">%s</Amt>
        <CdtDbtInd>%s</CdtDbtInd>
        <Dt>
          <Dt>%s</Dt>
        </Dt>
      </Bal>`, esc(bal.Code), esc(bal.Currency), esc(bal.Amount), esc(bal.Sign), esc(bal.Date))
	}

	for _, e := range in.Entries {
		// ReportEntry fixes the order: NtryRef, Amt, CdtDbtInd, RvslInd, Sts,
		// BookgDt, ValDt, AcctSvcrRef, BkTxCd, NtryDtls, AddtlNtryInf.
		fmt.Fprintf(&b, "\n      <Ntry>")
		// NONREF is how MT says there is no reference; carrying it across would
		// turn the absence of one into a reference that reads like data.
		if e.Reference != "NONREF" {
			b.WriteString(optionalIndented("NtryRef", e.Reference, "        "))
		}
		fmt.Fprintf(&b, `
        <Amt Ccy="%s">%s</Amt>
        <CdtDbtInd>%s</CdtDbtInd>`, esc(e.Currency), esc(e.Amount), esc(e.Sign))

		if e.Reversal {
			b.WriteString("\n        <RvslInd>true</RvslInd>")
		}
		b.WriteString("\n        <Sts>\n          <Cd>BOOK</Cd>\n        </Sts>")

		if e.BookingDate != "" {
			fmt.Fprintf(&b, "\n        <BookgDt>\n          <Dt>%s</Dt>\n        </BookgDt>", esc(e.BookingDate))
		}
		if e.ValueDate != "" {
			fmt.Fprintf(&b, "\n        <ValDt>\n          <Dt>%s</Dt>\n        </ValDt>", esc(e.ValueDate))
		}
		b.WriteString(optionalIndented("AcctSvcrRef", e.ServicerRef, "        "))

		// The MT transaction type belongs in the proprietary branch: it is
		// MT's own vocabulary, not an ISO 20022 domain code, and putting it
		// there is what lets a round trip give it back unchanged.
		fmt.Fprintf(&b, `
        <BkTxCd>
          <Prtry>
            <Cd>%s</Cd>
            <Issr>SWIFT</Issr>
          </Prtry>
        </BkTxCd>`, esc(e.TransactionType))

		if len(e.Information) > 0 {
			fmt.Fprintf(&b, "\n        <AddtlNtryInf>%s</AddtlNtryInf>",
				esc(strings.Join(e.Information, " ")))
		}
		b.WriteString("\n      </Ntry>")
	}

	b.WriteString(`
    </Stmt>
  </BkToCstmrStmt>
</Document>`)
	return b.String()
}

// sequenceNumber renders an MT :28C: statement number as ElctrncSeqNb. The MT
// field is "number/sequence"; only the leading number is a Number, so a
// non-numeric statement number is left out rather than forced in.
func sequenceNumber(v string) string {
	n, _, _ := strings.Cut(strings.TrimSpace(v), "/")
	n = strings.TrimLeft(n, "0")
	if n == "" || len(n) > 18 {
		return ""
	}
	for _, r := range n {
		if r < '0' || r > '9' {
			return ""
		}
	}
	return "\n      <ElctrncSeqNb>" + n + "</ElctrncSeqNb>"
}

// amountElement renders an optional currency-and-amount element.
func amountElement(name string, ca *CurrencyAmount) string {
	if ca == nil {
		return ""
	}
	return fmt.Sprintf("\n      <%s Ccy=\"%s\">%s</%s>", name, esc(ca.Currency), esc(ca.Amount), name)
}

// optionalElement renders an element only when it has a value.
func optionalElement(name, value string) string {
	if value == "" {
		return ""
	}
	return fmt.Sprintf("\n      <%s>%s</%s>", name, esc(value), name)
}

// chargesBlock renders a Charges7, which pairs an amount with the agent that
// took it. MT reports the amount without naming the agent, so the agent is
// derived from the side of the transfer the field belongs to.
func chargesBlock(ca *CurrencyAmount, agentBIC string) string {
	if ca == nil {
		return ""
	}
	return fmt.Sprintf(`
      <ChrgsInf>
        <Amt Ccy="%s">%s</Amt>
        <Agt>
          <FinInstnId>
            <BICFI>%s</BICFI>
          </FinInstnId>
        </Agt>
      </ChrgsInf>`, esc(ca.Currency), esc(ca.Amount), esc(agentBIC))
}

// pain001Input is a customer credit transfer initiation: one debtor party, one
// requested execution date, and one or more transactions.
type pain001Input struct {
	MsgID        string
	CreatedAt    string
	InitiatingBy partyInfo
	Instructions []pain001Instruction
}

// pain001Instruction is one PmtInf: an MT101 transaction, with the debtor
// details that apply to it.
type pain001Instruction struct {
	ID            string
	ExecutionDate string
	Debtor        partyInfo
	DebtorAgent   string
	Charges       string
	ChargesAcct   string
	Transaction   pain001Transaction
}

// pain001Transaction is one CdtTrfTxInf.
type pain001Transaction struct {
	EndToEndID      string
	InstructionID   string
	Currency        string
	Amount          string
	ExchangeRate    string
	CreditorAgent   string
	Creditor        partyInfo
	InstructionInfo string
	Remittance      string
}

func buildPain001(in pain001Input) string {
	var b strings.Builder

	fmt.Fprintf(&b, `<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pain.001.001.09">
  <CstmrCdtTrfInitn>
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
		// ReqdExctnDt, Dbtr, DbtrAcct, DbtrAgt, ChrgBr, ChrgsAcct, CdtTrfTxInf.
		fmt.Fprintf(&b, `
    <PmtInf>
      <PmtInfId>%s</PmtInfId>
      <PmtMtd>TRF</PmtMtd>
      <NbOfTxs>1</NbOfTxs>
      <ReqdExctnDt>
        <Dt>%s</Dt>
      </ReqdExctnDt>
      <Dbtr>
%s
      </Dbtr>
      <DbtrAcct>%s
      </DbtrAcct>
      <DbtrAgt>
        <FinInstnId>
          <BICFI>%s</BICFI>
        </FinInstnId>
      </DbtrAgt>`,
			esc(pmt.ID), esc(pmt.ExecutionDate),
			partyBlock(pmt.Debtor, "        "),
			// DbtrAcct is mandatory, so an MT101 that names no account still has
			// to produce a placeholder rather than an absent element.
			accountBlockOrPlaceholder(pmt.Debtor.Account, "        "),
			esc(pmt.DebtorAgent))

		if pmt.Charges != "" {
			fmt.Fprintf(&b, "\n      <ChrgBr>%s</ChrgBr>", esc(pmt.Charges))
		}
		if pmt.ChargesAcct != "" {
			fmt.Fprintf(&b, "\n      <ChrgsAcct>%s\n      </ChrgsAcct>",
				accountBlock(pmt.ChargesAcct, "        "))
		}

		fmt.Fprintf(&b, `
      <CdtTrfTxInf>
        <PmtId>%s
          <EndToEndId>%s</EndToEndId>
        </PmtId>
        <Amt>
          <InstdAmt Ccy="%s">%s</InstdAmt>
        </Amt>`,
			optionalIndented("InstrId", tx.InstructionID, "          "),
			esc(tx.EndToEndID), esc(tx.Currency), esc(tx.Amount))

		if tx.ExchangeRate != "" {
			fmt.Fprintf(&b, "\n        <XchgRateInf>\n          <XchgRate>%s</XchgRate>\n        </XchgRateInf>",
				esc(tx.ExchangeRate))
		}
		if tx.CreditorAgent != "" {
			fmt.Fprintf(&b, `
        <CdtrAgt>
          <FinInstnId>
            <BICFI>%s</BICFI>
          </FinInstnId>
        </CdtrAgt>`, esc(tx.CreditorAgent))
		}

		fmt.Fprintf(&b, "\n        <Cdtr>\n%s\n        </Cdtr>", partyBlock(tx.Creditor, "          "))
		if acct := accountBlock(tx.Creditor.Account, "          "); acct != "" {
			fmt.Fprintf(&b, "\n        <CdtrAcct>%s\n        </CdtrAcct>", acct)
		}
		if tx.InstructionInfo != "" {
			fmt.Fprintf(&b, "\n        <InstrForDbtrAgt>%s</InstrForDbtrAgt>", esc(tx.InstructionInfo))
		}
		if tx.Remittance != "" {
			fmt.Fprintf(&b, "\n        <RmtInf>\n          <Ustrd>%s</Ustrd>\n        </RmtInf>", esc(tx.Remittance))
		}

		b.WriteString("\n      </CdtTrfTxInf>\n    </PmtInf>")
	}

	b.WriteString("\n  </CstmrCdtTrfInitn>\n</Document>")
	return b.String()
}

// accountBlockOrPlaceholder renders a mandatory account element, naming the gap
// explicitly when the source carried no account.
func accountBlockOrPlaceholder(account, indent string) string {
	if account == "" {
		account = "NOTPROVIDED"
	}
	return accountBlock(account, indent)
}

// optionalIndented renders an element at a given indent only when it has a value.
func optionalIndented(name, value, indent string) string {
	if value == "" {
		return ""
	}
	return fmt.Sprintf("\n%s<%s>%s</%s>", indent, name, esc(value), name)
}
