// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package rules

import (
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sebastienrousseau/askiso/internal/converter"
)

// The CBPR+ profile deliberately models the live Standards Release rather than
// accepting every ISO 20022 version with a matching four-letter family. CBPR+
// is a set of Usage Guidelines: a base ISO message can be valid XML and still
// be the wrong version or carry the wrong Business Service for FINplus.
//
// These are the SR2025 Usage Identifiers. Variant services share a message
// definition, so dispatch must use both MsgDefIdr and BizSvc.
var cbprSR2025 = map[string][]string{
	"admi.024.001.01": {"swift.cbprplus.01"},
	"camt.025.001.08": {"swift.cbprplus.01"},
	"camt.029.001.09": {"swift.cbprplus.03"},
	"camt.052.001.08": {"swift.cbprplus.03"},
	"camt.053.001.08": {"swift.cbprplus.03"},
	"camt.054.001.08": {"swift.cbprplus.03"},
	"camt.055.001.08": {"swift.cbprplus.02"},
	"camt.056.001.08": {"swift.cbprplus.03"},
	"camt.057.001.06": {"swift.cbprplus.03"},
	"camt.058.001.08": {"swift.cbprplus.02"},
	"camt.060.001.05": {"swift.cbprplus.03"},
	"camt.105.001.02": {"swift.cbprplus.02", "swift.cbprplus.mlp.02"},
	"camt.106.001.02": {"swift.cbprplus.02", "swift.cbprplus.mlp.02"},
	"camt.107.001.01": {"swift.cbprplus.02"},
	"camt.108.001.01": {"swift.cbprplus.02"},
	"camt.109.001.01": {"swift.cbprplus.03"},
	"pacs.002.001.10": {"swift.cbprplus.03"},
	"pacs.003.001.08": {"swift.cbprplus.02"},
	"pacs.004.001.09": {"swift.cbprplus.03"},
	"pacs.008.001.08": {"swift.cbprplus.03", "swift.cbprplus.stp.03"},
	"pacs.009.001.08": {"swift.cbprplus.03", "swift.cbprplus.adv.03", "swift.cbprplus.cov.03"},
	"pacs.010.001.03": {"swift.cbprplus.03", "swift.cbprplus.col.02"},
	"pain.001.001.09": {"swift.cbprplus.03"},
	"pain.002.001.10": {"swift.cbprplus.03"},
	"pain.008.001.08": {"swift.cbprplus.02"},
}

// CBPRUsageGuideline identifies one message-definition and Business Service
// variant in a release collection. The inventory is public metadata; no Usage
// Guideline content is embedded in it.
type CBPRUsageGuideline struct {
	MessageID       string `json:"message_id"`
	UsageIdentifier string `json:"usage_identifier"`
}

// CBPRSR2025UsageGuidelines returns the expected live SR2025 collection in a
// stable order. Callers use it to report local pack completeness without
// claiming that an absent proprietary artefact was checked.
func CBPRSR2025UsageGuidelines() []CBPRUsageGuideline {
	out := make([]CBPRUsageGuideline, 0, 31)
	for messageID, services := range cbprSR2025 {
		for _, service := range services {
			out = append(out, CBPRUsageGuideline{MessageID: messageID, UsageIdentifier: service})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].MessageID != out[j].MessageID {
			return out[i].MessageID < out[j].MessageID
		}
		return out[i].UsageIdentifier < out[j].UsageIdentifier
	})
	return out
}

const (
	cbprReference = "https://www.swift.com/products/mystandards"
	cbpr2025Ref   = "https://www.swift.com/standards/iso-20022/iso-20022-financial-institutions-focus-payments-instructions"
	cbprHeaderNS  = "urn:iso:std:iso:20022:tech:xsd:head.001.001.02"
)

func isCBPRMessage(msgID string) bool {
	_, ok := cbprSR2025[msgID]
	return ok
}

func exemptOutsideCBPR(msgID string) bool { return !isCBPRMessage(msgID) }

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func firstLocated(root *converter.Node, name string) (Located, bool) {
	found := FindAll(root, name)
	if len(found) == 0 {
		return Located{}, false
	}
	return found[0], true
}

func attrValue(n *converter.Node, local string) string {
	for _, attr := range n.Attrs {
		if attr.Name.Local == local {
			return strings.TrimSpace(attr.Value)
		}
	}
	return ""
}

