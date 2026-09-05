// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

// Package swift parses SWIFT MT messages.
//
// An MT message is a sequence of curly-braced blocks:
//
//	{1:F01BANKGB2LAXXX0000000000}   basic header
//	{2:I103BANKDEFFXXXXN}           application header
//	{3:{121:<uetr>}}                user header, itself a block of blocks
//	{4:                             text block: the message itself
//	:20:REFERENCE
//	:32A:260824EUR25000,00
//	-}
//	{5:{CHK:...}}                   trailer
//
// The text block carries tag/value fields where the tag may end in an option
// letter -- ":50K:" and ":50A:" are the same field expressed two ways. Getting
// that distinction right matters, because the option decides how the value is
// laid out.
package swift

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Message is a parsed MT message.
type Message struct {
	// Type is the three-digit message type, for example "103".
	Type string
	// Blocks holds the raw text of each block, keyed by its number.
	Blocks map[string]string
	// Fields are the text-block fields in document order.
	Fields []Field
	// UETR is the unique end-to-end transaction reference from block 3, when present.
	UETR string
	// Sender and Receiver are the BICs of the two institutions, recovered from
	// the logical terminal addresses in the basic and application headers.
	Sender   string
	Receiver string
}

// Field is one tag/value pair from the text block.
type Field struct {
	// Tag is the numeric tag without its option letter, for example "50".
	Tag string
	// Option is the trailing option letter, for example "K". Empty when absent.
	Option string
	// Value is the field content, with continuation lines joined by newlines.
	Value string
}

// Name renders the tag as it appears in the message, for example "50K".
func (f Field) Name() string { return f.Tag + f.Option }

// Lines splits a multi-line field value.
func (f Field) Lines() []string {
	if f.Value == "" {
		return nil
	}
	return strings.Split(f.Value, "\n")
}

// Get returns the first field with the given tag, ignoring the option letter.
func (m *Message) Get(tag string) (Field, bool) {
	for _, f := range m.Fields {
		if f.Tag == tag {
			return f, true
		}
	}
	return Field{}, false
}

// GetExact returns the field matching a tag and option exactly, for example "50K".
func (m *Message) GetExact(name string) (Field, bool) {
	for _, f := range m.Fields {
		if f.Name() == name {
			return f, true
		}
	}
	return Field{}, false
}

// All returns every field with the given tag.
func (m *Message) All(tag string) []Field {
	var out []Field
	for _, f := range m.Fields {
		if f.Tag == tag {
			out = append(out, f)
		}
	}
	return out
}

// SplitAt divides the text block into the fields before the first occurrence of
// a repeating tag, and one group per occurrence after it.
//
// MT101 and MT104 are built this way: a message-level sequence A, then sequence
// B repeated once per transaction, each starting at its own :21:. The option
// letter must be empty for the split, because :21R: is a sequence A field and
// shares the tag.
func (m *Message) SplitAt(tag string) (head []Field, groups [][]Field) {
	return m.SplitAfter(tag, 0)
}

// SplitAfter is SplitAt with the first skip occurrences of the tag left in the
// head.
//
// MT204 needs this: its sequence A carries a :20: of its own, and sequence B
// repeats starting at :20: as well. Splitting on the first occurrence would put
// the message-level reference into the first transaction.
func (m *Message) SplitAfter(tag string, skip int) (head []Field, groups [][]Field) {
	seen := 0
	for _, f := range m.Fields {
		if f.Tag == tag && f.Option == "" {
			if seen < skip {
				seen++
				head = append(head, f)
				continue
			}
			groups = append(groups, []Field{f})
			continue
		}
		if len(groups) == 0 {
			head = append(head, f)
			continue
		}
		last := len(groups) - 1
		groups[last] = append(groups[last], f)
	}
	return head, groups
}

// FieldsOf finds a field within one group, ignoring the option letter.
func FieldsOf(group []Field, tag string) (Field, bool) {
	for _, f := range group {
		if f.Tag == tag {
			return f, true
		}
	}
	return Field{}, false
}

