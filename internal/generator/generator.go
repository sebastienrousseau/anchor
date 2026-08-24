// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package generator

import (
	"crypto/rand"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Options holds parameters for generating synthetic ISO 20022 messages.
type Options struct {
	MsgType      string // e.g. "pacs.008", "pain.001", "pacs.009", "camt.053"
	Amount       string // e.g. "15000.00"
	Currency     string // e.g. "EUR", "USD", "GBP"
	Debtor       string // e.g. "Acme Corporation"
	Creditor     string // e.g. "Global Suppliers Ltd"
	DebtorIBAN   string
	CreditorIBAN string
	DebtorBIC    string
	CreditorBIC  string
	EndToEndID   string
	UETR         string
	Preset       string // e.g. "sepa", "fednow", "target2", "chaps", "standard"
	WithBAH      bool   // Wrap inside Business Application Header (head.001.001.02)

	// AccountScheme selects how accounts are identified. "IBAN" uses
	// <IBAN>; "OTHR" uses <Othr><Id>, which is what rails without an IBAN
	// scheme require -- FedNow and Fedwire among them.
	AccountScheme string

	// ClearingSystem is the ISO 20022 external clearing-system code used in
	// <ClrSysMmbId>, for example "USABA". Empty means agents are addressed by
	// BIC alone.
	ClearingSystem    string
	DebtorMemberID    string
	CreditorMemberID  string
	DebtorAccountID   string
	CreditorAccountID string
}

// Account identification schemes.
const (
	SchemeIBAN  = "IBAN"
	SchemeOther = "OTHR"
)

// DefaultOptions returns standard sensible defaults for message generation.
func generateUUIDv4() string {
	var uuid [16]byte
	_, _ = rand.Read(uuid[:])
	uuid[6] = (uuid[6] & 0x0f) | 0x40 // Version 4
	uuid[8] = (uuid[8] & 0x3f) | 0x80 // RFC 4122 variant
	return fmt.Sprintf("%x-%x-%x-%x-%x", uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:])
}

func DefaultOptions(msgType string) Options {
	return Options{
		MsgType:      msgType,
		Amount:       "5000.00",
		Currency:     "EUR",
		Debtor:       "Acme International Corp",
		Creditor:     "Global Logistics GmbH",
		DebtorIBAN:   "DE89370400440532013000",
		CreditorIBAN: "FR7630006000011234567890189",
		DebtorBIC:    "DEUTDEDDXXX",
		CreditorBIC:  "BNPAFRPPXXX",
		EndToEndID:   fmt.Sprintf("E2E-%d", time.Now().Unix()),
		UETR:         generateUUIDv4(),
		Preset:       "standard",
		WithBAH:      false,
	}
}

// ApplyPreset customizes options based on regional clearing guidelines.
func (opt *Options) ApplyPreset() {
	preset := strings.ToLower(strings.TrimSpace(opt.Preset))
	switch preset {
	case "sepa":
		opt.Currency = "EUR"
		opt.AccountScheme = SchemeIBAN
		opt.DebtorIBAN = "DE89370400440532013000"
		opt.CreditorIBAN = "FR7630006000011234567890189"
		opt.DebtorBIC = "DEUTDEDDXXX"
		opt.CreditorBIC = "BNPAFRPPXXX"
	case "fednow", "fedwire":
		// The United States has no IBAN scheme. FedNow and Fedwire identify
		// accounts by an ABA routing number in ClrSysMmbId plus a domestic
		// account number in Othr.
		opt.Currency = "USD"
		opt.AccountScheme = SchemeOther
		opt.ClearingSystem = "USABA"
		opt.DebtorIBAN = ""
		opt.CreditorIBAN = ""
		opt.DebtorMemberID = "021000021"
		opt.CreditorMemberID = "026009593"
		opt.DebtorAccountID = "4451234567"
		opt.CreditorAccountID = "9876543210"
		opt.DebtorBIC = "CHASUS33XXX"
		opt.CreditorBIC = "BOFAUS3NXXX"
	case "target2":
		opt.Currency = "EUR"
		opt.AccountScheme = SchemeIBAN
		opt.DebtorIBAN = "DE89370400440532013000"
		opt.CreditorIBAN = "FR7630006000011234567890189"
		opt.DebtorBIC = "DEUTDEDDXXX"
		opt.CreditorBIC = "BNPAFRPPXXX"
	case "chaps":
		opt.Currency = "GBP"
		opt.AccountScheme = SchemeIBAN
		opt.DebtorIBAN = "GB29NWBK60161331926819"
		opt.CreditorIBAN = "GB33BUKB20201555555555"
		opt.DebtorBIC = "NWBKGB2LXXX"
		opt.CreditorBIC = "BARCGB22XXX"
	}
}

