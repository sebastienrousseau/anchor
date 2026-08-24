// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package swift

import (
	"fmt"
	"strings"
	"time"
)

// MT104, MT107 and MT204 are the direct debit family. They retire in the 2028
// milestone rather than 2026, which is why they come after the credit
// transfers, and they invert the same structure: one creditor collecting from
// many debtors instead of one debtor paying many creditors.
//
//	MT104  request for debit transfer        -> pain.008
//	MT107  general direct debit              -> pain.008
//	MT204  financial markets direct debit    -> pacs.010

// convert104 translates a request for debit transfer.
func convert104(m *Message) (*Conversion, error) {
	return convertDirectDebit(m, "104")
}

// convert107 translates a general direct debit. It shares MT104's structure
// exactly, which is why it shares its conversion.
func convert107(m *Message) (*Conversion, error) {
	return convertDirectDebit(m, "107")
}

func convertDirectDebit(m *Message, sourceType string) (*Conversion, error) {
	b := newBuilder(m)

	head, groups := m.SplitAt("21")
	if len(groups) == 0 {
		return nil, fmt.Errorf("MT%s has no transaction sequence; expected at least one :21: field", sourceType)
	}

	ref, _ := FieldsOf(head, "20")
	b.mapped("20", "/Document/CstmrDrctDbtInitn/GrpHdr/MsgId", ref.Value)

	collectionDate, err := collectionDate(b, head, sourceType)
	if err != nil {
		return nil, err
	}

	// Sequence A carries two parties under tag 50: the instructing party
	// (options C and L) and the creditor (options A and K). The option letter is
	// what tells them apart.
	instructing := instructingParty(b, head)
	headCreditor := creditorParty(b, head, "/Document/CstmrDrctDbtInitn/PmtInf/Cdtr", "")
	headAgent := groupAgent(b, head, "52", "/Document/CstmrDrctDbtInitn/PmtInf/CdtrAgt", m.Sender, "")
	headCharges := chargeBearer(b, head, "/Document/CstmrDrctDbtInitn/PmtInf/ChrgBr", "")

	initiating := instructing
	if initiating.Name == "" && initiating.BIC == "" {
		initiating = headCreditor
	}
	if initiating.Name == "" && initiating.BIC == "" {
		initiating.Name = normaliseBIC(m.Sender)
		b.derived("50", "/Document/CstmrDrctDbtInitn/GrpHdr/InitgPty",
			"the request named no instructing party; the sender BIC was used")
	}

	if f, ok := FieldsOf(head, "26"); ok {
		b.unmapped(f.Name(), "the transaction type code is scheme-specific; map it to Purp/Cd if your scheme defines one")
	}
	if f, ok := FieldsOf(head, "77"); ok {
		b.unmapped(f.Name(), "regulatory reporting has no equivalent without knowing the reporting scheme")
	}

	in := pain008Input{
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
		b.mapped("21"+suffix, "/Document/CstmrDrctDbtInitn/PmtInf/DrctDbtTxInf/PmtId/EndToEndId", txRef.Value)

		amt, ok := ExactOf(group, "32B")
		if !ok {
			return nil, fmt.Errorf("transaction %d has no :32B: currency and amount", i+1)
		}
		ca, err := ParseCurrencyAmount(amt.Value)
		if err != nil {
			return nil, fmt.Errorf("transaction %d, field :32B:: %w", i+1, err)
		}
		b.mapped("32B"+suffix, "/Document/CstmrDrctDbtInitn/PmtInf/DrctDbtTxInf/InstdAmt", amt.Value)

		creditor := headCreditor
		if _, ok := FieldsOf(group, "50"); ok {
			creditor = creditorParty(b, group, "/Document/CstmrDrctDbtInitn/PmtInf/Cdtr", suffix)
		}
		creditorAgent := headAgent
		if _, ok := FieldsOf(group, "52"); ok {
			creditorAgent = groupAgent(b, group, "52",
				"/Document/CstmrDrctDbtInitn/PmtInf/CdtrAgt", m.Sender, suffix)
		}

		charges := headCharges
		if _, ok := ExactOf(group, "71A"); ok {
			charges = chargeBearer(b, group, "/Document/CstmrDrctDbtInitn/PmtInf/ChrgBr", suffix)
		}

		chargesAcct := ""
		if f, ok := ExactOf(group, "25A"); ok {
			chargesAcct, _ = PartyLines(f)
			if chargesAcct == "" {
				chargesAcct = strings.TrimSpace(f.Value)
			}
			b.mapped(f.Name()+suffix, "/Document/CstmrDrctDbtInitn/PmtInf/ChrgsAcct", f.Value)
		}

		// The debtor is the party being collected from, which in MT104 is the
		// beneficiary field: the money moves the other way.
		debtor := groupParty(b, group, "59",
			"/Document/CstmrDrctDbtInitn/PmtInf/DrctDbtTxInf/Dbtr", suffix)
		debtorAgent := groupAgent(b, group, "57",
			"/Document/CstmrDrctDbtInitn/PmtInf/DrctDbtTxInf/DbtrAgt", m.Receiver, suffix)

		mandate := ""
		if f, ok := ExactOf(group, "21C"); ok {
			mandate, _ = truncate(strings.TrimSpace(f.Value), 35)
			b.mapped(f.Name()+suffix,
				"/Document/CstmrDrctDbtInitn/PmtInf/DrctDbtTxInf/DrctDbtTx/MndtRltdInf/MndtId", f.Value)
		}
		if f, ok := ExactOf(group, "21D"); ok {
			b.unmapped(f.Name()+suffix,
				"the direct debit reference identifies the collection to the debtor's bank; pain.008 has no field for it")
		}

		instrID := ""
		if f, ok := ExactOf(group, "21E"); ok {
			instrID, _ = truncate(f.Value, 35)
			b.mapped(f.Name()+suffix,
				"/Document/CstmrDrctDbtInitn/PmtInf/DrctDbtTxInf/PmtId/InstrId", f.Value)
		}

		instrInfo := ""
		if f, ok := ExactOf(group, "23E"); ok {
			instrInfo, _ = truncate(strings.TrimSpace(f.Value), 140)
			b.truncated(f.Name()+suffix,
				"/Document/CstmrDrctDbtInitn/PmtInf/DrctDbtTxInf/InstrForCdtrAgt",
				"the MT instruction code is carried as free text, which no agent is obliged to act on", f.Value)
		}

		remittance := ""
		if f, ok := FieldsOf(group, "70"); ok {
			joined := strings.Join(f.Lines(), " ")
			remittance, _ = truncate(joined, 140)
			path := "/Document/CstmrDrctDbtInitn/PmtInf/DrctDbtTxInf/RmtInf/Ustrd"
			if len([]rune(joined)) > 140 {
				b.truncated(f.Name()+suffix, path,
					fmt.Sprintf("remittance information is %d characters; Ustrd permits 140", len([]rune(joined))), joined)
			} else {
				b.mapped(f.Name()+suffix, path, joined)
			}
		}

		in.Instructions = append(in.Instructions, pain008Instruction{
			ID:             txRef.Value,
			CollectionDate: collectionDate,
			Creditor:       creditor,
			CreditorAgent:  creditorAgent,
			Charges:        charges,
			ChargesAcct:    chargesAcct,
			Transaction: pain008Transaction{
				EndToEndID:      txRef.Value,
				InstructionID:   instrID,
				Currency:        ca.Currency,
				Amount:          ca.Amount,
				MandateID:       mandate,
				DebtorAgent:     debtorAgent,
				Debtor:          debtor,
				InstructionInfo: instrInfo,
				Remittance:      remittance,
			},
		})
	}

	return &Conversion{
		SourceType: sourceType,
		TargetType: "pain.008.001.07",
		XML:        buildPain008(in),
		Report:     b.finish(m),
	}, nil
}

