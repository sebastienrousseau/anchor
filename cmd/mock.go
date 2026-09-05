// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/sebastienrousseau/askiso/internal/mock"
	"github.com/spf13/cobra"
)

var (
	mockPort     int
	mockHost     string
	mockScenario string
)

// validScenario reports whether the name is one the rail understands.
func validScenario(name string) bool {
	for _, s := range mock.Scenarios() {
		if s == name {
			return true
		}
	}
	return false
}

var mockCmd = &cobra.Command{
	Use:     "mock",
	Aliases: []string{"sandbox", "serve"},
	Short:   "Start a local HTTP mock ISO 20022 clearing rail server for testing",
	Long: `Mock starts an embedded HTTP server simulating a live ISO 20022 clearing network.
Accepts pacs.008 payments, runs semantic validation, and returns synchronous pacs.002 ACK/RJCT.`,
	Example: `  askiso mock --port 8080
	  askiso mock`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if mockPort < 0 || mockPort > 65535 {
			return fmt.Errorf("--port must be between 0 and 65535")
		}
		if mockScenario != "" && !validScenario(mockScenario) {
			return fmt.Errorf("unknown scenario %q (available: %s)",
				mockScenario, strings.Join(mock.Scenarios(), ", "))
		}

		srv := mock.NewServerWith(mock.Options{
			Host:     mockHost,
			Port:     mockPort,
			Scenario: mock.Scenario(mockScenario),
		})

		fmt.Printf("\n%s Mock ISO 20022 Clearing Rail\n\n", headStyle.Render(" ASKISO MOCK SANDBOX "))
		fmt.Printf("  • Listening on   : http://%s\n", srv.Addr())
		fmt.Printf("  • Health Check   : GET  /v1/health\n")
		fmt.Printf("  • Submit Payment : POST /v1/payments   (pacs.008 ➔ pacs.002)\n")
		fmt.Printf("  • Statements     : GET  /v1/accounts/{account}   (camt.053)\n")
		if mockScenario != "" {
			fmt.Printf("  • Scenario       : %s\n", titleStyle.Render(mockScenario))
		}
		if mockHost == "" {
			fmt.Printf("\n  %s\n", subtleStyle.Render(
				"Bound to loopback. The rail authenticates nothing; use --host to expose it deliberately."))
		}
		fmt.Println("\nPress Ctrl+C to stop.")

		// Shut down cleanly on interrupt so in-flight requests finish.
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
		defer signal.Stop(stop)

		return serveMockUntilSignal(srv, stop)
	},
}

type mockLifecycle interface {
	Start() error
	Shutdown(context.Context) error
}

func serveMockUntilSignal(srv mockLifecycle, stop <-chan os.Signal) error {
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start() }()

	select {
	case err := <-errCh:
		return err
	case <-stop:
		fmt.Println("\nShutting down...")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		shutdownErr := srv.Shutdown(ctx)
		serveErr := <-errCh
		if shutdownErr != nil {
			return shutdownErr
		}
		return serveErr
	}
}

func init() {
	mockCmd.Flags().IntVarP(&mockPort, "port", "p", 8080, "HTTP port for the mock clearing server")
	mockCmd.Flags().StringVar(&mockHost, "host", "",
		"Interface to bind (default: loopback only; the rail performs no authentication)")
	mockCmd.Flags().StringVar(&mockScenario, "scenario", "",
		"Force a rail behaviour ("+strings.Join(mock.Scenarios(), ", ")+")")
	RootCmd.AddCommand(mockCmd)
}
