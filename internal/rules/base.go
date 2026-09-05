// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package rules

// BaseRules are structural checks that do not require an installed schema.
var BaseRules = []Rule{
	{
		ID: "BASE-001", Name: "ISO 20022 namespace required", Severity: SeverityError,
		Description: "The document must identify an ISO 20022 message definition in its namespace.",
		Remediation: "Declare the message namespace on <Document>, for example urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10.",
		Check: func(ctx *Context) []Finding {
			if ctx.MsgID != "" {
				return nil
			}
			return []Finding{{Path: "/" + ctx.Root.Name, Message: "no ISO 20022 message namespace was found", Found: "(absent)", Expected: "an ISO 20022 message-definition namespace"}}
		},
	},
	{
		ID: "BASE-002", Name: "Business document content required", Severity: SeverityError,
		Description: "An ISO 20022 Document must contain a business message element.",
		Remediation: "Add the message's root business element beneath <Document>.",
		Check: func(ctx *Context) []Finding {
			doc := ctx.Root
			if doc.Name != "Document" {
				found := FindAll(ctx.Root, "Document")
				if len(found) == 0 {
					return []Finding{{Path: "/" + ctx.Root.Name, Message: "the XML has no <Document> element", Found: ctx.Root.Name, Expected: "Document"}}
				}
				doc = found[0].Node
			}
			if len(doc.Children) == 0 {
				return []Finding{{Path: "/Document", Message: "the ISO 20022 Document is empty", Found: "empty Document", Expected: "a business message element"}}
			}
			return nil
		},
	},
}
