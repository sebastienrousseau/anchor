// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

// Package mock runs a local clearing rail so a payment integration can be
// exercised end to end without a counterparty.
//
// It is a test fixture, not a service: it binds loopback by default, bounds
// every request, and carries no persistence beyond a session.
package mock

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/sebastienrousseau/askiso/internal/generator"
	"github.com/sebastienrousseau/askiso/internal/linter"
)

// Limits bound what one request may do.
const (
	// maxBodySize caps an inbound payment. Real messages are kilobytes; a
	// statement request carries no body at all.
	maxBodySize = 8 << 20 // 8 MiB

	readHeaderTimeout = 5 * time.Second
	readTimeout       = 30 * time.Second
	writeTimeout      = 30 * time.Second
	idleTimeout       = 60 * time.Second
)

// Scenario makes the rail behave like a real one having a bad day. Testing the
// unhappy path is the point of a sandbox.
type Scenario string

const (
	// ScenarioNormal accepts anything that passes the business rules.
	ScenarioNormal Scenario = ""
	// ScenarioRejectAC04 rejects every payment with "account closed".
	ScenarioRejectAC04 Scenario = "reject-ac04"
	// ScenarioRejectAC01 rejects with "incorrect account number".
	ScenarioRejectAC01 Scenario = "reject-ac01"
	// ScenarioDelayed accepts but reports settlement as still pending.
	ScenarioDelayed Scenario = "delayed-settlement"
	// ScenarioReturn accepts, then returns the funds as a pacs.004.
	ScenarioReturn Scenario = "return-pacs004"
)

// Scenarios lists the supported names.
func Scenarios() []string {
	return []string{
		string(ScenarioRejectAC01),
		string(ScenarioRejectAC04),
		string(ScenarioDelayed),
		string(ScenarioReturn),
	}
}

// Options configures a server.
type Options struct {
	// Host to bind. Empty means loopback, which is the safe default for a
	// fixture that performs no authentication.
	Host string
	Port int
	// Scenario forces a particular rail behaviour.
	Scenario Scenario
}

// Server is a mock clearing rail.
type Server struct {
	Port     int
	Srv      *http.Server
	scenario Scenario
}

// NewServer creates a mock rail on loopback.
func NewServer(port int) *Server {
	return NewServerWith(Options{Port: port})
}

// NewServerWith creates a mock rail from explicit options.
func NewServerWith(opt Options) *Server {
	s := &Server{Port: opt.Port, scenario: opt.Scenario}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/health", s.handleHealth)
	mux.HandleFunc("/v1/payments", s.handlePayment)
	mux.HandleFunc("/v1/accounts/", s.handleAccountStatement)

	host := opt.Host
	if host == "" {
		// Bind loopback: the rail authenticates nothing, so it must not be
		// reachable from the network unless the operator asks for that.
		host = "127.0.0.1"
	}

	s.Srv = &http.Server{
		Addr:    net.JoinHostPort(host, fmt.Sprintf("%d", opt.Port)),
		Handler: mux,
		// Without these a single slow client can hold a connection open
		// indefinitely, which is the Slowloris shape gosec G112 flags.
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}
	return s
}

// Addr reports the address the server is configured to bind.
func (s *Server) Addr() string { return s.Srv.Addr }

// Start begins listening. It returns nil on a clean shutdown.
func (s *Server) Start() error {
	err := s.Srv.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Shutdown stops the server, letting in-flight requests finish.
func (s *Server) Shutdown(ctx context.Context) error { return s.Srv.Shutdown(ctx) }

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":   "UP",
		"service":  "AskISO ISO 20022 Mock Clearing Rail",
		"scenario": scenarioName(s.scenario),
		"time":     time.Now().UTC().Format(time.RFC3339),
	})
}

func scenarioName(sc Scenario) string {
	if sc == ScenarioNormal {
		return "normal"
	}
	return string(sc)
}

