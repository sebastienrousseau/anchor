// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/sebastienrousseau/anchor/internal/translator"
	"github.com/spf13/cobra"
)

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate shell autocompletion script for anchor",
	Long: `To load completions:

Bash:
  $ source <(anchor completion bash)
  # To load completions for each session, execute once:
  # Linux:
  $ anchor completion bash > /etc/bash_completion.d/anchor
  # macOS:
  $ anchor completion bash > $(brew --prefix)/etc/bash_completion.d/anchor

Zsh:
  # If shell completion is not already enabled in your environment,
  # you will need to enable it.  You can execute the following once:
  $ echo "autoload -U compinit; compinit" >> ~/.zshrc

  # To load completions for each session, execute once:
  $ anchor completion zsh > "${fpath[1]}/_anchor"

Fish:
  $ anchor completion fish | source
  # To load completions for each session, execute once:
  $ anchor completion fish > ~/.config/fish/completions/anchor.fish

PowerShell:
  PS> anchor completion powershell | Out-String | Invoke-Expression
`,
	DisableFlagsInUseLine: true,
	ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
	// ExactArgs alone does not enforce ValidArgs, so an unsupported shell used
	// to exit 0 having produced nothing.
	Args: cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	RunE: func(cmd *cobra.Command, args []string) error {
		switch args[0] {
		case "bash":
			return RootCmd.GenBashCompletion(os.Stdout)
		case "zsh":
			return RootCmd.GenZshCompletion(os.Stdout)
		case "fish":
			return RootCmd.GenFishCompletion(os.Stdout, true)
		case "powershell":
			return RootCmd.GenPowerShellCompletionWithDesc(os.Stdout)
		}
		return fmt.Errorf("unsupported shell %q (choose bash, zsh, fish, or powershell)", args[0])
	},
}

func completeMessageIDs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	idx, err := loadCatalog()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var completions []string
	toLower := strings.ToLower(toComplete)
	for _, m := range idx.Messages {
		if strings.HasPrefix(strings.ToLower(m.ID), toLower) {
			completions = append(completions, m.ID+"\t"+m.Category)
		}
	}
	return completions, cobra.ShellCompDirectiveNoFileComp
}

func completeTranslationCodes(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	var completions []string
	toLower := strings.ToLower(toComplete)
	for _, m := range translator.GetAllMappings() {
		if strings.HasPrefix(strings.ToLower(m.MTCode), toLower) {
			completions = append(completions, m.MTCode+"\t"+m.MTTitle)
		}
		if strings.HasPrefix(strings.ToLower(m.MXCode), toLower) {
			completions = append(completions, m.MXCode+"\t"+m.MXTitle)
		}
	}
	return completions, cobra.ShellCompDirectiveNoFileComp
}

func init() {
	infoCmd.ValidArgsFunction = completeMessageIDs
	sampleCmd.ValidArgsFunction = completeMessageIDs
	schemaCmd.ValidArgsFunction = completeMessageIDs
	diffCmd.ValidArgsFunction = completeMessageIDs
	translateCmd.ValidArgsFunction = completeTranslationCodes

	RootCmd.AddCommand(completionCmd)
}
