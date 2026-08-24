// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

// Command anchor-lsp is a language server for ISO 20022 XML.
//
// It speaks the Language Server Protocol over stdin and stdout, so an editor
// can show the same verdicts the CLI gives: business-rule diagnostics, schema
// validation against the user's own downloaded XSDs, and the CBPR+ rules that
// take effect on 14 November 2026 -- as the message is typed.
//
// Neovim:
//
//	vim.lsp.start({
//	  name = 'anchor',
//	  cmd = { 'anchor-lsp' },
//	  root_dir = vim.fn.getcwd(),
//	})
//
// VS Code and other clients: run the binary with no arguments and connect it to
// XML documents. Nothing is written to stdout except protocol messages.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/sebastienrousseau/anchor/internal/lsp"
	"github.com/sebastienrousseau/anchor/pkg/iso20022"
)

// version is set at build time with -ldflags.
var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("anchor-lsp", flag.ContinueOnError)
	fs.SetOutput(stderr)
	showVersion := fs.Bool("version", false, "print the version and exit")
	profile := fs.String("profile", "cbpr-2026",
		"scheme rule profile applied alongside the linter; empty disables it")

	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, `anchor-lsp is a language server for ISO 20022 XML.

It reads Language Server Protocol messages from stdin and writes replies to
stdout. Run it from an editor rather than by hand.

Usage:
  anchor-lsp [flags]

Flags:
`)
		fs.PrintDefaults()
		_, _ = fmt.Fprintf(stderr, "\nProfiles: %v\n", iso20022.RuleProfiles())
	}

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	if *showVersion {
		_, _ = fmt.Fprintf(stdout, "anchor-lsp %s\n", version)
		return 0
	}

	if *profile != "" {
		if _, err := iso20022.CheckProfile([]byte("<Document/>"), *profile, ""); err != nil {
			_, _ = fmt.Fprintf(stderr, "anchor-lsp: %v\navailable profiles: %v\n",
				err, iso20022.RuleProfiles())
			return 2
		}
	}

	server := lsp.New(stdin, stdout, stderr)
	server.SetVersion(version)
	server.Profile = *profile

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := server.Serve(ctx); err != nil && !errors.Is(err, context.Canceled) {
		_, _ = fmt.Fprintf(stderr, "anchor-lsp: %v\n", err)
		return 1
	}
	return 0
}
