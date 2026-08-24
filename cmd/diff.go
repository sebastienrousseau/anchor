// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/sebastienrousseau/anchor/internal/catalog"
	"github.com/sebastienrousseau/anchor/internal/diff"
	"github.com/sebastienrousseau/anchor/internal/xsd"
	"github.com/spf13/cobra"
)

var (
	diffJSON         bool
	diffBreakingOnly bool
	diffStrict       bool
)

var diffCmd = &cobra.Command{
	Use:   "diff <from> <to>",
	Short: "Compare two ISO 20022 schemas and classify what breaks",
	Long: `Diff walks two schemas from their document root, flattens them into element
paths, and reports every structural difference.

The direction matters: <from> is the version a message was built against, <to>
is the one it must now satisfy. Each difference is classified as breaking or
compatible. A change is breaking when a message that satisfied the old schema
can be rejected by the new one -- an element removed, an element that became
mandatory, a repeat count reduced, a length shortened, a code withdrawn -- or
when a receiver loses a field it used to get.

Pattern changes are always reported as breaking. Deciding whether one regular
expression accepts everything another does is not something a tool should
guess at, and the answer that cannot mislead a migration is the cautious one.`,
	Example: `  anchor diff pacs.008.001.09 pacs.008.001.10
  anchor diff pacs.008.001.09 pacs.008.001.10 --breaking
  anchor diff old.xsd new.xsd --json
  anchor diff camt.053.001.10 camt.053.001.11 --strict`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		fromID, toID := args[0], args[1]

		idx, err := loadCatalog()
		if err != nil {
			// Two file paths need no catalogue at all.
			if !isReadableFile(fromID) || !isReadableFile(toID) {
				return notInstalled(fromID, "compare schema versions", err)
			}
			idx = &catalog.Index{}
		}

		fromPath, fromName := resolveSchemaPath(idx, fromID)
		toPath, toName := resolveSchemaPath(idx, toID)
		if fromPath == "" {
			return notInstalled(fromID, "compare schema versions", nil)
		}
		if toPath == "" {
			return notInstalled(toID, "compare schema versions", nil)
		}

		fromSchema, err := xsd.ParseFile(fromPath)
		if err != nil {
			return fmt.Errorf("parsing %s: %w", fromPath, err)
		}
		toSchema, err := xsd.ParseFile(toPath)
		if err != nil {
			return fmt.Errorf("parsing %s: %w", toPath, err)
		}

		report := diff.Compare(fromSchema, toSchema, fromName, toName)

		if diffJSON {
			out, err := json.MarshalIndent(report, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(out))
		} else {
			printDiff(report)
		}

		// --strict makes the command usable as a CI gate on a schema upgrade.
		if breaking, _ := report.Counts(); diffStrict && breaking > 0 {
			return errSilent
		}
		return nil
	},
}

func isReadableFile(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

func printDiff(report *diff.Report) {
	breaking, compatible := report.Counts()

	fmt.Printf("\n%s %s → %s\n\n", headStyle.Render(" SCHEMA DIFF "),
		titleStyle.Render(report.From), titleStyle.Render(report.To))
	fmt.Printf("  %d path(s) in common, %d breaking change(s), %d compatible change(s)\n\n",
		report.Common, breaking, compatible)

	if report.Identical() {
		fmt.Printf("  %s the two schemas are structurally identical\n\n", tickMark)
		return
	}

	shown := report.Changes
	if diffBreakingOnly {
		shown = report.Breaking()
		if len(shown) == 0 {
			fmt.Printf("  %s nothing breaks: every change is backwards compatible\n\n", tickMark)
			return
		}
	}

	for _, c := range shown {
		mark := warnMark
		if c.Severity == diff.Breaking {
			mark = crossMark
		}
		fmt.Printf("  %s %s  %s\n", mark, c.Path, subtleStyle.Render(string(c.Kind)))
		fmt.Printf("       %s\n", c.Detail)
		switch {
		case c.From != "" && c.To != "":
			fmt.Printf("       %s %s → %s\n", subtleStyle.Render("was"), c.From, c.To)
		case c.From != "":
			fmt.Printf("       %s %s\n", subtleStyle.Render("was"), c.From)
		case c.To != "":
			fmt.Printf("       %s %s\n", subtleStyle.Render("now"), c.To)
		}
		fmt.Println()
	}

	if breaking > 0 && !diffBreakingOnly {
		fmt.Printf("  %s list only what breaks:  anchor diff %s %s --breaking\n\n",
			subtleStyle.Render("→"), report.From, report.To)
	}
}

func resolveSchemaPath(idx *catalog.Index, idOrPath string) (string, string) {
	if isReadableFile(idOrPath) {
		return idOrPath, idOrPath
	}
	if msg, ok := idx.MessageMap[idOrPath]; ok && msg.XSDPath != "" {
		return msg.XSDPath, msg.ID
	}
	results := idx.Search(idOrPath)
	if len(results) > 0 && results[0].XSDPath != "" {
		return results[0].XSDPath, results[0].ID
	}
	return "", idOrPath
}

func init() {
	diffCmd.Flags().BoolVar(&diffJSON, "json", false, "Output the report as JSON")
	diffCmd.Flags().BoolVarP(&diffBreakingOnly, "breaking", "b", false, "Show only the changes that can reject a message")
	diffCmd.Flags().BoolVar(&diffStrict, "strict", false, "Exit non-zero when any breaking change is found")
	RootCmd.AddCommand(diffCmd)
}
