// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sebastienrousseau/askiso/internal/catalog"
	"github.com/sebastienrousseau/askiso/internal/importer"
	"github.com/sebastienrousseau/askiso/internal/registry"
	"github.com/spf13/cobra"
)

var (
	catalogAddDest     string
	catalogAddCategory string
	catalogAddVersion  string
	catalogAddDryRun   bool
	catalogStatusAll   bool
)

var catalogCmd = &cobra.Command{
	Use:   "catalog",
	Short: "Manage your local ISO 20022 catalogue",
	Long: `AskIso does not redistribute ISO 20022 specifications. Download the message
sets you need from https://www.iso20022.org/ and import them with 'catalog add'.

Use 'catalog where' to see which locations AskIso searches, and 'catalog status'
to see what is installed against the full published standard.`,
}

var catalogAddCmd = &cobra.Command{
	Use:   "add <zip-or-directory>...",
	Short: "Import ISO 20022 message sets you downloaded from iso20022.org",
	Long: `Add explodes ISO 20022 downloads into your local catalogue.

The Registration Authority ships message sets as zip archives, frequently with
more archives nested inside. Add unpacks them recursively, sorts each file into
Schemas, Sample Messages, Message Definition Reports, Message Usage Guidelines
or Documentation, and writes them under the catalogue layout AskIso reads.`,
	Example: `  askiso catalog add ~/Downloads/PaymentsClearingAndSettlement_v11.zip
  askiso catalog add ~/Downloads/*.zip
  askiso catalog add ~/Downloads --dry-run
  askiso catalog add set.zip --category "Payments Clearing and Settlement" --version "Version 11.0"`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dest := catalogAddDest
		if dest == "" {
			dest = catalogPath
		}
		if dest == "" {
			dest = os.Getenv(catalog.EnvCatalog)
		}
		if dest == "" {
			dest = catalog.DefaultDir()
		}
		if dest == "" {
			return errors.New("could not determine a catalogue location; pass --to")
		}

		opt := importer.Options{
			Root:     dest,
			Category: catalogAddCategory,
			Version:  catalogAddVersion,
			DryRun:   catalogAddDryRun,
		}

		if catalogAddDryRun {
			fmt.Printf("\n%s no files will be written\n", headStyle.Render(" DRY RUN "))
		}
		fmt.Printf("\nCatalogue: %s\n\n", subtleStyle.Render(dest))

		var (
			results []*importer.Result
			failed  int
		)
		for _, arg := range args {
			in := filepath.Clean(arg)
			st, err := os.Stat(in)
			if err != nil {
				fmt.Printf("  %s %s: %v\n", crossMark, filepath.Base(in), err)
				failed++
				continue
			}

			if st.IsDir() {
				rs, err := importer.ImportDir(in, opt)
				results = append(results, rs...)
				if err != nil {
					fmt.Printf("  %s %s: %v\n", crossMark, filepath.Base(in), err)
					failed++
				}
				continue
			}

			r, err := importer.ImportArchive(in, opt)
			if err != nil {
				fmt.Printf("  %s %s: %v\n", crossMark, filepath.Base(in), err)
				failed++
				continue
			}
			results = append(results, r)
			printImport(r)
		}

		for _, r := range results {
			if !strings.Contains(strings.Join(args, "|"), filepath.Base(r.Source)) {
				printImport(r)
			}
		}

		var schemas, samples, reports int
		cats := map[string]bool{}
		for _, r := range results {
			schemas += r.Schemas
			samples += r.Samples
			reports += r.Reports + r.Guidelines
			for c := range r.Categories {
				cats[c] = true
			}
		}

		fmt.Printf("\n  %d schema(s), %d sample(s), %d report(s) across %d categor%s\n",
			schemas, samples, reports, len(cats), plural(len(cats), "y", "ies"))

		if failed > 0 {
			return fmt.Errorf("%d source(s) could not be imported", failed)
		}
		if schemas == 0 && !catalogAddDryRun {
			return errors.New("no schemas were imported; is this an ISO 20022 message set download?")
		}
		if !catalogAddDryRun {
			fmt.Printf("\n  Verify with: askiso doctor\n\n")
		}
		return nil
	},
}

func printImport(r *importer.Result) {
	name := filepath.Base(r.Source)
	fmt.Printf("  %s %s\n", tickMark, name)
	fmt.Printf("      %d schemas, %d samples, %d reports, %d guidelines, %d docs",
		r.Schemas, r.Samples, r.Reports, r.Guidelines, r.Docs)
	if r.Skipped > 0 {
		fmt.Printf(", %d skipped", r.Skipped)
	}
	fmt.Println()
	for c := range r.Categories {
		fmt.Printf("      -> %s\n", subtleStyle.Render(c))
	}
}