// ExactOf finds a field within one group by tag and option, for example "32B".
func ExactOf(group []Field, name string) (Field, bool) {
	for _, f := range group {
		if f.Name() == name {
			return f, true
		}
	}
	return Field{}, false
}

// DateToISO expands a six-digit SWIFT date such as ":30:260824".
func DateToISO(v string) (string, error) { return yymmddToISO(strings.TrimSpace(v)) }

var (
	blockRe = regexp.MustCompile(`\{(\d):`)
	// A field starts at the beginning of a line: ":20:", ":50K:", ":32A:".
	fieldRe = regexp.MustCompile(`(?m)^:(\d{2})([A-Z]?):`)
	// Block 3 carries nested {tag:value} pairs.
	nestedRe = regexp.MustCompile(`\{(\w+):([^}]*)\}`)
	// The application header identifies the message type after I or O.
	appHeaderRe = regexp.MustCompile(`^[IO](\d{3})`)
)

// Parse reads an MT message.
func Parse(data []byte) (*Message, error) {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("empty message")
	}

	m := &Message{Blocks: map[string]string{}}

	if err := splitBlocks(text, m); err != nil {
		return nil, err
	}
	if len(m.Blocks) == 0 {
		return nil, fmt.Errorf("no SWIFT blocks found; expected {1:...}{2:...}{4:...}")
	}

	m.readHeaders()

	body, ok := m.Blocks["4"]
	if !ok {
		return nil, fmt.Errorf("message has no text block {4:}")
	}
	m.Fields = parseFields(body)
	if len(m.Fields) == 0 {
		return nil, fmt.Errorf("text block contains no fields")
	}
	return m, nil
}

// splitBlocks walks the top-level {n:...} blocks, tracking brace depth so a
// nested block such as {3:{121:...}} is captured whole.
func splitBlocks(text string, m *Message) error {
	for _, loc := range blockRe.FindAllStringSubmatchIndex(text, -1) {
		start := loc[0]
		num := text[loc[2]:loc[3]]

		depth := 0
		end := -1
		for i := start; i < len(text); i++ {
			switch text[i] {
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					end = i
				}
			}
			if end >= 0 {
				break
			}
		}
		if end < 0 {
			return fmt.Errorf("block {%s: is not closed", num)
		}

		// Content is what sits between "{n:" and the matching "}".  Only
		// block 4 has a dash terminator, and it is a line of its own. Removing
		// an arbitrary trailing dash corrupts legitimate values such as :20:-.
		content := strings.TrimSpace(text[loc[1]:end])
		if num == "4" {
			content = strings.TrimSuffix(content, "\n-")
		}
		m.Blocks[num] = content
	}
	return nil
}

// readHeaders pulls the sender, receiver, message type and UETR out of the
// header blocks.
func (m *Message) readHeaders() {
	// {1:F01BANKGB2LAXXX0000000000} -- the address is characters 3..14, so the
	// block must be at least 15 characters long to slice it.
	if b1, ok := m.Blocks["1"]; ok && len(b1) >= 15 {
		m.Sender = logicalTerminalToBIC(b1[3:15])
	}

	// {2:I103BANKDEFFXXXXN}
	if b2, ok := m.Blocks["2"]; ok {
		if mt := appHeaderRe.FindStringSubmatch(b2); mt != nil {
			m.Type = mt[1]
		}
		if len(b2) >= 16 {
			m.Receiver = logicalTerminalToBIC(b2[4:16])
		}
	}

	// {3:{121:uetr}}
	if b3, ok := m.Blocks["3"]; ok {
		for _, nested := range nestedRe.FindAllStringSubmatch(b3, -1) {
			if nested[1] == "121" {
				m.UETR = strings.TrimSpace(nested[2])
			}
		}
	}
}

