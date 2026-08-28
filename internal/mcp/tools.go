// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/sebastienrousseau/askiso/pkg/iso20022"
)

// Tool is one callable the server exposes.
type Tool struct {
	Name        string
	Title       string
	Description string
	// Schema is the JSON Schema for the tool's arguments.
	Schema map[string]any
	// ReadsCatalogue marks a tool that needs the user's downloaded schemas. The
	// description says so, so an agent knows why a call may report that nothing
	// is installed.
	ReadsCatalogue bool
	Handler        func(ctx context.Context, args map[string]any) (any, error)
}

func (s *Server) toolDescriptors() []map[string]any {
	out := make([]map[string]any, 0, len(s.tools))
	for _, t := range s.tools {
		desc := t.Description
		if t.ReadsCatalogue {
			desc += "\n\nNeeds the ISO 20022 schemas the user downloaded from iso20022.org. " +
				"If none are installed the tool says so and names the message set to download."
		}
		out = append(out, map[string]any{
			"name":        t.Name,
			"title":       t.Title,
			"description": desc,
			"inputSchema": t.Schema,
		})
	}
	return out
}

func (s *Server) callTool(ctx context.Context, params json.RawMessage) (any, *rpcError) {
	var p struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &rpcError{Code: codeInvalidParams, Message: "the parameters are not an object", Data: err.Error()}
	}

	tool, ok := s.byName[p.Name]
	if !ok {
		return nil, &rpcError{Code: codeInvalidParams, Message: "no such tool: " + p.Name}
	}
	if p.Arguments == nil {
		p.Arguments = map[string]any{}
	}

	result, err := tool.Handler(ctx, p.Arguments)
	if err != nil {
		// A tool that fails reports the failure in its result rather than as a
		// protocol error, so the model can read it and try something else.
		return toolResult(err.Error(), nil, true), nil
	}
	return toolResult("", result, false), nil
}

// toolResult renders a tool's outcome. Structured content is returned as JSON
// text as well as in structuredContent, because clients differ in what they
// read and a model handles either.
func toolResult(text string, data any, isError bool) map[string]any {
	out := map[string]any{"isError": isError}

	if data != nil {
		encoded, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			return map[string]any{
				"isError": true,
				"content": []map[string]any{{"type": "text", "text": "could not encode the result: " + err.Error()}},
			}
		}
		out["content"] = []map[string]any{{"type": "text", "text": string(encoded)}}
		out["structuredContent"] = data
		return out
	}

	out["content"] = []map[string]any{{"type": "text", "text": text}}
	return out
}

// ---------------------------------------------------------------------------
// Argument helpers
// ---------------------------------------------------------------------------

