// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

// Command validate checks a message against the XML Schema for it.
//
// This is the one flow that needs files on disk. AskISO redistributes no
// specification content, so the schemas come from a catalogue you downloaded
// from the Registration Authority. When the schema for a message is not there,
// this says so and names what is missing — rather than reporting the message as
// valid, which would be the same output as a genuine pass.
//
//	go run ./examples/validate payment.xml /path/to/catalogue
package main

import (
	"fmt"
	"os"

	"github.com/sebastienrousseau/askiso/pkg/iso20022"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr,
			"usage: validate <message.xml> <catalogue-dir>\nlooked for a catalogue in:\n")
		for _, d := range iso20022.CatalogueLocations() {
			fmt.Fprintf(os.Stderr, "  %s\n", d)
		}
		os.Exit(2)
	}

	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	msgID, err := iso20022.MessageIDFromInstance(data)
	if err != nil {
		fmt.Fprintf(os.Stderr,
			"this message declares no ISO 20022 namespace, so no schema can be resolved: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("message: %s\n", msgID)

	cat, err := iso20022.OpenCatalogue(os.Args[2])
	if err != nil {
		fmt.Fprintf(os.Stderr, "opening the catalogue: %v\n", err)
		os.Exit(1)
	}

	entry, err := cat.Lookup(msgID)
	if err != nil || !entry.Installed {
		fmt.Fprintf(os.Stderr,
			"the schema for %s is not in this catalogue.\n"+
				"Download the message set from https://www.iso20022.org/ and add it.\n", msgID)
		os.Exit(1)
	}

	res, err := iso20022.ValidateFile(data, entry.SchemaPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if res.Valid {
		fmt.Println("valid against the schema")
		return
	}

	fmt.Printf("%d schema error(s)\n\n", len(res.Errors))
	for _, e := range res.Errors {
		fmt.Printf("  %s\n    at %s\n    expected %s, found %s\n",
			e.Rule, e.Path, e.Expected, e.Actual)
	}
	os.Exit(1)
}
