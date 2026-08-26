# SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
# SPDX-License-Identifier: Apache-2.0 OR MIT

# Versions increment by 0.0.1 and only by 0.0.1: v0.1.0 follows v0.0.999, not
# v0.0.9. The slow climb is the point — maturity is earned across releases
# rather than declared by a version number. See CONTRIBUTING.md.
VERSION ?= 0.0.1
BINARY_NAME = askiso
CMD_PATH = ./cmd/askiso
MCP_BINARY = askiso-mcp
MCP_PATH = ./cmd/askiso-mcp
LSP_BINARY = askiso-lsp
LSP_PATH = ./cmd/askiso-lsp
LDFLAGS = -s -w -X github.com/sebastienrousseau/askiso/internal/tui.Version=$(VERSION)
SERVER_LDFLAGS = -s -w -X main.version=$(VERSION)
WASM_LDFLAGS = -s -w -X main.buildVersion=$(VERSION)
# 95.5, not 95 and not 98. The measurement is taken on a runner with no
# catalogue installed, which is the honest environment but also the one where
# the terminal UI, the browser opener and the AI client cannot run at all.
#
# What is left uncovered is 427 statements in 377 blocks across 63 files, and
# 338 of those blocks are a single `if err != nil { return err }`. Reaching 98%
# would mean contriving roughly 239 individual failure injections to assert
# that errors propagate — tests that make the suite slower and more brittle
# without making the tool more correct. The floor is set where it protects
# against regression rather than where it forces that work.
COVERAGE_FLOOR = 95.5

.PHONY: all build install test race cover conformance differential fuzz ci fmt vet lint vuln clean run catalog-info web web-test web-serve wasm sessions sessions-record links mcp lsp mcp-check lsp-check servers

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
	  | grep -q '"askiso_translate"' \
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
	ASKISO_DIFF_LIMIT=0 go test ./internal/validator/ -run Differential -v -timeout 30m

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
	@askiso_catalog=$${ASKISO_CATALOG:-$$HOME/Library/Application Support/askiso/catalog}; \
	test -d "$$askiso_catalog" || { echo "no catalogue at $$askiso_catalog"; echo "set ASKISO_CATALOG or run: askiso catalog add <zip>"; exit 1; }; \
	echo "Catalogue: $$askiso_catalog"; \
	ASKISO_CATALOG="$$askiso_catalog" go test ./internal/generator/ -run 'Schema|Linter|BAH|RoundTrip' -v; \
	ASKISO_CATALOG="$$askiso_catalog" go test ./internal/swift/ -run 'ConvertedMessagesValidate' -v; \
	ASKISO_CATALOG="$$askiso_catalog" ASKISO_GEN_LIMIT=0 go test ./internal/schemagen/ -run 'Installed|LintClean' -v -timeout 20m; \
	ASKISO_CATALOG="$$askiso_catalog" go test ./internal/validator/ -run 'StreamingAgrees' -v

# The terminal sessions on the website are executable. Every ```console block
# whose commands are `askiso` is replayed against testdata/sessions and its
# recorded output compared with what the binary actually writes, so the site
# cannot go on showing output the tool stopped producing.
links:
	@test -d $(WEB_OUT) || { echo "build the site first: make web"; exit 1; }
	python3 scripts/linkcheck.py $(WEB_OUT)

sessions:
	go run ./scripts/sessions

sessions-record:
	go run ./scripts/sessions -record

# --- website ---------------------------------------------------------------
# askiso.io is content built by ssg plus pkg/iso20022 compiled to WebAssembly,
# so the browser runs exactly the same engine as the CLI. It ships no schemas:
# light mode only.
#
# `wasm` is the bundle alone — the smoke test needs only that, and building it
# without ssg keeps `make ci` free of a Rust dependency.
WEB_OUT = web/public

wasm:
	@mkdir -p $(WEB_OUT)
	GOOS=js GOARCH=wasm go build -ldflags "$(WASM_LDFLAGS)" -o $(WEB_OUT)/askiso.wasm ./web/wasm
	@# -f matters: the source lives in the read-only module cache, so the copy
	@# it leaves behind is read-only too and a second build cannot overwrite it.
	@cp -f "$$(go env GOROOT)/lib/wasm/wasm_exec.js" $(WEB_OUT)/ 2>/dev/null || \
	 cp -f "$$(go env GOROOT)/misc/wasm/wasm_exec.js" $(WEB_OUT)/ 2>/dev/null || \
	 { echo "could not find wasm_exec.js in GOROOT"; exit 1; }
	@chmod u+w $(WEB_OUT)/wasm_exec.js
	@printf 'wasm: %s (%s gzipped)\n' \
	  "$$(du -h $(WEB_OUT)/askiso.wasm | cut -f1)" \
	  "$$(gzip -9 -c $(WEB_OUT)/askiso.wasm | wc -c | awk '{printf "%.1fM", $$1/1048576}')"

