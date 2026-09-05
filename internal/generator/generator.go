// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package generator

import (
	"bytes"
	"crypto/rand"
	"encoding/xml"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/sebastienrousseau/askiso/internal/linter"
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

	// presetApplied records that callers deliberately applied rail defaults
	// before setting explicit overrides such as Currency.
	presetApplied bool
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

// Presets returns the supported regional clearing presets.
func Presets() []string {
	return []string{"standard", "sepa", "fednow", "fedwire", "target2", "chaps"}
}

// ValidPreset reports whether name is a supported preset. An empty name uses
// the standard defaults for backwards compatibility.
func ValidPreset(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return true
	}
	for _, preset := range Presets() {
		if name == preset {
			return true
		}
	}
	return false
}

// ApplyPreset customizes options based on regional clearing guidelines.
func (opt *Options) ApplyPreset() {
	if opt.presetApplied {
		return
	}
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
	opt.presetApplied = true
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

// EscapeText returns s encoded for use as XML character data or an attribute
// value. Hand-written templates must never interpolate caller input directly.
func EscapeText(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// ValidateOptions rejects values that would make a hand-written payment
// template lexically or semantically invalid.
func ValidateOptions(opt Options) error {
	for name, value := range map[string]string{
		"amount": opt.Amount, "currency": opt.Currency, "debtor": opt.Debtor,
		"creditor": opt.Creditor, "debtor IBAN": opt.DebtorIBAN,
		"creditor IBAN": opt.CreditorIBAN, "debtor BIC": opt.DebtorBIC,
		"creditor BIC": opt.CreditorBIC, "end-to-end ID": opt.EndToEndID,
		"UETR": opt.UETR, "clearing system": opt.ClearingSystem,
		"debtor member ID": opt.DebtorMemberID, "creditor member ID": opt.CreditorMemberID,
		"debtor account ID": opt.DebtorAccountID, "creditor account ID": opt.CreditorAccountID,
	} {
		if !utf8.ValidString(value) || strings.IndexFunc(value, func(r rune) bool {
			return r != '\t' && r != '\n' && r != '\r' && (r < 0x20 || r == 0xFFFE || r == 0xFFFF)
		}) >= 0 {
			return fmt.Errorf("%s contains a character XML cannot represent", name)
		}
	}
	if ok, reason := linter.ValidateCurrencyAmount(opt.Currency, opt.Amount); !ok {
		return errors.New(reason)
	}
	if ok, reason := linter.ValidateBIC(opt.DebtorBIC); !ok {
		return fmt.Errorf("debtor BIC: %s", reason)
	}
	if ok, reason := linter.ValidateBIC(opt.CreditorBIC); !ok {
		return fmt.Errorf("creditor BIC: %s", reason)
	}
	if ok, reason := linter.ValidateUETR(opt.UETR); !ok {
		return fmt.Errorf("UETR: %s", reason)
	}
	if opt.AccountScheme != SchemeOther {
		if ok, reason := linter.ValidateIBAN(opt.DebtorIBAN); !ok {
			return fmt.Errorf("debtor IBAN: %s", reason)
		}
		if ok, reason := linter.ValidateIBAN(opt.CreditorIBAN); !ok {
			return fmt.Errorf("creditor IBAN: %s", reason)
		}
	}
	return nil
}

// accountBlock renders <Id> for an account, honouring the rail's scheme. Rails
// without an IBAN scheme identify accounts by a domestic number under <Othr>.
func AccountBlock(opt Options, iban, other string, indent string) string {
	if opt.AccountScheme == SchemeOther || iban == "" {
		id := other
		if id == "" {
			id = "000000000"
		}
		return fmt.Sprintf("%s<Othr>\n%s  <Id>%s</Id>\n%s</Othr>", indent, indent, EscapeText(id), indent)
	}
	return fmt.Sprintf("%s<IBAN>%s</IBAN>", indent, EscapeText(iban))
}

// agentBlock renders <FinInstnId> for an agent. On a rail with a clearing
// system, the member identifier is carried alongside the BIC.
func AgentBlock(opt Options, bic, memberID, indent string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s<BICFI>%s</BICFI>", indent, EscapeText(bic))
	if opt.ClearingSystem != "" && memberID != "" {
		fmt.Fprintf(&b, "\n%s<ClrSysMmbId>\n%s  <ClrSysId>\n%s    <Cd>%s</Cd>\n%s  </ClrSysId>\n%s  <MmbId>%s</MmbId>\n%s</ClrSysMmbId>",
			indent, indent, indent, EscapeText(opt.ClearingSystem), indent, indent, EscapeText(memberID), indent)
	}
	return b.String()
}

// Generate creates a compliant ISO 20022 XML message payload string.
func Generate(opt Options) (string, error) {
	if !ValidPreset(opt.Preset) {
		return "", fmt.Errorf("unknown preset %q (available: %s)",
			opt.Preset, strings.Join(Presets(), ", "))
	}
	if !opt.presetApplied {
		opt.ApplyPreset()
	}
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
	if err := ValidateOptions(opt); err != nil {
		return "", fmt.Errorf("invalid generator option: %w", err)
	}

	clearingSys := "TARGET2"
	if strings.ToLower(opt.Preset) == "fednow" {
		clearingSys = "FDNW"
	} else if strings.ToLower(opt.Preset) == "fedwire" {
		clearingSys = "FEDWIRE"
	} else if strings.ToLower(opt.Preset) == "chaps" {
		clearingSys = "CHAPS"
	} else if strings.ToLower(opt.Preset) == "sepa" {
		clearingSys = "SEPA"
	}

	var xmlDoc string
	safe := opt
	safe.Amount = EscapeText(opt.Amount)
	safe.Currency = EscapeText(opt.Currency)
	safe.Debtor = EscapeText(opt.Debtor)
	safe.Creditor = EscapeText(opt.Creditor)
	safe.DebtorIBAN = EscapeText(opt.DebtorIBAN)
	safe.CreditorIBAN = EscapeText(opt.CreditorIBAN)
	safe.DebtorBIC = EscapeText(opt.DebtorBIC)
	safe.CreditorBIC = EscapeText(opt.CreditorBIC)
	safe.EndToEndID = EscapeText(opt.EndToEndID)
	safe.UETR = EscapeText(opt.UETR)

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
</Document>`, msgID, now, clearingSys, safe.EndToEndID, safe.UETR, safe.Currency, safe.Amount, today,
			safe.Debtor,
			AccountBlock(opt, opt.DebtorIBAN, opt.DebtorAccountID, "          "),
			AgentBlock(opt, opt.DebtorBIC, opt.DebtorMemberID, "          "),
			AgentBlock(opt, opt.CreditorBIC, opt.CreditorMemberID, "          "),
			safe.Creditor,
			AccountBlock(opt, opt.CreditorIBAN, opt.CreditorAccountID, "          "),
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
</Document>`, msgID, now, clearingSys, safe.EndToEndID, safe.UETR, safe.Currency, safe.Amount, today,
			safe.DebtorBIC, safe.CreditorBIC,
			// In pacs.009 the debtor and creditor are financial institutions,
			// not customers: this is a bank-to-bank transfer.
			AgentBlock(opt, opt.DebtorBIC, opt.DebtorMemberID, "          "),
			AgentBlock(opt, opt.CreditorBIC, opt.CreditorMemberID, "          "))

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
%s
        </Id>
      </DbtrAcct>
      <DbtrAgt>
        <FinInstnId>
%s
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
      </CdtTrfTxInf>
    </PmtInf>
  </CstmrCdtTrfInitn>
</Document>`, msgID, now, safe.Debtor, time.Now().Unix()%100000, today, safe.Debtor,
			AccountBlock(opt, opt.DebtorIBAN, opt.DebtorAccountID, "          "),
			AgentBlock(opt, opt.DebtorBIC, opt.DebtorMemberID, "          "),
			safe.EndToEndID, safe.Currency, safe.Amount,
			AgentBlock(opt, opt.CreditorBIC, opt.CreditorMemberID, "            "),
			safe.Creditor,
			AccountBlock(opt, opt.CreditorIBAN, opt.CreditorAccountID, "            "))

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
%s
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
</Document>`, msgID, now, today, now,
			AccountBlock(opt, opt.DebtorIBAN, opt.DebtorAccountID, "          "),
			safe.Currency, safe.Amount, today, safe.Currency, safe.Amount, today)

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
</Envelope>`, safe.DebtorBIC, safe.CreditorBIC, msgID, messageDefinitionID(opt.MsgType, xmlDoc), now, xmlDoc), nil
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
