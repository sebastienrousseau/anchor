// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

//go:build js && wasm

// Command anchor-wasm exposes Anchor's core to the browser.
//
// It is the same pkg/iso20022 the CLI uses, compiled to WebAssembly, so a
// message linted on the website gets the identical verdict to one linted in a
// terminal. Nothing is uploaded: every byte stays in the tab.
//
// Build:
//
//	GOOS=js GOARCH=wasm go build -o web/site/anchor.wasm ./web/wasm
package main

import (
	"encoding/json"
	"strings"
	"syscall/js"

	"github.com/sebastienrousseau/anchor/pkg/iso20022"
)

func main() {
	api := map[string]any{
		"version":    js.FuncOf(version),
		"search":     js.FuncOf(search),
		"info":       js.FuncOf(info),
		"lint":       js.FuncOf(lint),
		"validate":   js.FuncOf(validate),
		"profiles":   js.FuncOf(listProfiles),
		"checkRules": js.FuncOf(checkRules),
		"addresses":  js.FuncOf(addresses),
		"generate":   js.FuncOf(generate),
		"toJSON":     js.FuncOf(toJSON),
		"codes":      js.FuncOf(lookupCodes),
		"translate":  js.FuncOf(translate),
		"convertMT":  js.FuncOf(convertMT),
		"diagram":    js.FuncOf(diagram),
		"stats":      js.FuncOf(stats),
		"sets":       js.FuncOf(messageSets),
		"lifecycle":  js.FuncOf(lifecycle),
		"checkIBAN":  js.FuncOf(checkIBAN),
		"checkBIC":   js.FuncOf(checkBIC),
		"checkUETR":  js.FuncOf(checkUETR),
	}
	js.Global().Set("anchor", js.ValueOf(api))

	if ready := js.Global().Get("anchorReady"); ready.Type() == js.TypeFunction {
		ready.Invoke()
	}

	select {} // keep the module alive for the page's lifetime
}

// ok and fail wrap every result so the JavaScript side has one shape to handle.
func ok(v any) any {
	return marshal(map[string]any{"ok": true, "data": v})
}

func fail(msg string) any {
	return marshal(map[string]any{"ok": false, "error": msg})
}

// marshal round-trips through JSON so Go structs cross the boundary as plain
// JavaScript objects rather than opaque handles.
func marshal(v any) any {
	b, err := json.Marshal(v)
	if err != nil {
		return js.ValueOf(map[string]any{"ok": false, "error": err.Error()})
	}
	var generic any
	if err := json.Unmarshal(b, &generic); err != nil {
		return js.ValueOf(map[string]any{"ok": false, "error": err.Error()})
	}
	return js.ValueOf(generic)
}

func arg(args []js.Value, i int) string {
	if i >= len(args) || args[i].IsUndefined() || args[i].IsNull() {
		return ""
	}
	return args[i].String()
}

func version(this js.Value, args []js.Value) any {
	return ok(map[string]any{
		"mode": "light",
		"note": "Runs entirely in your browser. Nothing is uploaded.",
	})
}

// The browser build has no catalogue, so every call passes a nil *Catalogue and
// the API answers from the embedded registry.
var browserCatalogue *iso20022.Catalogue

func search(this js.Value, args []js.Value) any {
	q := arg(args, 0)
	if strings.TrimSpace(q) == "" {
		return fail("enter a message identifier, domain, or keyword")
	}
	results, err := browserCatalogue.Search(q)
	if err != nil {
		return fail(err.Error())
	}
	const maxResults = 200
	if len(results) > maxResults {
		results = results[:maxResults]
	}
	return ok(results)
}

func info(this js.Value, args []js.Value) any {
	m, err := browserCatalogue.Lookup(arg(args, 0))
	if err != nil {
		return fail(err.Error())
	}
	return ok(m)
}

func lint(this js.Value, args []js.Value) any {
	payload := arg(args, 0)
	if strings.TrimSpace(payload) == "" {
		return fail("paste an ISO 20022 XML message to lint")
	}
	name := arg(args, 1)
	if name == "" {
		name = "message.xml"
	}
	res, err := iso20022.Lint([]byte(payload), name)
	if err != nil {
		return fail(err.Error())
	}
	return ok(res)
}

