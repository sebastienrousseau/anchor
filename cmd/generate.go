// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/atotto/clipboard"
	"github.com/sebastienrousseau/anchor/internal/catalog"
	"github.com/sebastienrousseau/anchor/internal/generator"
	"github.com/sebastienrousseau/anchor/internal/schemagen"
	"github.com/sebastienrousseau/anchor/internal/xsd"
	"github.com/spf13/cobra"
)

var (
	genFromSchema   bool
	genOptional     bool
	genRepeats      int
	genAmount       string
	genCurrency     string
	genDebtor       string
	genCreditor     string
	genDebtorIBAN   string
	genCreditorIBAN string
	genPreset       string
	genWithBAH      bool
	genCopy         bool
	genOutputFile   string
)

var generateCmd = &cobra.Command{
	Use:   "generate <message-type>",
	Short: "Generate compliant, synthetic ISO 20022 XML instances for testing",
	Long: `Generate builds a valid ISO 20022 message.

Four message types -- pacs.008, pacs.009, pain.001 and camt.053 -- are built
from templates with rail-aware defaults, so a payment comes out looking like a
payment: real parties, a real amount, the right clearing system for the preset.
Those need no catalogue.

Every other message is built from its schema. Anchor walks the XSD, emits every
mandatory element in the order the content model declares, takes the first
branch of each choice, and generates values that satisfy the type -- codes from
enumerations, strings that match their patterns, numbers within their digit
limits. That covers all 2,845 published messages, and needs the schema
installed.

Generated messages validate against their own schema and pass the linter. That
is asserted across the whole catalogue, not claimed.`,
	Example: `  anchor generate pacs.008 --preset sepa --amount 15000.00
  anchor generate pacs.008 --preset fednow --copy
  anchor generate seev.031.001.09                # from the schema
  anchor generate camt.053.001.11 --from-schema  # schema instead of the template
  anchor generate pacs.008 --optional            # include optional elements
  anchor generate pain.001 --debtor "Acme Corp" --output payload.xml`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		msgType := args[0]

		if genFromSchema || !generator.HasTemplate(msgType) {
			return generateFromSchema(msgType)
		}

		opts := generator.DefaultOptions(msgType)
		if genPreset != "" {
			opts.Preset = genPreset
		}
		if genAmount != "" {
			opts.Amount = genAmount
		}
		if genCurrency != "" {
			opts.Currency = genCurrency
		}
		if genDebtor != "" {
			opts.Debtor = genDebtor
		}
		if genCreditor != "" {
			opts.Creditor = genCreditor
		}
		if genDebtorIBAN != "" {
			opts.DebtorIBAN = genDebtorIBAN
		}
		if genCreditorIBAN != "" {
			opts.CreditorIBAN = genCreditorIBAN
		}
		opts.WithBAH = genWithBAH

		xmlContent, err := generator.Generate(opts)
		if err != nil {
			return err
		}

		if genCopy {
			if err := clipboard.WriteAll(xmlContent); err == nil {
				fmt.Printf("\n%s Copied generated %s payload to clipboard!\n\n", badgeStyle.Render(" COPIED "), msgType)
			}
		}

		if genOutputFile != "" {
			if err := os.WriteFile(genOutputFile, []byte(xmlContent), 0o600); err != nil {
				return fmt.Errorf("failed to write output file: %w", err)
			}
			fmt.Printf("\n%s Successfully generated %s instance to %s\n\n", badgeStyle.Render(" GENERATED "), msgType, genOutputFile)
			return nil
		}

		if !genCopy || genOutputFile != "" {
			fmt.Println(xmlContent)
		}
		return nil
	},
}