// collectionDate reads :30:, which pain.008 requires.
func collectionDate(b *builder, head []Field, sourceType string) (string, error) {
	f, ok := FieldsOf(head, "30")
	if !ok {
		return "", fmt.Errorf("MT%s has no :30: requested execution date", sourceType)
	}
	iso, err := DateToISO(f.Value)
	if err != nil {
		return "", fmt.Errorf("field :%s:: %w", f.Name(), err)
	}
	b.mapped(f.Name(), "/Document/CstmrDrctDbtInitn/PmtInf/ReqdColltnDt", f.Value)
	return iso, nil
}

// instructingParty reads the sequence A :50a: that carries the instructing
// party, which MT104 distinguishes from the creditor by its option letter.
func instructingParty(b *builder, group []Field) partyInfo {
	for _, name := range []string{"50C", "50L"} {
		f, ok := ExactOf(group, name)
		if !ok {
			continue
		}
		_, rest := PartyLines(f)
		if len(rest) == 0 {
			continue
		}
		b.mapped(f.Name(), "/Document/CstmrDrctDbtInitn/GrpHdr/InitgPty", f.Value)

		// Option C carries a BIC; option L carries a party identifier.
		if name == "50C" {
			return partyInfo{BIC: normaliseBIC(rest[0])}
		}
		trimmed, _ := truncate(rest[0], 140)
		return partyInfo{Name: trimmed}
	}
	return partyInfo{}
}