var catalogWhereCmd = &cobra.Command{
	Use:   "where",
	Short: "Show the locations AskIso searches for a catalogue",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("\n%s\n\n", headStyle.Render(" CATALOGUE RESOLUTION "))

		type candidate struct {
			label string
			path  string
		}
		cands := []candidate{
			{"--catalog", catalogPath},
			{"$" + catalog.EnvCatalog, os.Getenv(catalog.EnvCatalog)},
		}
		for _, d := range catalog.DefaultDirs() {
			cands = append(cands, candidate{"data directory", d})
		}
		if wd, err := os.Getwd(); err == nil {
			cands = append(cands, candidate{"working directory", wd})
		}

		for _, c := range cands {
			switch {
			case c.path == "":
				fmt.Printf("  %-22s %s\n", c.label, subtleStyle.Render("(not set)"))
			case catalog.IsCatalog(c.path):
				fmt.Printf("  %-22s %s  %s\n", c.label, c.path, tickMark)
			default:
				fmt.Printf("  %-22s %s  %s\n", c.label, c.path, subtleStyle.Render("(no catalogue)"))
			}
		}

		fmt.Println()
		if root, err := catalog.Resolve(catalogPath); err == nil {
			fmt.Printf("  Using: %s\n\n", titleStyle.Render(root))
		} else {
			fmt.Printf("  %s No catalogue installed.\n", crossMark)
			fmt.Printf("  Download message sets from https://www.iso20022.org/ then run:\n")
			fmt.Printf("    askiso catalog add <downloaded.zip>\n\n")
		}
		return nil
	},
}

var catalogStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Compare what you have installed against the published standard",
	RunE: func(cmd *cobra.Command, args []string) error {
		reg, err := registry.Load()
		if err != nil {
			return err
		}

		installed := map[string]bool{}
		root := ""
		if idx, err := loadCatalog(); err == nil {
			root = idx.RootDir
			for id := range idx.MessageMap {
				installed[id] = true
			}
		}

		fmt.Printf("\n%s\n\n", headStyle.Render(" CATALOGUE STATUS "))
		if root == "" {
			fmt.Printf("  %s no catalogue installed\n\n", crossMark)
		} else {
			fmt.Printf("  Location : %s\n", root)
		}

		// Group the standard by publishing message set.
		type row struct {
			set   registry.Set
			have  int
			total int
		}
		bySet := map[string]*row{}
		for _, m := range reg.Messages {
			for _, sid := range m.SetIDs {
				s, ok := reg.Set(sid)
				if !ok {
					continue
				}
				r := bySet[sid]
				if r == nil {
					r = &row{set: s}
					bySet[sid] = r
				}
				r.total++
				if installed[m.ID] {
					r.have++
				}
			}
		}

		rows := make([]*row, 0, len(bySet))
		for _, r := range bySet {
			rows = append(rows, r)
		}
		sort.Slice(rows, func(i, j int) bool {
			if rows[i].set.Name != rows[j].set.Name {
				return rows[i].set.Name < rows[j].set.Name
			}
			return rows[i].set.Version < rows[j].set.Version
		})

		var complete, partial, missing int
		fmt.Println()
		for _, r := range rows {
			switch {
			case r.have == r.total:
				complete++
			case r.have > 0:
				partial++
			default:
				missing++
				if !catalogStatusAll {
					continue
				}
			}
			mark := tickMark
			if r.have == 0 {
				mark = crossMark
			} else if r.have < r.total {
				mark = warnMark
			}
			fmt.Printf("  %s %-46s %3d/%-3d\n", mark, r.set.String(), r.have, r.total)
		}

		fmt.Printf("\n  %d complete, %d partial, %d not installed (of %d published sets)\n",
			complete, partial, missing, len(rows))
		if missing > 0 && !catalogStatusAll {
			fmt.Printf("  Use --all to list the sets you do not have.\n")
		}
		fmt.Println()
		return nil
	},
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func init() {
	catalogAddCmd.Flags().StringVar(&catalogAddDest, "to", "", "Catalogue directory to import into (default: the resolved catalogue location)")
	catalogAddCmd.Flags().StringVar(&catalogAddCategory, "category", "", "Override the category name derived from the archive")
	catalogAddCmd.Flags().StringVar(&catalogAddVersion, "version", "", "Override the version directory, e.g. \"Version 11.0\"")
	catalogAddCmd.Flags().BoolVar(&catalogAddDryRun, "dry-run", false, "Report what would be imported without writing anything")
	catalogStatusCmd.Flags().BoolVar(&catalogStatusAll, "all", false, "Include message sets that are not installed")

	catalogCmd.AddCommand(catalogAddCmd, catalogWhereCmd, catalogStatusCmd)
	RootCmd.AddCommand(catalogCmd)
}