func descendant(n *converter.Node, names ...string) (*converter.Node, bool) {
	cur := n
	for _, name := range names {
		next, ok := Child(cur, name)
		if !ok {
			return nil, false
		}
		cur = next
	}
	return cur, true
}

func absoluteSubtreePath(root Located, relative string) string {
	prefix := "/" + root.Node.Name
	return root.Path + strings.TrimPrefix(relative, prefix)
}

// CBPRMessageDefinition restricts the profile to message definitions that are
// actually in the live CBPR+ collection. Silently applying generic checks to a
// pacs.008 version used by another market practice is a false positive pass.
var CBPRMessageDefinition = Rule{
	ID:          "CBPR-SCOPE-001",
	Name:        "CBPR+ message definition",
	Severity:    SeverityError,
	Description: "A CBPR+ message must use a message definition published in the live CBPR+ Standards Release collection.",
	Remediation: "Use the message definition and Usage Identifier from the live CBPR+ collection in MyStandards.",
	Reference:   cbprReference,
	Check: func(ctx *Context) []Finding {
		if isCBPRMessage(ctx.MsgID) {
			return nil
		}
		found := ctx.MsgID
		if found == "" {
			found = "(absent)"
		}
		return []Finding{{
			Path: "/" + ctx.Root.Name, Message: fmt.Sprintf("%s is not a live SR2025 CBPR+ message definition", found),
			Found: found, Expected: "a message definition from the CBPR+ SR2025 collection",
		}}
	},
}

// CBPRBusinessHeader requires the Business Application Header fields FINplus
// uses to route a message and select its Usage Guideline.
var CBPRBusinessHeader = Rule{
	ID:          "CBPR-BAH-001",
	Name:        "CBPR+ Business Application Header",
	Severity:    SeverityError,
	Description: "A complete CBPR+ transmission carries a head.001 Business Application Header with sender, receiver, identifiers, service and creation time.",
	Remediation: "Add <AppHdr> with Fr, To, BizMsgIdr, MsgDefIdr, BizSvc and CreDt before <Document>.",
	Reference:   cbprReference,
	Exempt:      exemptOutsideCBPR,
	Check: func(ctx *Context) []Finding {
		header, ok := firstLocated(ctx.Root, "AppHdr")
		if !ok {
			return []Finding{{Path: "/" + ctx.Root.Name + "/AppHdr", Message: "the CBPR+ Business Application Header is absent", Found: "(absent)", Expected: "a head.001 AppHdr"}}
		}
		var out []Finding
		for _, name := range []string{"Fr", "To", "BizMsgIdr", "MsgDefIdr", "BizSvc", "CreDt"} {
			if child, present := Child(header.Node, name); present && (len(child.Children) > 0 || strings.TrimSpace(child.Text) != "") {
				continue
			}
			out = append(out, Finding{Path: header.Path + "/" + name, Message: fmt.Sprintf("the Business Application Header has no %s", name), Found: "(absent)", Expected: "one <" + name + "> element"})
		}
		return out
	},
}

var CBPRMessageDefinitionMatch = Rule{
	ID:          "CBPR-BAH-002",
	Name:        "Header message definition matches Document",
	Severity:    SeverityError,
	Description: "AppHdr/MsgDefIdr selects the Usage Guideline and must identify the enclosed ISO 20022 Document.",
	Remediation: "Set <MsgDefIdr> to the message identifier in the Document namespace.",
	Reference:   cbprReference,
	Exempt:      exemptOutsideCBPR,
	Check: func(ctx *Context) []Finding {
		header, ok := firstLocated(ctx.Root, "AppHdr")
		if !ok {
			return nil
		}
		got := ChildText(header.Node, "MsgDefIdr")
		if got == "" || got == ctx.MsgID {
			return nil
		}
		return []Finding{{Path: header.Path + "/MsgDefIdr", Message: fmt.Sprintf("header identifies %s but the Document is %s", got, ctx.MsgID), Found: got, Expected: ctx.MsgID}}
	},
}