func stringArg(args map[string]any, name string) string {
	if v, ok := args[name].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func boolArg(args map[string]any, name string) bool {
	v, _ := args[name].(bool)
	return v
}

// intArg reads a numeric argument. JSON has one number type, so a float is the
// only shape a decoded integer arrives in.
func intArg(args map[string]any, name string, fallback int) int {
	if v, ok := args[name].(float64); ok {
		return int(v)
	}
	return fallback
}

// required reads a string argument, failing with a message that names what was
// missing rather than returning an empty result.
func required(args map[string]any, name string) (string, error) {
	v := stringArg(args, name)
	if v == "" {
		return "", fmt.Errorf("%q is required", name)
	}
	return v, nil
}

func object(props map[string]any, requiredNames ...string) map[string]any {
	schema := map[string]any{
		"type":       "object",
		"properties": props,
	}
	if len(requiredNames) > 0 {
		schema["required"] = requiredNames
	}
	return schema
}

func prop(kind, description string) map[string]any {
	return map[string]any{"type": kind, "description": description}
}

// ---------------------------------------------------------------------------
// The catalogue, opened once
// ---------------------------------------------------------------------------

// CatalogueFunc opens the user's installed schemas. It is a field on the server
// so a test can supply one that is deliberately absent, and so a host embedding
// the server can point it at a catalogue of its own.
type CatalogueFunc func() (*iso20022.Catalogue, error)

// openInstalledCatalogue reads the user's catalogue once and caches the result.
// A missing catalogue is not fatal: most tools do not need one, and the ones
// that do say so.
func openInstalledCatalogue() CatalogueFunc {
	var (
		once sync.Once
		cat  *iso20022.Catalogue
		err  error
	)
	return func() (*iso20022.Catalogue, error) {
		once.Do(func() { cat, err = iso20022.OpenCatalogue("") })
		return cat, err
	}
}

// requireCatalogue returns the catalogue or an error saying how to install one.
func requireCatalogue(open CatalogueFunc) (*iso20022.Catalogue, error) {
	cat, err := open()
	if err != nil {
		return nil, fmt.Errorf("this needs the ISO 20022 schemas, and none are installed.\n\n"+
			"Download the message set from https://www.iso20022.org/ then run:\n"+
			"  askiso catalog add <downloaded.zip>\n\n%w", err)
	}
	return cat, nil
}

// ---------------------------------------------------------------------------
// Tools
// ---------------------------------------------------------------------------

func defaultTools(open CatalogueFunc) []Tool {
	return []Tool{
		searchTool(open),
		infoTool(open),
		lintTool(),
		checkProfileTool(),
		validateTool(open),
		generateTool(open),
		translateTool(),
		codeTool(open),
		diffTool(open),
		convertTool(),
	}
}

func searchTool(open CatalogueFunc) Tool {
	return Tool{
		Name:  "askiso_search",
		Title: "Search ISO 20022 messages",
		Description: "Search all 2,845 published ISO 20022 message definitions by identifier, " +
			"domain, or keyword. Returns the message identifier, its name, the message set " +
			"that publishes it, and whether the user has the schema installed. " +
			"Use this before answering any question about which message does what.",
		Schema: object(map[string]any{
			"query": prop("string", `What to search for: an identifier such as "pacs.008", a domain such as "camt", or a keyword such as "direct debit".`),
			"limit": prop("integer", "Maximum results to return. Defaults to 20."),
		}, "query"),
		Handler: func(_ context.Context, args map[string]any) (any, error) {
			query, err := required(args, "query")
			if err != nil {
				return nil, err
			}
			cat, _ := open()
			results, err := cat.Search(query)
			if err != nil {
				return nil, err
			}

			limit := intArg(args, "limit", 20)
			if limit > 0 && len(results) > limit {
				results = results[:limit]
			}
			return map[string]any{"query": query, "count": len(results), "messages": results}, nil
		},
	}
}

func infoTool(open CatalogueFunc) Tool {
	return Tool{
		Name:  "askiso_info",
		Title: "Look up one message definition",
		Description: "Return everything AskISO knows about one message identifier: its name, " +
			"the message sets that publish it, the download location at the Registration " +
			"Authority, and whether the schema is installed locally.",
		Schema: object(map[string]any{
			"message_id": prop("string", `A message identifier, for example "pacs.008.001.10" or "camt.053".`),
		}, "message_id"),
		Handler: func(_ context.Context, args map[string]any) (any, error) {
			id, err := required(args, "message_id")
			if err != nil {
				return nil, err
			}
			cat, _ := open()
			return cat.Lookup(id)
		},
	}
}

func lintTool() Tool {
	return Tool{
		Name:  "askiso_lint",
		Title: "Check a message's business rules",
		Description: "Run AskISO's semantic linter over an ISO 20022 XML message. It verifies " +
			"IBAN mod-97 checksums, BIC structure against ISO 9362, currency precision " +
			"against ISO 4217, UETR format against RFC 4122, and date sanity. " +
			"Call this before telling anyone a message is correct; it needs no schemas.",
		Schema: object(map[string]any{
			"xml":      prop("string", "The full ISO 20022 XML message."),
			"filename": prop("string", "A name for the message, used in the report. Optional."),
		}, "xml"),
		Handler: func(_ context.Context, args map[string]any) (any, error) {
			doc, err := required(args, "xml")
			if err != nil {
				return nil, err
			}
			name := stringArg(args, "filename")
			if name == "" {
				name = "message.xml"
			}
			return iso20022.Lint([]byte(doc), name)
		},
	}
}

func checkProfileTool() Tool {
	return Tool{
		Name:  "askiso_check_profile",
		Title: "Check a message against a scheme rule profile",
		Description: "Apply a scheme rule profile to a message. The cbpr-2026 profile checks " +
			"the CBPR+ structured-address requirement: an address must carry a town name " +
			"and a country, and fully unstructured addresses are rejected. Swift deferred " +
			"the 14 November 2026 cutover on 27 August 2026 and will confirm replacement " +
			"timing by December, so do not quote a date; the requirement itself stands. " +
			"Use this whenever the question involves structured addresses or cross-border " +
			"payment readiness.",
		Schema: object(map[string]any{
			"xml":     prop("string", "The full ISO 20022 XML message."),
			"profile": prop("string", "Profile name. Call with an empty profile to list the available ones."),
		}, "xml"),
		Handler: func(_ context.Context, args map[string]any) (any, error) {
			doc, err := required(args, "xml")
			if err != nil {
				return nil, err
			}
			profile := stringArg(args, "profile")
			if profile == "" {
				return map[string]any{
					"profiles": iso20022.RuleProfiles(),
					"hint":     "pass one of these as the profile argument",
				}, nil
			}
			return iso20022.CheckProfile([]byte(doc), profile, "message.xml")
		},
	}
}

func validateTool(open CatalogueFunc) Tool {
	return Tool{
		Name:  "askiso_validate",
		Title: "Validate a message against its XSD",
		Description: "Validate an ISO 20022 message against the schema its namespace names. " +
			"This is full XSD validation -- element order, cardinality, patterns, lengths, " +
			"enumerations -- performed in Go, with no external tools.",
		ReadsCatalogue: true,
		Schema: object(map[string]any{
			"xml": prop("string", "The full ISO 20022 XML message."),
		}, "xml"),
		Handler: func(_ context.Context, args map[string]any) (any, error) {
			doc, err := required(args, "xml")
			if err != nil {
				return nil, err
			}
			cat, err := requireCatalogue(open)
			if err != nil {
				return nil, err
			}
			return cat.Validate([]byte(doc))
		},
	}
}

func generateTool(open CatalogueFunc) Tool {
	return Tool{
		Name:  "askiso_generate",
		Title: "Generate a sample message",
		Description: "Build a valid ISO 20022 message. pacs.008, pacs.009, pain.001 and " +
			"camt.053 come from templates with rail-specific defaults and need nothing " +
			"installed. Any other message identifier is built from its schema, which " +
			"covers all 2,845 published messages and needs the user's catalogue. " +
			"Use this to show someone what a message looks like rather than writing one " +
			"from memory -- generated messages validate against their schema and lint clean.",
		Schema: object(map[string]any{
			"message_type": prop("string", `A message type or identifier: "pacs.008" for a template, or any identifier such as "seev.031.001.09" to build from the schema.`),
			"preset":       prop("string", `Rail preset for the template types: "sepa", "target2", "chaps", "fednow", or "standard".`),
			"amount":       prop("string", `The settlement amount, for example "25000.00".`),
			"currency":     prop("string", `An ISO 4217 code, for example "EUR".`),
			"with_bah":     prop("boolean", "Wrap the message in a business application header."),
			"optional":     prop("boolean", "Schema-built messages only: include optional elements as well as mandatory ones."),
		}, "message_type"),
		Handler: func(_ context.Context, args map[string]any) (any, error) {
			msgType, err := required(args, "message_type")
			if err != nil {
				return nil, err
			}

			// A message with no template is built from its schema, which is how
			// the other 2,841 are reachable at all.
			if !iso20022.HasTemplate(msgType) {
				cat, err := requireCatalogue(open)
				if err != nil {
					return nil, err
				}
				genOpts := iso20022.DefaultSchemaGenOptions()
				genOpts.Optional = boolArg(args, "optional")
				genOpts.Values = map[string]string{}
				if v := stringArg(args, "amount"); v != "" {
					genOpts.Values["InstdAmt"] = v
					genOpts.Values["IntrBkSttlmAmt"] = v
				}
				if v := stringArg(args, "currency"); v != "" {
					genOpts.Values["Ccy"] = v
				}

				res, err := cat.GenerateFromSchema(msgType, genOpts)
				if err != nil {
					return nil, err
				}
				return map[string]any{
					"message_type": msgType,
					"source":       "schema",
					"xml":          res.XML,
					"elements":     res.Elements,
					"notes":        res.Notes,
				}, nil
			}

			opts := iso20022.DefaultGeneratorOptions(msgType)
			if v := stringArg(args, "preset"); v != "" {
				opts.Preset = v
			}
			if v := stringArg(args, "amount"); v != "" {
				opts.Amount = v
			}
			if v := stringArg(args, "currency"); v != "" {
				opts.Currency = v
			}
			opts.WithBAH = boolArg(args, "with_bah")

			doc, err := iso20022.Generate(opts)
			if err != nil {
				return nil, err
			}
			return map[string]any{
				"message_type": msgType,
				"source":       "template",
				"preset":       opts.Preset,
				"xml":          doc,
			}, nil
		},
	}
}

func translateTool() Tool {
	return Tool{
		Name:  "askiso_translate",
		Title: "Convert a SWIFT MT message, or look up the mapping",
		Description: "Convert a real message in either direction, or look up the mapping. " +
			"With mt_message: MT101/104/107 become pain.001/pain.008, MT103 becomes pacs.008, " +
			"MT202 becomes pacs.009, MT204 becomes pacs.010, MT940 becomes camt.053. With " +
			"mx_message: pacs.008 becomes MT103, pacs.009 becomes MT202, camt.053 becomes " +
			"MT940. Either way the result includes a fidelity report naming every source " +
			"field and whether it was carried, shortened, inferred, or dropped -- nothing is " +
			"dropped silently. With code: return the field-level cross-reference. " +
			"MT to MX produces unstructured addresses, which CBPR+ stops accepting once " +
			"the deferred structured address requirement takes effect; check the result " +
			"with askiso_check_profile. MX to MT loses " +
			"purpose codes, legal entity identifiers and structured addresses outright.",
		Schema: object(map[string]any{
			"mt_message": prop("string", "A complete SWIFT MT message, including its {1:} and {2:} headers."),
			"mx_message": prop("string", "A complete ISO 20022 XML message, to convert to MT."),
			"code":       prop("string", `An identifier to cross-reference, for example "MT103" or "pacs.008". Ignored when a message is given.`),
		}),
		Handler: func(_ context.Context, args map[string]any) (any, error) {
			if raw := stringArg(args, "mt_message"); raw != "" {
				conv, err := iso20022.TranslateMT([]byte(raw))
				if err != nil {
					return nil, err
				}
				return conversionResult(conv, "MT"+conv.SourceType), nil
			}

			if raw := stringArg(args, "mx_message"); raw != "" {
				conv, err := iso20022.TranslateMX([]byte(raw))
				if err != nil {
					return nil, err
				}
				return conversionResult(conv, conv.SourceType), nil
			}

			code := stringArg(args, "code")
			if code == "" {
				return map[string]any{
					"convertible":    iso20022.TranslatableMT(),
					"convertible_mx": iso20022.TranslatableMX(),
					"mappings":       iso20022.AllMappings(),
				}, nil
			}
			m, ok := iso20022.TranslateSWIFT(code)
			if !ok {
				return nil, fmt.Errorf("no MT/MX mapping for %q", code)
			}
			return m, nil
		},
	}
}

func codeTool(open CatalogueFunc) Tool {
	return Tool{
		Name:  "askiso_code",
		Title: "Look up an ISO 20022 code",
		Description: "Look up a code such as AC04 or SALA: what it means, which set it belongs " +
			"to, and which messages use it. Codes come from AskISO's curated dictionary, from " +
			"the enumerations in the user's own schemas, and from the Registration Authority's " +
			"external code set publication where the user has imported one.",
		Schema: object(map[string]any{
			"query": prop("string", `A code, or text to search for, for example "AC04" or "insufficient funds".`),
		}, "query"),
		Handler: func(_ context.Context, args map[string]any) (any, error) {
			query, err := required(args, "query")
			if err != nil {
				return nil, err
			}

			results := iso20022.LookupCode(query)
			var external []iso20022.ExternalCode
			if cat, err := open(); err == nil {
				external = cat.LookupExternalCode(query)
			}

			if len(results) == 0 && len(external) == 0 {
				return nil, fmt.Errorf("no ISO 20022 code matches %q", query)
			}
			return map[string]any{
				"query":    query,
				"codes":    results,
				"external": external,
			}, nil
		},
	}
}

func diffTool(open CatalogueFunc) Tool {
	return Tool{
		Name:  "askiso_diff",
		Title: "Compare two schema versions",
		Description: "Compare two schema versions path by path and classify every difference " +
			"as breaking or compatible. A change is breaking when a message that satisfied " +
			"the old schema can be rejected by the new one. Use this for any question about " +
			"upgrading between versions.",
		ReadsCatalogue: true,
		Schema: object(map[string]any{
			"from":          prop("string", `The version a message was built against, for example "pacs.008.001.09".`),
			"to":            prop("string", `The version it must now satisfy, for example "pacs.008.001.10".`),
			"breaking_only": prop("boolean", "Return only the changes that can reject a message."),
		}, "from", "to"),
		Handler: func(_ context.Context, args map[string]any) (any, error) {
			from, err := required(args, "from")
			if err != nil {
				return nil, err
			}
			to, err := required(args, "to")
			if err != nil {
				return nil, err
			}
			cat, err := requireCatalogue(open)
			if err != nil {
				return nil, err
			}

			report, err := cat.Diff(from, to)
			if err != nil {
				return nil, err
			}
			breaking, compatible := report.Counts()

			out := map[string]any{
				"from": report.From, "to": report.To,
				"common_paths":       report.Common,
				"breaking_changes":   breaking,
				"compatible_changes": compatible,
				"structurally_equal": report.Identical(),
			}
			if boolArg(args, "breaking_only") {
				out["changes"] = report.Breaking()
				return out, nil
			}
			out["changes"] = report.Changes
			return out, nil
		},
	}
}

func convertTool() Tool {
	return Tool{
		Name:  "askiso_convert",
		Title: "Convert a message between XML and JSON",
		Description: "Convert an ISO 20022 message from XML to JSON or back. Element order is " +
			"preserved in both directions, which matters because ISO 20022 schemas are " +
			"ordered sequences and a reordered document is invalid.",
		Schema: object(map[string]any{
			"content": prop("string", "The message, as XML or as JSON."),
			"to":      prop("string", `Target format: "json" or "xml". Inferred from the content when omitted.`),
		}, "content"),
		Handler: func(_ context.Context, args map[string]any) (any, error) {
			content, err := required(args, "content")
			if err != nil {
				return nil, err
			}

			target := strings.ToLower(stringArg(args, "to"))
			if target == "" {
				target = "json"
				if strings.HasPrefix(content, "{") {
					target = "xml"
				}
			}

			switch target {
			case "json":
				out, err := iso20022.XMLToJSON([]byte(content))
				if err != nil {
					return nil, err
				}
				return map[string]any{"format": "json", "content": string(out)}, nil
			case "xml":
				out, err := iso20022.JSONToXML([]byte(content))
				if err != nil {
					return nil, err
				}
				return map[string]any{"format": "xml", "content": string(out)}, nil
			}
			return nil, fmt.Errorf("%q is not a target format; use \"json\" or \"xml\"", target)
		},
	}
}

// ToolNames lists the registered tools, sorted. Useful for tests and for the
// CLI's own description of what the server offers.
func (s *Server) ToolNames() []string {
	out := make([]string, 0, len(s.tools))
	for _, t := range s.tools {
		out = append(out, t.Name)
	}
	sort.Strings(out)
	return out
}

// conversionResult renders a conversion the same way whichever direction it
// went, so a model does not have to handle two shapes.
func conversionResult(conv *iso20022.Conversion, sourceLabel string) map[string]any {
	counts := conv.Counts()
	return map[string]any{
		"source_type": sourceLabel,
		"target_type": conv.TargetType,
		"xml":         conv.XML,
		"report":      conv.Report,
		"lossless":    conv.Lossless(),
		"summary": map[string]int{
			"mapped":    counts["mapped"],
			"derived":   counts["derived"],
			"truncated": counts["truncated"],
			"unmapped":  counts["unmapped"],
		},
	}
}
