# =============================================================================
# Chef Migration Metrics — Makefile
# =============================================================================
# Build, test, lint, package, version, and functional-test targets for local
# development. See each section's comments for details.
#
# Usage:
#   make help              — show all targets
#   make build             — compile Go binary for the host platform
#   make test              — run all unit tests
#   make lint              — run all linters
#   make package-all       — build RPM, DEB, and distribution archives
#   make bump-patch        — bump patch version and tag
#   make functional-test   — run against real Chef Server orgs from knife creds
# =============================================================================

SHELL := /bin/bash
.DEFAULT_GOAL := help

# ---------------------------------------------------------------------------
# Version — derived from the most recent git tag (vX.Y.Z format)
# ---------------------------------------------------------------------------
# --match 'v[0-9]*' is load-bearing. Without it any tag reachable from HEAD can
# shadow the real version — a docs tag named `specifications-retired-2026-08-04`
# did exactly that on 2026-08-08, and `make bump-minor-push` cheerfully derived
# and PUSHED a tag called `vspecifications-retired-2026-08-04.1.0`. The version
# tags were fine; nothing was reading them.
GIT_TAG       := $(shell git describe --tags --abbrev=0 --match 'v[0-9]*' 2>/dev/null || echo "v0.0.0")
GIT_COMMIT    := $(shell git rev-parse HEAD 2>/dev/null || echo "unknown")
GIT_COMMIT_SHORT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
GIT_DIRTY     := $(shell git diff --quiet 2>/dev/null && echo "" || echo "-dirty")
VERSION       := $(patsubst v%,%,$(GIT_TAG))
VERSION_FULL  := $(VERSION)$(if $(GIT_DIRTY),+dirty,)
BUILD_DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

# Split version into components for bumping
VERSION_MAJOR := $(word 1,$(subst ., ,$(VERSION)))
VERSION_MINOR := $(word 2,$(subst ., ,$(VERSION)))
VERSION_PATCH := $(word 3,$(subst ., ,$(VERSION)))

# ---------------------------------------------------------------------------
# Build settings
# ---------------------------------------------------------------------------
BINARY_NAME   := chef-migration-metrics
MODULE        := $(shell head -1 go.mod 2>/dev/null | awk '{print $$2}')
BUILD_DIR     := build
FRONTEND_DIR  := frontend

LDFLAGS := -X main.version=$(VERSION_FULL) \
           -X main.commit=$(GIT_COMMIT) \
           -X main.buildDate=$(BUILD_DATE)

# Host platform detection
HOST_OS   := $(shell go env GOOS 2>/dev/null || echo linux)
HOST_ARCH := $(shell go env GOARCH 2>/dev/null || echo amd64)

# nFPM
NFPM := $(shell command -v nfpm 2>/dev/null)

# Chef credentials for functional testing
CHEF_CREDENTIALS_FILE ?= $(HOME)/.chef/credentials
CHEF_CONFIG_RB        ?= $(HOME)/.chef/config.rb
CHEF_PROFILE          ?= $(shell cat $(HOME)/.chef/context 2>/dev/null || echo "default")
FUNCTIONAL_TEST_TAGS  := functional

