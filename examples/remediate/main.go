// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

// Command remediate shows the self-service flow: hand it a message, get back
// every problem with the rule that found it, the exact field, and what to
// change.
//
// This is the shape most integrations want. A finding without a path is a
// finding somebody has to go hunting for; a finding without a remediation is
// one they have to ask an expert about. Both are printed here because both come
// out of the library.
//
//	go run ./examples/remediate payment.xml
package main

import (
	"fmt"
	"os"

	"github.com/sebastienrousseau/askiso/pkg/iso20022"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: remediate <message.xml>")
		os.Exit(2)
	}

	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// Lint covers the checks that need no schema: checksums, code formats,
	// currency precision, temporal sanity.
	lint, err := iso20022.Lint(data, os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "this does not parse as XML: %v\n", err)
		os.Exit(1)
	}

	for _, issue := range lint.Issues {
		fmt.Printf("%s  %s\n", issue.Severity, issue.Rule)
		fmt.Printf("  %s\n", issue.Message)
		if issue.Path != "" {
			fmt.Printf("  at %s\n", issue.Path)
		}
		if issue.Expected != "" {
			fmt.Printf("  expected %s", issue.Expected)
			if issue.Actual != "" {
				fmt.Printf(", found %s", issue.Actual)
			}
			fmt.Println()
		}
		if issue.Remediation != "" {
			fmt.Printf("  fix: %s\n", issue.Remediation)
		}
		fmt.Println()
	}

	// Scheme rules sit on top of lint: a message can pass every checksum and
	// still be refused by a clearing system.
	rules, err := iso20022.CheckProfile(data, "cbpr-2026", os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "the rule profile could not run: %v\n", err)
		os.Exit(1)
	}

	for _, f := range rules.Findings {
		fmt.Printf("%s  %s\n", f.Severity, f.RuleID)
		fmt.Printf("  %s\n", f.Message)
		fmt.Printf("  at %s\n", f.Path)
		if f.Remediation != "" {
			fmt.Printf("  fix: %s\n", f.Remediation)
		}
		fmt.Println()
	}

	total := lint.Errors + rules.Errors
	fmt.Printf("%d error(s), %d warning(s), %d rule(s) applied\n",
		total, lint.Warnings, rules.Checked)

	// A non-zero exit is what makes this usable from a script or a pipeline.
	if total > 0 {
		os.Exit(1)
	}
}