// creditorParty reads the :50a: that carries the creditor: options A and K.
func creditorParty(b *builder, group []Field, path, suffix string) partyInfo {
	return partyByOptions(b, group, "50", []string{"50A", "50K"}, path, suffix,
		"the request named no creditor; a placeholder name was used")
}

// orderingCustomer reads the :50a: that carries the ordering customer, which
// MT101 writes with options F, G and H.
//
// Sequence A carries tag 50 twice -- once for the instructing party and once
// for the ordering customer -- and only the option letter tells them apart.
// Taking the first field with tag 50 picks whichever the sender wrote first,
// which is how an account can go missing.
func orderingCustomer(b *builder, group []Field, path, suffix string) partyInfo {
	return partyByOptions(b, group, "50", []string{"50F", "50G", "50H"}, path, suffix,
		"the request named no ordering customer; a placeholder name was used")
}

// partyByOptions reads a party field, preferring the given option letters and
// falling back to any field with the tag.
func partyByOptions(b *builder, group []Field, tag string, options []string, path, suffix, absent string) partyInfo {
	for _, name := range options {
		f, ok := ExactOf(group, name)
		if !ok {
			continue
		}
		return partyFromField(b, f, path, suffix)
	}
	// A message using none of them still names the party somewhere under the
	// tag; take whatever is there.
	if _, ok := FieldsOf(group, tag); ok {
		return groupParty(b, group, tag, path, suffix)
	}
	b.derived(tag+suffix, path, absent)
	return partyInfo{}
}

// chargeBearer reads :71A: from one sequence.
func chargeBearer(b *builder, group []Field, path, suffix string) string {
	f, ok := ExactOf(group, "71A")
	if !ok {
		return ""
	}
	mapped, known := ChargeBearer(f.Value)
	if !known {
		b.unmapped(f.Name()+suffix, fmt.Sprintf("%q is not a recognised charge bearer code", f.Value))
		return ""
	}
	b.mapped(f.Name()+suffix, path, f.Value+" -> "+mapped)
	return mapped
}

// headerBIC recovers a usable BIC from a header address, or the empty string
// when the header carried none.
func headerBIC(address string) string {
	if strings.TrimSpace(address) == "" {
		return ""
	}
	return normaliseBIC(address)
}

// ---------------------------------------------------------------------------
// MT204 -> pacs.010
// ---------------------------------------------------------------------------

