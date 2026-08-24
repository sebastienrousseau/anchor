// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package cmd

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/atotto/clipboard"
	"github.com/spf13/cobra"
)

var (
	formatMinify bool
	formatCopy   bool
	formatOutput string
)

var formatCmd = &cobra.Command{
	Use:     "format <xml-file>",
	Aliases: []string{"fmt"},
	Short:   "Pretty-print or minify ISO 20022 XML message instances",
	Long: `Format standardizes indentation and structure for ISO 20022 XML files.
Supports clean 2-space pretty printing and compact payload minification.`,
	Example: `  askiso format payload.xml
  askiso format payload.xml --minify --copy
  askiso format payload.xml -o formatted.xml`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		filePath := filepath.Clean(args[0])
		data, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("failed to read file '%s': %w", filePath, err)
		}

		var formatted string
		if formatMinify {
			min, err := minifyXML(data)
			if err != nil {
				return fmt.Errorf("minify error: %w", err)
			}
			formatted = min
		} else {
			pretty, err := prettyPrintXML(data)
			if err != nil {
				return fmt.Errorf("pretty-print error: %w", err)
			}
			formatted = pretty
		}

		if formatCopy {
			if err := clipboard.WriteAll(formatted); err == nil {
				fmt.Printf("\n%s Copied formatted XML to clipboard!\n\n", badgeStyle.Render(" COPIED "))
			}
		}

		if formatOutput != "" {
			if err := os.WriteFile(formatOutput, []byte(formatted), 0o600); err != nil {
				return fmt.Errorf("failed to write output file: %w", err)
			}
			fmt.Printf("\n%s Successfully formatted XML to %s\n\n", badgeStyle.Render(" FORMATTED "), formatOutput)
			return nil
		}

		if !formatCopy || formatOutput != "" {
			fmt.Println(formatted)
		}
		return nil
	},
}

func prettyPrintXML(data []byte) (string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var buf bytes.Buffer
	encoder := xml.NewEncoder(&buf)
	encoder.Indent("", "  ")

	for {
		tok, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return "", err
		}
		if err := encoder.EncodeToken(tok); err != nil {
			return "", err
		}
	}
	if err := encoder.Flush(); err != nil {
		return "", err
	}

	result := buf.String()
	if !strings.HasPrefix(result, "<?xml") {
		result = "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n" + result
	}
	return result, nil
}

func minifyXML(data []byte) (string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var buf bytes.Buffer
	encoder := xml.NewEncoder(&buf)

	for {
		tok, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return "", err
		}
		if cd, ok := tok.(xml.CharData); ok {
			trimmed := bytes.TrimSpace(cd)
			if len(trimmed) == 0 {
				continue
			}
			tok = xml.CharData(trimmed)
		}
		if err := encoder.EncodeToken(tok); err != nil {
			return "", err
		}
	}
	if err := encoder.Flush(); err != nil {
		return "", err
	}

	result := buf.String()
	if !strings.HasPrefix(result, "<?xml") {
		result = "<?xml version=\"1.0\" encoding=\"UTF-8\"?>" + result
	}
	return result, nil
}

func init() {
	formatCmd.Flags().BoolVarP(&formatMinify, "minify", "m", false, "Minify XML by stripping whitespace")
	formatCmd.Flags().BoolVarP(&formatCopy, "copy", "y", false, "Copy formatted XML to clipboard")
	formatCmd.Flags().StringVarP(&formatOutput, "output", "o", "", "Write formatted XML to output file")

	RootCmd.AddCommand(formatCmd)
}
