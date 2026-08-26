// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

// Command cbpr-2026 answers one question: would this message still be accepted
// after 14 November 2026?
//
// From that date CBPR+ requires postal addresses to be structured or hybrid. A
// fully unstructured address is rejected by the receiving institution rather
// than repaired, so this is worth knowing before the message is sent, not after
// the payment fails.
//
//	go run ./examples/cbpr-2026 payment.xml
package main

import (
	"fmt"
	"os"
	"sort"

	"github.com/sebastienrousseau/askiso/pkg/iso20022"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: cbpr-2026 <message.xml>")
		os.Exit(2)
	}

	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// Every address in the message, and the shape it is in. This is the
	// question the November 2026 change turns on, so it is worth seeing
	// directly rather than only through the findings.
	shapes, err := iso20022.ClassifyAddresses(data)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	paths := make([]string, 0, len(shapes))
	for p := range shapes {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	fmt.Printf("%d postal address(es)\n\n", len(paths))
	for _, p := range paths {
		fmt.Printf("  %-12s %s\n", shapes[p], p)
	}

	res, err := iso20022.CheckProfile(data, "cbpr-2026", os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if res.Valid() {
		fmt.Printf("\nReady: %d rule(s) applied, nothing to fix.\n", res.Checked)
		return
	}

	fmt.Printf("\nNot ready: %d finding(s).\n\n", len(res.Findings))
	for _, f := range res.Findings {
		fmt.Printf("  %s at %s\n    %s\n", f.RuleID, f.Path, f.Message)
		if f.Remediation != "" {
			fmt.Printf("    fix: %s\n", f.Remediation)
		}
	}
	os.Exit(1)
}
