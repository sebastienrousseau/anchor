// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

// Command lookup answers "what is this message, and what else is like it?"
// entirely offline.
//
// The whole published standard is indexed inside the binary, so this needs no
// catalogue on disk and no network — useful on a locked-down build agent, and
// the reason the website can answer the same questions inside a browser tab.
//
//	go run ./examples/lookup pacs.008
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/sebastienrousseau/askiso/pkg/iso20022"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: lookup <message id or search term>")
		os.Exit(2)
	}
	query := strings.ToLower(os.Args[1])

	std, err := iso20022.Standard()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	hits := std.Search(query)
	if len(hits) == 0 {
		fmt.Printf("nothing in the standard matches %q\n", os.Args[1])
		os.Exit(1)
	}

	fmt.Printf("%d match(es) for %q\n\n", len(hits), os.Args[1])
	for _, m := range hits {
		fmt.Printf("  %-22s %s\n", m.ID, iso20022.DomainName(m.Domain))
	}

	// What converts into this family, if anything does. "Can I get here from an
	// MT message?" is one of the first questions during a migration.
	latest := hits[len(hits)-1]
	if conv, ok := iso20022.TranslateSWIFT(latest.BaseCode); ok {
		fmt.Printf("\nSWIFT MT equivalent: MT%s\n", conv.MTCode)
	}
}
