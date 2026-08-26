// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package swift

import (
	"fmt"
	"strings"
	"time"
)

// The exception messages are numbered by category: MT192 cancels a customer
// payment, MT292 an institution one, MT592 a treasury one, and so on through
// the nine categories. What they do is identical, so the conversion matches on
// the last two digits rather than listing eighteen message types.
//
//	MTn92  request for cancellation -> camt.056  payment cancellation request
//	MTn95  queries                  -> camt.110  investigation request
//	MTn96  answers                  -> camt.111  investigation response

// exceptionKind reports which of the three an MT type is, or the empty string.
func exceptionKind(msgType string) string {
	if len(msgType) != 3 {
		return ""
	}
	switch msgType[1:] {
	case "92", "95", "96":
		return msgType[1:]
	}
	return ""
}

// convertCancellation translates an MTn92 request for cancellation.
func convertCancellation(m *Message) (*Conversion, error) {
	b := newBuilder(m)

	ref, ok := m.Get("20")
	if !ok {
		return nil, fmt.Errorf("MT%s has no :20: transaction reference", m.Type)
	}
	b.mapped("20", "/Document/FIToFIPmtCxlReq/Assgnmt/Id", ref.Value)

	related, ok := m.Get("21")
	if !ok {
		return nil, fmt.Errorf("MT%s has no :21: related reference, so there is nothing to cancel", m.Type)
	}
	b.mapped(related.Name(), "/Document/FIToFIPmtCxlReq/Undrlyg/TxInf/OrgnlEndToEndId", related.Value)

	in := camt056Input{
		AssignmentID:  ref.Value,
		CreatedAt:     time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		Assigner:      headerBICOrPlaceholder(m.Sender),
		Assignee:      headerBICOrPlaceholder(m.Receiver),
		CaseID:        ref.Value,
		OriginalEndID: related.Value,
		Reason:        exceptionNarrative(b, m, "/Document/FIToFIPmtCxlReq/Undrlyg/TxInf/CxlRsnInf/AddtlInf", 105),
	}

	original, name := originalMessage(b, m, "/Document/FIToFIPmtCxlReq/Undrlyg/TxInf/OrgnlGrpInf")
	in.OriginalMsgID = original
	in.OriginalMsgNm = name
	if in.OriginalMsgID == "" && in.OriginalMsgNm != "" {
		in.OriginalMsgID = related.Value
		b.derived("11S", "/Document/FIToFIPmtCxlReq/Undrlyg/TxInf/OrgnlGrpInf/OrgnlMsgId",
			"the request named the original message type but not its identifier; "+
				"the related reference was used")
	}
	if in.OriginalMsgID == "" {
		// OrgnlGrpInf needs a message identifier; without :11S: the related
		// reference is the only one there is.
		in.OriginalMsgID = related.Value
		in.OriginalMsgNm = "UNKNOWN"
		b.derived("11S", "/Document/FIToFIPmtCxlReq/Undrlyg/TxInf/OrgnlGrpInf",
			"the request did not name the original message; the related reference was used")
	}

	if uetr := m.UETR; uetr != "" {
		in.OriginalUETR = uetr
		b.mapped("121", "/Document/FIToFIPmtCxlReq/Undrlyg/TxInf/OrgnlUETR", uetr)
	}

	if amt, ok := m.GetExact("32A"); ok {
		vda, err := ParseValueDateAmount(amt.Value)
		if err == nil {
			in.Currency, in.Amount, in.SettlementDay = vda.Currency, vda.Amount, vda.Date
			b.mapped("32A", "/Document/FIToFIPmtCxlReq/Undrlyg/TxInf/OrgnlIntrBkSttlmAmt", amt.Value)
		} else {
			b.unmapped("32A", err.Error())
		}
	}

	return &Conversion{
		SourceType: m.Type,
		TargetType: "camt.056.001.11",
		XML:        buildCamt056(in),
		Report:     b.finish(m),
	}, nil
}

// convertQuery translates an MTn95 query.
func convertQuery(m *Message) (*Conversion, error) {
	b := newBuilder(m)

	ref, ok := m.Get("20")
	if !ok {
		return nil, fmt.Errorf("MT%s has no :20: transaction reference", m.Type)
	}
	b.mapped("20", "/Document/InvstgtnReq/InvstgtnReq/MsgId", ref.Value)

	in := camt110Input{
		MsgID: ref.Value,
		// An MT query carries prose where camt.110 wants a coded type and a
		// coded reason. AskISO will not invent a code it cannot verify, so the
		// proprietary branch names the message the query came from.
		Type:      "MT" + m.Type,
		Reason:    "MT" + m.Type,
		Requester: headerBICOrPlaceholder(m.Sender),
		Responder: headerBICOrPlaceholder(m.Receiver),
	}
	b.derived("(type)", "/Document/InvstgtnReq/InvstgtnReq/InvstgtnTp/Prtry",
		"camt.110 wants a coded investigation type; an MT query carries none, so the "+
			"proprietary branch names the source message")
	b.derived("(reason)", "/Document/InvstgtnReq/InvstgtnData/Rsn/Prtry",
		"camt.110 wants a coded investigation reason; an MT query carries prose, so the "+
			"proprietary branch names the source message and the text goes into ReqNrrtv")

	if related, ok := m.Get("21"); ok {
		in.OriginalEndID = related.Value
		b.mapped(related.Name(),
			"/Document/InvstgtnReq/InvstgtnReq/Undrlyg/IntrBk/OrgnlEndToEndId", related.Value)
	}
	if uetr := m.UETR; uetr != "" {
		in.OriginalUETR = uetr
		b.mapped("121", "/Document/InvstgtnReq/InvstgtnReq/Undrlyg/IntrBk/OrgnlUETR", uetr)
	}

	original, name := originalMessage(b, m,
		"/Document/InvstgtnReq/InvstgtnReq/Undrlyg/IntrBk/OrgnlGrpInf")
	in.OriginalMsgID, in.OriginalMsgNm = original, name

	// OrgnlMsgId is mandatory inside the group, so naming the message type
	// without an identifier would drop the name entirely. The reference the
	// query is about is the identifier there is.
	if in.OriginalMsgNm != "" && in.OriginalMsgID == "" {
		in.OriginalMsgID = firstNonEmpty(in.OriginalEndID, ref.Value)
		b.derived("11S", "/Document/InvstgtnReq/InvstgtnReq/Undrlyg/IntrBk/OrgnlGrpInf/OrgnlMsgId",
			"the query named the original message type but not its identifier; "+
				"the related reference was used")
	}

	in.Narrative = exceptionNarrative(b, m,
		"/Document/InvstgtnReq/InvstgtnData/AddtlReqData/ReqNrrtv", 500)

	return &Conversion{
		SourceType: m.Type,
		TargetType: "camt.110.001.01",
		XML:        buildCamt110(in),
		Report:     b.finish(m),
	}, nil
}

