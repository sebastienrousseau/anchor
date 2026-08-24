// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package rules

import (
	"strings"

	"github.com/sebastienrousseau/askiso/internal/converter"
)

// Two message families arrive with the modernisation programme, and both are
// easy to send in a form the receiver cannot act on.
//
// camt.110 and camt.111 replace the MT n92/n95/n96 investigation flow. An
// investigation that does not identify the payment it is about, or a response
// that does not quote the request it answers, is a message the other side has
// to chase by hand -- which is the thing the new flow exists to stop.
//
// acmt.023 and acmt.024 carry Verification of Payee. A verification report that
// says the name did not match, without saying what the account is actually
// registered as, forces the payer to guess.

// InvestigationIdentifiesPayment requires a camt.110 to reference the
// underlying transaction.
var InvestigationIdentifiesPayment = Rule{
	ID:       "INV-001",
	Name:     "Investigation identifies its payment",
	Severity: SeverityError,
	Description: "An investigation request has to say which payment it is about. " +
		"Without an underlying reference the responder has nothing to look up.",
	Remediation: "Populate <Undrlyg> with the original instruction, interbank " +
		"transaction, statement entry or account the investigation concerns.",
	Reference: "camt.110 InvestigationRequest",
	Exempt:    onlyFor("camt.110"),
	Check: func(ctx *Context) []Finding {
		var out []Finding
		for _, req := range investigationRequests(ctx.Root) {
			if _, ok := Child(req.Node, "Undrlyg"); ok {
				continue
			}
			out = append(out, Finding{
				Path:     req.Path + "/Undrlyg",
				Message:  "the investigation does not identify the payment it concerns",
				Expected: "an <Undrlyg> element",
			})
		}
		return out
	},
}

// investigationRequests finds the request blocks, not the message wrapper that
// shares their name. The wrapper element and the block inside it are both
// called InvstgtnReq; only the block carries a message identifier.
func investigationRequests(root *converter.Node) []Located {
	var out []Located
	for _, candidate := range FindAll(root, "InvstgtnReq") {
		if ChildText(candidate.Node, "MsgId") == "" {
			continue
		}
		out = append(out, candidate)
	}
	return out
}

// InvestigationCarriesTrackingReference asks for the tracking reference that
// makes an investigation matchable automatically.
var InvestigationCarriesTrackingReference = Rule{
	ID:       "INV-002",
	Name:     "Investigation carries a tracking reference",
	Severity: SeverityWarning,
	Description: "An investigation quoting the payment's UETR can be matched to it " +
		"automatically. One quoting only a message identifier cannot, because a " +
		"message identifier is unique to a sender rather than to a payment.",
	Remediation: "Populate <EIR> with the investigation's own reference, and quote the " +
		"payment's UETR inside <Undrlyg>.",
	Reference: "camt.110 InvestigationRequest",
	Exempt:    onlyFor("camt.110"),
	Check: func(ctx *Context) []Finding {
		var out []Finding
		for _, req := range investigationRequests(ctx.Root) {
			underlying, ok := Child(req.Node, "Undrlyg")
			if !ok {
				// INV-001 reports this; repeating it here would double-count.
				continue
			}
			if len(FindAll(underlying, "UETR")) > 0 {
				continue
			}
			out = append(out, Finding{
				Path:     req.Path + "/Undrlyg",
				Message:  "the underlying payment is identified without a UETR",
				Expected: "a <UETR> element inside <Undrlyg>",
			})
		}
		return out
	},
}

// InvestigationResponseQuotesRequest requires a camt.111 to quote the request
// it answers.
var InvestigationResponseQuotesRequest = Rule{
	ID:       "INV-003",
	Name:     "Investigation response quotes its request",
	Severity: SeverityError,
	Description: "A response has to carry the request it answers, or the requester " +
		"cannot tell which of its open investigations has been resolved.",
	Remediation: "Populate <OrgnlInvstgtnReq> with the original request.",
	Reference:   "camt.111 InvestigationResponse",
	Exempt:      onlyFor("camt.111"),
	Check: func(ctx *Context) []Finding {
		// The original request is a sibling of the response rather than part of
		// it, so this is a property of the message and is reported once.
		if len(FindAll(ctx.Root, "InvstgtnRspn")) == 0 {
			return nil
		}
		if len(FindAll(ctx.Root, "OrgnlInvstgtnReq")) > 0 {
			return nil
		}
		return []Finding{{
			Path:     "/" + ctx.Root.Name + "/InvstgtnRspn/OrgnlInvstgtnReq",
			Message:  "the response does not quote the request it answers",
			Expected: "an <OrgnlInvstgtnReq> element",
		}}
	},
}

