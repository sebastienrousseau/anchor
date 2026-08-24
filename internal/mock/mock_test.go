// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package mock

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sebastienrousseau/anchor/internal/generator"
)

func TestHealthEndpoint(t *testing.T) {
	srv := NewServer(0)
	req := httptest.NewRequest("GET", "/v1/health", nil)
	w := httptest.NewRecorder()

	srv.handleHealth(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Anchor") {
		t.Errorf("Expected Anchor in body, got %s", w.Body.String())
	}
}

func TestPaymentEndpoint(t *testing.T) {
	srv := NewServer(0)

	// Valid Payment
	validXML, _ := generator.Generate(generator.DefaultOptions("pacs.008"))
	req := httptest.NewRequest("POST", "/v1/payments", bytes.NewReader([]byte(validXML)))
	w := httptest.NewRecorder()

	srv.handlePayment(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for valid payment, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "ACCP") {
		t.Errorf("Expected ACCP status in response, got %s", w.Body.String())
	}
}
