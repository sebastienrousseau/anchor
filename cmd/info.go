// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package cmd

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/charmbracelet/lipgloss"
	"github.com/sebastienrousseau/anchor/internal/registry"
	"github.com/spf13/cobra"
)

var (
	badgeStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#000000")).Background(lipgloss.Color("#38BDF8")).Padding(0, 1)
	infoJSON   bool
)

var infoCmd = &cobra.Command{
	Use:   "info <message-id>",
	Short: "Display detailed metadata and schema paths for a message",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]

		idx, err := loadCatalog()
		if err != nil {
			// Without a catalogue Anchor still knows the message exists and
			// which set publishes it, which is exactly what the user needs in
			// order to install it.
			return infoFromRegistry(id)
		}

		msg, ok := idx.MessageMap[id]
		if !ok {
			results := idx.Search(id)
			if len(results) > 0 {
				msg = results[0]
				ok = true
			}
		}
		if !ok {
			return fmt.Errorf("message '%s' not found in catalog", id)
		}

		if infoJSON {
			data, err := json.MarshalIndent(msg, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(data))
			return nil
		}

		fmt.Printf("\n%s %s\n\n", badgeStyle.Render(" MESSAGE INFO "), titleStyle.Render(msg.ID))
		fmt.Printf("  • Domain/Category : %s\n", msg.Category)
		fmt.Printf("  • Version         : %s\n", msg.Version)
		fmt.Printf("  • Base Code       : %s\n", msg.BaseCode)
		fmt.Printf("  • XML Schema (XSD): %s\n", msg.XSDPath)
		fmt.Printf("  • Sample XML      : %s\n", msg.XMLSamplePath)
		if len(msg.MDRPaths) > 0 {
			fmt.Println("  • Message Definition Reports:")
			for _, r := range msg.MDRPaths {
				fmt.Printf("    - %s\n", filepath.Base(r))
			}
		}
		fmt.Println()
		return nil
	},
}

// infoFromRegistry reports what the embedded index knows and names the download
// that would make the schema available locally.
func infoFromRegistry(id string) error {
	reg, err := registry.Load()
	if err != nil {
		return err
	}

	msg, ok := reg.Lookup(id)
	if !ok {
		if results := reg.Search(id); len(results) > 0 {
			msg = results[0]
			ok = true
		}
	}
	if !ok {
		return fmt.Errorf("no ISO 20022 message matches %q", id)
	}

	sets := reg.SetsFor(msg.ID)

	if infoJSON {
		payload := struct {
			registry.Message
			Installed bool           `json:"installed"`
			Sets      []registry.Set `json:"message_sets"`
		}{Message: msg, Installed: false, Sets: sets}
		data, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	fmt.Printf("\n%s %s\n\n", badgeStyle.Render(" MESSAGE INFO "), titleStyle.Render(msg.ID))
	fmt.Printf("  • Base Code       : %s\n", msg.BaseCode)
	fmt.Printf("  • Domain          : %s\n", msg.Domain)
	fmt.Printf("  • Schema          : %s\n", subtleStyle.Render("not installed"))

	if len(sets) > 0 {
		fmt.Printf("\n  Published in %d message set(s):\n", len(sets))
		for _, s := range sets {
			fmt.Printf("    - %-52s %s\n", s, subtleStyle.Render(s.DownloadURL()))
		}
		fmt.Printf("\n  Download one from iso20022.org, then:\n")
		fmt.Printf("    anchor catalog add <downloaded.zip>\n")
	}
	fmt.Println()
	return nil
}

func init() {
	infoCmd.Flags().BoolVar(&infoJSON, "json", false, "Output message metadata as JSON")
	RootCmd.AddCommand(infoCmd)
}