// convert204 translates a financial markets direct debit. Every party is an
// institution, which is what separates pacs.010 from pain.008.
func convert204(m *Message) (*Conversion, error) {
	b := newBuilder(m)

	// Sequence A carries a :20: of its own and sequence B repeats starting at
	// :20:, so the first occurrence stays in the head.
	head, groups := m.SplitAfter("20", 1)
	if len(groups) == 0 {
		return nil, fmt.Errorf("MT204 has no transaction sequence; expected a second :20: field")
	}

	ref, _ := FieldsOf(head, "20")
	b.mapped("20", "/Document/FIDrctDbt/GrpHdr/MsgId", ref.Value)

	settlementDate := ""
	if f, ok := FieldsOf(head, "30"); ok {
		iso, err := DateToISO(f.Value)
		if err != nil {
			return nil, fmt.Errorf("field :%s:: %w", f.Name(), err)
		}
		settlementDate = iso
		b.mapped(f.Name(), "/Document/FIDrctDbt/CdtInstr/IntrBkSttlmDt", f.Value)
	}

	if f, ok := FieldsOf(head, "19"); ok {
		b.unmapped(f.Name(), "the sum of amounts is derivable from the transactions; pacs.010 carries it as an optional control sum")
	}

	// The creditor is the institution being paid: :58a: names it, and :57a: is
	// the account with institution when it does not.
	creditor := groupAgent(b, head, "58", "/Document/FIDrctDbt/CdtInstr/Cdtr", "", "")
	if creditor == "NOTPROVIDED" {
		creditor = groupAgent(b, head, "57", "/Document/FIDrctDbt/CdtInstr/Cdtr", m.Receiver, "")
	}

	in := pacs010Input{
		MsgID:          ref.Value,
		CreatedAt:      time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		CreditID:       ref.Value,
		Creditor:       creditor,
		SettlementDate: settlementDate,
		// InstgAgt is optional, so a header with no BIC leaves it out rather
		// than naming an institution that does not exist.
		Instructing: headerBIC(m.Sender),
	}

	for i, group := range groups {
		suffix := ""
		if i > 0 {
			suffix = fmt.Sprintf(" (txn %d)", i+1)
		}

		txRef, _ := FieldsOf(group, "20")
		endToEnd := txRef.Value
		if f, ok := FieldsOf(group, "21"); ok {
			endToEnd = f.Value
			b.mapped(f.Name()+suffix, "/Document/FIDrctDbt/CdtInstr/DrctDbtTxInf/PmtId/EndToEndId", f.Value)
		} else {
			b.mapped("20"+suffix, "/Document/FIDrctDbt/CdtInstr/DrctDbtTxInf/PmtId/EndToEndId", txRef.Value)
		}

		amt, ok := ExactOf(group, "32B")
		if !ok {
			return nil, fmt.Errorf("transaction %d has no :32B: currency and amount", i+1)
		}
		ca, err := ParseCurrencyAmount(amt.Value)
		if err != nil {
			return nil, fmt.Errorf("transaction %d, field :32B:: %w", i+1, err)
		}
		b.mapped("32B"+suffix, "/Document/FIDrctDbt/CdtInstr/DrctDbtTxInf/IntrBkSttlmAmt", amt.Value)

		// The debtor is the institution being debited: :53a:.
		debtor := groupAgent(b, group, "53",
			"/Document/FIDrctDbt/CdtInstr/DrctDbtTxInf/Dbtr", m.Sender, suffix)

		remittance := ""
		if f, ok := FieldsOf(group, "72"); ok {
			joined := strings.Join(f.Lines(), " ")
			remittance, _ = truncate(joined, 140)
			b.truncated(f.Name()+suffix, "/Document/FIDrctDbt/CdtInstr/DrctDbtTxInf/RmtInf/Ustrd",
				"sender to receiver information is structured in MT and free text here", joined)
		}

		in.Transactions = append(in.Transactions, pacs010Transaction{
			EndToEndID: endToEnd,
			Currency:   ca.Currency,
			Amount:     ca.Amount,
			Debtor:     debtor,
			Remittance: remittance,
		})
	}

	return &Conversion{
		SourceType: "204",
		TargetType: "pacs.010.001.06",
		XML:        buildPacs010(in),
		Report:     b.finish(m),
	}, nil
}
