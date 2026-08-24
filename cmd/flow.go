// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sebastienrousseau/askiso/internal/flow"
	"github.com/sebastienrousseau/askiso/internal/generator"
	"github.com/spf13/cobra"
)

var (
	flowPreset    string
	flowAmount    string
	flowCurrency  string
	flowOutputDir string
	flowJSON      bool
)

var flowCmd = &cobra.Command{
	Use:   "flow [message-type]",
	Short: "Simulate and generate linked 4-stage end-to-end payment lifecycle flows",
	Long: `Flow simulates a complete multi-hop transaction lifecycle (pain.001 -> pacs.008 -> 
pacs.002 -> camt.053) with shared UETR, EndToEndId, and settlement amounts. 
Can export all 4 connected XML payloads into an output directory for integration testing.`,
	Example: `  askiso flow pacs.008 --preset sepa
  askiso flow --preset fednow --output-dir ./test-suite/
  askiso flow --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		msgType := "pacs.008"
		if len(args) > 0 {
			msgType = args[0]
		}

		opts := generator.DefaultOptions(msgType)
		if flowPreset != "" {
			opts.Preset = flowPreset
		}
		if flowAmount != "" {
			opts.Amount = flowAmount
		}
		if flowCurrency != "" {
			opts.Currency = flowCurrency
		}

		chain, err := flow.GenerateLifecycle(opts)
		if err != nil {
			return err
		}

		if flowJSON {
			data, err := json.MarshalIndent(chain, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(data))
			return nil
		}

		if flowOutputDir != "" {
			if err := os.MkdirAll(flowOutputDir, 0o750); err != nil {
				return fmt.Errorf("failed to create output directory: %w", err)
			}
			for _, step := range chain.Steps {
				targetFile := filepath.Join(flowOutputDir, step.FileName)
				if err := os.WriteFile(targetFile, []byte(step.XMLPayload), 0o600); err != nil {
					return fmt.Errorf("failed to write %s: %w", targetFile, err)
				}
			}
			fmt.Printf("\n%s Successfully exported 4-step payment flow to %s\n\n", badgeStyle.Render(" FLOW GENERATED "), flowOutputDir)
		}

		fmt.Printf("\n%s End-to-End Payment Lifecycle Simulator\n\n", headStyle.Render(" PAYMENT LIFECYCLE FLOW "))
		fmt.Printf("  • Shared UETR        : %s\n", titleStyle.Render(chain.UETR))
		fmt.Printf("  • Shared EndToEndId  : %s\n", chain.EndToEndID)
		fmt.Printf("  • Transaction Amount : %s %s\n", chain.Currency, chain.Amount)
		fmt.Printf("  • Clearing Preset    : %s\n\n", strings.ToUpper(chain.Preset))

		fmt.Println("  Transaction Chain Stages:")
		for _, s := range chain.Steps {
			fmt.Printf("  [%d] %-18s ➔ %s\n", s.Index, titleStyle.Render(s.MsgType), s.Title)
			fmt.Printf("      %s\n", subtleStyle.Render(s.Description))
			if flowOutputDir != "" {
				fmt.Printf("      File: %s\n", filepath.Join(flowOutputDir, s.FileName))
			}
			fmt.Println()
		}

		if flowOutputDir == "" {
			fmt.Println("To export all 4 linked XML instances to disk: askiso flow --output-dir ./suite/")
		}

		return nil
	},
}

func init() {
	flowCmd.Flags().StringVarP(&flowPreset, "preset", "p", "sepa", "Clearing network preset (sepa, fednow, target2, chaps)")
	flowCmd.Flags().StringVarP(&flowAmount, "amount", "a", "15000.00", "Transfer / settlement amount")
	flowCmd.Flags().StringVarP(&flowCurrency, "currency", "c", "EUR", "ISO 4217 Currency code")
	flowCmd.Flags().StringVarP(&flowOutputDir, "output-dir", "o", "", "Export all 4 XML payloads into a directory")
	flowCmd.Flags().BoolVar(&flowJSON, "json", false, "Output complete flow metadata as JSON")

	RootCmd.AddCommand(flowCmd)
}
