// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package swift

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Translating MT to MX is lossy in both directions, and the losses are the
// interesting part. A converter that silently drops a field is worse than one
// that refuses, so every conversion returns a report saying what was carried,
// what was truncated, and what had nowhere to go.

// Fidelity records how faithfully one field survived.
type Fidelity string

const (
	// FidelityMapped means the field was carried across intact.
	FidelityMapped Fidelity = "mapped"
	// FidelityTruncated means the value was carried but shortened to fit.
	FidelityTruncated Fidelity = "truncated"
	// FidelityUnmapped means the field has no equivalent and was dropped.
	FidelityUnmapped Fidelity = "unmapped"
	// FidelityDerived means the target value was inferred rather than copied.
	FidelityDerived Fidelity = "derived"
)

// FieldReport is what happened to one MT field.
type FieldReport struct {
	Tag      string   `json:"tag"`
	Path     string   `json:"path,omitempty"`
	Fidelity Fidelity `json:"fidelity"`
	Note     string   `json:"note,omitempty"`
	Value    string   `json:"value,omitempty"`
}

// Conversion is the outcome of translating one message.
type Conversion struct {
	// SourceType is the MT message type, for example "103".
	SourceType string `json:"source_type"`
	// TargetType is the ISO 20022 message identifier produced.
	TargetType string `json:"target_type"`
	// XML is the generated message.
	XML string `json:"xml"`
	// Report lists every field and how it fared.
	Report []FieldReport `json:"report"`
}

// Counts summarises the report.
func (c *Conversion) Counts() map[Fidelity]int {
	out := map[Fidelity]int{}
	for _, r := range c.Report {
		out[r.Fidelity]++
	}
	return out
}

// Lossless reports whether every field was carried across intact.
func (c *Conversion) Lossless() bool {
	for _, r := range c.Report {
		if r.Fidelity == FidelityUnmapped || r.Fidelity == FidelityTruncated {
			return false
		}
	}
	return true
}

// Unmapped lists the fields that had nowhere to go.
func (c *Conversion) Unmapped() []FieldReport {
	var out []FieldReport
	for _, r := range c.Report {
		if r.Fidelity == FidelityUnmapped {
			out = append(out, r)
		}
	}
	return out
}

// Convert translates a parsed MT message to its ISO 20022 equivalent.
func Convert(m *Message) (*Conversion, error) {
	switch m.Type {
	case "101":
		return convert101(m)
	case "104":
		return convert104(m)
	case "107":
		return convert107(m)
	case "103":
		return convert103(m)
	case "202":
		return convert202(m)
	case "204":
		return convert204(m)
	case "940":
		return convert940(m)
	}

	// The exception messages are numbered by category -- MT192, MT292, MT592
	// and so on -- and all do the same thing, so they match on the last two
	// digits rather than being listed eighteen times.
	switch exceptionKind(m.Type) {
	case "92":
		return convertCancellation(m)
	case "95":
		return convertQuery(m)
	case "96":
		return convertAnswer(m)
	}
	return nil, fmt.Errorf("MT%s has no conversion yet (supported: %s)", m.Type, strings.Join(supportedNames(), ", "))
}

// builder accumulates the report while assembling the target message.
type builder struct {
	reports []FieldReport
	seen    map[string]bool
}

func newBuilder(m *Message) *builder {
	b := &builder{seen: map[string]bool{}}
	return b
}

// note marks a tag as visited. Reports for repeated sequences carry a suffix
// such as " (txn 2)", so the bare tag is marked too: without it finish would
// report a field as dropped merely because only its second occurrence was
// handled.
func (b *builder) note(tag string) {
	b.seen[tag] = true
	if base, _, found := strings.Cut(tag, " ("); found {
		b.seen[strings.TrimSpace(base)] = true
	}
}

func (b *builder) mapped(tag, path, value string) {
	b.note(tag)
	b.reports = append(b.reports, FieldReport{
		Tag: tag, Path: path, Fidelity: FidelityMapped, Value: value,
	})
}

func (b *builder) derived(tag, path, note string) {
	b.note(tag)
	b.reports = append(b.reports, FieldReport{
		Tag: tag, Path: path, Fidelity: FidelityDerived, Note: note,
	})
}

func (b *builder) truncated(tag, path, note, value string) {
	b.note(tag)
	b.reports = append(b.reports, FieldReport{
		Tag: tag, Path: path, Fidelity: FidelityTruncated, Note: note, Value: value,
	})
}

func (b *builder) unmapped(tag, note string) {
	b.note(tag)
	b.reports = append(b.reports, FieldReport{
		Tag: tag, Fidelity: FidelityUnmapped, Note: note,
	})
}