var CBPRNamespaces = Rule{
	ID:          "CBPR-NS-001",
	Name:        "CBPR+ header and document namespaces",
	Severity:    SeverityError,
	Description: "The Business Application Header uses head.001.001.02 and Document uses the namespace named by MsgDefIdr.",
	Remediation: "Put AppHdr in urn:iso:std:iso:20022:tech:xsd:head.001.001.02 and Document in the selected message-definition namespace.",
	Reference:   cbprReference,
	Exempt:      exemptOutsideCBPR,
	Check: func(ctx *Context) []Finding {
		var out []Finding
		if header, ok := firstLocated(ctx.Root, "AppHdr"); ok {
			if header.Node.Space != cbprHeaderNS {
				found := header.Node.Space
				if found == "" {
					found = "(none)"
				}
				out = append(out, Finding{Path: header.Path, Message: "AppHdr is not in the CBPR+ Business Application Header namespace", Found: found, Expected: cbprHeaderNS})
			}
			for _, item := range Walk(header.Node) {
				// XML Signature owns the contents of Sgntr and legitimately uses
				// another namespace.
				if strings.Contains(item.Path, "/Sgntr/") || item.Node == header.Node {
					continue
				}
				if item.Node.Space != cbprHeaderNS {
					found := item.Node.Space
					if found == "" {
						found = "(none)"
					}
					out = append(out, Finding{Path: absoluteSubtreePath(header, item.Path), Message: "element in AppHdr uses a foreign namespace", Found: found, Expected: cbprHeaderNS})
				}
			}
		}
		if document, ok := firstLocated(ctx.Root, "Document"); ok {
			expected := "urn:iso:std:iso:20022:tech:xsd:" + ctx.MsgID
			if document.Node.Space != expected {
				found := document.Node.Space
				if found == "" {
					found = "(none)"
				}
				out = append(out, Finding{Path: document.Path, Message: "Document namespace does not match the CBPR+ message definition", Found: found, Expected: expected})
			}
			for _, item := range Walk(document.Node) {
				// SupplementaryData/Envelope is the standard extension point.
				if strings.Contains(item.Path, "/SplmtryData/Envlp/") || item.Node == document.Node {
					continue
				}
				if item.Node.Space != expected {
					found := item.Node.Space
					if found == "" {
						found = "(none)"
					}
					out = append(out, Finding{Path: absoluteSubtreePath(document, item.Path), Message: "business element uses a foreign namespace outside SupplementaryData/Envlp", Found: found, Expected: expected})
				}
			}
		}
		return out
	},
}

var CBPRBusinessService = Rule{
	ID:          "CBPR-BAH-003",
	Name:        "CBPR+ Usage Identifier",
	Severity:    SeverityError,
	Description: "AppHdr/BizSvc selects the CBPR+ Usage Guideline, including STP, advice, cover and collection variants.",
	Remediation: "Use a Business Service published for this message definition in the live CBPR+ collection.",
	Reference:   cbprReference,
	Exempt:      exemptOutsideCBPR,
	Check: func(ctx *Context) []Finding {
		header, ok := firstLocated(ctx.Root, "AppHdr")
		if !ok {
			return nil
		}
		got := ChildText(header.Node, "BizSvc")
		if got == "" || contains(cbprSR2025[ctx.MsgID], got) {
			return nil
		}
		expected := strings.Join(cbprSR2025[ctx.MsgID], ", ")
		return []Finding{{Path: header.Path + "/BizSvc", Message: fmt.Sprintf("%q is not a CBPR+ Usage Identifier for %s", got, ctx.MsgID), Found: got, Expected: expected}}
	},
}

var CBPRBusinessMessageID = Rule{
	ID:          "CBPR-BAH-004",
	Name:        "Business message identifier consistency",
	Severity:    SeverityError,
	Description: "The Business Message Identifier must transport the Message Identification from the business message Group Header.",
	Remediation: "Set AppHdr/BizMsgIdr and GrpHdr/MsgId to the same value.",
	Reference:   cbprReference,
	Exempt: func(msgID string) bool {
		if !isCBPRMessage(msgID) {
			return true
		}
		switch baseCode(msgID) {
		case "camt.029", "camt.055", "camt.056":
			return true
		}
		return false
	},
	Check: func(ctx *Context) []Finding {
		header, hOK := firstLocated(ctx.Root, "AppHdr")
		group, gOK := firstLocated(ctx.Root, "GrpHdr")
		if !hOK || !gOK {
			return nil
		}
		businessID := ChildText(header.Node, "BizMsgIdr")
		messageID := ChildText(group.Node, "MsgId")
		if businessID == "" || messageID == "" || businessID == messageID {
			return nil
		}
		return []Finding{{Path: header.Path + "/BizMsgIdr", Message: "the Business Message Identifier does not match GrpHdr/MsgId", Found: businessID, Expected: messageID}}
	},
}