# ssg does not copy its template-directory assets into the output — the theme
# suite's own build script copies them explicitly, and so must this one.
#
# ssg runs first and the WebAssembly bundle is built into the result afterwards:
# the content build clears its output directory, so a bundle written before it
# is deleted rather than published.
web:
	@command -v ssg >/dev/null || { echo "ssg is required: cargo install ssg"; exit 1; }
	@# Always from empty. ssg keeps a plugin cache in its output directory and
	@# an incremental run skips the agentic-discovery files, so a local rebuild
	@# would otherwise produce a different site from CI, which always starts on
	@# a fresh checkout. Reproducibility is worth the second of build time.
	@rm -rf $(WEB_OUT)
	@# One page per message definition, generated from the embedded registry.
	@# They are derived data, so they are not tracked — regenerating is a
	@# second of work and a stale copy in the tree would be worse than none.
	go run ./scripts/gen-message-pages -out web/content/messages
	ssg build -f web/ssg.toml
	@$(MAKE) --no-print-directory wasm
	@for a in styles.css brand.css playground.css workspace.css main.js theme-init.js deadline.js playground.js catalogue.js evidence.js workspace.js workspace-boot.js terminal.js logo.svg favicon.ico; do \
	  test -f "web/_layouts/$$a" && cp -f "web/_layouts/$$a" "$(WEB_OUT)/$$a"; \
	done
	@# ssg fingerprints its syntax-highlighting stylesheet but emits the page
	@# referencing the bare name, so /highlight.css was a 404 on every page.
	@h=$$(ls $(WEB_OUT)/highlight.*.css 2>/dev/null | head -1); \
	 test -n "$$h" && cp -f "$$h" "$(WEB_OUT)/highlight.css" || true
	@printf 'askiso.io\n' > $(WEB_OUT)/CNAME
	@# Without this GitHub Pages runs its Jekyll filter over the artefact and
	@# drops anything beginning with a dot or an underscore — which silently
	@# removes /.well-known/mcp.json, the file that tells an assistant AskISO
	@# has an MCP server it can use.
	@touch $(WEB_OUT)/.nojekyll
	@# GitHub Pages will not serve a dot directory even with .nojekyll present:
	@# the files are provably in the uploaded artefact and /.well-known/mcp.json
	@# still returns 404. Publish the same manifests at the site root so they
	@# are reachable at all, and point agents.txt at those copies. The canonical
	@# paths stay in place for the day the site moves to a host that serves them.
	@test -f $(WEB_OUT)/.well-known/mcp.json && cp -f $(WEB_OUT)/.well-known/mcp.json $(WEB_OUT)/mcp.json || true
	@test -f $(WEB_OUT)/.well-known/ai-plugin.json && cp -f $(WEB_OUT)/.well-known/ai-plugin.json $(WEB_OUT)/ai-plugin.json || true
	@# ssg writes a copy of the site-level files into every page directory. With
	@# one page that is invisible; with 2,845 it is 110 MB of duplicated
	@# sitemaps, and every one of them is a wrong URL set for that subdirectory
	@# anyway. Keep the copies at the root and drop the rest.
	@for f in sitemap.xml news-sitemap.xml rss.xml robots.txt manifest.json; do \
	  find $(WEB_OUT) -mindepth 2 -name "$$f" -delete; \
	done
	@# Build bookkeeping, not site content: front matter ssg already rendered
	@# into the pages, plus its incremental caches. Publishing it serves nobody
	@# and adds 12 MB to the artefact.
	@rm -rf $(WEB_OUT)/.meta $(WEB_OUT)/.ssg-cache $(WEB_OUT)/.ssg-plugin-cache.json
	@python3 scripts/gen-sitemap.py $(WEB_OUT)
	@printf 'site: %s page(s), %s\n' \
	  "$$(find $(WEB_OUT) -name '*.html' | wc -l | xargs)" \
	  "$$(du -sh $(WEB_OUT) | cut -f1)"

web-test: wasm
	@command -v node >/dev/null || { echo "node is required for the wasm smoke test"; exit 1; }
	node web/wasm/smoke_test.mjs

web-serve: web
	@echo "http://127.0.0.1:8765"
	@cd $(WEB_OUT) && python3 -m http.server 8765

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
ci: fmt vet lint test cover vuln build sessions web-test mcp-check lsp-check

catalog-info:
	@./$(BINARY_NAME) doctor || true

clean:
	rm -f $(BINARY_NAME) $(MCP_BINARY) $(LSP_BINARY) coverage.out coverage.html
	rm -rf $(WEB_OUT)