// finish records every field the conversion never looked at, so nothing is
// silently dropped.
func (b *builder) finish(m *Message) []FieldReport {
	for _, f := range m.Fields {
		if !b.seen[f.Name()] && !b.seen[f.Tag] {
			b.reports = append(b.reports, FieldReport{
				Tag:      f.Name(),
				Fidelity: FidelityUnmapped,
				Note:     "no equivalent in the target message",
				Value:    firstLine(f.Value),
			})
			b.seen[f.Name()] = true
		}
	}
	sort.SliceStable(b.reports, func(i, j int) bool { return b.reports[i].Tag < b.reports[j].Tag })
	return b.reports
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// truncate shortens a value to an ISO 20022 maximum length.
func truncate(s string, max int) (string, bool) {
	r := []rune(s)
	if len(r) <= max {
		return s, false
	}
	return string(r[:max]), true
}

// ---------------------------------------------------------------------------
// MT103 -> pacs.008
// ---------------------------------------------------------------------------

func convert103(m *Message) (*Conversion, error) {
	b := newBuilder(m)

	ref, _ := m.Get("20")
	b.mapped("20", "/Document/FIToFICstmrCdtTrf/GrpHdr/MsgId", ref.Value)

	amt, ok := m.GetExact("32A")
	if !ok {
		return nil, fmt.Errorf("MT103 has no :32A: value date, currency and amount")
	}
	vda, err := ParseValueDateAmount(amt.Value)
	if err != nil {
		return nil, fmt.Errorf("field :32A:: %w", err)
	}
	b.mapped("32A", "/Document/FIToFICstmrCdtTrf/CdtTrfTxInf/IntrBkSttlmAmt", amt.Value)

	uetr := m.UETR
	if uetr == "" {
		uetr = generateUETR()
		b.derived("121", "/Document/FIToFICstmrCdtTrf/CdtTrfTxInf/PmtId/UETR",
			"the source carried no UETR; a new one was generated")
	} else {
		b.mapped("121", "/Document/FIToFICstmrCdtTrf/CdtTrfTxInf/PmtId/UETR", uetr)
	}

	debtor := party(b, m, "50", "/Document/FIToFICstmrCdtTrf/CdtTrfTxInf/Dbtr")
	creditor := party(b, m, "59", "/Document/FIToFICstmrCdtTrf/CdtTrfTxInf/Cdtr")

	debtorAgent := agent(b, m, "52", "/Document/FIToFICstmrCdtTrf/CdtTrfTxInf/DbtrAgt", m.Sender)
	creditorAgent := agent(b, m, "57", "/Document/FIToFICstmrCdtTrf/CdtTrfTxInf/CdtrAgt", m.Receiver)

	charges := "SHAR"
	// Tag 71 also carries the charge amounts (:71F:, :71G:), so the bearer must
	// be matched on its exact option letter.
	if f, ok := m.GetExact("71A"); ok {
		if mapped, ok := ChargeBearer(f.Value); ok {
			charges = mapped
			b.mapped(f.Name(), "/Document/FIToFICstmrCdtTrf/CdtTrfTxInf/ChrgBr", f.Value+" -> "+mapped)
		} else {
			b.unmapped(f.Name(), fmt.Sprintf("%q is not a recognised charge bearer code", f.Value))
		}
	} else {
		b.derived("71A", "/Document/FIToFICstmrCdtTrf/CdtTrfTxInf/ChrgBr",
			"the source carried no charge bearer; SHAR assumed")
	}

	remittance := ""
	if f, ok := m.Get("70"); ok {
		joined := strings.Join(f.Lines(), " ")
		short, cut := truncate(joined, 140)
		remittance = short
		path := "/Document/FIToFICstmrCdtTrf/CdtTrfTxInf/RmtInf/Ustrd"
		if cut {
			b.truncated(f.Name(), path,
				fmt.Sprintf("remittance information is %d characters; Ustrd permits 140", len([]rune(joined))), joined)
		} else {
			b.mapped(f.Name(), path, joined)
		}
	}

	// :33B: is the amount the debtor was instructed to send, before any charges
	// or conversion; :36: is the rate that took it to the settlement amount.
	instructed := currencyAmount(b, m, "33", "/Document/FIToFICstmrCdtTrf/CdtTrfTxInf/InstdAmt")
	rate := ""
	if f, ok := m.Get("36"); ok {
		rate = strings.Replace(strings.TrimSpace(f.Value), ",", ".", 1)
		b.mapped(f.Name(), "/Document/FIToFICstmrCdtTrf/CdtTrfTxInf/XchgRate", f.Value)
	}

	senderCharges := currencyAmount(b, m, "71F", "/Document/FIToFICstmrCdtTrf/CdtTrfTxInf/ChrgsInf")
	receiverCharges := currencyAmount(b, m, "71G", "/Document/FIToFICstmrCdtTrf/CdtTrfTxInf/ChrgsInf")

	instrCode, instrInfo := instruction(b, m, "/Document/FIToFICstmrCdtTrf/CdtTrfTxInf/InstrForNxtAgt")

	// :23B: names the service level in MT terms; ISO 20022 expresses that through
	// PmtTpInf, whose codes are scheme-specific rather than a direct translation.
	if f, ok := m.GetExact("23B"); ok {
		b.unmapped(f.Name(), "bank operation code has no direct equivalent; carry it as a local instrument if the rail defines one")
	}

	now := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	xml := buildPacs008(pacs008Input{
		MsgID:           ref.Value,
		CreatedAt:       now,
		UETR:            uetr,
		EndToEndID:      endToEnd(m, b),
		Currency:        vda.Currency,
		Amount:          vda.Amount,
		SettlementDay:   vda.Date,
		Instructed:      instructed,
		ExchangeRate:    rate,
		Charges:         charges,
		SenderCharges:   senderCharges,
		ReceiverCharges: receiverCharges,
		Debtor:          debtor,
		Creditor:        creditor,
		DebtorAgent:     debtorAgent,
		CreditorAgent:   creditorAgent,
		InstructionCode: instrCode,
		InstructionInfo: instrInfo,
		Remittance:      remittance,
	})

	return &Conversion{
		SourceType: "103",
		TargetType: "pacs.008.001.10",
		XML:        xml,
		Report:     b.finish(m),
	}, nil
}

// currencyAmount reads an MT field that carries a currency and an amount with no
// value date. The tag may be given with its option letter, as ":71F:" is.
func currencyAmount(b *builder, m *Message, tag, path string) *CurrencyAmount {
	f, ok := m.GetExact(tag)
	if !ok {
		if f, ok = m.Get(tag); !ok {
			return nil
		}
	}
	ca, err := ParseCurrencyAmount(f.Value)
	if err != nil {
		b.unmapped(f.Name(), err.Error())
		return nil
	}
	b.mapped(f.Name(), path, f.Value)
	return &ca
}

// instructionCodes maps the MT :23E: codes that have an Instruction4Code
// equivalent. The remainder are carried as free text rather than forced into a
// code they do not mean.
var instructionCodes = map[string]string{
	"PHON": "PHOA",
	"PHOB": "PHOA",
	"PHOI": "PHOA",
	"TELE": "TELA",
	"TELB": "TELA",
	"TELI": "TELA",
}

// instruction reads :23E:, splitting the code from any trailing narrative.
func instruction(b *builder, m *Message, path string) (code, info string) {
	f, ok := m.GetExact("23E")
	if !ok {
		return "", ""
	}

	raw := strings.TrimSpace(f.Value)
	head, tail, _ := strings.Cut(raw, "/")
	head = strings.ToUpper(strings.TrimSpace(head))

	mapped, known := instructionCodes[head]
	if !known {
		// Instruction4Code has two members; anything else survives as narrative.
		text, cut := truncate(raw, 140)
		if cut {
			b.truncated(f.Name(), path+"/InstrInf",
				fmt.Sprintf("%q has no Instruction4Code equivalent and is longer than the 140 characters InstrInf permits", head), raw)
		} else {
			b.truncated(f.Name(), path+"/InstrInf",
				fmt.Sprintf("%q has no Instruction4Code equivalent; it is carried as free text, which no agent is obliged to act on", head), raw)
		}
		return "", text
	}

	b.mapped(f.Name(), path+"/Cd", head+" -> "+mapped)
	info, _ = truncate(strings.TrimSpace(tail), 140)
	return mapped, info
}

// endToEnd reads :21: when present, falling back to the transaction reference.
func endToEnd(m *Message, b *builder) string {
	if f, ok := m.Get("21"); ok {
		b.mapped(f.Name(), "/Document/FIToFICstmrCdtTrf/CdtTrfTxInf/PmtId/EndToEndId", f.Value)
		return f.Value
	}
	ref, _ := m.Get("20")
	return ref.Value
}

// partyInfo is a name, an optional account, and address lines.
type partyInfo struct {
	Name    string
	Account string
	Address []string
	BIC     string
}

// party reads an ordering or beneficiary customer field.
func party(b *builder, m *Message, tag, path string) partyInfo {
	f, ok := m.Get(tag)
	if !ok {
		b.derived(tag, path, "the source carried no party; a placeholder name was used")
		// partyBlock supplies the placeholder, because Nm is mandatory.
		return partyInfo{}
	}

	account, rest := PartyLines(f)
	info := partyInfo{Account: account}

	// Option A carries a BIC rather than a name and address.
	if f.Option == "A" && len(rest) > 0 {
		info.BIC = rest[0]
		b.mapped(f.Name(), path, f.Value)
		return info
	}

	if len(rest) > 0 {
		name, cut := truncate(rest[0], 140)
		info.Name = name
		if cut {
			b.truncated(f.Name(), path+"/Nm",
				fmt.Sprintf("name is %d characters; Nm permits 140", len([]rune(rest[0]))), rest[0])
		} else {
			b.mapped(f.Name(), path, f.Value)
		}
		info.Address = rest[1:]
	} else {
		b.derived(f.Name(), path, "the party carried no name")
	}

	// Address lines from an MT party are unstructured, which CBPR+ stops
	// accepting on 14 November 2026.
	if len(info.Address) > 0 {
		b.note(f.Name() + " (address)")
		b.reports = append(b.reports, FieldReport{
			Tag:      f.Name() + " (address)",
			Path:     path + "/PstlAdr/AdrLine",
			Fidelity: FidelityTruncated,
			Note: "MT addresses are unstructured; CBPR+ rejects those from 14 November 2026. " +
				"Populate TwnNm and Ctry before then.",
			Value: strings.Join(info.Address, " / "),
		})
	}
	return info
}

// agent reads an institution field, falling back to the header BIC.
func agent(b *builder, m *Message, tag, path, fallbackBIC string) string {
	f, ok := m.Get(tag)
	if !ok {
		if fallbackBIC == "" {
			b.derived(tag, path, "no agent in the message or the header")
			return "NOTPROVIDED"
		}
		b.derived(tag, path, "taken from the message header")
		return normaliseBIC(fallbackBIC)
	}

	_, rest := PartyLines(f)
	if len(rest) == 0 {
		b.derived(f.Name(), path, "the agent field carried no identifier")
		return normaliseBIC(fallbackBIC)
	}

	// Option A is a BIC; option D is a name and address, which cannot become a
	// BICFI.
	if f.Option == "D" {
		b.truncated(f.Name(), path+"/FinInstnId/Nm",
			"option D carries a name and address, not a BIC; the agent cannot be identified by BICFI",
			strings.Join(rest, " / "))
		return normaliseBIC(fallbackBIC)
	}

	b.mapped(f.Name(), path, rest[0])
	return normaliseBIC(rest[0])
}

// normaliseBIC pads an 8-character BIC to 11, which BICFI permits either way.
func normaliseBIC(bic string) string {
	b := strings.ToUpper(strings.TrimSpace(bic))
	if b == "" {
		return "NOTPROVIDED"
	}
	if i := strings.IndexAny(b, " \n/"); i > 0 {
		b = b[:i]
	}
	return b
}

// ---------------------------------------------------------------------------
// MT202 -> pacs.009
// ---------------------------------------------------------------------------

func convert202(m *Message) (*Conversion, error) {
	b := newBuilder(m)

	ref, _ := m.Get("20")
	b.mapped("20", "/Document/FICdtTrf/GrpHdr/MsgId", ref.Value)

	amt, ok := m.GetExact("32A")
	if !ok {
		return nil, fmt.Errorf("MT202 has no :32A: value date, currency and amount")
	}
	vda, err := ParseValueDateAmount(amt.Value)
	if err != nil {
		return nil, fmt.Errorf("field :32A:: %w", err)
	}
	b.mapped("32A", "/Document/FICdtTrf/CdtTrfTxInf/IntrBkSttlmAmt", amt.Value)

	uetr := m.UETR
	if uetr == "" {
		uetr = generateUETR()
		b.derived("121", "/Document/FICdtTrf/CdtTrfTxInf/PmtId/UETR",
			"the source carried no UETR; a new one was generated")
	} else {
		b.mapped("121", "/Document/FICdtTrf/CdtTrfTxInf/PmtId/UETR", uetr)
	}

	e2e := ref.Value
	if f, ok := m.Get("21"); ok {
		e2e = f.Value
		b.mapped(f.Name(), "/Document/FICdtTrf/CdtTrfTxInf/PmtId/EndToEndId", f.Value)
	}

	// In pacs.009 the debtor and creditor are financial institutions.
	debtor := agent(b, m, "52", "/Document/FICdtTrf/CdtTrfTxInf/Dbtr", m.Sender)
	creditor := agent(b, m, "58", "/Document/FICdtTrf/CdtTrfTxInf/Cdtr", m.Receiver)
	instructing := agent(b, m, "53", "/Document/FICdtTrf/CdtTrfTxInf/InstgAgt", m.Sender)
	instructed := agent(b, m, "57", "/Document/FICdtTrf/CdtTrfTxInf/InstdAgt", m.Receiver)

	xml := buildPacs009(pacs009Input{
		MsgID:         ref.Value,
		CreatedAt:     time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		UETR:          uetr,
		EndToEndID:    e2e,
		Currency:      vda.Currency,
		Amount:        vda.Amount,
		SettlementDay: vda.Date,
		Debtor:        debtor,
		Creditor:      creditor,
		Instructing:   instructing,
		Instructed:    instructed,
	})

	return &Conversion{
		SourceType: "202",
		TargetType: "pacs.009.001.10",
		XML:        xml,
		Report:     b.finish(m),
	}, nil
}

// ---------------------------------------------------------------------------
// MT940 -> camt.053
// ---------------------------------------------------------------------------

func convert940(m *Message) (*Conversion, error) {
	b := newBuilder(m)

	ref, _ := m.Get("20")
	b.mapped("20", "/Document/BkToCstmrStmt/GrpHdr/MsgId", ref.Value)

	acct, ok := m.Get("25")
	if !ok {
		return nil, fmt.Errorf("MT940 has no :25: account identification")
	}
	b.mapped(acct.Name(), "/Document/BkToCstmrStmt/Stmt/Acct/Id", acct.Value)

	stmtNo := ""
	if f, ok := m.Get("28"); ok {
		stmtNo = f.Value
		path := "/Document/BkToCstmrStmt/Stmt/ElctrncSeqNb"
		if sequenceNumber(stmtNo) == "" {
			b.unmapped(f.Name(), fmt.Sprintf("%q is not a number, so it cannot become ElctrncSeqNb", stmtNo))
		} else if strings.Contains(stmtNo, "/") {
			b.truncated(f.Name(), path,
				"only the statement number is carried; the sequence number after the slash has no equivalent",
				stmtNo)
		} else {
			b.mapped(f.Name(), path, stmtNo)
		}
	}

	opening, err := balance(b, m, "60", "OPBD", "/Document/BkToCstmrStmt/Stmt/Bal")
	if err != nil {
		return nil, err
	}
	closing, err := balance(b, m, "62", "CLBD", "/Document/BkToCstmrStmt/Stmt/Bal")
	if err != nil {
		return nil, err
	}

	entries, err := statementEntries(b, m)
	if err != nil {
		return nil, err
	}
	// An MT statement line carries no currency of its own; it is the account's,
	// which the balances name.
	for i := range entries {
		entries[i].Currency = closing.Currency
	}

	xml := buildCamt053(camt053Input{
		MsgID:       ref.Value,
		CreatedAt:   time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		StatementNo: stmtNo,
		Account:     acct.Value,
		Opening:     opening,
		Closing:     closing,
		Entries:     entries,
	})

	return &Conversion{
		SourceType: "940",
		TargetType: "camt.053.001.11",
		XML:        xml,
		Report:     b.finish(m),
	}, nil
}

// statementEntries reads the :61: statement lines and the :86: information that
// follows each of them.
//
// MT940 pairs them by position: an :86: belongs to the :61: it follows. There
// is nothing else linking them, which is why the fields are walked in order
// rather than gathered by tag.
func statementEntries(b *builder, m *Message) ([]entryInfo, error) {
	var out []entryInfo

	for _, f := range m.Fields {
		switch f.Tag {
		case "61":
			line, err := ParseStatementLine(f.Value)
			if err != nil {
				return nil, fmt.Errorf("field :%s:: %w", f.Name(), err)
			}

			sign := "CRDT"
			if !line.Credit {
				sign = "DBIT"
			}
			out = append(out, entryInfo{
				Reference:       line.Reference,
				ServicerRef:     line.ServicerReference,
				Amount:          line.Amount,
				Sign:            sign,
				Reversal:        line.Reversal,
				BookingDate:     line.BookingDate,
				ValueDate:       line.ValueDate,
				TransactionType: line.TransactionType,
			})
			b.note("61")

		case "86":
			if len(out) == 0 {
				b.unmapped(f.Name(), "the information precedes any statement line, so it belongs to nothing")
				continue
			}
			last := &out[len(out)-1]
			for _, l := range f.Lines() {
				if trimmed := strings.TrimSpace(l); trimmed != "" {
					last.Information = append(last.Information, trimmed)
				}
			}
			b.note("86")
		}
	}

	if len(out) > 0 {
		b.mapped("61", "/Document/BkToCstmrStmt/Stmt/Ntry",
			fmt.Sprintf("%d statement entr(y/ies)", len(out)))
	}
	return out, nil
}

// balanceInfo is one camt.053 balance.
type balanceInfo struct {
	Code     string
	Sign     string // CRDT or DBIT
	Date     string
	Currency string
	Amount   string
}

// balance reads an MT balance field such as ":60F:C260824EUR1000,00".
func balance(b *builder, m *Message, tag, code, path string) (balanceInfo, error) {
	f, ok := m.Get(tag)
	if !ok {
		return balanceInfo{}, fmt.Errorf("MT940 has no :%s: balance", tag)
	}

	v := strings.TrimSpace(f.Value)
	if len(v) < 10 {
		return balanceInfo{}, fmt.Errorf("field :%s: is too short to be a balance", f.Name())
	}

	sign := "CRDT"
	if strings.HasPrefix(v, "D") {
		sign = "DBIT"
	}

	rest := v[1:]
	vda, err := ParseValueDateAmount(rest)
	if err != nil {
		return balanceInfo{}, fmt.Errorf("field :%s:: %w", f.Name(), err)
	}

	b.mapped(f.Name(), path+" ("+code+")", f.Value)
	return balanceInfo{
		Code: code, Sign: sign, Date: vda.Date,
		Currency: vda.Currency, Amount: vda.Amount,
	}, nil
}

// Supported lists the MT message types Convert can translate.
//
// The n9x entries stand for every category: MT192, MT292, MT592 and the rest
// all convert, because the exception messages differ only in their category
// digit.
func Supported() []string {
	return []string{"101", "103", "104", "107", "202", "204", "940", "n92", "n95", "n96"}
}

// supportedNames renders the supported types the way an error message should.
func supportedNames() []string {
	out := make([]string, 0, len(Supported()))
	for _, t := range Supported() {
		out = append(out, "MT"+t)
	}
	return out
}

// ---------------------------------------------------------------------------
// MT101 -> pain.001
// ---------------------------------------------------------------------------

// MT101 is a request for transfer: an instructing party asks its bank to make
// one or more payments. It retires alongside the rest of the MT payments suite,
// and pain.001 is what replaces it.
//
// The message has two sequences: A carries what applies to the whole request,
// B repeats once per transaction. Each transaction becomes its own PmtInf,
// because MT101 lets a transaction override the ordering customer and the
// account servicing institution that sequence A named.
func convert101(m *Message) (*Conversion, error) {
	b := newBuilder(m)

	head, groups := m.SplitAt("21")
	if len(groups) == 0 {
		return nil, fmt.Errorf("MT101 has no transaction sequence; expected at least one :21: field")
	}

	ref, _ := FieldsOf(head, "20")
	b.mapped("20", "/Document/CstmrCdtTrfInitn/GrpHdr/MsgId", ref.Value)

	// Sequence A defaults, which a transaction may override.
	execDate, err := executionDate(b, head)
	if err != nil {
		return nil, err
	}
	// Sequence A carries tag 50 twice: the instructing party under options C
	// and L, the ordering customer under F, G and H.
	headInstructing := instructingParty(b, head)
	headDebtor := orderingCustomer(b, head, "/Document/CstmrCdtTrfInitn/PmtInf/Dbtr", "")
	headAgent := groupAgent(b, head, "52", "/Document/CstmrCdtTrfInitn/PmtInf/DbtrAgt", m.Sender, "")

	// The instructing party leads when the message names one; otherwise the
	// ordering customer is the party initiating the request.
	initiating := headInstructing
	if initiating.Name == "" && initiating.BIC == "" {
		initiating = headDebtor
	}
	if initiating.Name == "" && initiating.BIC == "" {
		initiating.Name = normaliseBIC(m.Sender)
		b.derived("50", "/Document/CstmrCdtTrfInitn/GrpHdr/InitgPty",
			"the request named no instructing party; the sender BIC was used")
	}

	if f, ok := FieldsOf(head, "28"); ok {
		b.unmapped(f.Name(), "the message index and total describe MT chaining, which pain.001 does not use")
	}
	if f, ok := FieldsOf(head, "25"); ok {
		b.unmapped(f.Name(), "the authorisation field is scheme-specific; carry it in Authstn if your scheme defines one")
	}

	in := pain001Input{
		MsgID:        ref.Value,
		CreatedAt:    time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		InitiatingBy: initiating,
	}

	for i, group := range groups {
		suffix := ""
		if i > 0 {
			suffix = fmt.Sprintf(" (txn %d)", i+1)
		}

		txRef, _ := FieldsOf(group, "21")
		b.mapped("21"+suffix, "/Document/CstmrCdtTrfInitn/PmtInf/CdtTrfTxInf/PmtId/EndToEndId", txRef.Value)

		amt, ok := ExactOf(group, "32B")
		if !ok {
			return nil, fmt.Errorf("transaction %d has no :32B: currency and amount", i+1)
		}
		ca, err := ParseCurrencyAmount(amt.Value)
		if err != nil {
			return nil, fmt.Errorf("transaction %d, field :32B:: %w", i+1, err)
		}
		b.mapped("32B"+suffix, "/Document/CstmrCdtTrfInitn/PmtInf/CdtTrfTxInf/Amt/InstdAmt", amt.Value)

		debtor := headDebtor
		if _, ok := FieldsOf(group, "50"); ok {
			debtor = orderingCustomer(b, group, "/Document/CstmrCdtTrfInitn/PmtInf/Dbtr", suffix)
		}
		agent := headAgent
		if _, ok := FieldsOf(group, "52"); ok {
			agent = groupAgent(b, group, "52", "/Document/CstmrCdtTrfInitn/PmtInf/DbtrAgt", m.Sender, suffix)
		}

		charges := ""
		if f, ok := ExactOf(group, "71A"); ok {
			if mapped, ok := ChargeBearer(f.Value); ok {
				charges = mapped
				b.mapped(f.Name()+suffix, "/Document/CstmrCdtTrfInitn/PmtInf/ChrgBr", f.Value+" -> "+mapped)
			} else {
				b.unmapped(f.Name()+suffix, fmt.Sprintf("%q is not a recognised charge bearer code", f.Value))
			}
		}

		chargesAcct := ""
		if f, ok := ExactOf(group, "25A"); ok {
			chargesAcct, _ = PartyLines(f)
			if chargesAcct == "" {
				chargesAcct = strings.TrimSpace(f.Value)
			}
			b.mapped(f.Name()+suffix, "/Document/CstmrCdtTrfInitn/PmtInf/ChrgsAcct", f.Value)
		}

		rate := ""
		if f, ok := FieldsOf(group, "36"); ok {
			rate = strings.Replace(strings.TrimSpace(f.Value), ",", ".", 1)
			b.mapped(f.Name()+suffix, "/Document/CstmrCdtTrfInitn/PmtInf/CdtTrfTxInf/XchgRateInf/XchgRate", f.Value)
		}

		instrID := ""
		if f, ok := ExactOf(group, "21F"); ok {
			instrID, _ = truncate(f.Value, 35)
			b.mapped(f.Name()+suffix, "/Document/CstmrCdtTrfInitn/PmtInf/CdtTrfTxInf/PmtId/InstrId", f.Value)
		}

		instrInfo := ""
		if f, ok := ExactOf(group, "23E"); ok {
			// pain.001 carries an instruction to the debtor agent as free text,
			// which is the closest equivalent MT101's code has.
			instrInfo, _ = truncate(strings.TrimSpace(f.Value), 140)
			b.truncated(f.Name()+suffix, "/Document/CstmrCdtTrfInitn/PmtInf/CdtTrfTxInf/InstrForDbtrAgt",
				"the MT instruction code is carried as free text, which no agent is obliged to act on", f.Value)
		}

		remittance := ""
		if f, ok := FieldsOf(group, "70"); ok {
			joined := strings.Join(f.Lines(), " ")
			remittance, _ = truncate(joined, 140)
			path := "/Document/CstmrCdtTrfInitn/PmtInf/CdtTrfTxInf/RmtInf/Ustrd"
			if len([]rune(joined)) > 140 {
				b.truncated(f.Name()+suffix, path,
					fmt.Sprintf("remittance information is %d characters; Ustrd permits 140", len([]rune(joined))), joined)
			} else {
				b.mapped(f.Name()+suffix, path, joined)
			}
		}

		creditor := groupParty(b, group, "59", "/Document/CstmrCdtTrfInitn/PmtInf/CdtTrfTxInf/Cdtr", suffix)
		creditorAgent := ""
		if _, ok := FieldsOf(group, "57"); ok {
			creditorAgent = groupAgent(b, group, "57",
				"/Document/CstmrCdtTrfInitn/PmtInf/CdtTrfTxInf/CdtrAgt", "", suffix)
			if creditorAgent == "NOTPROVIDED" {
				creditorAgent = ""
			}
		}

		txExec := execDate
		in.Instructions = append(in.Instructions, pain001Instruction{
			ID:            txRef.Value,
			ExecutionDate: txExec,
			Debtor:        debtor,
			DebtorAgent:   agent,
			Charges:       charges,
			ChargesAcct:   chargesAcct,
			Transaction: pain001Transaction{
				EndToEndID:      txRef.Value,
				InstructionID:   instrID,
				Currency:        ca.Currency,
				Amount:          ca.Amount,
				ExchangeRate:    rate,
				CreditorAgent:   creditorAgent,
				Creditor:        creditor,
				InstructionInfo: instrInfo,
				Remittance:      remittance,
			},
		})
	}

	return &Conversion{
		SourceType: "101",
		TargetType: "pain.001.001.09",
		XML:        buildPain001(in),
		Report:     b.finish(m),
	}, nil
}

// executionDate reads :30:, which pain.001 requires.
func executionDate(b *builder, head []Field) (string, error) {
	f, ok := FieldsOf(head, "30")
	if !ok {
		return "", fmt.Errorf("MT101 has no :30: requested execution date")
	}
	iso, err := DateToISO(f.Value)
	if err != nil {
		return "", fmt.Errorf("field :%s:: %w", f.Name(), err)
	}
	b.mapped(f.Name(), "/Document/CstmrCdtTrfInitn/PmtInf/ReqdExctnDt/Dt", f.Value)
	return iso, nil
}

// groupParty reads a party from one sequence rather than from the whole message.
func groupParty(b *builder, group []Field, tag, path, suffix string) partyInfo {
	f, ok := FieldsOf(group, tag)
	if !ok {
		b.derived(tag+suffix, path, "the source carried no party; a placeholder name was used")
		return partyInfo{}
	}
	return partyFromField(b, f, path, suffix)
}

// partyFromField decomposes one party field into a name, an account and address
// lines, reporting what it had to do along the way.
func partyFromField(b *builder, f Field, path, suffix string) partyInfo {
	account, rest := PartyLines(f)
	info := partyInfo{Account: account}

	if f.Option == "A" && len(rest) > 0 {
		info.BIC = normaliseBIC(rest[0])
		b.mapped(f.Name()+suffix, path, f.Value)
		return info
	}

	if len(rest) > 0 {
		name, cut := truncate(rest[0], 140)
		info.Name = name
		if cut {
			b.truncated(f.Name()+suffix, path+"/Nm",
				fmt.Sprintf("name is %d characters; Nm permits 140", len([]rune(rest[0]))), rest[0])
		} else {
			b.mapped(f.Name()+suffix, path, f.Value)
		}
		info.Address = rest[1:]
	} else {
		b.derived(f.Name()+suffix, path, "the party carried no name")
	}

	if len(info.Address) > 0 {
		b.note(f.Name() + suffix + " (address)")
		b.reports = append(b.reports, FieldReport{
			Tag:      f.Name() + suffix + " (address)",
			Path:     path + "/PstlAdr/AdrLine",
			Fidelity: FidelityTruncated,
			Note: "MT addresses are unstructured; CBPR+ rejects those from 14 November 2026. " +
				"Populate TwnNm and Ctry before then.",
			Value: strings.Join(info.Address, " / "),
		})
	}
	return info
}

// groupAgent reads an institution from one sequence.
func groupAgent(b *builder, group []Field, tag, path, fallbackBIC, suffix string) string {
	f, ok := FieldsOf(group, tag)
	if !ok {
		if fallbackBIC == "" {
			b.derived(tag+suffix, path, "no agent in the message or the header")
			return "NOTPROVIDED"
		}
		b.derived(tag+suffix, path, "taken from the message header")
		return normaliseBIC(fallbackBIC)
	}

	_, rest := PartyLines(f)
	if len(rest) == 0 {
		b.derived(f.Name()+suffix, path, "the agent field carried no identifier")
		return normaliseBIC(fallbackBIC)
	}
	if f.Option == "D" {
		b.truncated(f.Name()+suffix, path+"/FinInstnId/Nm",
			"option D carries a name and address, not a BIC; the agent cannot be identified by BICFI",
			strings.Join(rest, " / "))
		return normaliseBIC(fallbackBIC)
	}

	b.mapped(f.Name()+suffix, path, rest[0])
	return normaliseBIC(rest[0])
}
