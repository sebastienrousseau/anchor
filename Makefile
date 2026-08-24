# SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
# SPDX-License-Identifier: Apache-2.0 OR MIT

VERSION ?= 0.1.0
BINARY_NAME = anchor
CMD_PATH = ./cmd/anchor
MCP_BINARY = anchor-mcp
MCP_PATH = ./cmd/anchor-mcp
LSP_BINARY = anchor-lsp
LSP_PATH = ./cmd/anchor-lsp
LDFLAGS = -s -w -X github.com/sebastienrousseau/anchor/internal/tui.Version=$(VERSION)
SERVER_LDFLAGS = -s -w -X main.version=$(VERSION)
COVERAGE_FLOOR = 95

.PHONY: all build install test race cover conformance differential fuzz ci fmt vet lint vuln clean run catalog-info web web-test web-serve mcp lsp mcp-check lsp-check servers

all: build

build: servers
	go build -ldflags "$(LDFLAGS)" -o $(BINARY_NAME) $(CMD_PATH)

# The two protocol servers are separate binaries because a client launches each
# and takes over its stdin and stdout; neither can share a process with the TUI.
servers: mcp lsp

mcp:
	go build -ldflags "$(SERVER_LDFLAGS)" -o $(MCP_BINARY) $(MCP_PATH)

lsp:
	go build -ldflags "$(SERVER_LDFLAGS)" -o $(LSP_BINARY) $(LSP_PATH)

# A handshake against the real binary: the protocol is easy to break in ways
# unit tests on the server object do not notice, such as writing to stdout.
mcp-check: mcp
	@printf '%s\n' \
	  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}' \
	  '{"jsonrpc":"2.0","method":"notifications/initialized"}' \
	  '{"jsonrpc":"2.0","id":2,"method":"tools/list"}' \
	  | ./$(MCP_BINARY) \
	  | grep -q '"anchor_translate"' \
	  && echo "mcp: handshake and tools/list ok" \
	  || { echo "mcp: handshake failed"; exit 1; }