// generateFromSchema builds a message by walking its schema, which is how every
// message outside the four templates is produced.
func generateFromSchema(msgType string) error {
	idx, err := loadCatalog()
	if err != nil {
		return notInstalled(msgType, "generate this message from its schema", err)
	}

	schemaPath, resolved := resolveSchemaPath(idx, msgType)
	if schemaPath == "" {
		return notInstalled(msgType, "generate this message from its schema", nil)
	}
	if err := catalog.CheckEvicted(schemaPath); err != nil {
		return err
	}

	schema, err := xsd.ParseFile(schemaPath)
	if err != nil {
		return fmt.Errorf("parsing %s: %w", schemaPath, err)
	}

	opts := schemagen.DefaultOptions()
	opts.Optional = genOptional
	opts.Repeats = genRepeats
	opts.Values = map[string]string{}
	if genAmount != "" {
		opts.Values["InstdAmt"] = genAmount
		opts.Values["IntrBkSttlmAmt"] = genAmount
	}
	if genCurrency != "" {
		opts.Values["Ccy"] = genCurrency
	}
	if genDebtor != "" {
		opts.Values["Nm"] = genDebtor
	}
	if genDebtorIBAN != "" {
		opts.Values["IBAN"] = genDebtorIBAN
	}

	res, err := schemagen.Generate(schema, opts)
	if err != nil {
		return err
	}

	if genCopy {
		if err := clipboard.WriteAll(res.XML); err == nil {
			fmt.Printf("\n%s Copied generated %s payload to clipboard!\n\n",
				badgeStyle.Render(" COPIED "), resolved)
		}
	}
	if genOutputFile != "" {
		if err := os.WriteFile(genOutputFile, []byte(res.XML+"\n"), 0o600); err != nil {
			return fmt.Errorf("failed to write output file: %w", err)
		}
		fmt.Printf("\n%s %s → %s  (%d elements)\n\n",
			headStyle.Render(" GENERATED "), resolved, genOutputFile, res.Elements)
		printGenNotes(res.Notes)
		return nil
	}

	fmt.Println(res.XML)
	if len(res.Notes) > 0 {
		fmt.Fprintln(os.Stderr)
		printGenNotesTo(os.Stderr, res.Notes)
	}
	return nil
}

// printGenNotes reports the decisions a schema walk had to make. They go to
// stderr when the message goes to stdout, so piping the output stays clean.
func printGenNotes(notes []string) { printGenNotesTo(os.Stdout, notes) }

func printGenNotesTo(w io.Writer, notes []string) {
	if len(notes) == 0 {
		return
	}
	_, _ = fmt.Fprintf(w, "%s the schema left %d decision(s) to the generator:\n", warnMark, len(notes))
	for _, n := range notes {
		_, _ = fmt.Fprintf(w, "   • %s\n", n)
	}
	_, _ = fmt.Fprintln(w)
}

func init() {
	generateCmd.Flags().StringVarP(&genAmount, "amount", "a", "", "Settlement / transfer amount")
	generateCmd.Flags().StringVarP(&genCurrency, "currency", "c", "", "Currency code (e.g. EUR, USD, GBP, CHF)")
	generateCmd.Flags().StringVar(&genDebtor, "debtor", "", "Ordering customer / debtor name")
	generateCmd.Flags().StringVar(&genCreditor, "creditor", "", "Beneficiary customer / creditor name")
	generateCmd.Flags().StringVar(&genDebtorIBAN, "debtor-iban", "", "Debtor IBAN account")
	generateCmd.Flags().StringVar(&genCreditorIBAN, "creditor-iban", "", "Creditor IBAN account")
	generateCmd.Flags().StringVarP(&genPreset, "preset", "p", "standard", "Regional clearing preset (sepa, fednow, target2, chaps, standard)")
	generateCmd.Flags().BoolVar(&genWithBAH, "bah", false, "Wrap generated message in Business Application Header (head.001.001.02)")
	generateCmd.Flags().BoolVarP(&genCopy, "copy", "y", false, "Copy generated XML to system clipboard")
	generateCmd.Flags().StringVarP(&genOutputFile, "output", "o", "", "Write generated XML to file instead of stdout")
	generateCmd.Flags().BoolVar(&genFromSchema, "from-schema", false,
		"Build from the installed schema rather than a template (automatic for messages with no template)")
	generateCmd.Flags().BoolVar(&genOptional, "optional", false,
		"Include elements the schema marks optional, not just the mandatory ones")
	generateCmd.Flags().IntVar(&genRepeats, "repeats", 1,
		"How many times to emit an element that may occur more than once")

	RootCmd.AddCommand(generateCmd)
}
