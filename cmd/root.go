// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package cmd

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/sebastienrousseau/askiso/internal/catalog"
	"github.com/sebastienrousseau/askiso/internal/tui"
	"github.com/spf13/cobra"
)

var (
	showLogo    bool
	quiet       bool
	catalogPath string
)

// loadCatalog resolves and loads the ISO 20022 catalogue.
//
// AskIso does not ship the catalogue; the user supplies it. Resolution order is
// --catalog, $ASKISO_CATALOG, the platform data directory, then the working
// directory and its parents. A missing catalogue is a hard error carrying the
// command that fixes it -- never an empty result set.
func loadCatalog() (*catalog.Index, error) {
	return catalog.LoadResolved(catalogPath)
}

// RootCmd represents the base command when called without any subcommands.
var RootCmd = &cobra.Command{
	Use:   "askiso",
	Short: "AskIso — High-performance ISO 20022 Message Explorer & Assistant",
	// Execute prints the error; usage on a runtime failure is noise.
	SilenceUsage:  true,
	SilenceErrors: true,
	Long: `AskIso provides a full-featured Bubble Tea interactive terminal UI,
fuzzy search engine, and local AI assistant for exploring, inspecting, and
validating all 4,746+ ISO 20022 Message Definition Reports (MDRs), Schemas (XSD),
and XML Sample Messages.

Run it bare to browse the catalogue, or put a question straight on the command
line — no quoting needed:

  askiso                                   browse in the terminal UI
  askiso compare pacs.008 and pacs.009     ask the assistant
  askiso validate payment.xml              run a command`,

	// Free text is allowed so a question needs no subcommand and no quotes.
	// Cobra still resolves a real subcommand before this ever runs, so
	// `askiso validate x.xml` remains the validate command.
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// A bare invocation browses; anything else is a question. Words that
		// are not a known subcommand fall through to here, which is what lets
		// the query be typed unquoted.
		if len(args) > 0 {
			return runAsk(cmd, args)
		}

		idx, err := loadCatalog()
		if err != nil {
			return err
		}

		return tui.RunSelector(context.Background(), idx)
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	os.Exit(Run(os.Stderr))
}

// Run executes the command tree and returns the process exit code, writing any
// diagnostic to errOut. Keeping the exit out of here is what lets a test drive
// the whole program.
func Run(errOut io.Writer) int {
	err := RootCmd.Execute()
	if err == nil {
		return 0
	}
	// A command that has already printed its diagnostics signals failure through
	// the exit code alone.
	if err.Error() != "" {
		_, _ = fmt.Fprintf(errOut, "Error: %v\n", err)
	}
	return 1
}

func init() {
	RootCmd.PersistentFlags().StringVar(&catalogPath, "catalog", "", "Path to the ISO 20022 catalogue (overrides $ASKISO_CATALOG)")
	RootCmd.PersistentFlags().BoolVar(&showLogo, "logo", true, "Display ASCII AskIso logo banner")
	RootCmd.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, "Suppress banner and non-essential output")

	helpTemplate := tui.GetStyledLogo() + `{{.Long}}

Usage:{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

Aliases:
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

Examples:
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}{{$cmds := .Commands}}{{if eq (len .Groups) 0}}

Available Commands:{{range $cmds}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{else}}{{range $group := .Groups}}

{{.Title}}{{range $cmds}}{{if (and (eq .GroupID $group.ID) (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if not .AllChildCommandsHaveGroup}}

Additional Commands:{{range $cmds}}{{if (and (eq .GroupID "") (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

Global Flags:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

Additional help topics:{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}
`
	RootCmd.SetHelpTemplate(helpTemplate)
}