# Colour helpers (disabled when not a terminal)
ifneq ($(TERM),)
  GREEN  := \033[0;32m
  YELLOW := \033[0;33m
  CYAN   := \033[0;36m
  RED    := \033[0;31m
  BOLD   := \033[1m
  RESET  := \033[0m
else
  GREEN  :=
  YELLOW :=
  CYAN   :=
  RED    :=
  BOLD   :=
  RESET  :=
endif

# =============================================================================
# Help
# =============================================================================

.PHONY: help
help: ## Show this help message
	@echo ""
	@echo "$(BOLD)Chef Migration Metrics$(RESET) — development targets"
	@echo ""
	@echo "$(BOLD)Version:$(RESET)  $(VERSION_FULL)"
	@echo "$(BOLD)Commit:$(RESET)   $(GIT_COMMIT_SHORT)$(GIT_DIRTY)"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  $(CYAN)%-28s$(RESET) %s\n", $$1, $$2}'
	@echo ""

# =============================================================================
# Build
# =============================================================================

.PHONY: build
build: build-frontend ## Compile Go binary for the host platform
	@echo "$(GREEN)Building $(BINARY_NAME) $(VERSION_FULL) ($(HOST_OS)/$(HOST_ARCH))...$(RESET)"
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=$(HOST_OS) GOARCH=$(HOST_ARCH) go build \
		-ldflags "$(LDFLAGS)" \
		-o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/chef-migration-metrics/
	@echo "$(GREEN)Binary: $(BUILD_DIR)/$(BINARY_NAME)$(RESET)"

.PHONY: build-linux-amd64
build-linux-amd64: build-frontend ## Cross-compile for linux/amd64
	@echo "$(GREEN)Building $(BINARY_NAME) $(VERSION_FULL) (linux/amd64)...$(RESET)"
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
		-ldflags "$(LDFLAGS)" \
		-o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 ./cmd/chef-migration-metrics/

.PHONY: build-linux-arm64
build-linux-arm64: build-frontend ## Cross-compile for linux/arm64
	@echo "$(GREEN)Building $(BINARY_NAME) $(VERSION_FULL) (linux/arm64)...$(RESET)"
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build \
		-ldflags "$(LDFLAGS)" \
		-o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 ./cmd/chef-migration-metrics/

.PHONY: build-darwin-amd64
build-darwin-amd64: build-frontend ## Cross-compile for macOS/amd64 (Intel)
	@echo "$(GREEN)Building $(BINARY_NAME) $(VERSION_FULL) (darwin/amd64)...$(RESET)"
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build \
		-ldflags "$(LDFLAGS)" \
		-o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 ./cmd/chef-migration-metrics/

.PHONY: build-darwin-arm64
build-darwin-arm64: build-frontend ## Cross-compile for macOS/arm64 (Apple Silicon)
	@echo "$(GREEN)Building $(BINARY_NAME) $(VERSION_FULL) (darwin/arm64)...$(RESET)"
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build \
		-ldflags "$(LDFLAGS)" \
		-o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 ./cmd/chef-migration-metrics/

.PHONY: build-windows-amd64
build-windows-amd64: build-frontend ## Cross-compile for windows/amd64
	@echo "$(GREEN)Building $(BINARY_NAME) $(VERSION_FULL) (windows/amd64)...$(RESET)"
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build \
		-ldflags "$(LDFLAGS)" \
		-o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe ./cmd/chef-migration-metrics/

.PHONY: build-windows-arm64
build-windows-arm64: build-frontend ## Cross-compile for windows/arm64
	@echo "$(GREEN)Building $(BINARY_NAME) $(VERSION_FULL) (windows/arm64)...$(RESET)"
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build \
		-ldflags "$(LDFLAGS)" \
		-o $(BUILD_DIR)/$(BINARY_NAME)-windows-arm64.exe ./cmd/chef-migration-metrics/

.PHONY: build-all
build-all: build-linux-amd64 build-linux-arm64 build-darwin-amd64 build-darwin-arm64 build-windows-amd64 build-windows-arm64 ## Cross-compile for all supported platforms

.PHONY: build-frontend
build-frontend: ## Build the React SPA frontend (creates placeholder dist/ if npm unavailable)
	@if [ -d "$(FRONTEND_DIR)" ] && [ -f "$(FRONTEND_DIR)/package.json" ] && command -v npm >/dev/null 2>&1; then \
		echo "$(GREEN)Building frontend...$(RESET)"; \
		cd $(FRONTEND_DIR) && npm install --prefer-offline && npm run build; \
	else \
		echo "$(YELLOW)npm not found or frontend/ missing — creating placeholder dist/$(RESET)"; \
	fi
	@# Touch embed.go so that Go's build cache sees the embedding source file
	@# as changed and re-embeds the fresh dist/ contents into the binary.
	@touch $(FRONTEND_DIR)/embed.go
	@# Ensure dist/ always exists with at least a placeholder index.html so
	@# that the go:embed directive in frontend/embed.go succeeds at compile time.
	@mkdir -p $(FRONTEND_DIR)/dist
	@if [ ! -f "$(FRONTEND_DIR)/dist/index.html" ]; then \
		echo '<!DOCTYPE html><html lang="en"><head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1.0"><title>Chef Migration Metrics</title><style>body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;display:flex;align-items:center;justify-content:center;min-height:100vh;margin:0;background:#f9fafb;color:#374151}.p{text-align:center;max-width:480px;padding:2rem}h1{font-size:1.25rem;margin-bottom:.5rem}p{color:#6b7280;font-size:.875rem;line-height:1.5}code{background:#f3f4f6;padding:.15em .4em;border-radius:4px;font-size:.8125rem}</style></head><body><div class="p"><h1>Frontend Not Built</h1><p>Build the React SPA: <code>cd frontend &amp;&amp; npm ci &amp;&amp; npm run build</code></p><p>API available at <a href="/api/v1/health">/api/v1/health</a></p></div></body></html>' \
		> "$(FRONTEND_DIR)/dist/index.html"; \
	fi

# =============================================================================
# Test
# =============================================================================

.PHONY: test
test: ## Run all Go unit tests with race detection
	@echo "$(GREEN)Running Go unit tests...$(RESET)"
	go test -race -coverprofile=$(BUILD_DIR)/coverage.out ./...
	@echo "$(GREEN)Coverage report: $(BUILD_DIR)/coverage.out$(RESET)"

.PHONY: test-verbose
test-verbose: ## Run all Go unit tests with verbose output
	go test -race -v -coverprofile=$(BUILD_DIR)/coverage.out ./...

.PHONY: test-short
test-short: ## Run only short/fast Go unit tests
	go test -short -race ./...

.PHONY: test-frontend
test-frontend: ## Run frontend unit tests
	@if [ -d "$(FRONTEND_DIR)" ] && [ -f "$(FRONTEND_DIR)/package.json" ]; then \
		echo "$(GREEN)Running frontend tests...$(RESET)"; \
		cd $(FRONTEND_DIR) && npm ci --prefer-offline && npm test; \
	else \
		echo "$(YELLOW)Skipping frontend tests — $(FRONTEND_DIR)/ not found$(RESET)"; \
	fi
	@# NOTE: no --coverage — @vitest/coverage-v8 is not a declared devDependency,
	@# so `vitest run --coverage` errored and broke `make ci`. Add that package
	@# (supply-chain checked) and restore --coverage if frontend coverage is wanted.

.PHONY: test-all
test-all: test test-frontend ## Run all unit tests (Go + frontend)

.PHONY: coverage
coverage: test ## Generate and open HTML coverage report
	go tool cover -html=$(BUILD_DIR)/coverage.out -o $(BUILD_DIR)/coverage.html
	@echo "$(GREEN)Coverage HTML: $(BUILD_DIR)/coverage.html$(RESET)"
	@command -v open >/dev/null 2>&1 && open $(BUILD_DIR)/coverage.html || true

# =============================================================================
# Functional Testing (against real Chef Server organisations)
# =============================================================================
# Reads Chef credentials from ~/.chef/credentials (TOML profiles) or
# ~/.chef/config.rb (Ruby) to run integration tests against live Chef
# Server organisations.
#
# Override defaults:
#   make functional-test CHEF_PROFILE=staging
#   make functional-test CHEF_CREDENTIALS_FILE=/path/to/credentials
#   make functional-test CHEF_CONFIG_RB=/path/to/knife.rb
#   make functional-test CHEF_ORG=myorg CHEF_SERVER_URL=https://chef.example.com
#
# Profiles from ~/.chef/credentials are passed to tests via environment
# variables. The Go test suite reads these to configure live API clients.
# =============================================================================

.PHONY: functional-test
functional-test: _resolve-chef-creds ## Run functional tests against a real Chef Server
	@echo "$(GREEN)Running functional tests (profile: $(CHEF_PROFILE))...$(RESET)"
	@echo "  Chef Server URL:  $${CHEF_SERVER_URL:-<from credentials>}"
	@echo "  Client name:      $${CHEF_CLIENT_NAME:-<from credentials>}"
	@echo "  Organisation:     $${CHEF_ORG:-<from credentials>}"
	@echo ""
	CHEF_CREDENTIALS_FILE="$(CHEF_CREDENTIALS_FILE)" \
	CHEF_CONFIG_RB="$(CHEF_CONFIG_RB)" \
	CHEF_PROFILE="$(CHEF_PROFILE)" \
	CHEF_SERVER_URL="$${CHEF_SERVER_URL:-}" \
	CHEF_CLIENT_NAME="$${CHEF_CLIENT_NAME:-}" \
	CHEF_CLIENT_KEY="$${CHEF_CLIENT_KEY:-}" \
	CHEF_ORG="$${CHEF_ORG:-}" \
	go test -race -v -count=1 -tags $(FUNCTIONAL_TEST_TAGS) -run 'TestFunctional' ./...

# =============================================================================
# SQL Server for the database ownership ingest
#
# No arm64 image exists, so it runs under emulation on Apple Silicon: slower to
# start, but it works. A permanent Linux VM in the Proxmox lab is the better
# long-term home (MVP2).
#
#   make mssql-up      # start it and wait until it answers
#   make seed-mssql    # create the cmdb database and its sample data
#   make test-mssql    # run the SQL Server functional tests against it
#   make mssql-down    # stop it
# =============================================================================

MSSQL_SA_PASSWORD ?= Cmm_Dev_Password_2026!
MSSQL_DSN         ?= sqlserver://sa:$(MSSQL_SA_PASSWORD)@localhost:1433?database=cmdb
COMPOSE_FILE      := deploy/docker-compose/docker-compose.yml

.PHONY: mssql-up
mssql-up: ## Start the SQL Server container for the ownership ingest
	@echo "$(GREEN)Starting SQL Server (emulated on Apple Silicon, give it a moment)...$(RESET)"
	docker compose -f $(COMPOSE_FILE) --profile mssql up -d mssql
	@printf "Waiting for it to answer"
	@for i in $$(seq 1 60); do \
		if docker compose -f $(COMPOSE_FILE) exec -T mssql /opt/mssql-tools18/bin/sqlcmd \
			-S localhost -U sa -P "$(MSSQL_SA_PASSWORD)" -C -Q "SELECT 1" >/dev/null 2>&1; then \
			echo " ready."; exit 0; fi; \
		printf "."; sleep 2; \
	done; \
	echo " gave up after 2 minutes."; exit 1

.PHONY: seed-mssql
seed-mssql: ## Create the sample ownership database in SQL Server
	@echo "$(GREEN)Seeding the sample ownership database...$(RESET)"
	docker compose -f $(COMPOSE_FILE) exec -T mssql /opt/mssql-tools18/bin/sqlcmd \
		-S localhost -U sa -P "$(MSSQL_SA_PASSWORD)" -C \
		< deploy/docker-compose/seed-mssql.sql
	@echo "$(GREEN)Seeded. DSN: $(MSSQL_DSN)$(RESET)"

.PHONY: test-mssql
test-mssql: ## Run the SQL Server ownership-ingest functional tests
	CMM_TEST_MSSQL_DSN="$(MSSQL_DSN)" \
	go test -count=1 -tags $(FUNCTIONAL_TEST_TAGS) -run 'TestFunctional_MSSQL' -v ./internal/ownershipsql/

.PHONY: mssql-down
mssql-down: ## Stop the SQL Server container
	docker compose -f $(COMPOSE_FILE) --profile mssql down

.PHONY: functional-test-list-profiles
functional-test-list-profiles: ## List available Chef credential profiles
	@echo "$(BOLD)Chef credential profiles$(RESET)"
	@echo ""
	@if [ -f "$(CHEF_CREDENTIALS_FILE)" ]; then \
		echo "$(CYAN)Credentials file:$(RESET) $(CHEF_CREDENTIALS_FILE)"; \
		echo "$(CYAN)Active profile:$(RESET)   $(CHEF_PROFILE)"; \
		echo ""; \
		echo "$(BOLD)Available profiles:$(RESET)"; \
		grep -E '^\[' "$(CHEF_CREDENTIALS_FILE)" | \
			sed "s/\[//g; s/\]//g; s/'//g" | \
			grep -v '\.' | \
			while read -r profile; do \
				server=$$(awk -v p="$$profile" ' \
						/^\[/ { current=$$0; gsub(/[\[\]'"'"']/, "", current) } \
						current==p && /chef_server_url/ { gsub(/.*= *['"'"'"]*/, ""); gsub(/['"'"'"]*$$/, ""); print; exit }' \
						"$(CHEF_CREDENTIALS_FILE)"); \
					client=$$(awk -v p="$$profile" ' \
						/^\[/ { current=$$0; gsub(/[\[\]'"'"']/, "", current) } \
						current==p && /client_name|node_name/ { gsub(/.*= *['"'"'"]*/, ""); gsub(/['"'"'"]*$$/, ""); print; exit }' \
						"$(CHEF_CREDENTIALS_FILE)"); \
				if [ "$$profile" = "$(CHEF_PROFILE)" ]; then \
					printf "  $(GREEN)*$(RESET) $(BOLD)%-20s$(RESET)  client=%-25s  server=%s\n" "$$profile" "$$client" "$$server"; \
				else \
					printf "    %-20s  client=%-25s  server=%s\n" "$$profile" "$$client" "$$server"; \
				fi; \
			done; \
	elif [ -f "$(CHEF_CONFIG_RB)" ]; then \
		echo "$(CYAN)Config file:$(RESET) $(CHEF_CONFIG_RB)"; \
		echo "$(YELLOW)Note: config.rb does not support profiles. Set CHEF_SERVER_URL, CHEF_CLIENT_NAME, CHEF_CLIENT_KEY, and CHEF_ORG directly.$(RESET)"; \
	else \
		echo "$(RED)No Chef credentials found.$(RESET)"; \
		echo ""; \
		echo "Expected one of:"; \
		echo "  $(CHEF_CREDENTIALS_FILE)"; \
		echo "  $(CHEF_CONFIG_RB)"; \
		echo ""; \
		echo "Or set environment variables directly:"; \
		echo "  CHEF_SERVER_URL  CHEF_CLIENT_NAME  CHEF_CLIENT_KEY  CHEF_ORG"; \
	fi
	@echo ""

.PHONY: functional-test-validate
functional-test-validate: _resolve-chef-creds ## Validate Chef credentials without running full tests
	@echo "$(GREEN)Validating Chef credentials (profile: $(CHEF_PROFILE))...$(RESET)"
	CHEF_CREDENTIALS_FILE="$(CHEF_CREDENTIALS_FILE)" \
	CHEF_CONFIG_RB="$(CHEF_CONFIG_RB)" \
	CHEF_PROFILE="$(CHEF_PROFILE)" \
	CHEF_SERVER_URL="$${CHEF_SERVER_URL:-}" \
	CHEF_CLIENT_NAME="$${CHEF_CLIENT_NAME:-}" \
	CHEF_CLIENT_KEY="$${CHEF_CLIENT_KEY:-}" \
	CHEF_ORG="$${CHEF_ORG:-}" \
	go test -race -v -count=1 -tags $(FUNCTIONAL_TEST_TAGS) -run 'TestFunctionalValidateCredentials' ./...

# Internal target: verify that credentials are available before running functional tests
.PHONY: _resolve-chef-creds
_resolve-chef-creds:
	@# Prefer explicit environment variables
	@if [ -n "$${CHEF_SERVER_URL:-}" ] && [ -n "$${CHEF_CLIENT_NAME:-}" ] && [ -n "$${CHEF_CLIENT_KEY:-}" ]; then \
		echo "$(CYAN)Using explicit Chef environment variables$(RESET)"; \
	elif [ -f "$(CHEF_CREDENTIALS_FILE)" ]; then \
		echo "$(CYAN)Using credentials file: $(CHEF_CREDENTIALS_FILE) [$(CHEF_PROFILE)]$(RESET)"; \
	elif [ -f "$(CHEF_CONFIG_RB)" ]; then \
		echo "$(CYAN)Using config.rb: $(CHEF_CONFIG_RB)$(RESET)"; \
	else \
		echo "$(RED)ERROR: No Chef credentials found.$(RESET)" >&2; \
		echo "" >&2; \
		echo "Provide credentials in one of these ways:" >&2; \
		echo "  1. ~/.chef/credentials file (TOML profiles — recommended)" >&2; \
		echo "  2. ~/.chef/config.rb (knife configuration)" >&2; \
		echo "  3. Environment variables:" >&2; \
		echo "       CHEF_SERVER_URL   — https://chef.example.com" >&2; \
		echo "       CHEF_CLIENT_NAME  — name of the API client" >&2; \
		echo "       CHEF_CLIENT_KEY   — path to the client's PEM key" >&2; \
		echo "       CHEF_ORG          — organisation name (optional)" >&2; \
		echo "" >&2; \
		echo "Run 'make functional-test-list-profiles' to see available profiles." >&2; \
		exit 1; \
	fi

# =============================================================================
# Lint
# =============================================================================

.PHONY: lint
lint: lint-go lint-frontend ## Run all linters

.PHONY: lint-go
lint-go: ## Run golangci-lint on Go source
	@echo "$(GREEN)Running golangci-lint...$(RESET)"
	golangci-lint run ./...

.PHONY: lint-frontend
lint-frontend: ## Run frontend linter
	@if [ -d "$(FRONTEND_DIR)" ] && [ -f "$(FRONTEND_DIR)/package.json" ]; then \
		echo "$(GREEN)Running frontend linter...$(RESET)"; \
		cd $(FRONTEND_DIR) && npm ci --prefer-offline && npm run lint; \
	else \
		echo "$(YELLOW)Skipping frontend lint — $(FRONTEND_DIR)/ not found$(RESET)"; \
	fi

.PHONY: fmt
fmt: ## Format Go source code
	@echo "$(GREEN)Formatting Go source...$(RESET)"
	go fmt ./...

.PHONY: vet
vet: ## Run go vet
	@echo "$(GREEN)Running go vet...$(RESET)"
	go vet ./...

# =============================================================================
# Security / Supply Chain
# =============================================================================

.PHONY: scan-npm
scan-npm: ## Offline supply-chain scan of frontend npm dependencies
	@./scripts/npm-supply-chain-scan.sh

.PHONY: vuln-go
vuln-go: ## Scan Go module + reachable code for known vulnerabilities (govulncheck)
	@if command -v govulncheck >/dev/null 2>&1; then \
		echo "$(GREEN)Running govulncheck...$(RESET)"; \
		govulncheck ./...; \
	else \
		echo "$(YELLOW)govulncheck not found — install the CI-pinned version:$(RESET)"; \
		echo "  go install golang.org/x/vuln/cmd/govulncheck@v1.1.4"; \
		exit 1; \
	fi

# trivy-npm mirrors the BLOCKING Trivy gates in the CI security job: npm
# production dependencies at MEDIUM+ fail the build. It scans the LOCKFILE only
# (never node_modules / npm ci), so a compromised dep's install hook can't run.
# ignore-unfixed is left at its default (false) to match CI.
.PHONY: trivy-npm
trivy-npm: ## Trivy scan of npm production deps (MEDIUM/HIGH/CRITICAL, blocking — mirrors CI)
	@if command -v trivy >/dev/null 2>&1; then \
		echo "$(GREEN)Running Trivy on frontend/package-lock.json...$(RESET)"; \
		trivy fs --scanners vuln --severity MEDIUM,HIGH,CRITICAL --exit-code 1 \
			--ignorefile .trivyignore.yaml frontend/package-lock.json; \
	else \
		echo "$(YELLOW)trivy not found — install with:$(RESET)"; \
		echo "  brew install trivy   # or see https://trivy.dev/latest/getting-started/installation/"; \
		exit 1; \
	fi

# security aggregates the two blocking supply-chain gates from the CI security
# job so `make ci` predicts CI. (CI also runs a non-blocking Tier-2 Trivy SARIF
# report to the Security tab — informational, intentionally omitted here.)
.PHONY: security
security: vuln-go trivy-npm ## Run the blocking supply-chain gates (govulncheck + Trivy) — mirrors CI

.PHONY: scan-trivy
scan-trivy: ## Filesystem scan (vuln + secret + misconfig) with Trivy
	@if command -v trivy >/dev/null 2>&1; then \
		echo "$(GREEN)Running trivy fs (HIGH,CRITICAL gate)...$(RESET)"; \
		trivy fs --scanners vuln,secret,misconfig \
			--severity HIGH,CRITICAL --exit-code 1 \
			--skip-dirs frontend/node_modules --skip-dirs .samples \
			. ; \
	else \
		echo "$(YELLOW)trivy not found — skipping (install: brew install trivy)$(RESET)"; \
	fi

.PHONY: scan
scan: vuln-go scan-npm scan-trivy ## Run all supply-chain / vulnerability scans (Go + npm + Trivy)

# =============================================================================
# Packaging
# =============================================================================

.PHONY: _check-nfpm
_check-nfpm:
	@if [ -z "$(NFPM)" ]; then \
		echo "$(RED)ERROR: nfpm not found. Install it with:$(RESET)" >&2; \
		echo "  go install github.com/goreleaser/nfpm/v2/cmd/nfpm@latest" >&2; \
		exit 1; \
	fi

.PHONY: package-rpm
package-rpm: _check-nfpm build ## Build RPM package
	@echo "$(GREEN)Building RPM package...$(RESET)"
	VERSION=$(VERSION) ARCH=$(HOST_ARCH) nfpm package --packager rpm --target $(BUILD_DIR)/
	@echo "$(GREEN)RPM: $(BUILD_DIR)/*.rpm$(RESET)"

.PHONY: package-deb
package-deb: _check-nfpm build ## Build DEB package
	@echo "$(GREEN)Building DEB package...$(RESET)"
	VERSION=$(VERSION) ARCH=$(HOST_ARCH) nfpm package --packager deb --target $(BUILD_DIR)/
	@echo "$(GREEN)DEB: $(BUILD_DIR)/*.deb$(RESET)"

.PHONY: package-archives
package-archives: build-all ## Build distribution archives (tar.gz / zip) for all platforms
	@echo "$(GREEN)Building distribution archives for $(VERSION_FULL)...$(RESET)"
	@mkdir -p $(BUILD_DIR)/archives
	@# --- Linux amd64 (tar.gz) ---
	@STAGING=$(BUILD_DIR)/stage/$(BINARY_NAME)-$(VERSION)-linux-amd64 && \
		mkdir -p "$$STAGING" && \
		cp $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 "$$STAGING/$(BINARY_NAME)" && \
		cp deploy/pkg/config.yml "$$STAGING/config.yml.example" && \
		cp README.md LICENSE "$$STAGING/" && \
		tar -czf $(BUILD_DIR)/archives/$(BINARY_NAME)-$(VERSION)-linux-amd64.tar.gz -C $(BUILD_DIR)/stage $(BINARY_NAME)-$(VERSION)-linux-amd64 && \
		rm -rf "$$STAGING"
	@# --- Linux arm64 (tar.gz) ---
	@STAGING=$(BUILD_DIR)/stage/$(BINARY_NAME)-$(VERSION)-linux-arm64 && \
		mkdir -p "$$STAGING" && \
		cp $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 "$$STAGING/$(BINARY_NAME)" && \
		cp deploy/pkg/config.yml "$$STAGING/config.yml.example" && \
		cp README.md LICENSE "$$STAGING/" && \
		tar -czf $(BUILD_DIR)/archives/$(BINARY_NAME)-$(VERSION)-linux-arm64.tar.gz -C $(BUILD_DIR)/stage $(BINARY_NAME)-$(VERSION)-linux-arm64 && \
		rm -rf "$$STAGING"
	@# --- macOS amd64 (tar.gz) ---
	@STAGING=$(BUILD_DIR)/stage/$(BINARY_NAME)-$(VERSION)-darwin-amd64 && \
		mkdir -p "$$STAGING" && \
		cp $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 "$$STAGING/$(BINARY_NAME)" && \
		cp deploy/pkg/config.yml "$$STAGING/config.yml.example" && \
		cp README.md LICENSE "$$STAGING/" && \
		tar -czf $(BUILD_DIR)/archives/$(BINARY_NAME)-$(VERSION)-darwin-amd64.tar.gz -C $(BUILD_DIR)/stage $(BINARY_NAME)-$(VERSION)-darwin-amd64 && \
		rm -rf "$$STAGING"
	@# --- macOS arm64 (tar.gz) ---
	@STAGING=$(BUILD_DIR)/stage/$(BINARY_NAME)-$(VERSION)-darwin-arm64 && \
		mkdir -p "$$STAGING" && \
		cp $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 "$$STAGING/$(BINARY_NAME)" && \
		cp deploy/pkg/config.yml "$$STAGING/config.yml.example" && \
		cp README.md LICENSE "$$STAGING/" && \
		tar -czf $(BUILD_DIR)/archives/$(BINARY_NAME)-$(VERSION)-darwin-arm64.tar.gz -C $(BUILD_DIR)/stage $(BINARY_NAME)-$(VERSION)-darwin-arm64 && \
		rm -rf "$$STAGING"
	@# --- Windows amd64 (zip) ---
	@STAGING=$(BUILD_DIR)/stage/$(BINARY_NAME)-$(VERSION)-windows-amd64 && \
		mkdir -p "$$STAGING" && \
		cp $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe "$$STAGING/$(BINARY_NAME).exe" && \
		cp deploy/pkg/config.yml "$$STAGING/config.yml.example" && \
		cp README.md LICENSE "$$STAGING/" && \
		cd $(BUILD_DIR)/stage && zip -rq ../archives/$(BINARY_NAME)-$(VERSION)-windows-amd64.zip $(BINARY_NAME)-$(VERSION)-windows-amd64 && \
		rm -rf "$$STAGING"
	@# --- Windows arm64 (zip) ---
	@STAGING=$(BUILD_DIR)/stage/$(BINARY_NAME)-$(VERSION)-windows-arm64 && \
		mkdir -p "$$STAGING" && \
		cp $(BUILD_DIR)/$(BINARY_NAME)-windows-arm64.exe "$$STAGING/$(BINARY_NAME).exe" && \
		cp deploy/pkg/config.yml "$$STAGING/config.yml.example" && \
		cp README.md LICENSE "$$STAGING/" && \
		cd $(BUILD_DIR)/stage && zip -rq ../archives/$(BINARY_NAME)-$(VERSION)-windows-arm64.zip $(BINARY_NAME)-$(VERSION)-windows-arm64 && \
		rm -rf "$$STAGING"
	@rm -rf $(BUILD_DIR)/stage
	@echo "$(GREEN)Archives:$(RESET)"
	@ls -lh $(BUILD_DIR)/archives/
	@echo ""

.PHONY: package-all
package-all: package-rpm package-deb package-archives ## Build RPM, DEB, and distribution archives

# =============================================================================
# Semver Version Management
# =============================================================================
# Version tags follow the vMAJOR.MINOR.PATCH convention. These targets
# calculate the next version, create an annotated git tag, and optionally
# push it to the remote to trigger the release workflow.
#
# Prerequisites:
#   - Clean working tree (no uncommitted changes)
#   - On the main branch (configurable via RELEASE_BRANCH)
#
# Usage:
#   make bump-patch          — 0.1.0 -> 0.1.1
#   make bump-minor          — 0.1.1 -> 0.2.0
#   make bump-major          — 0.2.0 -> 1.0.0
#   make bump-patch-push     — bump + push tag (triggers release CI)
#   make bump-minor-push     — bump + push tag (triggers release CI)
#   make bump-major-push     — bump + push tag (triggers release CI)
#   make bump-prerelease PRE=rc.1 — 1.0.0 -> 1.0.1-rc.1
# =============================================================================

RELEASE_BRANCH ?= main

.PHONY: _check-version-preconditions
_check-version-preconditions:
	@# Ensure working tree is clean
	@if [ -n "$$(git status --porcelain 2>/dev/null)" ]; then \
		echo "$(RED)ERROR: Working tree is dirty. Commit or stash changes before bumping version.$(RESET)" >&2; \
		exit 1; \
	fi
	@# Warn (but don't fail) if not on the release branch
	@CURRENT_BRANCH=$$(git rev-parse --abbrev-ref HEAD 2>/dev/null); \
	if [ "$$CURRENT_BRANCH" != "$(RELEASE_BRANCH)" ]; then \
		echo "$(YELLOW)WARNING: You are on '$$CURRENT_BRANCH', not '$(RELEASE_BRANCH)'.$(RESET)"; \
		echo "$(YELLOW)         Version tags are typically created on '$(RELEASE_BRANCH)'.$(RESET)"; \
		printf "$(YELLOW)Continue? [y/N] $(RESET)"; \
		read -r answer; \
		case "$$answer" in \
			[yY]|[yY][eE][sS]) ;; \
			*) echo "Aborted."; exit 1 ;; \
		esac; \
	fi

.PHONY: version
version: ## Show current version information
	@echo "$(BOLD)Version:$(RESET)       $(VERSION)"
	@echo "$(BOLD)Full:$(RESET)          $(VERSION_FULL)"
	@echo "$(BOLD)Git tag:$(RESET)       $(GIT_TAG)"
	@echo "$(BOLD)Git commit:$(RESET)    $(GIT_COMMIT_SHORT)$(GIT_DIRTY)"
	@echo "$(BOLD)Build date:$(RESET)    $(BUILD_DATE)"
	@echo ""
	@echo "$(BOLD)Components:$(RESET)"
	@echo "  Major: $(VERSION_MAJOR)"
	@echo "  Minor: $(VERSION_MINOR)"
	@echo "  Patch: $(VERSION_PATCH)"
	@echo ""
	@echo "$(BOLD)Next versions:$(RESET)"
	@echo "  Patch: $(VERSION_MAJOR).$(VERSION_MINOR).$(shell echo $$(($(VERSION_PATCH) + 1)))"
	@echo "  Minor: $(VERSION_MAJOR).$(shell echo $$(($(VERSION_MINOR) + 1))).0"
	@echo "  Major: $(shell echo $$(($(VERSION_MAJOR) + 1))).0.0"

.PHONY: bump-patch
bump-patch: _check-version-preconditions ## Bump patch version and create git tag (x.y.Z)
	$(eval NEW_VERSION := $(VERSION_MAJOR).$(VERSION_MINOR).$(shell echo $$(($(VERSION_PATCH) + 1))))
	@$(MAKE) _apply-tag NEW_VERSION=$(NEW_VERSION)

.PHONY: bump-minor
bump-minor: _check-version-preconditions ## Bump minor version and create git tag (x.Y.0)
	$(eval NEW_VERSION := $(VERSION_MAJOR).$(shell echo $$(($(VERSION_MINOR) + 1))).0)
	@$(MAKE) _apply-tag NEW_VERSION=$(NEW_VERSION)

.PHONY: bump-major
bump-major: _check-version-preconditions ## Bump major version and create git tag (X.0.0)
	$(eval NEW_VERSION := $(shell echo $$(($(VERSION_MAJOR) + 1))).0.0)
	@$(MAKE) _apply-tag NEW_VERSION=$(NEW_VERSION)

.PHONY: bump-prerelease
bump-prerelease: _check-version-preconditions ## Bump patch with pre-release suffix (PRE=rc.1)
	@if [ -z "$(PRE)" ]; then \
		echo "$(RED)ERROR: PRE is required. Example: make bump-prerelease PRE=rc.1$(RESET)" >&2; \
		exit 1; \
	fi
	$(eval NEW_VERSION := $(VERSION_MAJOR).$(VERSION_MINOR).$(shell echo $$(($(VERSION_PATCH) + 1)))-$(PRE))
	@$(MAKE) _apply-tag NEW_VERSION=$(NEW_VERSION)

.PHONY: _apply-tag
_apply-tag:
	@echo "$(GREEN)Bumping version: $(VERSION) → $(NEW_VERSION)$(RESET)"
	@echo ""
	@printf "$(YELLOW)Create tag v$(NEW_VERSION)? [y/N] $(RESET)"
	@read -r answer; \
	case "$$answer" in \
		[yY]|[yY][eE][sS]) \
			git tag -a "v$(NEW_VERSION)" -m "Release v$(NEW_VERSION)"; \
			echo ""; \
			echo "$(GREEN)Created tag: v$(NEW_VERSION)$(RESET)"; \
			echo ""; \
			echo "To trigger the release workflow, push the tag:"; \
			echo "  $(CYAN)git push origin v$(NEW_VERSION)$(RESET)"; \
			echo ""; \
			echo "Or use one of the convenience targets:"; \
			echo "  $(CYAN)make bump-patch-push$(RESET)"; \
			echo "  $(CYAN)make bump-minor-push$(RESET)"; \
			echo "  $(CYAN)make bump-major-push$(RESET)"; \
			;; \
		*) echo "Aborted."; exit 1 ;; \
	esac

.PHONY: bump-patch-push
bump-patch-push: bump-patch _push-tag ## Bump patch version, tag, and push to trigger release

.PHONY: bump-minor-push
bump-minor-push: bump-minor _push-tag ## Bump minor version, tag, and push to trigger release

.PHONY: bump-major-push
bump-major-push: bump-major _push-tag ## Bump major version, tag, and push to trigger release

.PHONY: _push-tag
_push-tag:
	$(eval LATEST_TAG := $(shell git describe --tags --abbrev=0 2>/dev/null))
	@echo "$(GREEN)Pushing branch to origin...$(RESET)"
	git push origin HEAD
	@echo "$(GREEN)Pushing tag $(LATEST_TAG) to origin...$(RESET)"
	git push origin "$(LATEST_TAG)"
	@echo "$(GREEN)Branch and tag pushed. Release workflow should start shortly.$(RESET)"

.PHONY: version-tags
version-tags: ## List all version tags in reverse chronological order
	@echo "$(BOLD)Version tags:$(RESET)"
	@git tag -l 'v*' --sort=-version:refname | head -20
	@echo ""
	@TOTAL=$$(git tag -l 'v*' | wc -l | tr -d ' '); \
	if [ "$$TOTAL" -gt 20 ]; then \
		echo "(showing 20 of $$TOTAL — use 'git tag -l v*' to see all)"; \
	fi

.PHONY: version-delete-tag
version-delete-tag: ## Delete a version tag locally and remotely (TAG=v1.2.3)
	@if [ -z "$(TAG)" ]; then \
		echo "$(RED)ERROR: TAG is required. Example: make version-delete-tag TAG=v1.2.3$(RESET)" >&2; \
		exit 1; \
	fi
	@echo "$(RED)This will delete tag $(TAG) both locally and from origin.$(RESET)"
	@printf "$(YELLOW)Continue? [y/N] $(RESET)"
	@read -r answer; \
	case "$$answer" in \
		[yY]|[yY][eE][sS]) \
			git tag -d "$(TAG)" 2>/dev/null || true; \
			git push origin --delete "$(TAG)" 2>/dev/null || true; \
			echo "$(GREEN)Deleted tag $(TAG)$(RESET)"; \
			;; \
		*) echo "Aborted." ;; \
	esac