var CBPRHeaderParties = Rule{
	ID:          "CBPR-BAH-006",
	Name:        "CBPR+ sender and receiver BIC",
	Severity:    SeverityError,
	Description: "The CBPR+ Business Application Header identifies both From and To financial institutions with BICFI.",
	Remediation: "Populate Fr/FIId/FinInstnId/BICFI and To/FIId/FinInstnId/BICFI in AppHdr.",
	Reference:   cbprReference,
	Exempt:      exemptOutsideCBPR,
	Check: func(ctx *Context) []Finding {
		header, ok := firstLocated(ctx.Root, "AppHdr")
		if !ok {
			return nil
		}
		var out []Finding
		for _, side := range []string{"Fr", "To"} {
			node, present := descendant(header.Node, side, "FIId", "FinInstnId", "BICFI")
			if present && strings.TrimSpace(node.Text) != "" {
				continue
			}
			out = append(out, Finding{Path: header.Path + "/" + side + "/FIId/FinInstnId/BICFI", Message: fmt.Sprintf("AppHdr/%s does not identify a financial institution by BICFI", side), Found: "(absent)", Expected: "one <BICFI>"})
		}
		return out
	},
}

// CBPRPartyNameWithAddress is a repeated formal rule across the CBPR+ Usage
// Guidelines. It applies to party and financial-institution components alike.
var CBPRPartyNameWithAddress = Rule{
	ID:          "CBPR-PTY-001",
	Name:        "Name required with postal address",
	Severity:    SeverityError,
	Description: "If a party or agent carries PostalAddress, it must also carry Name.",
	Remediation: "Populate <Nm> in the party or financial institution that contains <PstlAdr>.",
	Reference:   cbprReference,
	Exempt:      exemptOutsideCBPR,
	Check: func(ctx *Context) []Finding {
		var out []Finding
		for _, item := range Walk(ctx.Root) {
			if _, hasAddress := Child(item.Node, "PstlAdr"); !hasAddress {
				continue
			}
			if ChildText(item.Node, "Nm") != "" {
				continue
			}
			out = append(out, Finding{Path: item.Path + "/Nm", Message: "postal address is present but the associated name is absent", Found: "(absent)", Expected: "a <Nm> element"})
		}
		return out
	},
}

var cbprPartyNames = map[string]bool{
	"Cdtr": true, "Dbtr": true, "InitgPty": true, "InstgPty": true,
	"UltmtCdtr": true, "UltmtDbtr": true, "Orgtr": true, "Cretr": true,
	"Pty": true,
}

// CBPRPartyNameWithoutAnyBIC implements the other common party-name formal
// rule: an organisation may use AnyBIC as its primary identity; without it the
// human-readable Name cannot be omitted.
var CBPRPartyNameWithoutAnyBIC = Rule{
	ID:          "CBPR-PTY-002",
	Name:        "Party name required without AnyBIC",
	Severity:    SeverityError,
	Description: "When a CBPR+ party is not identified by OrganisationIdentification/AnyBIC, Name is mandatory.",
	Remediation: "Populate the party <Nm>, or supply its valid <AnyBIC> where that identification option applies.",
	Reference:   cbprReference,
	Exempt:      exemptOutsideCBPR,
	Check: func(ctx *Context) []Finding {
		var out []Finding
		for _, item := range Walk(ctx.Root) {
			if !cbprPartyNames[item.Node.Name] {
				continue
			}
			// Wrapper choices such as <Dbtr><Pty>...</Pty></Dbtr> are not
			// party components themselves. Only inspect a node carrying direct
			// party data.
			_, hasID := Child(item.Node, "Id")
			_, hasAddress := Child(item.Node, "PstlAdr")
			_, hasName := Child(item.Node, "Nm")
			if !hasID && !hasAddress && !hasName {
				continue
			}
			if ChildText(item.Node, "Nm") != "" {
				continue
			}
			if anyBIC, ok := descendant(item.Node, "Id", "OrgId", "AnyBIC"); ok && strings.TrimSpace(anyBIC.Text) != "" {
				continue
			}
			out = append(out, Finding{Path: item.Path + "/Nm", Message: "party has no AnyBIC, so its name is mandatory", Found: "(absent)", Expected: "a <Nm> element"})
		}
		return out
	},
}