// logicalTerminalToBIC recovers a BIC from a SWIFT address.
//
// A header carries a twelve-character logical terminal address: the eight-
// character BIC, then a single character identifying the terminal (an "X" on
// the receiving side), then the three-character branch code. Copying all twelve
// into a BICFI element produces a message the linter rejects, so the terminal
// character is removed.
func logicalTerminalToBIC(address string) string {
	a := strings.TrimSpace(address)
	if len(a) != 12 {
		return a
	}
	return a[:8] + a[9:]
}

// parseFields splits the text block into tag/value pairs, preserving order and
// joining continuation lines.
func parseFields(body string) []Field {
	locs := fieldRe.FindAllStringSubmatchIndex(body, -1)
	if len(locs) == 0 {
		return nil
	}

	fields := make([]Field, 0, len(locs))
	for i, loc := range locs {
		tag := body[loc[2]:loc[3]]
		option := body[loc[4]:loc[5]]

		valueStart := loc[1]
		valueEnd := len(body)
		if i+1 < len(locs) {
			valueEnd = locs[i+1][0]
		}

		value := strings.Trim(body[valueStart:valueEnd], "\n")

		fields = append(fields, Field{
			Tag:    tag,
			Option: option,
			Value:  strings.TrimSpace(value),
		})
	}
	return fields
}

// ---------------------------------------------------------------------------
// Field value shapes
// ---------------------------------------------------------------------------

// amountRe matches field 32A: a six-digit value date, a currency, and an amount
// written with a comma as the decimal separator.
var amountRe = regexp.MustCompile(`^(\d{6})([A-Z]{3})(\d+[,]?\d*)$`)

// ValueDateAmount is the decomposition of a :32A: field.
type ValueDateAmount struct {
	// Date is the value date in ISO form, for example "2026-08-24".
	Date string
	// Currency is the ISO 4217 code.
	Currency string
	// Amount uses a full stop as the decimal separator, as ISO 20022 requires.
	Amount string
}

// ParseValueDateAmount decomposes ":32A:260824EUR25000,00".
func ParseValueDateAmount(value string) (ValueDateAmount, error) {
	v := strings.TrimSpace(value)
	m := amountRe.FindStringSubmatch(v)
	if m == nil {
		return ValueDateAmount{}, fmt.Errorf("%q is not a value-date/currency/amount field", value)
	}

	date, err := yymmddToISO(m[1])
	if err != nil {
		return ValueDateAmount{}, err
	}

	// MT writes decimals with a comma; ISO 20022 requires a full stop.
	amount := strings.Replace(m[3], ",", ".", 1)
	amount = strings.TrimSuffix(amount, ".")
	if amount == "" {
		return ValueDateAmount{}, fmt.Errorf("%q has no amount", value)
	}

	return ValueDateAmount{Date: date, Currency: m[2], Amount: amount}, nil
}

// yymmddToISO expands a two-digit year. SWIFT dates carry no century; the
// convention is that years below 80 are 2000s.
func yymmddToISO(v string) (string, error) {
	if len(v) != 6 {
		return "", fmt.Errorf("%q is not a six-digit date", v)
	}
	yy := v[0:2]
	century := "20"
	if yy >= "80" {
		century = "19"
	}
	iso := century + yy + "-" + v[2:4] + "-" + v[4:6]

	if _, err := time.Parse("2006-01-02", iso); err != nil {
		return "", fmt.Errorf("%q is not a valid date", v)
	}
	return iso, nil
}

// ccyAmountRe matches a currency and amount without a date, as fields 33B, 71F
// and 71G carry them.
var ccyAmountRe = regexp.MustCompile(`^([A-Z]{3})(\d+[,]?\d*)$`)

// CurrencyAmount is a currency and an amount with no value date.
type CurrencyAmount struct {
	Currency string
	Amount   string
}

// ParseCurrencyAmount decomposes ":33B:EUR25000,00".
func ParseCurrencyAmount(value string) (CurrencyAmount, error) {
	v := strings.TrimSpace(value)
	m := ccyAmountRe.FindStringSubmatch(v)
	if m == nil {
		return CurrencyAmount{}, fmt.Errorf("%q is not a currency and amount field", value)
	}
	amount := strings.Replace(m[2], ",", ".", 1)
	amount = strings.TrimSuffix(amount, ".")
	if amount == "" {
		return CurrencyAmount{}, fmt.Errorf("%q has no amount", value)
	}
	return CurrencyAmount{Currency: m[1], Amount: amount}, nil
}

