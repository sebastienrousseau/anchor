// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

// Command mt-to-mx converts a SWIFT MT message to ISO 20022 and prints the
// fidelity report alongside it.
//
// The report is the point. Conversion between MT and MX is lossy by nature:
// fields are derived, truncated, or have no equivalent at all. A converter that
// hands back only the output is asking you to assume nothing was lost.
//
//	go run ./examples/mt-to-mx payment.mt103
package main

import (
	"fmt"
	"os"

	"github.com/sebastienrousseau/askiso/pkg/iso20022"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: mt-to-mx <message.mt>\nsupported: %v\n",
			iso20022.TranslatableMT())
		os.Exit(2)
	}

	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// An unsupported MT type is refused rather than guessed at. That refusal is
	// a feature: a plausible-looking conversion nobody verified is worse than
	// no conversion.
	conv, err := iso20022.TranslateMT(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot convert: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(conv.XML)
	fmt.Fprintf(os.Stderr, "\n--- fidelity: %d entry(s) ---\n", len(conv.Report))
	for _, r := range conv.Report {
		fmt.Fprintf(os.Stderr, "%-10s %-6s %-28s %s\n", r.Fidelity, r.Tag, r.Path, r.Note)
	}
}
