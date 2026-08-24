// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/atotto/clipboard"
	"github.com/sebastienrousseau/anchor/internal/converter"
	"github.com/spf13/cobra"
)

var (
	convertOutput string
	convertToJSON bool
	convertToXML  bool
	convertCopy   bool
)

var convertCmd = &cobra.Command{
	Use:   "convert <file>",
	Short: "Convert between ISO 20022 XML and structured JSON payloads",
	Long: `Convert transforms ISO 20022 messages bidirectionally between XML and 
structured JSON formats for modern REST APIs and event streams.`,
	Example: `  anchor convert payment.xml --to-json
  anchor convert payment.xml -o payment.json
  anchor convert payload.json --to-xml -o payload.xml`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		filePath := filepath.Clean(args[0])
		data, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("failed to read file '%s': %w", filePath, err)
		}

		// Auto-detect direction if not explicitly set
		isXML := strings.HasSuffix(strings.ToLower(filePath), ".xml") || strings.Contains(string(data[:min(100, len(data))]), "<")
		isJSON := strings.HasSuffix(strings.ToLower(filePath), ".json") || strings.HasPrefix(strings.TrimSpace(string(data)), "{")

		var result []byte

		if convertToJSON || (isXML && !convertToXML) {
			jsonBytes, err := converter.XMLToJSON(data)
			if err != nil {
				return fmt.Errorf("XML to JSON conversion failed: %w", err)
			}
			result = jsonBytes
		} else if convertToXML || (isJSON && !convertToJSON) {
			xmlBytes, err := converter.JSONToXML(data)
			if err != nil {
				return fmt.Errorf("JSON to XML conversion failed: %w", err)
			}
			result = xmlBytes
		} else {
			return fmt.Errorf("please specify conversion direction using --to-json or --to-xml")
		}

		if convertCopy {
			if err := clipboard.WriteAll(string(result)); err == nil {
				fmt.Printf("\n%s Copied converted payload to clipboard!\n\n", badgeStyle.Render(" COPIED "))
			}
		}

		if convertOutput != "" {
			if err := os.WriteFile(convertOutput, result, 0o600); err != nil {
				return fmt.Errorf("failed to write output file: %w", err)
			}
			fmt.Printf("\n%s Successfully converted payload to %s\n\n", badgeStyle.Render(" CONVERTED "), convertOutput)
			return nil
		}

		if !convertCopy || convertOutput != "" {
			fmt.Println(string(result))
		}
		return nil
	},
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func init() {
	convertCmd.Flags().BoolVar(&convertToJSON, "to-json", false, "Convert XML to structured JSON")
	convertCmd.Flags().BoolVar(&convertToXML, "to-xml", false, "Convert JSON to ISO 20022 XML")
	convertCmd.Flags().StringVarP(&convertOutput, "output", "o", "", "Write converted payload to file")
	convertCmd.Flags().BoolVarP(&convertCopy, "copy", "y", false, "Copy converted output to clipboard")

	RootCmd.AddCommand(convertCmd)
}