# The LSP transport frames messages with headers rather than newlines, and a
# framing bug looks like the server hanging rather than failing. This drives the
# real binary the way an editor does.
lsp-check: lsp
	@body='{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"capabilities":{}}}'; \
	exit_body='{"jsonrpc":"2.0","method":"exit"}'; \
	{ printf 'Content-Length: %d\r\n\r\n%s' $${#body} "$$body"; \
	  printf 'Content-Length: %d\r\n\r\n%s' $${#exit_body} "$$exit_body"; } \
	| ./$(LSP_BINARY) \
	| grep -q 'hoverProvider' \
	&& echo "lsp: handshake and capabilities ok" \
	|| { echo "lsp: handshake failed"; exit 1; }

install:
	go install -ldflags "$(LDFLAGS)" $(CMD_PATH)
	go install -ldflags "$(SERVER_LDFLAGS)" $(MCP_PATH)
	go install -ldflags "$(SERVER_LDFLAGS)" $(LSP_PATH)

test:
	go test ./...

race:
	go test -race ./...

# -coverpkg=./... credits code executed by any package's tests, not just its
# own, which is the honest measure for a project where the CLI drives the
# internals.
cover:
	go test -coverpkg=./... -coverprofile=coverage.out -covermode=atomic ./...
	@go tool cover -func=coverage.out | tail -1
	@pct=$$(go tool cover -func=coverage.out | tail -1 | grep -oE '[0-9]+\.[0-9]+'); \
	awk -v p="$$pct" -v f="$(COVERAGE_FLOOR)" \
	  'BEGIN { if (p+0 < f+0) { printf "coverage %s%% is below floor %s%%\n", p, f; exit 1 } }'

# Schema conformance needs a catalogue and xmllint; both are absent on a clean
# CI runner, so these tests skip there. Run this locally before tagging.
# Differential agreement with libxml2 across every schema in the catalogue: the
# correctness bar for a validator written from scratch.
differential:
	@command -v xmllint >/dev/null || { echo "xmllint not found - install libxml2"; exit 1; }
	ANCHOR_DIFF_LIMIT=0 go test ./internal/validator/ -run Differential -v -timeout 30m

# The parsers all take input nobody vetted: a schema the user downloaded, a
# message that arrived over a wire, an MT file from another bank's system. These
# targets assert the one property that matters -- they always return -- and have
# already found one off-by-one slice that would have crashed the CLI.
FUZZTIME ?= 60s
fuzz:
	go test ./internal/xsd/       -run '^$$' -fuzz FuzzParse     -fuzztime $(FUZZTIME)
	go test ./internal/validator/ -run '^$$' -fuzz FuzzValidate  -fuzztime $(FUZZTIME)
	go test ./internal/swift/     -run '^$$' -fuzz FuzzParse     -fuzztime $(FUZZTIME)
	go test ./internal/converter/ -run '^$$' -fuzz FuzzRoundTrip -fuzztime $(FUZZTIME)

conformance:
	@command -v xmllint >/dev/null || { echo "xmllint not found - install libxml2"; exit 1; }
	@anchor_catalog=$${ANCHOR_CATALOG:-$$HOME/Library/Application Support/anchor/catalog}; \
	test -d "$$anchor_catalog" || { echo "no catalogue at $$anchor_catalog"; echo "set ANCHOR_CATALOG or run: anchor catalog add <zip>"; exit 1; }; \
	echo "Catalogue: $$anchor_catalog"; \
	ANCHOR_CATALOG="$$anchor_catalog" go test ./internal/generator/ -run 'Schema|Linter|BAH|RoundTrip' -v; \
	ANCHOR_CATALOG="$$anchor_catalog" go test ./internal/swift/ -run 'ConvertedMessagesValidate' -v; \
	ANCHOR_CATALOG="$$anchor_catalog" ANCHOR_GEN_LIMIT=0 go test ./internal/schemagen/ -run 'Installed|LintClean' -v -timeout 20m; \
	ANCHOR_CATALOG="$$anchor_catalog" go test ./internal/validator/ -run 'StreamingAgrees' -v

# --- website ---------------------------------------------------------------
# The site is pkg/iso20022 compiled to WebAssembly, so the browser runs exactly
# the same engine as the CLI. It ships no schemas: light mode only.
web:
	GOOS=js GOARCH=wasm go build -o web/site/anchor.wasm ./web/wasm
	@cp "$$(go env GOROOT)/lib/wasm/wasm_exec.js" web/site/ 2>/dev/null || \
	 cp "$$(go env GOROOT)/misc/wasm/wasm_exec.js" web/site/ 2>/dev/null || \
	 { echo "could not find wasm_exec.js in GOROOT"; exit 1; }
	@printf 'wasm: %s (%s gzipped)\n' \
	  "$$(du -h web/site/anchor.wasm | cut -f1)" \
	  "$$(gzip -9 -c web/site/anchor.wasm | wc -c | awk '{printf "%.1fM", $$1/1048576}')"

web-test: web
	@command -v node >/dev/null || { echo "node is required for the wasm smoke test"; exit 1; }
	node web/wasm/smoke_test.mjs

web-serve: web
	@echo "http://127.0.0.1:8765"
	@cd web/site && python3 -m http.server 8765

fmt:
	gofmt -s -w cmd/ internal/ pkg/ scripts/ web/

vet:
	go vet ./...

lint:
	@command -v golangci-lint >/dev/null || { echo "golangci-lint not installed"; exit 1; }
	golangci-lint run --max-issues-per-linter 0 --max-same-issues 0

vuln:
	@command -v govulncheck >/dev/null || go install golang.org/x/vuln/cmd/govulncheck@latest
	govulncheck ./...

# The full gate CI runs, minus the catalogue-dependent conformance suite.
ci: fmt vet lint test cover vuln build web-test mcp-check lsp-check

catalog-info:
	@./$(BINARY_NAME) doctor || true

clean:
	rm -f $(BINARY_NAME) $(MCP_BINARY) $(LSP_BINARY) coverage.out coverage.html web/site/anchor.wasm web/site/wasm_exec.js