// messageDefinitionID returns the full identifier the Business Application
// Header must carry, for example "pacs.008.001.10". The base code alone is not
// a message definition identifier.
func messageDefinitionID(msgType, doc string) string {
	if m := namespaceMsgID.FindStringSubmatch(doc); m != nil {
		return m[1]
	}
	return msgType
}

var namespaceMsgID = regexp.MustCompile(`urn:iso:std:iso:20022:tech:xsd:([a-z]{4}\.\d{3}\.\d{3}\.\d{2})`)

// accountBlock renders <Id> for an account, honouring the rail's scheme. Rails
// without an IBAN scheme identify accounts by a domestic number under <Othr>.
func accountBlock(opt Options, iban, other string, indent string) string {
	if opt.AccountScheme == SchemeOther || iban == "" {
		id := other
		if id == "" {
			id = "000000000"
		}
		return fmt.Sprintf("%s<Othr>\n%s  <Id>%s</Id>\n%s</Othr>", indent, indent, id, indent)
	}
	return fmt.Sprintf("%s<IBAN>%s</IBAN>", indent, iban)
}

// agentBlock renders <FinInstnId> for an agent. On a rail with a clearing
// system, the member identifier is carried alongside the BIC.
func agentBlock(opt Options, bic, memberID, indent string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s<BICFI>%s</BICFI>", indent, bic)
	if opt.ClearingSystem != "" && memberID != "" {
		fmt.Fprintf(&b, "\n%s<ClrSysMmbId>\n%s  <ClrSysId>\n%s    <Cd>%s</Cd>\n%s  </ClrSysId>\n%s  <MmbId>%s</MmbId>\n%s</ClrSysMmbId>",
			indent, indent, indent, opt.ClearingSystem, indent, indent, memberID, indent)
	}
	return b.String()
}