// CBPRCurrentAddress implements the live SR2025 address shapes. Fully
// unstructured remains accepted in SR2025; when structured data is used, Town
// Name and Country are mandatory, and a hybrid has the two-line limit.
var CBPRCurrentAddress = Rule{
	ID:          "CBPR-ADDR-006",
	Name:        "Live CBPR+ postal address shape",
	Severity:    SeverityError,
	Description: "SR2025 accepts structured, hybrid and fully unstructured addresses. Structured and hybrid forms require TownName and Country; hybrid permits two 70-character AddressLine values.",
	Remediation: "For structured or hybrid addresses populate <TwnNm> and <Ctry>; keep a hybrid to two <AdrLine> values of at most 70 characters.",
	Reference:   cbpr2025Ref,
	Exempt:      exemptOutsideCBPR,
	Check: func(ctx *Context) []Finding {
		var out []Finding
		for _, addr := range FindAll(ctx.Root, "PstlAdr") {
			shape := Classify(addr.Node)
			if shape == ShapeEmpty || shape == ShapeUnstructured {
				continue
			}
			if ChildText(addr.Node, "TwnNm") == "" {
				out = append(out, Finding{Path: addr.Path + "/TwnNm", Message: "a structured or hybrid address has no town name", Found: "(absent)", Expected: "a <TwnNm> element"})
			}
			if ChildText(addr.Node, "Ctry") == "" {
				out = append(out, Finding{Path: addr.Path + "/Ctry", Message: "a structured or hybrid address has no country", Found: "(absent)", Expected: "a <Ctry> element"})
			}
			if shape != ShapeHybrid {
				continue
			}
			lines := Children(addr.Node, "AdrLine")
			if len(lines) > 2 {
				out = append(out, Finding{Path: addr.Path + "/AdrLine", Message: fmt.Sprintf("hybrid address has %d address lines; CBPR+ permits two", len(lines)), Found: strconv.Itoa(len(lines)), Expected: "at most 2"})
			}
			for i, line := range lines {
				if n := len([]rune(strings.TrimSpace(line.Text))); n > 70 {
					out = append(out, Finding{Path: fmt.Sprintf("%s/AdrLine[%d]", addr.Path, i+1), Message: fmt.Sprintf("hybrid address line is %d characters; CBPR+ permits 70", n), Found: fmt.Sprintf("%d characters", n), Expected: "at most 70 characters"})
				}
			}
		}
		return out
	},
}

// CBPRCountryCode applies the assigned-code check to every live CBPR+ message.
// CountryCodeFormat's exemption is intentionally tied to the future structured
// address cutover; reporting messages are exempt from that cutover, not from
// carrying a real ISO 3166 country whenever Ctry is present.
var CBPRCountryCode = func() Rule {
	rule := CountryCodeFormat
	rule.ID = "CBPR-CTRY-001"
	rule.Exempt = exemptOutsideCBPR
	return rule
}()

var CBPRCommodityCurrency = Rule{
	ID:          "CBPR-CCY-001",
	Name:        "Payment currency excludes precious metals",
	Severity:    SeverityError,
	Description: "CBPR+ amount currency does not permit the ISO 4217 precious-metal codes XAU, XAG, XPD or XPT.",
	Remediation: "Use the monetary settlement currency; precious-metal commodity codes are not payment currencies in CBPR+.",
	Reference:   cbprReference,
	Exempt:      exemptOutsideCBPR,
	Check: func(ctx *Context) []Finding {
		var out []Finding
		for _, item := range Walk(ctx.Root) {
			ccy := strings.ToUpper(attrValue(item.Node, "Ccy"))
			switch ccy {
			case "XAU", "XAG", "XPD", "XPT":
				out = append(out, Finding{Path: item.Path + "/@Ccy", Message: fmt.Sprintf("%s is a precious-metal code and is not allowed as a CBPR+ payment currency", ccy), Found: ccy, Expected: "an ISO 4217 monetary currency"})
			}
		}
		return out
	},
}