# =============================================================================
# Development Convenience
# =============================================================================

.PHONY: run
run: build ## Build and run the application locally
	@echo "$(GREEN)Starting $(BINARY_NAME)...$(RESET)"
	$(BUILD_DIR)/$(BINARY_NAME) --config deploy/pkg/config.yml

.PHONY: run-privileged
run-privileged: build ## Grant CAP_NET_BIND_SERVICE then run (binds ports <1024 in dev)
	@echo "$(YELLOW)Granting CAP_NET_BIND_SERVICE to the binary (needs sudo)...$(RESET)"
	sudo setcap cap_net_bind_service=+ep $(BUILD_DIR)/$(BINARY_NAME)
	@echo "$(GREEN)Starting $(BINARY_NAME) (can bind privileged ports)...$(RESET)"
	$(BUILD_DIR)/$(BINARY_NAME) --config deploy/pkg/config.yml

.PHONY: dev
dev: ## Run with go run (faster iteration, no binary output)
	go run ./cmd/chef-migration-metrics/ --config deploy/pkg/config.yml

.PHONY: stop
stop: ## Stop any running chef-migration-metrics processes
	@echo "$(GREEN)Stopping $(BINARY_NAME)...$(RESET)"
	@pkill -f '$(BINARY_NAME)' 2>/dev/null && echo "$(GREEN)Stopped.$(RESET)" || echo "$(YELLOW)No running $(BINARY_NAME) processes found.$(RESET)"

