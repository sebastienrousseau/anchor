// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package flow

import (
	"crypto/rand"
	"fmt"
	"strings"
	"time"

	"github.com/sebastienrousseau/askiso/internal/generator"
)

// Step represents a single message stage in the payment lifecycle.
type Step struct {
	Index       int    `json:"step_index"`
	MsgType     string `json:"msg_type"`
	Title       string `json:"title"`
	Description string `json:"description"`
	FileName    string `json:"file_name"`
	XMLPayload  string `json:"xml_payload"`
}

// LifecycleChain holds the linked 4-stage transaction flow.
type LifecycleChain struct {
	UETR         string `json:"uetr"`
	EndToEndID   string `json:"end_to_end_id"`
	Amount       string `json:"amount"`
	Currency     string `json:"currency"`
	Debtor       string `json:"debtor"`
	Creditor     string `json:"creditor"`
	DebtorIBAN   string `json:"debtor_iban"`
	CreditorIBAN string `json:"creditor_iban"`
	Preset       string `json:"preset"`
	Steps        []Step `json:"steps"`
}

func generateUUIDv4() string {
	var uuid [16]byte
	_, _ = rand.Read(uuid[:])
	uuid[6] = (uuid[6] & 0x0f) | 0x40 // Version 4
	uuid[8] = (uuid[8] & 0x3f) | 0x80 // RFC 4122 variant
	return fmt.Sprintf("%x-%x-%x-%x-%x", uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:])
}

// GenerateLifecycle builds a complete connected 4-stage transaction chain.
func GenerateLifecycle(opt generator.Options) (*LifecycleChain, error) {
	if !generator.ValidPreset(opt.Preset) {
		return nil, fmt.Errorf("unknown preset %q (available: %s)",
			opt.Preset, strings.Join(generator.Presets(), ", "))
	}
	// Fill blanks from the same defaults the generator uses, so the chain's
	// metadata always describes the payloads it carries. Without this a caller
	// passing bare options gets empty party names beside XML that has them.
	defaults := generator.DefaultOptions(opt.MsgType)
	if opt.Debtor == "" {
		opt.Debtor = defaults.Debtor
	}
	if opt.Creditor == "" {
		opt.Creditor = defaults.Creditor
	}
	if opt.DebtorIBAN == "" {
		opt.DebtorIBAN = defaults.DebtorIBAN
	}
	if opt.CreditorIBAN == "" {
		opt.CreditorIBAN = defaults.CreditorIBAN
	}
	if opt.DebtorBIC == "" {
		opt.DebtorBIC = defaults.DebtorBIC
	}
	if opt.CreditorBIC == "" {
		opt.CreditorBIC = defaults.CreditorBIC
	}

	opt.ApplyPreset()
	if opt.UETR == "" {
		opt.UETR = generateUUIDv4()
	}
	if opt.EndToEndID == "" {
		opt.EndToEndID = fmt.Sprintf("E2E-%d", time.Now().Unix())
	}
	if opt.Amount == "" {
		opt.Amount = "15000.00"
	}
	if opt.Currency == "" {
		opt.Currency = "EUR"
	}
	if err := generator.ValidateOptions(opt); err != nil {
		return nil, fmt.Errorf("invalid lifecycle option: %w", err)
	}
	rawOpt := opt

	chain := &LifecycleChain{
		UETR:         opt.UETR,
		EndToEndID:   opt.EndToEndID,
		Amount:       opt.Amount,
		Currency:     opt.Currency,
		Debtor:       opt.Debtor,
		Creditor:     opt.Creditor,
		DebtorIBAN:   opt.DebtorIBAN,
		CreditorIBAN: opt.CreditorIBAN,
		Preset:       opt.Preset,
		Steps:        make([]Step, 4),
	}

	// Keep the public chain metadata as the caller supplied it, but encode every
	// value before placing it in a hand-written XML template.
	opt.Amount = generator.EscapeText(opt.Amount)
	opt.Currency = generator.EscapeText(opt.Currency)
	opt.Debtor = generator.EscapeText(opt.Debtor)
	opt.Creditor = generator.EscapeText(opt.Creditor)
	opt.DebtorIBAN = generator.EscapeText(opt.DebtorIBAN)
	opt.CreditorIBAN = generator.EscapeText(opt.CreditorIBAN)
	opt.DebtorBIC = generator.EscapeText(opt.DebtorBIC)
	opt.CreditorBIC = generator.EscapeText(opt.CreditorBIC)
	opt.EndToEndID = generator.EscapeText(opt.EndToEndID)
	opt.UETR = generator.EscapeText(opt.UETR)

	now := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	today := time.Now().UTC().Format("2006-01-02")
	baseTime := time.Now().UnixNano() / 1000000

	// Step 1: pain.001 Initiation
	pain001XML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pain.001.001.11">
  <CstmrCdtTrfInitn>
    <GrpHdr>
      <MsgId>MSG-PAIN001-%d</MsgId>
      <CreDtTm>%s</CreDtTm>
      <NbOfTxs>1</NbOfTxs>
      <InitgPty>
        <Nm>%s</Nm>
      </InitgPty>
    </GrpHdr>
    <PmtInf>
      <PmtInfId>PMT-%s</PmtInfId>
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
          <UETR>%s</UETR>
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
</Document>`, baseTime, now, opt.Debtor, opt.EndToEndID, today, opt.Debtor,
		generator.AccountBlock(rawOpt, rawOpt.DebtorIBAN, rawOpt.DebtorAccountID, "          "),
		generator.AgentBlock(rawOpt, rawOpt.DebtorBIC, rawOpt.DebtorMemberID, "          "),
		opt.EndToEndID, opt.UETR, opt.Currency, opt.Amount,
		generator.AgentBlock(rawOpt, rawOpt.CreditorBIC, rawOpt.CreditorMemberID, "            "),
		opt.Creditor,
		generator.AccountBlock(rawOpt, rawOpt.CreditorIBAN, rawOpt.CreditorAccountID, "            "))

	chain.Steps[0] = Step{
		Index:       1,
		MsgType:     "pain.001.001.11",
		Title:       "Customer Credit Transfer Initiation",
		Description: "Ordering customer transmits payment instruction to their debtor institution.",
		FileName:    "01_pain.001_Initiation.xml",
		XMLPayload:  pain001XML,
	}

	// Step 2: pacs.008 Interbank Settlement
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

	pacs008XML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10">
  <FIToFICstmrCdtTrf>
    <GrpHdr>
      <MsgId>MSG-PACS008-%d</MsgId>
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
    </CdtTrfTxInf>
  </FIToFICstmrCdtTrf>
</Document>`, baseTime+1, now, clearingSys, opt.EndToEndID, opt.UETR, opt.Currency, opt.Amount, today, opt.Debtor,
		generator.AccountBlock(rawOpt, rawOpt.DebtorIBAN, rawOpt.DebtorAccountID, "          "),
		generator.AgentBlock(rawOpt, rawOpt.DebtorBIC, rawOpt.DebtorMemberID, "          "),
		generator.AgentBlock(rawOpt, rawOpt.CreditorBIC, rawOpt.CreditorMemberID, "          "),
		opt.Creditor,
		generator.AccountBlock(rawOpt, rawOpt.CreditorIBAN, rawOpt.CreditorAccountID, "          "))

	chain.Steps[1] = Step{
		Index:       2,
		MsgType:     "pacs.008.001.10",
		Title:       "FI-to-FI Customer Credit Transfer",
		Description: "Debtor bank submits payment to clearing / RTGS rails for interbank settlement.",
		FileName:    "02_pacs.008_Interbank.xml",
		XMLPayload:  pacs008XML,
	}

	// Step 3: pacs.002 Payment Status Report (ACK / ACTC)
	pacs002XML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.002.001.12">
  <FIToFIPmtStsRpt>
    <GrpHdr>
      <MsgId>MSG-PACS002-%d</MsgId>
      <CreDtTm>%s</CreDtTm>
    </GrpHdr>
    <OrgnlGrpInfAndSts>
      <OrgnlMsgId>MSG-PACS008-%d</OrgnlMsgId>
      <OrgnlMsgNmId>pacs.008.001.10</OrgnlMsgNmId>
      <GrpSts>ACTC</GrpSts>
    </OrgnlGrpInfAndSts>
    <TxInfAndSts>
      <OrgnlEndToEndId>%s</OrgnlEndToEndId>
      <OrgnlUETR>%s</OrgnlUETR>
      <TxSts>ACCP</TxSts>
      <StsRsnInf>
        <Rsn>
          <Cd>AC00</Cd>
        </Rsn>
        <AddtlInf>Settlement completed successfully</AddtlInf>
      </StsRsnInf>
    </TxInfAndSts>
  </FIToFIPmtStsRpt>
