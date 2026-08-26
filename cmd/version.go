// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package cmd

import (
	"fmt"

	"github.com/sebastienrousseau/askiso/internal/tui"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of AskISO",
	Run: func(cmd *cobra.Command, args []string) {
		if !quiet {
			fmt.Print(tui.GetStyledLogo())
		}
		fmt.Printf("AskISO version %s\n", tui.Version)
	},
}

func init() {
	RootCmd.AddCommand(versionCmd)
}