.PHONY: deps
deps: ## Download and verify Go module dependencies
	@echo "$(GREEN)Downloading Go dependencies...$(RESET)"
	go mod download
	go mod verify

.PHONY: deps-tidy
deps-tidy: ## Tidy Go module dependencies
	@echo "$(GREEN)Tidying Go dependencies...$(RESET)"
	go mod tidy

.PHONY: deps-frontend
deps-frontend: ## Install frontend dependencies
	@if [ -d "$(FRONTEND_DIR)" ] && [ -f "$(FRONTEND_DIR)/package.json" ]; then \
		echo "$(GREEN)Installing frontend dependencies...$(RESET)"; \
		cd $(FRONTEND_DIR) && npm ci; \
	fi

.PHONY: generate
generate: ## Run go generate
	@echo "$(GREEN)Running go generate...$(RESET)"
	go generate ./...

# =============================================================================
# Git Hooks and Secret Scanning
# =============================================================================

.PHONY: install-hooks
install-hooks: ## Install git hooks (secret scanning, spec-size checks, commit-msg trailer strip)
	@echo "$(GREEN)Installing git hooks...$(RESET)"
	@git config core.hooksPath .githooks
	@echo "$(GREEN)Git hooks installed: pre-commit (secret scanning + spec size) and commit-msg (AI-trailer strip) are now active.$(RESET)"
	@echo "  Hook directory: .githooks/"
	@echo "  To bypass (use with caution): git commit --no-verify"

