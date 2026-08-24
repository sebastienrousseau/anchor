// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package swift

import (
	"strings"
	"testing"
)

// The exception messages are where the migration is least mechanical: MT
// carries prose where camt.110 wants a coded reason. These tests are as much
// about what the conversion refuses to invent as about what it produces.

const mt192 = `{1:F01BANKGB2LAXXX0000000000}{2:I192BANKDEFFXXXXN}{3:{121:f3a1b2c4-5d6e-4f70-8a91-2b3c4d5e6f70}}{4:
:20:CANC20260824001
:21:REF20260824001
:11S:103
260824
:32A:260824EUR25000,00
:79:/AC03/BENEFICIARY ACCOUNT CLOSED
PLEASE CANCEL AND RETURN FUNDS
-}`

func TestConvertCancellationRequest(t *testing.T) {
	c := convert(t, mt192)

	if c.SourceType != "192" || c.TargetType != "camt.056.001.11" {
		t.Errorf("got %s -> %s", c.SourceType, c.TargetType)
	}

	for _, want := range []string{
		"<Id>CANC20260824001</Id>",
		"<BICFI>BANKGB2LXXX</BICFI>",
		"<BICFI>BANKDEFFXXX</BICFI>",
		"<OrgnlMsgNmId>MT103</OrgnlMsgNmId>",
		"<OrgnlEndToEndId>REF20260824001</OrgnlEndToEndId>",
		"<OrgnlUETR>f3a1b2c4-5d6e-4f70-8a91-2b3c4d5e6f70</OrgnlUETR>",
		`<OrgnlIntrBkSttlmAmt Ccy="EUR">25000.00</OrgnlIntrBkSttlmAmt>`,
		"<OrgnlIntrBkSttlmDt>2026-08-24</OrgnlIntrBkSttlmDt>",
		"BENEFICIARY ACCOUNT CLOSED",
	} {
		if !strings.Contains(c.XML, want) {
			t.Errorf("output is missing %s\n%s", want, c.XML)
		}
	}
}

func TestEveryCategoryConverts(t *testing.T) {
	// MT192 cancels a customer payment, MT292 an institution one, MT592 a
	// treasury one. They do the same thing, and listing eighteen message types
	// would be a list nobody maintains.
	for _, category := range []string{"1", "2", "4", "5", "7", "9"} {
		for kind, target := range map[string]string{
			"92": "camt.056.001.11",
			"95": "camt.110.001.01",
			"96": "camt.111.001.02",
		} {
			msgType := category + kind
			raw := "{1:F01BANKGB2LAXXX0000000000}{2:I" + msgType + "BANKDEFFXXXXN}{4:\n" +
				":20:REF-1\n:21:REL-1\n:79:SOME NARRATIVE\n-}"

			c, err := Convert(mustParse(t, raw))
			if err != nil {
				t.Errorf("MT%s: %v", msgType, err)
				continue
			}
			if c.TargetType != target {
				t.Errorf("MT%s produced %s, want %s", msgType, c.TargetType, target)
			}
		}
	}
}