</Document>`, baseTime+2, now, baseTime+1, opt.EndToEndID, opt.UETR)

	chain.Steps[2] = Step{
		Index:       3,
		MsgType:     "pacs.002.001.12",
		Title:       "Payment Status Report (ACK/Settlement Confirmed)",
		Description: "Clearing rail confirms accepted customer credit transfer (ACCP) settlement.",
		FileName:    "03_pacs.002_StatusReport.xml",
		XMLPayload:  pacs002XML,
	}

	// Step 4: camt.053 Bank to Customer Statement
	camt053XML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:camt.053.001.11">
  <BkToCstmrStmt>
    <GrpHdr>
      <MsgId>MSG-CAMT053-%d</MsgId>
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
            <Cd>CLBD</Cd>
          </CdOrPrtry>
        </Tp>
        <Amt Ccy="%s">%s</Amt>
        <CdtDbtInd>CRDT</CdtDbtInd>
        <Dt>
          <Dt>%s</Dt>
        </Dt>
      </Bal>
      <Ntry>
        <Amt Ccy="%s">%s</Amt>
        <CdtDbtInd>CRDT</CdtDbtInd>
        <Sts>
          <Cd>BOOK</Cd>
        </Sts>
        <BkTxCd>
          <Prtry>
            <Cd>PMT-CRDT</Cd>
          </Prtry>
        </BkTxCd>
        <NtryDtls>
          <TxDtls>
            <Refs>
              <EndToEndId>%s</EndToEndId>
              <UETR>%s</UETR>
            </Refs>
            <Amt Ccy="%s">%s</Amt>
          </TxDtls>
        </NtryDtls>
      </Ntry>
    </Stmt>
  </BkToCstmrStmt>
</Document>`, baseTime+3, now, today, now,
		generator.AccountBlock(rawOpt, rawOpt.CreditorIBAN, rawOpt.CreditorAccountID, "          "),
		opt.Currency, opt.Amount, today, opt.Currency, opt.Amount, opt.EndToEndID, opt.UETR, opt.Currency, opt.Amount)

	chain.Steps[3] = Step{
		Index:       4,
		MsgType:     "camt.053.001.11",
		Title:       "Bank to Customer Statement (Reconciliation)",
		Description: "Creditor bank reports credited funds on beneficiary end-of-day statement.",
		FileName:    "04_camt.053_Statement.xml",
		XMLPayload:  camt053XML,
	}

	return chain, nil
}