// StatementLine is the decomposition of an MT940 :61: field.
type StatementLine struct {
	// ValueDate is the value date in ISO form.
	ValueDate string
	// BookingDate is the entry date in ISO form, when the field carries one.
	BookingDate string
	// Credit reports the direction; Reversal marks a reversal of it.
	Credit   bool
	Reversal bool
	// Amount uses a full stop as the decimal separator.
	Amount string
	// TransactionType is the four-character type identification code.
	TransactionType string
	// Reference is the reference for the account owner.
	Reference string
	// ServicerReference is what follows the double slash, when present.
	ServicerReference string
}

// statementLineRe decomposes ":61:260824C25000,00NTRFE2E-0001//SVCR-9001".
//
// The entry date is four digits that appear only when it differs from the value
// date, and it is followed by the debit or credit mark; requiring that mark is
// what keeps the amount from being read as a date.
var statementLineRe = regexp.MustCompile(
	`^(\d{6})(\d{4})?(RC|RD|EC|ED|C|D)([A-Z])?(\d+[,]?\d*)([NFS][A-Z0-9]{3})(.*)$`)

// ParseStatementLine decomposes an MT940 statement line.
func ParseStatementLine(value string) (StatementLine, error) {
	v := strings.TrimSpace(strings.Split(value, "\n")[0])

	m := statementLineRe.FindStringSubmatch(v)
	if m == nil {
		return StatementLine{}, fmt.Errorf("%q is not a statement line", value)
	}

	valueDate, err := yymmddToISO(m[1])
	if err != nil {
		return StatementLine{}, err
	}

	line := StatementLine{ValueDate: valueDate, TransactionType: m[6]}

	mark := m[3]
	line.Reversal = strings.HasPrefix(mark, "R")
	line.Credit = strings.HasSuffix(mark, "C")

	// The entry date carries no year of its own; it belongs to the value date's.
	if m[2] != "" {
		booking, err := yymmddToISO(m[1][:2] + m[2])
		if err != nil {
			return StatementLine{}, fmt.Errorf("entry date %q: %w", m[2], err)
		}
		line.BookingDate = booking
	}

	amount := strings.Replace(m[5], ",", ".", 1)
	amount = strings.TrimSuffix(amount, ".")
	if amount == "" {
		return StatementLine{}, fmt.Errorf("%q has no amount", value)
	}
	line.Amount = amount

	reference, servicer, _ := strings.Cut(strings.TrimSpace(m[7]), "//")
	line.Reference = strings.TrimSpace(reference)
	line.ServicerReference = strings.TrimSpace(servicer)
	return line, nil
}

// PartyLines splits a party field into its account line and name/address lines.
//
// Options A and D carry an optional account on the first line, prefixed "/".
// The remainder is a BIC (option A) or free-form name and address (option K/D).
func PartyLines(f Field) (account string, rest []string) {
	for i, line := range f.Lines() {
		line = strings.TrimSpace(line)
		if i == 0 && strings.HasPrefix(line, "/") {
			account = strings.TrimPrefix(line, "/")
			// A double slash marks a national identifier rather than an account.
			account = strings.TrimPrefix(account, "/")
			continue
		}
		if line != "" {
			rest = append(rest, line)
		}
	}
	return account, rest
}

// ChargeBearer maps an MT :71A: code to its ISO 20022 equivalent.
//
//	OUR -> DEBT   the debtor pays
//	BEN -> CRED   the creditor pays
//	SHA -> SHAR   shared
func ChargeBearer(mt string) (string, bool) {
	switch strings.ToUpper(strings.TrimSpace(mt)) {
	case "OUR":
		return "DEBT", true
	case "BEN":
		return "CRED", true
	case "SHA":
		return "SHAR", true
	}
	return "", false
}
