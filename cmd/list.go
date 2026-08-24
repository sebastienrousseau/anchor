// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all 56 ISO 20022 message set categories",
	RunE: func(cmd *cobra.Command, args []string) error {
		idx, err := loadCatalog()
		if err != nil {
			return err
		}

		fmt.Printf("\n%s (Total: %d Categories, %d Messages)\n\n",
			headStyle.Render(" ISO 20022 MESSAGE CATEGORIES "), len(idx.Categories), len(idx.Messages))
		for i, c := range idx.Categories {
			fmt.Printf(" %2d. %-48s (%2d versions, %3d schemas, %2d reports)\n",
				i+1, titleStyle.Render(c.Name), len(c.Versions), c.TotalSchemas, c.TotalReports)
		}
		fmt.Println()
		return nil
	},
}

func init() {
	RootCmd.AddCommand(listCmd)
}