func (s *Server) handlePayment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodySize))
	if err != nil {
		http.Error(w, "Request body too large or unreadable", http.StatusRequestEntityTooLarge)
		return
	}
	if len(body) == 0 {
		http.Error(w, "Empty or invalid body", http.StatusBadRequest)
		return
	}

	wantsJSON := strings.Contains(r.Header.Get("Accept"), "application/json")
	now := time.Now().UTC().Format(time.RFC3339)
	ref := time.Now().Unix()

	// A forced rejection scenario short-circuits the business rules.
	if reason, forced := forcedRejection(s.scenario); forced {
		s.reject(w, wantsJSON, reason, "Rejected by the "+scenarioName(s.scenario)+" scenario", nil, now, ref)
		return
	}

	res, lintErr := linter.Lint(body, "inbound_payment.xml")
	if lintErr != nil {
		s.reject(w, wantsJSON, "FF01", "The message is not well-formed XML", nil, now, ref)
		return
	}
	if res.Errors > 0 {
		reason := reasonForFindings(res)
		s.reject(w, wantsJSON, reason, "Business rule validation failed", res.Issues, now, ref)
		return
	}

	status, detail := "ACSC", "Settlement completed"
	if s.scenario == ScenarioDelayed {
		status, detail = "PDNG", "Accepted; settlement pending"
	}

	if wantsJSON {
		payload := map[string]any{
			"status":      status,
			"description": detail,
			"timestamp":   now,
		}
		if s.scenario == ScenarioReturn {
			payload["follow_up"] = "pacs.004"
		}
		writeJSON(w, http.StatusOK, payload)
		return
	}

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(acceptedReport(status, detail, ref, now)))
}

// reject writes a pacs.002 rejection, or its JSON equivalent.
func (s *Server) reject(w http.ResponseWriter, wantsJSON bool, reason, detail string,
	issues []linter.Issue, now string, ref int64) {

	if wantsJSON {
		payload := map[string]any{
			"status": "RJCT",
			"reason": reason,
			"detail": detail,
		}
		if len(issues) > 0 {
			payload["errors"] = issues
		}
		writeJSON(w, http.StatusUnprocessableEntity, payload)
		return
	}

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusUnprocessableEntity)
	_, _ = w.Write([]byte(rejectedReport(reason, detail, ref, now)))
}

// forcedRejection maps a scenario to the reason code it always returns.
func forcedRejection(sc Scenario) (string, bool) {
	switch sc {
	case ScenarioRejectAC01:
		return "AC01", true
	case ScenarioRejectAC04:
		return "AC04", true
	}
	return "", false
}

// reasonForFindings picks the external status reason that matches what actually
// failed, rather than returning one code for everything.
func reasonForFindings(res *linter.Result) string {
	for _, issue := range res.Issues {
		if issue.Severity != linter.SeverityError {
			continue
		}
		switch {
		case strings.Contains(issue.Rule, "IBAN"):
			// The account identifier is malformed or its checksum fails.
			return "AC01"
		case strings.Contains(issue.Rule, "BIC"):
			// The agent cannot be identified.
			return "RC01"
		case strings.Contains(issue.Rule, "Currency"):
			return "AM11"
		case strings.Contains(issue.Rule, "UETR"):
			return "FF01"
		}
	}
	return "NARR"
}

func acceptedReport(status, detail string, ref int64, now string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.002.001.12">
  <FIToFIPmtStsRpt>
    <GrpHdr>
      <MsgId>MSG-ACCP-%d</MsgId>
      <CreDtTm>%s</CreDtTm>
    </GrpHdr>
    <TxInfAndSts>
      <TxSts>%s</TxSts>
      <StsRsnInf>
        <AddtlInf>%s</AddtlInf>
      </StsRsnInf>
    </TxInfAndSts>
  </FIToFIPmtStsRpt>
</Document>`, ref, now, status, detail)
}

func rejectedReport(reason, detail string, ref int64, now string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.002.001.12">
  <FIToFIPmtStsRpt>
    <GrpHdr>
      <MsgId>MSG-REJECT-%d</MsgId>
      <CreDtTm>%s</CreDtTm>
    </GrpHdr>
    <TxInfAndSts>
      <TxSts>RJCT</TxSts>
      <StsRsnInf>
        <Rsn><Cd>%s</Cd></Rsn>
        <AddtlInf>%s</AddtlInf>
      </StsRsnInf>
    </TxInfAndSts>
  </FIToFIPmtStsRpt>
</Document>`, ref, now, reason, detail)
}

func (s *Server) handleAccountStatement(w http.ResponseWriter, r *http.Request) {
	account := strings.TrimPrefix(r.URL.Path, "/v1/accounts/")
	if i := strings.IndexByte(account, '/'); i >= 0 {
		account = account[:i]
	}
	if account == "" {
		http.Error(w, "Account identifier required in path", http.StatusBadRequest)
		return
	}

	opts := generator.DefaultOptions("camt.053")
	opts.DebtorIBAN = account
	opts.Amount = "125000.00"

	doc, err := generator.Generate(opts)
	if err != nil {
		http.Error(w, "Could not build a statement", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/xml")
	_, _ = w.Write([]byte(doc))
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