.PHONY: uninstall-hooks
uninstall-hooks: ## Remove custom git hooks path (revert to default)
	@echo "$(GREEN)Removing custom git hooks path...$(RESET)"
	@git config --unset core.hooksPath || true
	@echo "$(GREEN)Git hooks path reset to default.$(RESET)"

# =============================================================================
# Database (standalone PostgreSQL for local development)
# =============================================================================
# These targets start only the PostgreSQL container from the Docker Compose
# stack, so you can run the app on the host with `make run` or `make dev`.

COMPOSE_FILE := deploy/docker-compose/docker-compose.yml
DB_SERVICE   := db
DB_USER      := $(shell grep POSTGRES_USER deploy/docker-compose/.env 2>/dev/null | cut -d= -f2 || echo chef_migration_metrics)
DB_NAME      := $(shell grep POSTGRES_DB deploy/docker-compose/.env 2>/dev/null | cut -d= -f2 || echo chef_migration_metrics)

.PHONY: db-up
db-up: ## Start only the PostgreSQL database container
	@echo "$(GREEN)Starting PostgreSQL...$(RESET)"
	docker compose -f $(COMPOSE_FILE) up -d $(DB_SERVICE)
	@echo "$(GREEN)Waiting for PostgreSQL to be healthy...$(RESET)"
	@until docker compose -f $(COMPOSE_FILE) exec -T $(DB_SERVICE) pg_isready -U $(DB_USER) -d $(DB_NAME) >/dev/null 2>&1; do \
		sleep 1; \
	done
	@echo "$(GREEN)PostgreSQL is ready on localhost:$${POSTGRES_PORT:-5432}$(RESET)"

