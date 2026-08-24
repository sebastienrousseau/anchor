// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

// Command askiso-mcp serves AskIso over the Model Context Protocol.
//
// It speaks newline-delimited JSON-RPC 2.0 on stdin and stdout, which is what
// the protocol's stdio transport specifies. Point an MCP client at this binary:
//
//	{
//	  "mcpServers": {
//	    "askiso": { "command": "askiso-mcp" }
//	  }
//	}
//
// Nothing is written to stdout except protocol messages. Diagnostics go to
// stderr, because a stray line on stdout corrupts the stream.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/sebastienrousseau/askiso/internal/mcp"
)

// version is set at build time with -ldflags.
var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("askiso-mcp", flag.ContinueOnError)
	fs.SetOutput(stderr)
	showVersion := fs.Bool("version", false, "print the version and exit")
	listTools := fs.Bool("tools", false, "list the tools this server exposes and exit")

	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, `askiso-mcp serves AskIso over the Model Context Protocol.

It reads JSON-RPC requests from stdin and writes replies to stdout, one per
line. Run it from an MCP client rather than by hand.

Usage:
  askiso-mcp [flags]

Flags:
`)
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	if *showVersion {
		_, _ = fmt.Fprintf(stdout, "askiso-mcp %s\n", version)
		return 0
	}

	server := mcp.New(stdin, stdout, stderr)
	server.SetVersion(version)

	if *listTools {
		_, _ = fmt.Fprintf(stdout, "askiso-mcp %s exposes %d tool(s):\n  %s\n",
			version, len(server.Tools()), strings.Join(server.ToolNames(), "\n  "))
		return 0
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := server.Serve(ctx); err != nil && !errors.Is(err, context.Canceled) {
		_, _ = fmt.Fprintf(stderr, "askiso-mcp: %v\n", err)
		return 1
	}
	return 0
}
