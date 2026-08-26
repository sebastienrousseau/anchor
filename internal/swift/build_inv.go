// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package swift

import (
	"fmt"
	"strings"
)

// The MT n9x family is how exceptions have been handled for decades: a request
// for cancellation, a free-text query, a free-text answer. camt.056 and the
// camt.110/111 pair replace them with something a machine can route.
//
// The conversion is honest about what that costs. An MT query carries prose
// where the new messages want a coded reason, and AskISO will not invent a code
// it cannot verify: the reason goes into the proprietary branch of the choice,
// naming the MT field it came from, and the prose goes into the narrative.

// camt056Input is a payment cancellation request.
type camt056Input struct {
	AssignmentID  string
	CreatedAt     string
	Assigner      string
	Assignee      string
	CaseID        string
	OriginalMsgID string
	OriginalMsgNm string
	OriginalEndID string
	OriginalUETR  string
	Currency      string
	Amount        string
	SettlementDay string
	Reason        string
}

func buildCamt056(in camt056Input) string {
	var b strings.Builder

	fmt.Fprintf(&b, `<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:camt.056.001.11">
  <FIToFIPmtCxlReq>
    <Assgnmt>
      <Id>%s</Id>
      <Assgnr>
        <Agt>
          <FinInstnId>
            <BICFI>%s</BICFI>
          </FinInstnId>
        </Agt>
      </Assgnr>
      <Assgne>
        <Agt>
          <FinInstnId>
            <BICFI>%s</BICFI>
          </FinInstnId>
        </Agt>
      </Assgne>
      <CreDtTm>%s</CreDtTm>
    </Assgnmt>`,
		esc(in.AssignmentID), esc(in.Assigner), esc(in.Assignee), esc(in.CreatedAt))

	if in.CaseID != "" {
		fmt.Fprintf(&b, `
    <Case>
      <Id>%s</Id>
      <Cretr>
        <Agt>
          <FinInstnId>
            <BICFI>%s</BICFI>
          </FinInstnId>
        </Agt>
      </Cretr>
    </Case>`, esc(in.CaseID), esc(in.Assigner))
	}

	b.WriteString("\n    <Undrlyg>\n      <TxInf>")
	// Both members of the group are mandatory, so the caller supplies a name
	// whenever it supplies an identifier.
	if in.OriginalMsgID != "" {
		fmt.Fprintf(&b, `
        <OrgnlGrpInf>
          <OrgnlMsgId>%s</OrgnlMsgId>
          <OrgnlMsgNmId>%s</OrgnlMsgNmId>
        </OrgnlGrpInf>`, esc(in.OriginalMsgID), esc(in.OriginalMsgNm))
	}
	b.WriteString(optionalIndented("OrgnlEndToEndId", in.OriginalEndID, "        "))
	b.WriteString(optionalIndented("OrgnlUETR", in.OriginalUETR, "        "))

	if in.Amount != "" {
		fmt.Fprintf(&b, "\n        <OrgnlIntrBkSttlmAmt Ccy=\"%s\">%s</OrgnlIntrBkSttlmAmt>",
			esc(in.Currency), esc(in.Amount))
	}
	b.WriteString(optionalIndented("OrgnlIntrBkSttlmDt", in.SettlementDay, "        "))

	if in.Reason != "" {
		fmt.Fprintf(&b, "\n        <CxlRsnInf>\n          <AddtlInf>%s</AddtlInf>\n        </CxlRsnInf>",
			esc(in.Reason))
	}

	b.WriteString("\n      </TxInf>\n    </Undrlyg>\n  </FIToFIPmtCxlReq>\n</Document>")
	return b.String()
}

// camt110Input is an investigation request.
type camt110Input struct {
	MsgID         string
	Type          string
	Requester     string
	Responder     string
	OriginalMsgID string
	OriginalMsgNm string
	OriginalEndID string
	OriginalUETR  string
	Reason        string
	Narrative     string
}

func buildCamt110(in camt110Input) string {
	var b strings.Builder

	fmt.Fprintf(&b, `<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:camt.110.001.01">
  <InvstgtnReq>
    <InvstgtnReq>
      <MsgId>%s</MsgId>
      <InvstgtnTp>
        <Prtry>%s</Prtry>
      </InvstgtnTp>
      <Undrlyg>
        <IntrBk>`, esc(in.MsgID), esc(in.Type))

	if in.OriginalMsgID != "" {
		fmt.Fprintf(&b, `
          <OrgnlGrpInf>
            <OrgnlMsgId>%s</OrgnlMsgId>
            <OrgnlMsgNmId>%s</OrgnlMsgNmId>
          </OrgnlGrpInf>`, esc(in.OriginalMsgID), esc(in.OriginalMsgNm))
	}
	b.WriteString(optionalIndented("OrgnlEndToEndId", in.OriginalEndID, "          "))
	b.WriteString(optionalIndented("OrgnlUETR", in.OriginalUETR, "          "))

	fmt.Fprintf(&b, `
        </IntrBk>
      </Undrlyg>
      <Rqstr>
        <Agt>
          <FinInstnId>
            <BICFI>%s</BICFI>
          </FinInstnId>
        </Agt>
      </Rqstr>
      <Rspndr>
        <Agt>
          <FinInstnId>
            <BICFI>%s</BICFI>
          </FinInstnId>
        </Agt>
      </Rspndr>
    </InvstgtnReq>
    <InvstgtnData>
      <Rsn>
        <Prtry>%s</Prtry>
      </Rsn>`, esc(in.Requester), esc(in.Responder), esc(in.Reason))

	if in.Narrative != "" {
		fmt.Fprintf(&b, "\n      <AddtlReqData>\n        <ReqNrrtv>%s</ReqNrrtv>\n      </AddtlReqData>",
			esc(in.Narrative))
	}

	b.WriteString("\n    </InvstgtnData>\n  </InvstgtnReq>\n</Document>")
	return b.String()
}

// camt111Input is an investigation response.
type camt111Input struct {
	MsgID         string
	Status        string
	Type          string
	Requester     string
	Responder     string
	OriginalMsgID string
	Narrative     string
}

func buildCamt111(in camt111Input) string {
	var b strings.Builder

	fmt.Fprintf(&b, `<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:camt.111.001.02">
  <InvstgtnRspn>
    <InvstgtnRspn>
      <MsgId>%s</MsgId>
      <InvstgtnSts>
        <Sts>%s</Sts>
      </InvstgtnSts>`, esc(in.MsgID), esc(in.Status))

	if in.Narrative != "" {
		fmt.Fprintf(&b, `
      <InvstgtnData>
        <RspnData>
          <RspnNrrtv>%s</RspnNrrtv>
        </RspnData>
      </InvstgtnData>`, esc(in.Narrative))
	}

	fmt.Fprintf(&b, `
    </InvstgtnRspn>
    <OrgnlInvstgtnReq>
      <MsgId>%s</MsgId>
      <InvstgtnTp>
        <Prtry>%s</Prtry>
      </InvstgtnTp>
      <Rqstr>
        <Agt>
          <FinInstnId>
            <BICFI>%s</BICFI>
          </FinInstnId>
        </Agt>
      </Rqstr>
      <Rspndr>
        <Agt>
          <FinInstnId>
            <BICFI>%s</BICFI>
          </FinInstnId>
        </Agt>
      </Rspndr>
    </OrgnlInvstgtnReq>
  </InvstgtnRspn>
</Document>`, esc(in.OriginalMsgID), esc(in.Type), esc(in.Requester), esc(in.Responder))

	return b.String()
}