func TestExceptionKind(t *testing.T) {
	cases := map[string]string{
		"192": "92", "292": "92", "992": "92",
		"195": "95", "596": "96",
		"103": "", "940": "", "1922": "", "92": "",
	}
	for in, want := range cases {
		if got := exceptionKind(in); got != want {
			t.Errorf("exceptionKind(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCancellationRejects(t *testing.T) {
	base := "{1:F01BANKGB2LAXXX0000000000}{2:I192BANKDEFFXXXXN}{4:\n"
	for name, body := range map[string]string{
		"no reference":         ":21:REL-1\n:79:X\n",
		"no related reference": ":20:REF-1\n:79:X\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Convert(mustParse(t, base+body+"-}")); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestCancellationWithoutTheOriginalMessage(t *testing.T) {
	// OrgnlGrpInf needs a message identifier. Without :11S: the related
	// reference is the only one there is, and the report says so.
	raw := "{1:F01BANKGB2LAXXX0000000000}{2:I192BANKDEFFXXXXN}{4:\n" +
		":20:CANC-1\n:21:REF-1\n:79:REASON\n-}"

	c := convert(t, raw)
	if !strings.Contains(c.XML, "<OrgnlMsgId>REF-1</OrgnlMsgId>") {
		t.Errorf("no original message identifier was produced\n%s", c.XML)
	}
	if !strings.Contains(c.XML, "<OrgnlMsgNmId>UNKNOWN</OrgnlMsgNmId>") {
		t.Errorf("the unknown message name was not marked as such\n%s", c.XML)
	}
	if got := fidelityOf(t, c, "11S").Fidelity; got != FidelityDerived {
		t.Errorf(":11S: fidelity = %q, want derived", got)
	}
}

func TestCancellationWithABadAmount(t *testing.T) {
	raw := "{1:F01BANKGB2LAXXX0000000000}{2:I192BANKDEFFXXXXN}{4:\n" +
		":20:CANC-1\n:21:REF-1\n:32A:NONSENSE\n:79:REASON\n-}"

	c := convert(t, raw)
	// A malformed amount is reported rather than aborting a cancellation that
	// is otherwise complete.
	if got := fidelityOf(t, c, "32A").Fidelity; got != FidelityUnmapped {
		t.Errorf(":32A: fidelity = %q, want unmapped", got)
	}
	if strings.Contains(c.XML, "OrgnlIntrBkSttlmAmt") {
		t.Errorf("a malformed amount was emitted\n%s", c.XML)
	}
}

const mt195 = `{1:F01BANKGB2LAXXX0000000000}{2:I195BANKDEFFXXXXN}{4:
:20:QRY20260824001
:21:REF20260824001
:11S:103
260824
:75:/1/WAS THIS PAYMENT CREDITED
/2/PLEASE CONFIRM VALUE DATE
-}`

func TestConvertQuery(t *testing.T) {
	c := convert(t, mt195)

	if c.TargetType != "camt.110.001.01" {
		t.Errorf("TargetType = %q", c.TargetType)
	}
	for _, want := range []string{
		"<MsgId>QRY20260824001</MsgId>",
		"<OrgnlEndToEndId>REF20260824001</OrgnlEndToEndId>",
		"<OrgnlMsgNmId>MT103</OrgnlMsgNmId>",
		"WAS THIS PAYMENT CREDITED",
		"PLEASE CONFIRM VALUE DATE",
	} {
		if !strings.Contains(c.XML, want) {
			t.Errorf("output is missing %s\n%s", want, c.XML)
		}
	}

	// camt.110 wants a coded type and a coded reason, and an MT query carries
	// neither. Anchor names the source message in the proprietary branch rather
	// than inventing a code nobody can verify.
	if !strings.Contains(c.XML, "<Prtry>MT195</Prtry>") {
		t.Errorf("the proprietary type was not used\n%s", c.XML)
	}
	if strings.Contains(c.XML, "<InvstgtnTp>\n        <Cd>") {
		t.Errorf("a code was invented for the investigation type\n%s", c.XML)
	}

	derived := map[string]bool{}
	for _, r := range c.Report {
		if r.Fidelity == FidelityDerived {
			derived[r.Tag] = true
		}
	}
	if !derived["(type)"] || !derived["(reason)"] {
		t.Errorf("the derived type and reason were not reported: %+v", c.Report)
	}

	// The numbering survives, so a reader can still tell which query is which.
	if !strings.Contains(c.XML, "/1/") || !strings.Contains(c.XML, "/2/") {
		t.Errorf("the query numbering was lost\n%s", c.XML)
	}
}

func TestQueryRejectsWithoutAReference(t *testing.T) {
	raw := "{1:F01BANKGB2LAXXX0000000000}{2:I195BANKDEFFXXXXN}{4:\n:21:REL-1\n:75:X\n-}"
	if _, err := Convert(mustParse(t, raw)); err == nil {
		t.Fatal("a query with no transaction reference was accepted")
	}
}

func TestQueryWithoutARelatedReference(t *testing.T) {
	// A query need not name a payment: it may be about a whole message.
	raw := "{1:F01BANKGB2LAXXX0000000000}{2:I195BANKDEFFXXXXN}{4:\n:20:QRY-1\n:75:WHY\n-}"

	c := convert(t, raw)
	if strings.Contains(c.XML, "<OrgnlEndToEndId>") {
		t.Errorf("an end-to-end identification was invented\n%s", c.XML)
	}
	if !strings.Contains(c.XML, "WHY") {
		t.Errorf("the query text was lost\n%s", c.XML)
	}
}

const mt196 = `{1:F01BANKDEFFAXXX0000000000}{2:I196BANKGB2LXXXXN}{4:
:20:ANS20260824001
:21:QRY20260824001
:76:/1/CREDITED 24 AUGUST 2026
/2/VALUE DATE CONFIRMED
-}`

func TestConvertAnswer(t *testing.T) {
	c := convert(t, mt196)

	if c.TargetType != "camt.111.001.02" {
		t.Errorf("TargetType = %q", c.TargetType)
	}
	for _, want := range []string{
		"<MsgId>ANS20260824001</MsgId>",
		"<MsgId>QRY20260824001</MsgId>",
		"<Sts>CLSD</Sts>",
		"CREDITED 24 AUGUST 2026",
		"VALUE DATE CONFIRMED",
	} {
		if !strings.Contains(c.XML, want) {
			t.Errorf("output is missing %s\n%s", want, c.XML)
		}
	}

	// The responder is the sender of the answer and the requester the receiver,
	// which is the other way round from a query.
	if !strings.Contains(c.XML, "<BICFI>BANKDEFFXXX</BICFI>") {
		t.Errorf("the responder is not the sender\n%s", c.XML)
	}

	// The status is mandatory and an MT answer carries none.
	var reported bool
	for _, r := range c.Report {
		if r.Tag == "(status)" && r.Fidelity == FidelityDerived {
			reported = true
		}
	}
	if !reported {
		t.Errorf("the derived status was not reported: %+v", c.Report)
	}
}

func TestAnswerRejects(t *testing.T) {
	base := "{1:F01BANKDEFFAXXX0000000000}{2:I196BANKGB2LXXXXN}{4:\n"
	for name, body := range map[string]string{
		"no reference":         ":21:QRY-1\n:76:X\n",
		"no related reference": ":20:ANS-1\n:76:X\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Convert(mustParse(t, base+body+"-}")); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestExceptionNarrativeIsTruncatedAndReported(t *testing.T) {
	long := strings.Repeat("A", 600)
	raw := "{1:F01BANKGB2LAXXX0000000000}{2:I195BANKDEFFXXXXN}{4:\n" +
		":20:QRY-1\n:21:REF-1\n:75:" + long + "\n-}"

	c := convert(t, raw)
	if strings.Contains(c.XML, long) {
		t.Error("a 600-character narrative was emitted into a Max500Text element")
	}
	var reported bool
	for _, r := range c.Report {
		if r.Tag == "(narrative)" && r.Fidelity == FidelityTruncated {
			reported = true
			if !strings.Contains(r.Note, "500") {
				t.Errorf("the note does not give the limit: %q", r.Note)
			}
		}
	}
	if !reported {
		t.Errorf("the truncated narrative was not reported: %+v", c.Report)
	}
}

func TestExceptionWithNoNarrativeAtAll(t *testing.T) {
	// A cancellation with no reason is still a cancellation.
	raw := "{1:F01BANKGB2LAXXX0000000000}{2:I192BANKDEFFXXXXN}{4:\n:20:CANC-1\n:21:REF-1\n-}"

	c := convert(t, raw)
	if strings.Contains(c.XML, "<CxlRsnInf>") {
		t.Errorf("an empty reason was emitted\n%s", c.XML)
	}
	if !strings.Contains(c.XML, "<Id>CANC-1</Id>") {
		t.Errorf("the cancellation was not produced\n%s", c.XML)
	}
}

func TestExceptionWithEmptyNarrativeFields(t *testing.T) {
	raw := "{1:F01BANKGB2LAXXX0000000000}{2:I195BANKDEFFXXXXN}{4:\n" +
		":20:QRY-1\n:21:REF-1\n:75:\n:79:\n-}"

	c := convert(t, raw)
	if strings.Contains(c.XML, "<ReqNrrtv>") {
		t.Errorf("an empty narrative element was emitted\n%s", c.XML)
	}
	// Empty fields must not be reported as dropped: they carried nothing.
	for _, r := range c.Report {
		if (r.Tag == "75" || r.Tag == "79") && r.Fidelity == FidelityUnmapped {
			t.Errorf("an empty field was reported as dropped: %+v", r)
		}
	}
}

func TestExceptionWithoutHeaderBICs(t *testing.T) {
	// The BICFI elements are mandatory, so a truncated header still has to
	// produce a valid document.
	raw := "{1:F01}{2:I195}{4:\n:20:QRY-1\n:21:REF-1\n:75:WHY\n-}"

	c := convert(t, raw)
	if strings.Count(c.XML, "<BICFI>NOTPROVIDED</BICFI>") != 2 {
		t.Errorf("the mandatory institutions have no placeholders\n%s", c.XML)
	}
}

func TestOriginalMessageFromField11R(t *testing.T) {
	// :11R: names the original message on the receiving side, and carries the
	// same two lines as :11S:.
	raw := "{1:F01BANKGB2LAXXX0000000000}{2:I192BANKDEFFXXXXN}{4:\n" +
		":20:CANC-1\n:21:REF-1\n:11R:202\n260824\n:79:REASON\n-}"

	c := convert(t, raw)
	if !strings.Contains(c.XML, "<OrgnlMsgNmId>MT202</OrgnlMsgNmId>") {
		t.Errorf(":11R: was not read\n%s", c.XML)
	}
	if !strings.Contains(c.XML, "<OrgnlMsgId>260824</OrgnlMsgId>") {
		t.Errorf("the original message identifier was not read\n%s", c.XML)
	}
}

func TestOriginalMessageWithOneLine(t *testing.T) {
	// :11S: with only a message type and no date still names the message.
	raw := "{1:F01BANKGB2LAXXX0000000000}{2:I195BANKDEFFXXXXN}{4:\n" +
		":20:QRY-1\n:21:REF-1\n:11S:103\n:75:WHY\n-}"

	c := convert(t, raw)
	if !strings.Contains(c.XML, "<OrgnlMsgNmId>MT103</OrgnlMsgNmId>") {
		t.Errorf(":11S: was not read\n%s", c.XML)
	}
}

func TestSupportedListsTheExceptionFamily(t *testing.T) {
	got := Supported()
	for _, want := range []string{"n92", "n95", "n96"} {
		found := false
		for _, g := range got {
			if g == want {
				found = true
			}
		}
		if !found {
			t.Errorf("%s is missing from %v", want, got)
		}
	}
}

func TestCancellationWithoutACaseIdentifier(t *testing.T) {
	// The case is optional. Building one from the request's own reference is
	// what lets a receiver group the exchange, but a request that carries no
	// reference at all cannot be converted anyway.
	c := convert(t, mt192)
	if !strings.Contains(c.XML, "<Case>") {
		t.Errorf("no case was opened\n%s", c.XML)
	}
	if !strings.Contains(c.XML, "<Id>CANC20260824001</Id>") {
		t.Errorf("the case does not carry the request's reference\n%s", c.XML)
	}
}

func TestExceptionOriginalMessageNameWithoutAnIdentifier(t *testing.T) {
	// :11S: names the message type without its identifier. OrgnlMsgId is
	// mandatory inside the group, so dropping the name would be the easy wrong
	// answer; the related reference stands in and the report says so.
	for _, tc := range []struct{ msgType, want string }{
		{"192", "/Document/FIToFIPmtCxlReq/Undrlyg/TxInf/OrgnlGrpInf/OrgnlMsgId"},
		{"195", "/Document/InvstgtnReq/InvstgtnReq/Undrlyg/IntrBk/OrgnlGrpInf/OrgnlMsgId"},
	} {
		t.Run("MT"+tc.msgType, func(t *testing.T) {
			raw := "{1:F01BANKGB2LAXXX0000000000}{2:I" + tc.msgType + "BANKDEFFXXXXN}{4:\n" +
				":20:REF-1\n:21:REL-1\n:11S:103\n:79:TEXT\n-}"

			c := convert(t, raw)
			if !strings.Contains(c.XML, "<OrgnlMsgNmId>MT103</OrgnlMsgNmId>") {
				t.Errorf("the original message name was dropped\n%s", c.XML)
			}
			if !strings.Contains(c.XML, "<OrgnlMsgId>REL-1</OrgnlMsgId>") {
				t.Errorf("the related reference was not used as the identifier\n%s", c.XML)
			}

			var reported bool
			for _, r := range c.Report {
				if r.Path == tc.want && r.Fidelity == FidelityDerived {
					reported = true
				}
			}
			if !reported {
				t.Errorf("the derived identifier was not reported: %+v", c.Report)
			}
		})
	}
}

func TestQueryOriginalMessageWithoutAnyReference(t *testing.T) {
	// No :21: either: the query's own reference is the last one available.
	raw := "{1:F01BANKGB2LAXXX0000000000}{2:I195BANKDEFFXXXXN}{4:\n" +
		":20:QRY-1\n:11S:103\n:75:WHY\n-}"

	c := convert(t, raw)
	if !strings.Contains(c.XML, "<OrgnlMsgId>QRY-1</OrgnlMsgId>") {
		t.Errorf("the query's own reference was not used\n%s", c.XML)
	}
}

func TestExceptionWithAnEmptyOriginalMessageField(t *testing.T) {
	// A :11S: with nothing in it names no message, so no group is emitted.
	raw := "{1:F01BANKGB2LAXXX0000000000}{2:I195BANKDEFFXXXXN}{4:\n" +
		":20:QRY-1\n:21:REL-1\n:11S:\n:75:WHY\n-}"

	c := convert(t, raw)
	if strings.Contains(c.XML, "<OrgnlGrpInf>") {
		t.Errorf("an empty original message field produced a group\n%s", c.XML)
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("", "  ", "third"); got != "third" {
		t.Errorf("firstNonEmpty = %q", got)
	}
	if got := firstNonEmpty("", "  "); got != "" {
		t.Errorf("firstNonEmpty = %q", got)
	}
	if got := firstNonEmpty(); got != "" {
		t.Errorf("firstNonEmpty() = %q", got)
	}
}

func TestQueryCarriesTheTrackingReference(t *testing.T) {
	// A query quoting the payment's UETR can be matched to it automatically,
	// which is the point of the modernised flow.
	raw := `{1:F01BANKGB2LAXXX0000000000}{2:I195BANKDEFFXXXXN}{3:{121:f3a1b2c4-5d6e-4f70-8a91-2b3c4d5e6f70}}{4:
:20:QRY-1
:21:REF-1
:75:WHY WAS THIS RETURNED
-}`
	c := convert(t, raw)
	if !strings.Contains(c.XML, "<OrgnlUETR>f3a1b2c4-5d6e-4f70-8a91-2b3c4d5e6f70</OrgnlUETR>") {
		t.Errorf("the tracking reference was dropped\n%s", c.XML)
	}
	if got := fidelityOf(t, c, "121").Fidelity; got != FidelityMapped {
		t.Errorf(":121: fidelity = %q, want mapped", got)
	}
}
