// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package cmd

import (
	"fmt"

	"github.com/atotto/clipboard"
	"github.com/sebastienrousseau/askiso/internal/generator"
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
	Args: cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		format, err := normalizeChoice("format", graphFormat, "ascii", "mermaid")
		if err != nil {
			return err
		}
		preset, err := normalizeChoice("preset", graphPreset, generator.Presets()...)
		if err != nil {
			return err
		}
		msgType := "pacs.008"
		if len(args) > 0 {
			msgType = args[0]
		}

		var output string
		if format == "mermaid" {
			output = graph.GenerateMermaid(msgType, preset)
		} else {
			output = graph.GenerateASCII(msgType, preset)
		}

		if graphCopy {
			if err := clipboard.WriteAll(output); err == nil {
				fmt.Printf("\n%s Diagram copied to clipboard!\n\n", badgeStyle.Render(" COPIED "))
			}
		}

		if !graphCopy || format == "ascii" {
			fmt.Println(output)
		}
		return nil
	},
}

func init() {
	graphCmd.Flags().StringVarP(&graphPreset, "preset", "p", "sepa", "Clearing network preset (standard, sepa, fednow, fedwire, target2, chaps)")
	graphCmd.Flags().StringVarP(&graphFormat, "format", "f", "ascii", "Diagram format: ascii or mermaid")
	graphCmd.Flags().BoolVarP(&graphCopy, "copy", "y", false, "Copy diagram to clipboard")

	RootCmd.AddCommand(graphCmd)
}
