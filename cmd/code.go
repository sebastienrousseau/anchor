// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/sebastienrousseau/askiso/internal/codes"
	"github.com/spf13/cobra"
)

var (
	codeImport   string
	codeJSON     bool
	codeCategory string
	codeSet      string
	codeListSets bool
	codeLimit    int
	codeAll      bool
)

var codeCmd = &cobra.Command{
	Use:     "code [code-or-keyword]",
	Aliases: []string{"codes"},
	Short:   "Lookup and explain ISO 20022 external codes (reasons, purpose, charges, status)",
	Long: `Code looks up ISO 20022 code values.

AskISO carries a curated dictionary of the codes that come up most often. With a
catalogue installed it also reads every code set enumerated in your schemas --
several thousand values across the whole standard -- so a lookup covers far more
than the curated set.

Codes maintained separately by the Registration Authority as "external code
sets" are referenced by name in the schemas rather than enumerated. AskISO
redistributes that publication no more than it redistributes the schemas:
download it from iso20022.org and import it with --import, and every lookup
searches it thereafter.`,
	Example: `  askiso code AC04
  askiso code SALA
  askiso code "insufficient funds"
  askiso code --set ChargeBearerType1Code
  askiso code --sets
  askiso code --import ~/Downloads/ExternalCodeSets.xlsx
  askiso code --category reason
  askiso code --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		query := ""
		if len(args) > 0 {
			query = strings.Join(args, " ")
		}

		// Reading every schema takes a moment, so it is done only when the
		// curated dictionary cannot answer, or when the whole index is asked for.
		schemaIndex := func() *codes.SchemaIndex {
			idx, err := loadCatalog()
			if err != nil {
				return nil
			}
			built, err := codes.LoadIndex(idx)
			if err != nil {
				return nil
			}
			return built
		}

		if codeImport != "" {
			return importExternalCodes(codeImport)
		}

		external := externalSets()

		if codeListSets {
			return listCodeSets(schemaIndex(), external)
		}
		if codeSet != "" {
			// An external set is the Registration Authority's own publication,
			// so it answers before the schemas do.
			if members := external.Set(codeSet); len(members) > 0 {
				return showExternalSet(members)
			}
			return showCodeSet(schemaIndex(), codeSet)
		}

		results := codes.Lookup(query)
		if codeCategory != "" {
			catLower := strings.ToLower(codeCategory)
			var filtered []codes.CodeItem
			for _, item := range results {
				if strings.Contains(strings.ToLower(string(item.Category)), catLower) {
					filtered = append(filtered, item)
				}
			}
			results = filtered
		}

		// Fall back to the imported publication and then the schemas for
		// anything the curated set does not cover.
		var fromExternal []codes.ExternalCode
		var fromSchema []codes.SchemaCode
		var schemaIdx *codes.SchemaIndex
		if codeCategory == "" && (len(results) == 0 || codeAll) {
			fromExternal = external.Search(query)
			if schemaIdx = schemaIndex(); schemaIdx != nil {
				fromSchema = schemaIdx.Search(query)
			}
		}

		if len(results) == 0 && len(fromExternal) == 0 && len(fromSchema) == 0 {
			return noCodeMatch(query, schemaIdx, external)
		}

		if codeJSON {
			payload := struct {
				Curated  []codes.CodeItem     `json:"curated"`
				External []codes.ExternalCode `json:"external,omitempty"`
				Schema   []codes.SchemaCode   `json:"schema,omitempty"`
			}{
				Curated:  results,
				External: capExternal(fromExternal, codeLimit),
				Schema:   capCodes(fromSchema, codeLimit),
			}
			out, err := json.MarshalIndent(payload, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(out))
			return nil
		}

		fmt.Printf("\n%s Found %d code definition(s):\n\n",
			headStyle.Render(" ISO 20022 CODE DICTIONARY "),
			len(results)+len(fromExternal)+len(fromSchema))

		for _, item := range results {
			fmt.Printf("  %s  %-32s [%s]\n", badgeStyle.Render(" "+item.Code+" "), titleStyle.Render(item.Name), item.Category)
			fmt.Printf("      %s\n", item.Description)
			fmt.Printf("      %s %s\n\n", subtleStyle.Render("Applies to:"), item.AppliesTo)
		}

		printExternalCodes(fromExternal, len(results) > 0)
		printSchemaCodes(fromSchema, len(results)+len(fromExternal) > 0)

		if len(results) > 0 && !codeAll {
			fmt.Printf("  %s\n\n", subtleStyle.Render(
				"curated result — add --all to also search every code set in your schemas"))
		}
		return nil
	},
}

// printSchemaCodes renders codes read from the installed schemas.
func printSchemaCodes(list []codes.SchemaCode, hadCurated bool) {
	if len(list) == 0 {
		return
	}
	if hadCurated {
		fmt.Printf("  %s\n\n", subtleStyle.Render("── from your installed schemas ──"))
	}

	shown := capCodes(list, codeLimit)
	for _, c := range shown {
		fmt.Printf("  %s  %s\n", badgeStyle.Render(" "+c.Code+" "), titleStyle.Render(c.Set))
		if c.Description != "" {
			fmt.Printf("      %s\n", c.Description)
		}
		if len(c.Messages) > 0 {
			fmt.Printf("      %s %s\n", subtleStyle.Render("Used by:"), strings.Join(c.Messages, ", "))
		}
		fmt.Println()
	}
	if len(list) > len(shown) {
		fmt.Printf("  %s\n\n", subtleStyle.Render(
			fmt.Sprintf("... and %d more; narrow the query or raise --limit", len(list)-len(shown))))
	}
}

// externalSets loads the Registration Authority publication the user imported,
// if they have.
func externalSets() *codes.ExternalSets {
	idx, err := loadCatalog()
	if err != nil {
		return nil
	}
	return codes.ExternalSetsFor(idx.RootDir)
}

// importExternalCodes reads a publication and stores it beside the catalogue.
func importExternalCodes(path string) error {
	idx, err := loadCatalog()
	if err != nil {
		return fmt.Errorf("the external code sets are stored beside your catalogue, "+
			"and none is installed:\n\n%w", err)
	}

	sets, err := codes.ImportExternalSets(path)
	if err != nil {
		return err
	}
	stored, err := codes.SaveExternalSets(idx.RootDir, sets)
	if err != nil {
		return err
	}
	codes.ForgetExternalSets(idx.RootDir)

	fmt.Printf("\n%s %d code(s) across %d set(s)\n\n",
		headStyle.Render(" EXTERNAL CODE SETS "), sets.Total(), len(sets.SetNames()))
	fmt.Printf("  %-10s %s\n", "from", path)
	fmt.Printf("  %-10s %s\n\n", "stored", stored)
	fmt.Printf("  %s askiso code SALA\n", subtleStyle.Render("→"))
	fmt.Printf("  %s askiso code --sets\n\n", subtleStyle.Render("→"))
	return nil
}

// noCodeMatch explains an empty result, and says what would widen the search.
func noCodeMatch(query string, schemaIdx *codes.SchemaIndex, external *codes.ExternalSets) error {
	var b strings.Builder
	fmt.Fprintf(&b, "no ISO 20022 code matches %q", query)

	var missing []string
	if schemaIdx == nil {
		missing = append(missing, "  Install a message set so AskISO can read the code sets your schemas enumerate:\n"+
			"    askiso catalog add <downloaded.zip>")
	}
	if external.Total() == 0 {
		missing = append(missing, "  Import the external code sets, which the Registration Authority publishes separately:\n"+
			"    askiso code --import <ExternalCodeSets.xlsx>")
	}
	if len(missing) > 0 {
		b.WriteString("\n\n" + strings.Join(missing, "\n\n"))
	}
	return errors.New(b.String())
}

// printExternalCodes renders codes from the imported publication.
func printExternalCodes(list []codes.ExternalCode, hadCurated bool) {
	if len(list) == 0 {
		return
	}
	if hadCurated {
		fmt.Printf("  %s\n\n", subtleStyle.Render("── from the external code sets you imported ──"))
	}

	shown := capExternal(list, codeLimit)
	for _, c := range shown {
		fmt.Printf("  %s  %-32s [%s]\n",
			badgeStyle.Render(" "+c.Code+" "), titleStyle.Render(c.Name), c.Set)
		if c.Definition != "" {
			fmt.Printf("      %s\n", c.Definition)
		}
		fmt.Println()
	}
	if len(list) > len(shown) {
		fmt.Printf("  %s\n\n", subtleStyle.Render(
			fmt.Sprintf("... and %d more; narrow the query or raise --limit", len(list)-len(shown))))
	}
}

// showExternalSet prints the members of one external code set.
func showExternalSet(members []codes.ExternalCode) error {
	if codeJSON {
		out, err := json.MarshalIndent(members, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(out))
		return nil
	}

	fmt.Printf("\n%s %s — %d code(s)\n\n",
		headStyle.Render(" EXTERNAL CODE SET "), titleStyle.Render(members[0].Set), len(members))
	for _, c := range members {
		fmt.Printf("  %-12s %s\n", c.Code, firstOfDefinition(c))
	}
	fmt.Println()
	return nil
}

func firstOfDefinition(c codes.ExternalCode) string {
	if c.Definition != "" {
		return c.Definition
	}
	return c.Name
}

func capExternal(list []codes.ExternalCode, limit int) []codes.ExternalCode {
	if limit <= 0 || len(list) <= limit {
		return list
	}
	return list[:limit]
}

// listCodeSets prints every code set AskISO can see: those enumerated in the
// installed schemas, and those from an imported publication.
func listCodeSets(idx *codes.SchemaIndex, external *codes.ExternalSets) error {
	if idx == nil && external.Total() == 0 {
		return errNoCatalogueForCodes()
	}

	if external.Total() > 0 {
		names := external.SetNames()
		fmt.Printf("\n%s %d external set(s), %d code(s)\n\n",
			headStyle.Render(" EXTERNAL CODE SETS "), len(names), external.Total())
		for _, n := range names {
			fmt.Printf("  %-52s %d\n", n, len(external.Set(n)))
		}
		fmt.Println()
	}

	if idx == nil {
		fmt.Printf("  %s\n\n", subtleStyle.Render(
			"No schemas are installed, so the enumerated code sets are not listed."))
		return nil
	}

	names := idx.SetNames()
	fmt.Printf("%s %d code set(s), %d code(s)\n\n",
		headStyle.Render(" CODE SETS "), len(names), idx.Codes)
	for _, n := range names {
		fmt.Printf("  %-52s %d\n", n, len(idx.Sets[n]))
	}
	fmt.Println()
	return nil
}

// showCodeSet prints the members of one named set.
func showCodeSet(idx *codes.SchemaIndex, name string) error {
	if idx == nil {
		return errNoCatalogueForCodes()
	}
	members := idx.Set(name)
	if len(members) == 0 {
		return fmt.Errorf("no code set named %q (list them with: askiso code --sets)", name)
	}

	if codeJSON {
		out, err := json.MarshalIndent(members, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(out))
		return nil
	}

	fmt.Printf("\n%s %s — %d code(s)\n\n",
		headStyle.Render(" CODE SET "), titleStyle.Render(members[0].Set), len(members))
	for _, c := range members {
		fmt.Printf("  %-12s %s\n", c.Code, c.Description)
	}
	fmt.Println()
	return nil
}

func errNoCatalogueForCodes() error {
	return fmt.Errorf("code sets come from your installed schemas and from the external code " +
		"sets the Registration Authority publishes separately; neither is installed\n\n" +
		"Download a message set from https://www.iso20022.org/ then:\n" +
		"  askiso catalog add <downloaded.zip>\n\n" +
		"And for the external code sets:\n" +
		"  askiso code --import <ExternalCodeSets.xlsx>")
}

func capCodes(list []codes.SchemaCode, limit int) []codes.SchemaCode {
	if limit <= 0 || len(list) <= limit {
		return list
	}
	return list[:limit]
}

func init() {
	codeCmd.Flags().StringVar(&codeSet, "set", "", "Show every member of a named code set")
	codeCmd.Flags().BoolVar(&codeListSets, "sets", false, "List every code set found in your schemas")
	codeCmd.Flags().IntVar(&codeLimit, "limit", 25, "Maximum schema-backed results to print")
	codeCmd.Flags().StringVar(&codeImport, "import", "",
		"Import the external code sets the Registration Authority publishes (.xlsx or .json)")
	codeCmd.Flags().BoolVar(&codeAll, "all", false, "Search the schemas even when a curated result exists")
	codeCmd.Flags().BoolVar(&codeJSON, "json", false, "Output results as formatted JSON")
	codeCmd.Flags().StringVarP(&codeCategory, "category", "c", "", "Filter by category (reason, purpose, charge, status, balance)")
	RootCmd.AddCommand(codeCmd)
}