// convertAnswer translates an MTn96 answer.
func convertAnswer(m *Message) (*Conversion, error) {
	b := newBuilder(m)

	ref, ok := m.Get("20")
	if !ok {
		return nil, fmt.Errorf("MT%s has no :20: transaction reference", m.Type)
	}
	b.mapped("20", "/Document/InvstgtnRspn/InvstgtnRspn/MsgId", ref.Value)

	related, ok := m.Get("21")
	if !ok {
		return nil, fmt.Errorf("MT%s has no :21: related reference, so there is no request to answer", m.Type)
	}
	b.mapped(related.Name(), "/Document/InvstgtnRspn/OrgnlInvstgtnReq/MsgId", related.Value)

	in := camt111Input{
		MsgID: ref.Value,
		// The status is mandatory and an MT answer carries none. CLSD says the
		// investigation is closed, which is what an answer means.
		Status:        "CLSD",
		Type:          "MT" + m.Type,
		Requester:     headerBICOrPlaceholder(m.Receiver),
		Responder:     headerBICOrPlaceholder(m.Sender),
		OriginalMsgID: related.Value,
	}
	b.derived("(status)", "/Document/InvstgtnRspn/InvstgtnRspn/InvstgtnSts/Sts",
		"camt.111 requires an investigation status; an MT answer carries none, so CLSD was used")

	in.Narrative = exceptionNarrative(b, m,
		"/Document/InvstgtnRspn/InvstgtnRspn/InvstgtnData/RspnData/RspnNrrtv", 500)

	return &Conversion{
		SourceType: m.Type,
		TargetType: "camt.111.001.02",
		XML:        buildCamt111(in),
		Report:     b.finish(m),
	}, nil
}

// exceptionNarrative gathers the free-text fields an exception message carries
// and folds them into one narrative.
//
// MT numbers its queries and answers -- ":75:/1/why was this returned" -- and
// the new messages have one narrative element. Joining them keeps the numbering
// visible so a reader can still tell which answer belongs to which question.
func exceptionNarrative(b *builder, m *Message, path string, max int) string {
	var parts []string
	for _, tag := range []string{"75", "76", "77", "79"} {
		for _, f := range m.All(tag) {
			joined := strings.Join(f.Lines(), " ")
			if strings.TrimSpace(joined) == "" {
				b.note(f.Name())
				continue
			}
			parts = append(parts, ":"+f.Name()+": "+joined)
		}
	}
	if len(parts) == 0 {
		return ""
	}

	joined := strings.Join(parts, " | ")
	short, cut := truncate(joined, max)
	if cut {
		b.truncated("(narrative)", path,
			fmt.Sprintf("the free-text fields total %d characters; the element permits %d",
				len([]rune(joined)), max), joined)
		return short
	}
	b.mapped("(narrative)", path, joined)
	// Mark the fields that fed it, so finish does not report them as dropped.
	for _, tag := range []string{"75", "76", "77", "79"} {
		for _, f := range m.All(tag) {
			b.note(f.Name())
		}
	}
	return short
}

// originalMessage reads :11S: or :11R:, which name the message an exception is
// about and the date it was sent.
func originalMessage(b *builder, m *Message, path string) (id, name string) {
	for _, tag := range []string{"11S", "11R"} {
		f, ok := m.GetExact(tag)
		if !ok {
			continue
		}
		lines := f.Lines()
		if len(lines) == 0 {
			continue
		}
		// The first line is the message type, the second its date.
		name = "MT" + strings.TrimSpace(lines[0])
		b.mapped(f.Name(), path+"/OrgnlMsgNmId", f.Value)
		if len(lines) > 1 {
			id = strings.TrimSpace(lines[1])
		}
		return id, name
	}
	return "", ""
}

// firstNonEmpty returns the first value that is not empty.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// headerBICOrPlaceholder gives a BIC for a header address, or a placeholder
// where a BICFI element is mandatory.
func headerBICOrPlaceholder(address string) string {
	if bic := headerBIC(address); bic != "" {
		return bic
	}
	return "NOTPROVIDED"
}