.PHONY: db-down
db-down: ## Stop the PostgreSQL database container
	@echo "$(GREEN)Stopping PostgreSQL...$(RESET)"
	docker compose -f $(COMPOSE_FILE) stop $(DB_SERVICE)

.PHONY: db-reset
db-reset: ## Stop PostgreSQL, destroy its volume, and start fresh
	@echo "$(YELLOW)Destroying PostgreSQL data and restarting...$(RESET)"
	docker compose -f $(COMPOSE_FILE) rm -sfv $(DB_SERVICE)
	docker volume rm -f docker-compose_pgdata 2>/dev/null || true
	$(MAKE) db-up
	@echo "$(GREEN)Database reset complete — run 'make run' to apply migrations.$(RESET)"

.PHONY: db-psql
db-psql: ## Open a psql shell in the running database container
	docker compose -f $(COMPOSE_FILE) exec $(DB_SERVICE) psql -U $(DB_USER) -d $(DB_NAME)

# =============================================================================
# Docker Compose (local development stack)
# =============================================================================

.PHONY: compose-up
compose-up: ## Start the local Docker Compose stack (PostgreSQL only; the app runs on the host)
	@echo "$(GREEN)Starting Docker Compose stack...$(RESET)"
	docker compose -f $(COMPOSE_FILE) up -d --build

