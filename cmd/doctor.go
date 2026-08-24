// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package cmd

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/sebastienrousseau/anchor/internal/catalog"
	"github.com/spf13/cobra"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Run system diagnostics, catalog health checks, and toolchain verification",
	Long: `Doctor performs a comprehensive diagnostic audit of Anchor including 
catalog index integrity, binary cache status, system toolchain dependencies 
(xmllint, clipboard), and local AI LLM connectivity.`,
	Example: `  anchor doctor`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("\n%s Anchor Environment & System Diagnostics\n\n", headStyle.Render(" ANCHOR DOCTOR "))

		var problems int

		// 1. Catalogue integrity. A missing catalogue is a failure, not a note --
		// reporting "0 message definitions" with a tick is how a broken install
		// goes unnoticed.
		idx, err := loadCatalog()
		if err != nil {
			problems++
			fmt.Printf("  %s [Catalogue] not found\n", crossMark)
			for _, line := range strings.Split(strings.TrimSpace(err.Error()), "\n") {
				fmt.Printf("      %s\n", line)
			}
		} else {
			fmt.Printf("  %s [Catalogue] %d message definitions across %d categories\n",
				tickMark, len(idx.Messages), len(idx.Categories))
			fmt.Printf("      %s\n", idx.RootDir)

			if strings.Contains(idx.RootDir, "com~apple~CloudDocs") {
				problems++
				fmt.Printf("  %s [Catalogue] lives in iCloud Drive\n", warnMark)
				fmt.Printf("      macOS may evict these files and replace them with placeholders,\n")
				fmt.Printf("      which breaks validation silently. Move it to:\n")
				fmt.Printf("      %s\n", catalog.DefaultDir())
			}
		}

		// 2. Toolchain Dependencies
		if xmllintPath, err := exec.LookPath("xmllint"); err == nil {
			fmt.Printf("  ✅ [XML Schema Validator] xmllint detected at %s (XXE --nonet enabled)\n", xmllintPath)
		} else {
			fmt.Println("  ⚠️  [XML Schema Validator] xmllint not found; using pure-Go fallback validator")
		}

		// Check clipboard
		hasClipboard := false
		for _, tool := range []string{"pbcopy", "wl-copy", "xclip", "xsel"} {
			if _, err := exec.LookPath(tool); err == nil {
				hasClipboard = true
				fmt.Printf("  ✅ [Clipboard Engine] System clipboard utility detected (%s)\n", tool)
				break
			}
		}
		if !hasClipboard {
			fmt.Println("  ℹ️  [Clipboard Engine] Standard clipboard interface active")
		}

		// 3. AI / LLM Connectivity
		ollamaHost := os.Getenv("OLLAMA_HOST")
		if ollamaHost == "" {
			ollamaHost = "http://localhost:11434"
		}
		checkOllamaConnectivity(ollamaHost)

		openaiKey := os.Getenv("OPENAI_API_KEY")
		if openaiKey != "" {
			fmt.Println("  ✅ [OpenAI API] OPENAI_API_KEY detected in environment")
		} else {
			fmt.Println("  ℹ️  [OpenAI API] OPENAI_API_KEY not set (offline RAG assistant active)")
		}

		fmt.Println()
		if problems > 0 {
			return fmt.Errorf("%d issue(s) need attention", problems)
		}
		fmt.Println("All core systems operating normally! ⚓")
		return nil
	},
}

const (
	tickMark  = "✅"
	crossMark = "❌"
	warnMark  = "⚠️ "
)

func checkOllamaConnectivity(host string) {
	ctx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
	defer cancel()

	url := fmt.Sprintf("%s/api/tags", host)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		fmt.Printf("  ℹ️  [Ollama Assistant] Local Ollama not reachable at %s (offline RAG active)\n", host)
		return
	}

	client := &http.Client{Timeout: 800 * time.Millisecond}
	resp, err := client.Do(req)
	if err == nil && resp.StatusCode == http.StatusOK {
		fmt.Printf("  ✅ [Ollama Assistant] Connected to local Ollama instance at %s\n", host)
		_ = resp.Body.Close()
	} else {
		fmt.Printf("  ℹ️  [Ollama Assistant] Local Ollama not running at %s (offline RAG active)\n", host)
	}
}

func init() {
	RootCmd.AddCommand(doctorCmd)
}