// validate checks a pasted message against a pasted schema. Anchor ships no
// specification content, so the browser can only validate against an XSD the
// user supplies -- downloaded from iso20022.org and dropped in alongside.
func validate(this js.Value, args []js.Value) any {
	instance := arg(args, 0)
	schema := arg(args, 1)
	if strings.TrimSpace(instance) == "" {
		return fail("paste an ISO 20022 XML message to validate")
	}
	if strings.TrimSpace(schema) == "" {
		msgID, err := iso20022.MessageIDFromInstance([]byte(instance))
		if err != nil {
			return fail("paste the XSD schema to validate against")
		}
		hint := "paste the XSD for " + msgID + " to validate against"
		if sets, err := iso20022.Standard(); err == nil {
			if found := sets.SetsFor(msgID); len(found) > 0 {
				hint += " — download it from " + found[0].DownloadURL()
			}
		}
		return fail(hint)
	}

	res, err := iso20022.ValidateAgainst([]byte(instance), []byte(schema))
	if err != nil {
		return fail("could not read the schema: " + err.Error())
	}
	return ok(res)
}

// listProfiles reports the scheme rule profiles available.
func listProfiles(this js.Value, args []js.Value) any {
	type row struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	names := iso20022.RuleProfiles()
	out := make([]row, len(names))
	for i, n := range names {
		out[i] = row{Name: n, Description: iso20022.DescribeProfile(n)}
	}
	return ok(out)
}

// checkRules applies a scheme rule profile. This needs no schema, so the
// November 2026 address requirement can be checked in the browser.
func checkRules(this js.Value, args []js.Value) any {
	payload := arg(args, 0)
	if strings.TrimSpace(payload) == "" {
		return fail("paste an ISO 20022 XML message to check")
	}
	profile := arg(args, 1)
	if profile == "" {
		profile = "cbpr-2026"
	}
	res, err := iso20022.CheckProfile([]byte(payload), profile, "message.xml")
	if err != nil {
		return fail(err.Error())
	}
	return ok(res)
}

// addresses classifies every postal address in a message.
func addresses(this js.Value, args []js.Value) any {
	payload := arg(args, 0)
	if strings.TrimSpace(payload) == "" {
		return fail("paste an ISO 20022 XML message")
	}
	shapes, err := iso20022.ClassifyAddresses([]byte(payload))
	if err != nil {
		return fail(err.Error())
	}
	type row struct {
		Path  string `json:"path"`
		Shape string `json:"shape"`
	}
	out := make([]row, 0, len(shapes))
	for p, shape := range shapes {
		out = append(out, row{Path: p, Shape: shape})
	}
	sortRows(out, func(a, b row) bool { return a.Path < b.Path })
	return ok(out)
}

func generate(this js.Value, args []js.Value) any {
	msgType := arg(args, 0)
	if msgType == "" {
		msgType = "pacs.008"
	}
	opts := iso20022.DefaultGeneratorOptions(msgType)
	if p := arg(args, 1); p != "" {
		opts.Preset = p
	}
	if a := arg(args, 2); a != "" {
		opts.Amount = a
	}
	if c := arg(args, 3); c != "" {
		opts.Currency = c
	}
	if len(args) > 4 && args[4].Type() == js.TypeBoolean {
		opts.WithBAH = args[4].Bool()
	}

	xml, err := iso20022.Generate(opts)
	if err != nil {
		return fail(err.Error())
	}
	return ok(map[string]any{"xml": xml, "messageType": msgType, "preset": opts.Preset})
}

func toJSON(this js.Value, args []js.Value) any {
	payload := arg(args, 0)
	if strings.TrimSpace(payload) == "" {
		return fail("paste an ISO 20022 XML message to convert")
	}
	out, err := iso20022.XMLToJSON([]byte(payload))
	if err != nil {
		return fail(err.Error())
	}
	return ok(map[string]any{"json": string(out)})
}

func lookupCodes(this js.Value, args []js.Value) any {
	return ok(iso20022.LookupCode(arg(args, 0)))
}

func translate(this js.Value, args []js.Value) any {
	q := arg(args, 0)
	if strings.TrimSpace(q) == "" {
		return ok(iso20022.AllMappings())
	}
	m, found := iso20022.TranslateSWIFT(q)
	if !found {
		return fail("no MT/MX mapping for " + q + " (try MT103, MT202, MT940, pacs.008, camt.053)")
	}
	return ok(m)
}