// VerificationReportExplainsAMismatch requires a negative Verification of Payee
// report to say why.
var VerificationReportExplainsAMismatch = Rule{
	ID:       "VOP-001",
	Name:     "Verification report explains a mismatch",
	Severity: SeverityError,
	Description: "A report saying the payee could not be verified, with no reason and " +
		"no corrected details, leaves the payer with nothing to act on. That is the " +
		"outcome Verification of Payee exists to prevent.",
	Remediation: "Populate <Rsn> with the reason, and <UpdtdPtyAndAcctId> with the " +
		"details the account is actually registered under where the scheme permits it.",
	Reference: "acmt.024 IdentificationVerificationReport",
	Exempt:    onlyFor("acmt.024"),
	Check: func(ctx *Context) []Finding {
		var out []Finding
		for _, report := range FindAll(ctx.Root, "Rpt") {
			verified := strings.ToLower(ChildText(report.Node, "Vrfctn"))
			// The indicator is a boolean: true means the details matched.
			if verified == "" || verified == "true" || verified == "1" {
				continue
			}
			_, hasReason := Child(report.Node, "Rsn")
			_, hasUpdated := Child(report.Node, "UpdtdPtyAndAcctId")
			if hasReason || hasUpdated {
				continue
			}
			out = append(out, Finding{
				Path:     report.Path + "/Rsn",
				Message:  "the payee could not be verified, and the report says neither why nor what the correct details are",
				Found:    "Vrfctn=" + verified,
				Expected: "a <Rsn> or <UpdtdPtyAndAcctId> element",
			})
		}
		return out
	},
}

// VerificationRequestIdentifiesTheAccount requires a VoP request to carry the
// party and account being checked.
var VerificationRequestIdentifiesTheAccount = Rule{
	ID:       "VOP-002",
	Name:     "Verification request identifies the account",
	Severity: SeverityError,
	Description: "A verification request has to carry the name and the account being " +
		"checked; there is nothing to verify otherwise.",
	Remediation: "Populate <Vrfctn><PtyAndAcctId> with the payee's name and account " +
		"identification as the payer entered them.",
	Reference: "acmt.023 IdentificationVerificationRequest",
	Exempt:    onlyFor("acmt.023"),
	Check: func(ctx *Context) []Finding {
		var out []Finding
		for _, v := range FindAll(ctx.Root, "Vrfctn") {
			// Only the request's own Vrfctn element holds a party; the report's
			// Vrfctn is a boolean indicator.
			if len(v.Node.Children) == 0 {
				continue
			}
			if _, ok := Child(v.Node, "PtyAndAcctId"); ok {
				continue
			}
			out = append(out, Finding{
				Path:     v.Path + "/PtyAndAcctId",
				Message:  "the request does not say which party and account to verify",
				Expected: "a <PtyAndAcctId> element",
			})
		}
		return out
	},
}

// InvestigationRules covers the camt.110 and camt.111 investigation flow.
var InvestigationRules = []Rule{
	InvestigationIdentifiesPayment,
	InvestigationCarriesTrackingReference,
	InvestigationResponseQuotesRequest,
}

// VerificationRules covers Verification of Payee.
var VerificationRules = []Rule{
	VerificationRequestIdentifiesTheAccount,
	VerificationReportExplainsAMismatch,
}

// onlyFor builds an exemption that leaves a rule applying to one message family
// alone. Most rules exempt a few messages; these apply to a few.
func onlyFor(bases ...string) func(string) bool {
	wanted := make(map[string]bool, len(bases))
	for _, b := range bases {
		wanted[b] = true
	}
	return func(msgID string) bool { return !wanted[baseCode(msgID)] }
}