// Generate creates a compliant ISO 20022 XML message payload string.
func Generate(opt Options) (string, error) {
	opt.ApplyPreset()
	norm := strings.ToLower(strings.TrimSpace(opt.MsgType))
	now := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	today := time.Now().UTC().Format("2006-01-02")
	msgID := fmt.Sprintf("MSG-%d", time.Now().UnixNano()/1000000)

	if opt.Amount == "" {
		opt.Amount = "5000.00"
	}
	if opt.Currency == "" {
		opt.Currency = "EUR"
	}
	if opt.Debtor == "" {
		opt.Debtor = "Acme International Corp"
	}
	if opt.Creditor == "" {
		opt.Creditor = "Global Logistics GmbH"
	}
	if opt.DebtorIBAN == "" {
		opt.DebtorIBAN = "DE89370400440532013000"
	}
	if opt.CreditorIBAN == "" {
		opt.CreditorIBAN = "FR7630006000011234567890189"
	}
	if opt.DebtorBIC == "" {
		opt.DebtorBIC = "DEUTDEDDXXX"
	}
	if opt.CreditorBIC == "" {
		opt.CreditorBIC = "BNPAFRPPXXX"
	}
	if opt.EndToEndID == "" {
		opt.EndToEndID = fmt.Sprintf("E2E-%d", time.Now().Unix())
	}
	if opt.UETR == "" {
		opt.UETR = generateUUIDv4()
	}

	clearingSys := "TARGET2"
	if strings.ToLower(opt.Preset) == "fednow" {
		clearingSys = "FDNW"
	} else if strings.ToLower(opt.Preset) == "chaps" {
		clearingSys = "CHAPS"
	} else if strings.ToLower(opt.Preset) == "sepa" {
		clearingSys = "SEPA"
	}

	var xmlDoc string

	switch {
	case strings.Contains(norm, "pacs.008") || strings.Contains(norm, "pacs008"):
		xmlDoc = fmt.Sprintf(`<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">
  <FIToFICstmrCdtTrf>
    <GrpHdr>
      <MsgId>%s</MsgId>
      <CreDtTm>%s</CreDtTm>
      <NbOfTxs>1</NbOfTxs>
      <SttlmInf>
        <SttlmMtd>CLRG</SttlmMtd>
        <ClrSys>
          <Prtry>%s</Prtry>
        </ClrSys>
      </SttlmInf>
    </GrpHdr>
    <CdtTrfTxInf>
      <PmtId>
        <EndToEndId>%s</EndToEndId>
        <UETR>%s</UETR>
      </PmtId>
      <IntrBkSttlmAmt Ccy="%s">%s</IntrBkSttlmAmt>
      <IntrBkSttlmDt>%s</IntrBkSttlmDt>
      <ChrgBr>SHAR</ChrgBr>
      <Dbtr>
        <Nm>%s</Nm>
      </Dbtr>
      <DbtrAcct>
        <Id>
%s
        </Id>
      </DbtrAcct>
      <DbtrAgt>
        <FinInstnId>
%s
        </FinInstnId>
      </DbtrAgt>
      <CdtrAgt>
        <FinInstnId>
%s
        </FinInstnId>
      </CdtrAgt>
      <Cdtr>
        <Nm>%s</Nm>
      </Cdtr>
      <CdtrAcct>
        <Id>
%s
        </Id>
      </CdtrAcct>
      <RmtInf>
        <Ustrd>Invoice Ref INV-%d - Commercial Settlement</Ustrd>
      </RmtInf>
    </CdtTrfTxInf>
  </FIToFICstmrCdtTrf>
</Document>`, msgID, now, clearingSys, opt.EndToEndID, opt.UETR, opt.Currency, opt.Amount, today,
			opt.Debtor,
			accountBlock(opt, opt.DebtorIBAN, opt.DebtorAccountID, "          "),
			agentBlock(opt, opt.DebtorBIC, opt.DebtorMemberID, "          "),
			agentBlock(opt, opt.CreditorBIC, opt.CreditorMemberID, "          "),
			opt.Creditor,
			accountBlock(opt, opt.CreditorIBAN, opt.CreditorAccountID, "          "),
			time.Now().Unix()%100000)

	case strings.Contains(norm, "pacs.009") || strings.Contains(norm, "pacs009"):
		xmlDoc = fmt.Sprintf(`<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.009.001.10">
  <FICdtTrf>
    <GrpHdr>
      <MsgId>%s</MsgId>
      <CreDtTm>%s</CreDtTm>
      <NbOfTxs>1</NbOfTxs>
      <SttlmInf>
        <SttlmMtd>CLRG</SttlmMtd>
        <ClrSys>
          <Prtry>%s</Prtry>
        </ClrSys>
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
%s
        </FinInstnId>
      </Dbtr>
      <Cdtr>
        <FinInstnId>
%s
        </FinInstnId>
      </Cdtr>
    </CdtTrfTxInf>
  </FICdtTrf>
</Document>`, msgID, now, clearingSys, opt.EndToEndID, opt.UETR, opt.Currency, opt.Amount, today,
			opt.DebtorBIC, opt.CreditorBIC,
			// In pacs.009 the debtor and creditor are financial institutions,
			// not customers: this is a bank-to-bank transfer.
			agentBlock(opt, opt.DebtorBIC, opt.DebtorMemberID, "          "),
			agentBlock(opt, opt.CreditorBIC, opt.CreditorMemberID, "          "))

	case strings.Contains(norm, "pain.001") || strings.Contains(norm, "pain001"):
		xmlDoc = fmt.Sprintf(`<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pain.001.001.11">
  <CstmrCdtTrfInitn>
    <GrpHdr>
      <MsgId>%s</MsgId>
      <CreDtTm>%s</CreDtTm>
      <NbOfTxs>1</NbOfTxs>
      <InitgPty>
        <Nm>%s</Nm>
      </InitgPty>
    </GrpHdr>
    <PmtInf>
      <PmtInfId>PMT-%d</PmtInfId>
      <PmtMtd>TRF</PmtMtd>
      <ReqdExctnDt>
        <Dt>%s</Dt>
      </ReqdExctnDt>
      <Dbtr>
        <Nm>%s</Nm>
      </Dbtr>
      <DbtrAcct>
        <Id>
          <IBAN>%s</IBAN>
        </Id>
      </DbtrAcct>
      <DbtrAgt>
        <FinInstnId>
          <BICFI>%s</BICFI>
        </FinInstnId>
      </DbtrAgt>
      <CdtTrfTxInf>
        <PmtId>
          <EndToEndId>%s</EndToEndId>
        </PmtId>
        <Amt>
          <InstdAmt Ccy="%s">%s</InstdAmt>
        </Amt>
        <CdtrAgt>
          <FinInstnId>
            <BICFI>%s</BICFI>
          </FinInstnId>
        </CdtrAgt>
        <Cdtr>
          <Nm>%s</Nm>
        </Cdtr>
        <CdtrAcct>
          <Id>
            <IBAN>%s</IBAN>
          </Id>
        </CdtrAcct>
      </CdtTrfTxInf>
    </PmtInf>
  </CstmrCdtTrfInitn>
</Document>`, msgID, now, opt.Debtor, time.Now().Unix()%100000, today, opt.Debtor, opt.DebtorIBAN, opt.DebtorBIC, opt.EndToEndID, opt.Currency, opt.Amount, opt.CreditorBIC, opt.Creditor, opt.CreditorIBAN)

	case strings.Contains(norm, "camt.053") || strings.Contains(norm, "camt053"):
		xmlDoc = fmt.Sprintf(`<Document xmlns="urn:iso:std:iso:20022:tech:xsd:camt.053.001.11">
  <BkToCstmrStmt>
    <GrpHdr>
      <MsgId>%s</MsgId>
      <CreDtTm>%s</CreDtTm>
    </GrpHdr>
    <Stmt>
      <Id>STMT-%s</Id>
      <CreDtTm>%s</CreDtTm>
      <Acct>
        <Id>
          <IBAN>%s</IBAN>
        </Id>
      </Acct>
      <Bal>
        <Tp>
          <CdOrPrtry>
            <Cd>OPBD</Cd>
          </CdOrPrtry>
        </Tp>
        <Amt Ccy="%s">%s</Amt>
        <CdtDbtInd>CRDT</CdtDbtInd>
        <Dt>
          <Dt>%s</Dt>
        </Dt>
      </Bal>
      <Bal>
        <Tp>
          <CdOrPrtry>
            <Cd>CLBD</Cd>
          </CdOrPrtry>
        </Tp>
        <Amt Ccy="%s">%s</Amt>
        <CdtDbtInd>CRDT</CdtDbtInd>
        <Dt>
          <Dt>%s</Dt>
        </Dt>
      </Bal>
    </Stmt>
  </BkToCstmrStmt>
</Document>`, msgID, now, today, now, opt.DebtorIBAN, opt.Currency, opt.Amount, today, opt.Currency, opt.Amount, today)

	default:
		return "", fmt.Errorf("generator for message type '%s' is not supported yet (supported: pacs.008, pacs.009, pain.001, camt.053)", opt.MsgType)
	}

	if opt.WithBAH {
		// head.001.001.02 declares AppHdr as its only global element, so the
		// header and the document are siblings inside an envelope the network
		// defines -- they are not nested under a BusMsg in the head namespace.
		// Each part carries its own namespace, which is what makes the pair
		// validate when split and checked individually.
		return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Envelope>
  <AppHdr xmlns="urn:iso:std:iso:20022:tech:xsd:head.001.001.02">
    <Fr>
      <FIId>
        <FinInstnId>
          <BICFI>%s</BICFI>
        </FinInstnId>
      </FIId>
    </Fr>
    <To>
      <FIId>
        <FinInstnId>
          <BICFI>%s</BICFI>
        </FinInstnId>
      </FIId>
    </To>
    <BizMsgIdr>%s</BizMsgIdr>
    <MsgDefIdr>%s</MsgDefIdr>
    <CreDt>%s</CreDt>
  </AppHdr>
  %s
</Envelope>`, opt.DebtorBIC, opt.CreditorBIC, msgID, messageDefinitionID(opt.MsgType, xmlDoc), now, xmlDoc), nil
	}

	return fmt.Sprintf("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n%s", xmlDoc), nil
}

// HasTemplate reports whether a message type has a hand-written template.
//
// Everything else is built from its schema, which needs one installed. Keeping
// the list here rather than in the command means the two can never disagree
// about which is which.
func HasTemplate(msgType string) bool {
	switch strings.ToLower(strings.TrimSpace(msgType)) {
	case "pacs.008", "pacs.009", "pain.001", "camt.053":
		return true
	}
	return false
}

// TemplateTypes lists the message types with a hand-written template.
func TemplateTypes() []string {
	return []string{"pacs.008", "pacs.009", "pain.001", "camt.053"}
}
