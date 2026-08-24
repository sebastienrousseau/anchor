// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sebastienrousseau/askiso/internal/converter"
	"github.com/sebastienrousseau/askiso/internal/linter"
	"github.com/sebastienrousseau/askiso/internal/rules"
	"github.com/sebastienrousseau/askiso/pkg/iso20022"
	"github.com/spf13/cobra"
)

var (
	lintProfile string
	lintFormat  string
	lintJSON    bool
	lintStrict  bool
)

var lintCmd = &cobra.Command{
	Use:   "lint <xml-file>",
	Short: "Lint ISO 20022 message instances against semantic business rules",
	Long: `Lint evaluates ISO 20022 XML messages against rigorous semantic business rules 
including ISO 13616 IBAN Modulo-97 checksums, ISO 9362 BIC validation, ISO 4217 
currency decimal precision, and RFC 4122 UUIDv4 UETR formats.`,
	Example: `  askiso lint sample.xml
  askiso lint payload.xml --strict
  askiso lint sample.xml --json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		filePath := filepath.Clean(args[0])
		data, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("failed to read file '%s': %w", filePath, err)
		}

		res, err := linter.Lint(data, filePath)
		if err != nil {
			return fmt.Errorf("lint error: %w", err)
		}

		// Scheme-level rules run on top of the business-rule checks.
		var ruleRes *rules.Result
		if lintProfile != "" {
			ruleRes, err = runProfile(data, filePath, lintProfile)
			if err != nil {
				return err
			}
		}

		if lintFormat == "sarif" {
			if ruleRes == nil {
				return fmt.Errorf("--format sarif needs a rule profile; add --profile %s",
					rules.Names()[0])
			}
			if err := rules.WriteSARIF(os.Stdout, ruleRes); err != nil {
				return err
			}
			if lintFailed(res, ruleRes) {
				return errSilent
			}
			return nil
		}

		if lintJSON {
			payload := struct {
				*linter.Result
				Profile *rules.Result `json:"profile,omitempty"`
			}{Result: res, Profile: ruleRes}
			out, err := json.MarshalIndent(payload, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(out))
			// Signal failure through the exit code without printing again;
			// os.Exit here would skip deferred cleanup and make the command
			// impossible to drive from a test or another Go program.
			if lintFailed(res, ruleRes) {
				return errSilent
			}
			return nil
		}

		fmt.Printf("\n%s Semantic Business Rule Linter: %s\n\n", headStyle.Render(" LINTER "), titleStyle.Render(filepath.Base(filePath)))

		if len(res.Issues) == 0 {
			fmt.Printf("  ✅ All %d semantic checks passed with zero issues!\n", res.Passed)
			fmt.Println("     • IBAN Modulo-97 Checksums : Verified")
			fmt.Println("     • BIC / SWIFT Structure   : Verified")
			fmt.Println("     • ISO 4217 Decimals       : Verified")
			fmt.Println("     • RFC 4122 UUIDv4 UETR    : Verified")
			fmt.Println()

			// The profile runs on top of the business rules, so its findings
			// must still be reported when the business rules are clean.
			printProfileFindings(ruleRes)
			if lintFailed(res, ruleRes) {
				return fmt.Errorf("profile %s found %d error(s)", lintProfile, ruleRes.Errors)
			}
			return nil
		}

		for _, issue := range res.Issues {
			switch issue.Severity {
			case linter.SeverityError:
				fmt.Printf("  ❌ [%s] %s\n", issue.Rule, issue.Message)
				fmt.Printf("     Field: %s | Value: '%s'\n\n", issue.Field, issue.Value)
			case linter.SeverityWarning:
				fmt.Printf("  ⚠️  [%s] %s\n", issue.Rule, issue.Message)
				fmt.Printf("     Field: %s | Value: '%s'\n\n", issue.Field, issue.Value)
			}
		}

		printProfileFindings(ruleRes)

		total := res.Errors
		if ruleRes != nil {
			total += ruleRes.Errors
		}
		fmt.Printf("  Summary: %d passed, %d error(s), %d warning(s)\n\n",
			res.Passed, total, res.Warnings)

		if lintFailed(res, ruleRes) {
			return fmt.Errorf("linter found %d error(s)", total)
		}

		return nil
	},
}

// runProfile parses the message and applies a named rule profile.
func runProfile(data []byte, filePath, profile string) (*rules.Result, error) {
	p, err := rules.Get(profile)
	if err != nil {
		return nil, err
	}
	root, err := converter.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", filepath.Base(filePath), err)
	}
	msgID, _ := iso20022.MessageIDFromInstance(data)
	return rules.Run(p, root, msgID, filePath), nil
}

// printProfileFindings renders scheme-level findings beneath the business rules.
func printProfileFindings(res *rules.Result) {
	if res == nil {
		return
	}

	fmt.Printf("  %s %s — %s\n\n",
		badgeStyle.Render(" PROFILE "), titleStyle.Render(res.Profile), rules.Describe(res.Profile))

	if len(res.Findings) == 0 {
		if res.Checked == 0 {
			// Saying "passed" would imply the rules were evaluated.
			fmt.Printf("  %s this message type is out of scope: all %d rule(s) skipped.\n\n",
				subtleStyle.Render("exempt —"), res.Skipped)
			return
		}
		fmt.Printf("  ✅ All %d profile rule(s) passed.\n", res.Checked)
		if res.Skipped > 0 {
			fmt.Printf("     %s\n", subtleStyle.Render(
				fmt.Sprintf("%d rule(s) do not apply to this message type", res.Skipped)))
		}
		fmt.Println()
		return
	}

	for _, f := range res.Findings {
		icon := "❌"
		switch f.Severity {
		case rules.SeverityWarning:
			icon = "⚠️ "
		case rules.SeverityInfo:
			icon = "ℹ️ "
		}
		fmt.Printf("  %s [%s] %s\n", icon, f.RuleID, f.Message)
		fmt.Printf("     at       %s\n", subtleStyle.Render(f.Path))
		if f.Expected != "" {
			fmt.Printf("     expected %s\n", subtleStyle.Render(f.Expected))
		}
		if f.Remediation != "" {
			fmt.Printf("     fix      %s\n", subtleStyle.Render(f.Remediation))
		}
		fmt.Println()
	}

	if res.Skipped > 0 {
		fmt.Printf("  %s\n\n", subtleStyle.Render(
			fmt.Sprintf("%d rule(s) do not apply to this message type", res.Skipped)))
	}
}

// lintFailed reports whether the command should exit non-zero.
func lintFailed(res *linter.Result, profile *rules.Result) bool {
	if res.Errors > 0 || (lintStrict && res.Warnings > 0) {
		return true
	}
	if profile == nil {
		return false
	}
	return profile.Errors > 0 || (lintStrict && profile.Warnings > 0)
}

func init() {
	lintCmd.Flags().StringVar(&lintFormat, "format", "text",
		"Output format: text, json, or sarif (sarif uploads to GitHub code scanning)")
	lintCmd.Flags().StringVar(&lintProfile, "profile", "",
		"Scheme rule profile to apply ("+strings.Join(rules.Names(), ", ")+")")
	lintCmd.Flags().BoolVar(&lintJSON, "json", false, "Output lint results as JSON")
	lintCmd.Flags().BoolVar(&lintStrict, "strict", false, "Treat warnings as errors")
	RootCmd.AddCommand(lintCmd)
}
