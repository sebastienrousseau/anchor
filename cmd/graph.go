// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package cmd

import (
	"fmt"
	"strings"

	"github.com/atotto/clipboard"
	"github.com/sebastienrousseau/askiso/internal/graph"
	"github.com/spf13/cobra"
)

var (
	graphPreset string
	graphFormat string
	graphCopy   bool
)

var graphCmd = &cobra.Command{
	Use:     "graph [message-type]",
	Aliases: []string{"diagram", "flowchart"},
	Short:   "Generate visual sequence diagrams and flowcharts (Mermaid / ASCII)",
	Long: `Graph generates multi-actor sequence diagrams illustrating ISO 20022 message 
flows between Debtor, Debtor Bank, Clearing Network, Creditor Bank, and Creditor. 
Outputs formatted Mermaid markdown diagrams or terminal ASCII flows.`,
	Example: `  askiso graph pacs.008
  askiso graph --preset fednow --format ascii
  askiso graph --format mermaid --copy`,
	RunE: func(cmd *cobra.Command, args []string) error {
		msgType := "pacs.008"
		if len(args) > 0 {
			msgType = args[0]
		}

		var output string
		if strings.ToLower(graphFormat) == "mermaid" {
			output = graph.GenerateMermaid(msgType, graphPreset)
		} else {
			output = graph.GenerateASCII(msgType, graphPreset)
		}

		if graphCopy {
			if err := clipboard.WriteAll(output); err == nil {
				fmt.Printf("\n%s Diagram copied to clipboard!\n\n", badgeStyle.Render(" COPIED "))
			}
		}

		if !graphCopy || graphFormat == "ascii" {
			fmt.Println(output)
		}
		return nil
	},
}

func init() {
	graphCmd.Flags().StringVarP(&graphPreset, "preset", "p", "sepa", "Clearing network preset (sepa, fednow, target2, chaps)")
	graphCmd.Flags().StringVarP(&graphFormat, "format", "f", "ascii", "Diagram format: ascii or mermaid")
	graphCmd.Flags().BoolVarP(&graphCopy, "copy", "y", false, "Copy diagram to clipboard")

	RootCmd.AddCommand(graphCmd)
}
