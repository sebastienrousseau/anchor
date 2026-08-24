// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package cmd

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/sebastienrousseau/anchor/internal/registry"
	"github.com/spf13/cobra"
)

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#22D3EE"))
	headStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(lipgloss.Color("#06B6D4")).Padding(0, 1)
	subtleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#94A3B8"))
	searchJSON  bool
)

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search ISO 20022 messages by identifier, domain, or keyword",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		query := strings.Join(args, " ")

		idx, err := loadCatalog()
		if err != nil {
			// No catalogue installed. The embedded registry still knows which
			// messages exist and where the RA publishes them, so answer from
			// that rather than failing outright.
			return searchRegistry(query, false)
		}

		results := idx.Search(query)
		if len(results) == 0 {
			// The catalogue index knows only what is installed. The embedded
			// registry knows the whole standard, so a query the local files
			// cannot answer still gets an answer -- and names the download.
			return searchRegistry(query, true)
		}

		if searchJSON {
			data, err := json.MarshalIndent(results, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(data))
			return nil
		}

		fmt.Printf("\n%s Found %d results matching '%s':\n\n", headStyle.Render(" SEARCH RESULTS "), len(results), query)
		for _, m := range results {
			fmt.Printf("  • %-24s %-36s [%s]\n", titleStyle.Render(m.ID), m.Category, m.Version)
			if m.XMLSamplePath != "" {
				relXML, _ := filepath.Rel(idx.RootDir, m.XMLSamplePath)
				fmt.Printf("    XML: %s\n", subtleStyle.Render(relXML))
			}
			if m.XSDPath != "" {
				relXSD, _ := filepath.Rel(idx.RootDir, m.XSDPath)
				fmt.Printf("    XSD: %s\n", subtleStyle.Render(relXSD))
			}
		}
		fmt.Println()
		return nil
	},
}

// searchRegistry answers from Anchor's embedded index of the standard. Results
// carry no file paths, because nothing is installed -- they carry the message
// set to download instead.
func searchRegistry(query string, haveCatalogue bool) error {
	reg, err := registry.Load()
	if err != nil {
		return err
	}
	results := reg.Search(query)

	if searchJSON {
		data, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	fmt.Printf("\n%s Found %d results matching '%s':\n\n",
		headStyle.Render(" SEARCH RESULTS "), len(results), query)

	const maxRows = 40
	for i, m := range results {
		if i == maxRows {
			fmt.Printf("  ... and %d more\n", len(results)-maxRows)
			break
		}
		sets := reg.SetsFor(m.ID)
		label := ""
		if len(sets) > 0 {
			label = sets[0].Name
		}
		fmt.Printf("  • %-24s %s\n", titleStyle.Render(m.ID), label)
	}

	if len(results) == 0 {
		// An empty list is not an answer. Say what is searchable, because the
		// Registration Authority publishes titles per message set rather than
		// per message, and a query like "direct debit" has nothing to match.
		fmt.Printf("  %s\n", subtleStyle.Render(
			"Search matches message identifiers (pacs.008), domains (camt), and the names"))
		fmt.Printf("  %s\n\n", subtleStyle.Render(
			"of published message sets (Payments Mandates). Message titles are not published"+
				" per message, so a plain-English description may find nothing."))
		return nil
	}

	if haveCatalogue {
		fmt.Printf("\n%s nothing installed matched, so these come from Anchor's index of the\n",
			subtleStyle.Render("note:"))
		fmt.Printf("      whole standard and have no local schema paths.\n")
	} else {
		fmt.Printf("\n%s no catalogue installed, so these results have no schema paths.\n",
			subtleStyle.Render("note:"))
	}
	if sets := reg.SetsFor(results[0].ID); len(sets) > 0 {
		fmt.Printf("      Get %s from %s\n", sets[0], sets[0].DownloadURL())
		fmt.Printf("      then: anchor catalog add <downloaded.zip>\n")
	}
	fmt.Println()
	return nil
}

func init() {
	searchCmd.Flags().BoolVar(&searchJSON, "json", false, "Output results as formatted JSON")
	RootCmd.AddCommand(searchCmd)
}