.PHONY: compose-down
compose-down: ## Stop the local Docker Compose stack
	docker compose -f $(COMPOSE_FILE) down

.PHONY: compose-down-volumes
compose-down-volumes: ## Stop the stack and remove all volumes
	docker compose -f $(COMPOSE_FILE) down -v

.PHONY: compose-logs
compose-logs: ## Tail logs from the Docker Compose stack
	docker compose -f $(COMPOSE_FILE) logs -f

.PHONY: compose-ps
compose-ps: ## Show status of Docker Compose services
	docker compose -f $(COMPOSE_FILE) ps

# ---------------------------------------------------------------------------
# ELK Testing Stack
# ---------------------------------------------------------------------------

.PHONY: elk-up
elk-up: ## Start the ELK testing stack
	docker compose -f deploy/elk/docker-compose.yml up -d

.PHONY: elk-down
elk-down: ## Stop the ELK testing stack
	docker compose -f deploy/elk/docker-compose.yml down

.PHONY: elk-down-volumes
elk-down-volumes: ## Stop the ELK stack and remove all volumes
	docker compose -f deploy/elk/docker-compose.yml down -v

# =============================================================================
# Clean
# =============================================================================

.PHONY: clean
clean: ## Remove all build artifacts
	@echo "$(GREEN)Cleaning build artifacts...$(RESET)"
	rm -rf $(BUILD_DIR)/
	rm -rf $(FRONTEND_DIR)/build/ $(FRONTEND_DIR)/dist/
	@echo "$(GREEN)Clean.$(RESET)"

.PHONY: clean-all
clean-all: clean ## Remove build artifacts, caches, and downloaded dependencies
	@echo "$(GREEN)Cleaning caches and dependencies...$(RESET)"
	rm -rf $(FRONTEND_DIR)/node_modules/
	go clean -cache -testcache
	@echo "$(GREEN)Clean all.$(RESET)"

# =============================================================================
# CI (run the full pipeline locally before pushing)
# =============================================================================

.PHONY: ci
ci: deps lint test-all security build ## Mirror GitHub CI gates locally (lint, test, security, build) — run before every release
	@echo ""
	@echo "$(GREEN)$(BOLD)CI pipeline passed.$(RESET)"
	@echo "$(GREEN)This mirrors the blocking gates in .github/workflows/ci.yml.$(RESET)"
