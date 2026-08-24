// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package cmd

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/sebastienrousseau/anchor/internal/registry"
	"github.com/spf13/cobra"
)

var statsJSON bool

type DomainStats struct {
	DomainCode   string  `json:"domain_code"`
	Name         string  `json:"name"`
	MessageCount int     `json:"message_count"`
	Percentage   float64 `json:"percentage"`
}

type CatalogSummary struct {
	TotalCategories int           `json:"total_categories"`
	TotalMessages   int           `json:"total_messages"`
	TotalDomains    int           `json:"total_domains"`
	DomainBreakdown []DomainStats `json:"domain_breakdown"`
}

var domainNames = map[string]string{
	"pacs": "Payments Clearing and Settlement",
	"pain": "Payments Initiation & Mandates",
	"camt": "Cash Management & Statements",
	"auth": "Regulatory Reporting (MiFIR/EMIR/SFTR)",
	"seev": "Securities Events & Corporate Actions",
	"sese": "Securities Settlement & Reconciliation",
	"setr": "Investment Funds & Order Routing",
	"acmt": "Account Management & Switching",
	"tsmt": "Trade Finance & Services Management",
	"colr": "Collateral Management",
	"fxtr": "Foreign Exchange Trade Instructions",
	"caaa": "Card Payments - Acceptor to Acquirer",
	"catp": "ATM Interface & Processing",
	"catm": "Terminal Management",
	"casp": "Retailer Protocol",
	"cain": "Acquirer to Issuer Card Messages",
	"caiu": "Card Payments Exchanges",
	"remt": "Stand-Alone Remittance Advice",
	"head": "Business Application Header (BAH)",
	"redp": "Post-Trade Matching",
}

var statsCmd = &cobra.Command{
	Use:     "stats",
	Aliases: []string{"metrics", "inventory"},
	Short:   "Display comprehensive domain statistics and catalog metrics",
	Long: `Stats analyzes all ISO 20022 message definitions in the repository,
providing a detailed breakdown by business domain, version density, and category totals.`,
	Example: `  anchor stats
  anchor stats --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		domainCounts := make(map[string]int)
		var totalCategories int
		source := "installed catalogue"

		idx, err := loadCatalog()
		if err != nil {
			// The embedded registry covers the whole published standard, so
			// light mode gives a complete picture of the standard rather than
			// of what happens to be installed.
			reg, regErr := registry.Load()
			if regErr != nil {
				return regErr
			}
			domainCounts = reg.Domains()
			totalCategories = len(reg.Sets)
			source = "embedded registry (whole published standard)"
		} else {
			for _, m := range idx.Messages {
				parts := strings.Split(m.ID, ".")
				if len(parts) > 0 {
					dCode := strings.ToLower(parts[0])
					domainCounts[dCode]++
				}
			}
			totalCategories = len(idx.Categories)
		}

		var stats []DomainStats
		totalMsgs := 0
		for _, n := range domainCounts {
			totalMsgs += n
		}

		for code, count := range domainCounts {
			name, ok := domainNames[code]
			if !ok {
				name = "Other Business Domain (" + strings.ToUpper(code) + ")"
			}
			pct := float64(count) / float64(totalMsgs) * 100.0
			stats = append(stats, DomainStats{
				DomainCode:   code,
				Name:         name,
				MessageCount: count,
				Percentage:   pct,
			})
		}

		sort.Slice(stats, func(i, j int) bool {
			return stats[i].MessageCount > stats[j].MessageCount
		})

		summary := CatalogSummary{
			TotalCategories: totalCategories,
			TotalMessages:   totalMsgs,
			TotalDomains:    len(stats),
			DomainBreakdown: stats,
		}

		if statsJSON {
			data, err := json.MarshalIndent(summary, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(data))
			return nil
		}

		fmt.Printf("\n%s ISO 20022 Repository Metrics & Inventory\n\n", headStyle.Render(" REPOSITORY METRICS "))
		fmt.Printf("  • Total Message Definitions : %s\n", titleStyle.Render(fmt.Sprintf("%d", totalMsgs)))
		fmt.Printf("  • Total Message Sets        : %d\n", totalCategories)
		fmt.Printf("  • Total Active Domains      : %d\n\n", len(stats))

		fmt.Printf("  %-8s %-42s %-12s %-10s\n", "Domain", "Business Area", "Messages", "Share")
		fmt.Println("  " + strings.Repeat("─", 74))

		for _, s := range stats {
			barLen := int(s.Percentage / 2.5)
			if barLen < 1 && s.MessageCount > 0 {
				barLen = 1
			}
			bar := strings.Repeat("■", barLen)
			fmt.Printf("  %-8s %-42s %-12d %5.1f%% %s\n", titleStyle.Render(s.DomainCode), s.Name, s.MessageCount, s.Percentage, subtleStyle.Render(bar))
		}
		fmt.Println("  " + strings.Repeat("─", 74))
		fmt.Printf("  Total: %d messages across %d sets\n", totalMsgs, totalCategories)
		fmt.Printf("  Source: %s\n\n", subtleStyle.Render(source))

		return nil
	},
}

func init() {
	statsCmd.Flags().BoolVar(&statsJSON, "json", false, "Output domain metrics as JSON")
	RootCmd.AddCommand(statsCmd)
}