// convertMT converts a pasted message in whichever direction it needs to go.
// The browser runs the same converter as the CLI, so a message translated on
// the site gets the same output and the same fidelity report as one translated
// in a terminal.
func convertMT(this js.Value, args []js.Value) any {
	raw := strings.TrimSpace(arg(args, 0))
	if raw == "" {
		return fail("paste a SWIFT MT message (" + strings.Join(mtList(), ", ") +
			") or an ISO 20022 message (" + strings.Join(iso20022.TranslatableMX(), ", ") + ")")
	}

	// An ISO 20022 message is XML; an MT message begins with its block
	// structure. Which direction to go is a property of what was pasted.
	convert := iso20022.TranslateMT
	if strings.HasPrefix(raw, "<") {
		convert = iso20022.TranslateMX
	}

	conv, err := convert([]byte(raw))
	if err != nil {
		return fail(err.Error())
	}

	counts := conv.Counts()
	return ok(map[string]any{
		"source_type": conv.SourceType,
		"target_type": conv.TargetType,
		"xml":         conv.XML,
		"report":      conv.Report,
		"lossless":    conv.Lossless(),
		"mapped":      counts["mapped"],
		"derived":     counts["derived"],
		"truncated":   counts["truncated"],
		"unmapped":    counts["unmapped"],
	})
}

// mtList renders the supported types the way a reader expects to see them.
func mtList() []string {
	var out []string
	for _, t := range iso20022.TranslatableMT() {
		out = append(out, "MT"+t)
	}
	return out
}

func diagram(this js.Value, args []js.Value) any {
	msgType := arg(args, 0)
	if msgType == "" {
		msgType = "pacs.008"
	}
	preset := arg(args, 1)
	if preset == "" {
		preset = "sepa"
	}
	format := arg(args, 2)
	if format == "" {
		format = "mermaid"
	}
	return ok(map[string]any{"diagram": iso20022.SequenceDiagram(msgType, preset, format), "format": format})
}

func stats(this js.Value, args []js.Value) any {
	counts, err := browserCatalogue.DomainCounts()
	if err != nil {
		return fail(err.Error())
	}

	type row struct {
		Domain string `json:"domain"`
		Name   string `json:"name"`
		Count  int    `json:"count"`
	}
	rows := make([]row, 0, len(counts))
	total := 0
	for d, n := range counts {
		rows = append(rows, row{Domain: d, Name: iso20022.DomainName(d), Count: n})
		total += n
	}
	sortRows(rows, func(a, b row) bool {
		if a.Count != b.Count {
			return a.Count > b.Count
		}
		return a.Domain < b.Domain
	})

	sets, _ := iso20022.MessageSets()
	return ok(map[string]any{"domains": rows, "total": total, "messageSets": len(sets)})
}

func messageSets(this js.Value, args []js.Value) any {
	sets, err := iso20022.MessageSets()
	if err != nil {
		return fail(err.Error())
	}
	type row struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Version string `json:"version"`
		URL     string `json:"url"`
	}
	out := make([]row, len(sets))
	for i, s := range sets {
		out[i] = row{ID: s.ID, Name: s.Name, Version: s.Version, URL: s.DownloadURL()}
	}
	return ok(out)
}

func lifecycle(this js.Value, args []js.Value) any {
	opts := iso20022.DefaultGeneratorOptions("pacs.008")
	if p := arg(args, 0); p != "" {
		opts.Preset = p
	}
	chain, err := iso20022.GenerateLifecycle(opts)
	if err != nil {
		return fail(err.Error())
	}
	return ok(chain)
}

func checkIBAN(this js.Value, args []js.Value) any {
	valid, reason := iso20022.ValidateIBAN(arg(args, 0))
	return ok(map[string]any{"valid": valid, "reason": reason})
}

func checkBIC(this js.Value, args []js.Value) any {
	valid, reason := iso20022.ValidateBIC(arg(args, 0))
	return ok(map[string]any{"valid": valid, "reason": reason})
}

func checkUETR(this js.Value, args []js.Value) any {
	valid, reason := iso20022.ValidateUETR(arg(args, 0))
	return ok(map[string]any{"valid": valid, "reason": reason})
}

// sortRows is a tiny insertion sort; the row count is small and this avoids
// pulling reflection into the WebAssembly binary.
func sortRows[T any](rows []T, less func(a, b T) bool) {
	for i := 1; i < len(rows); i++ {
		for j := i; j > 0 && less(rows[j], rows[j-1]); j-- {
			rows[j], rows[j-1] = rows[j-1], rows[j]
		}
	}
}