var transactionElements = map[string]string{
	"pacs.002": "TxInfAndSts", "pacs.003": "DrctDbtTxInf", "pacs.004": "TxInf",
	"pacs.008": "CdtTrfTxInf", "pacs.009": "CdtTrfTxInf", "pacs.010": "DrctDbtTxInf",
	"pain.001": "CdtTrfTxInf", "pain.002": "TxInfAndSts", "pain.008": "DrctDbtTxInf",
}

var CBPRTransactionCount = Rule{
	ID:          "CBPR-GRP-001",
	Name:        "Number of transactions consistency",
	Severity:    SeverityError,
	Description: "GrpHdr/NbOfTxs must equal the number of transaction blocks in the message.",
	Remediation: "Recalculate <NbOfTxs> from the transaction blocks in this message.",
	Reference:   cbprReference,
	Exempt:      exemptOutsideCBPR,
	Check: func(ctx *Context) []Finding {
		name := transactionElements[baseCode(ctx.MsgID)]
		if name == "" {
			return nil
		}
		group, ok := firstLocated(ctx.Root, "GrpHdr")
		if !ok {
			return nil
		}
		value := ChildText(group.Node, "NbOfTxs")
		if value == "" {
			return nil
		}
		declared, err := strconv.Atoi(value)
		if err != nil {
			return nil // the Usage Guideline schema reports lexical errors
		}
		actual := len(FindAll(ctx.Root, name))
		if declared == actual {
			return nil
		}
		return []Finding{{Path: group.Path + "/NbOfTxs", Message: fmt.Sprintf("GrpHdr declares %d transaction(s), but the message contains %d", declared, actual), Found: strconv.Itoa(declared), Expected: strconv.Itoa(actual)}}
	},
}

var controlSumAmounts = map[string]string{
	"pacs.003": "IntrBkSttlmAmt", "pacs.008": "IntrBkSttlmAmt", "pacs.009": "IntrBkSttlmAmt",
	"pain.001": "InstdAmt", "pain.008": "InstdAmt",
}

var CBPRControlSum = Rule{
	ID:          "CBPR-GRP-002",
	Name:        "Control sum consistency",
	Severity:    SeverityError,
	Description: "When GrpHdr/CtrlSum is present, it must equal the arithmetic sum of the message transaction amounts.",
	Remediation: "Recalculate <CtrlSum> from the transaction amount elements without rounding.",
	Reference:   cbprReference,
	Exempt:      exemptOutsideCBPR,
	Check: func(ctx *Context) []Finding {
		amountName := controlSumAmounts[baseCode(ctx.MsgID)]
		if amountName == "" {
			return nil
		}
		group, ok := firstLocated(ctx.Root, "GrpHdr")
		if !ok {
			return nil
		}
		declaredText := ChildText(group.Node, "CtrlSum")
		if declaredText == "" {
			return nil
		}
		declared, ok := new(big.Rat).SetString(declaredText)
		if !ok {
			return nil
		}
		total := new(big.Rat)
		for _, amount := range FindAll(ctx.Root, amountName) {
			value, valid := new(big.Rat).SetString(strings.TrimSpace(amount.Node.Text))
			if !valid {
				return nil
			}
			total.Add(total, value)
		}
		if declared.Cmp(total) == 0 {
			return nil
		}
		return []Finding{{Path: group.Path + "/CtrlSum", Message: fmt.Sprintf("control sum %s does not equal transaction total %s", declaredText, total.FloatString(10)), Found: declaredText, Expected: strings.TrimRight(strings.TrimRight(total.FloatString(10), "0"), ".")}}
	},
}

var CBPRUETRRequired = Rule{
	ID:          "CBPR-PMT-001",
	Name:        "UETR required on credit transfer",
	Severity:    SeverityError,
	Description: "Each CBPR+ pacs.008 and pacs.009 transaction must carry its Unique End-to-end Transaction Reference.",
	Remediation: "Populate PmtId/UETR with the transaction's RFC 4122 version 4 UUID and preserve it through the payment chain.",
	Reference:   cbprReference,
	Exempt: func(msgID string) bool {
		return msgID != "pacs.008.001.08" && msgID != "pacs.009.001.08"
	},
	Check: func(ctx *Context) []Finding {
		var out []Finding
		for _, tx := range FindAll(ctx.Root, "CdtTrfTxInf") {
			pmtID, ok := Child(tx.Node, "PmtId")
			if !ok || ChildText(pmtID, "UETR") == "" {
				out = append(out, Finding{Path: tx.Path + "/PmtId/UETR", Message: "the credit transfer has no UETR", Found: "(absent)", Expected: "one <UETR>"})
			}
		}
		return out
	},
}

var CBPRPacs009Variant = Rule{
	ID:          "CBPR-VAR-001",
	Name:        "pacs.009 Usage Guideline variant",
	Severity:    SeverityError,
	Description: "The pacs.009 cover Usage Identifier requires the underlying customer credit transfer; core and advice variants must not carry it.",
	Remediation: "Use swift.cbprplus.cov.03 with <UndrlygCstmrCdtTrf>, or remove that block and select the core/advice Usage Identifier.",
	Reference:   cbprReference,
	Exempt:      func(msgID string) bool { return msgID != "pacs.009.001.08" },
	Check: func(ctx *Context) []Finding {
		header, ok := firstLocated(ctx.Root, "AppHdr")
		if !ok {
			return nil
		}
		service := ChildText(header.Node, "BizSvc")
		underlying := FindAll(ctx.Root, "UndrlygCstmrCdtTrf")
		if service == "swift.cbprplus.cov.03" && len(underlying) == 0 {
			return []Finding{{Path: "/" + ctx.Root.Name + "/Document/FICdtTrf/CdtTrfTxInf/UndrlygCstmrCdtTrf", Message: "pacs.009 COV has no underlying customer credit transfer", Found: "(absent)", Expected: "one <UndrlygCstmrCdtTrf>"}}
		}
		if (service == "swift.cbprplus.03" || service == "swift.cbprplus.adv.03") && len(underlying) > 0 {
			return []Finding{{Path: underlying[0].Path, Message: "underlying customer credit transfer is only valid in the pacs.009 COV Usage Guideline", Found: service, Expected: "swift.cbprplus.cov.03"}}
		}
		return nil
	},
}

// CBPRCreationTime catches a common gap when messages are assembled without
// validating the head.001 schema. This is kept here because profile checking is
// intentionally usable without an installed catalogue.
var CBPRCreationTime = Rule{
	ID:          "CBPR-BAH-005",
	Name:        "Business header creation time",
	Severity:    SeverityError,
	Description: "AppHdr/CreDt is an ISO 8601 date-time with an explicit timezone.",
	Remediation: "Write <CreDt> as an ISO 8601 timestamp including Z or an offset, for example 2025-11-24T07:41:50Z.",
	Reference:   cbprReference,
	Exempt:      exemptOutsideCBPR,
	Check: func(ctx *Context) []Finding {
		header, ok := firstLocated(ctx.Root, "AppHdr")
		if !ok {
			return nil
		}
		value := ChildText(header.Node, "CreDt")
		if value == "" {
			return nil
		}
		// RFC3339Nano accepts the fractional seconds used by FINplus and, unlike
		// a timezone-free layout, requires an explicit offset.
		if _, err := time.Parse(time.RFC3339Nano, value); err == nil {
			return nil
		}
		return []Finding{{Path: header.Path + "/CreDt", Message: fmt.Sprintf("%q is not an ISO 8601 date-time with a timezone", value), Found: value, Expected: "RFC 3339 date-time with Z or UTC offset"}}
	},
}

// CBPRRules is the complete embedded cross-message SR2025 rule layer. Exact
// per-message cardinalities and patterns belong to the Usage Guideline XSD and
// are validated by the CBPR+ bundle path, not duplicated here.
var CBPRRules = []Rule{
	CBPRMessageDefinition,
	CBPRBusinessHeader,
	CBPRMessageDefinitionMatch,
	CBPRNamespaces,
	CBPRBusinessService,
	CBPRBusinessMessageID,
	CBPRCreationTime,
	CBPRHeaderParties,
	CBPRPartyNameWithAddress,
	CBPRPartyNameWithoutAnyBIC,
	CBPRCurrentAddress,
	CBPRCountryCode,
	CBPRCommodityCurrency,
	CBPRTransactionCount,
	CBPRControlSum,
	CBPRUETRRequired,
	CBPRPacs009Variant,
}
