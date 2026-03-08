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
#   make package-all       — build RPM, DEB, and container image
#   make bump-patch        — bump patch version and tag
#   make functional-test   — run against real Chef Server orgs from knife creds
# =============================================================================

SHELL := /bin/bash
.DEFAULT_GOAL := help

# ---------------------------------------------------------------------------
# Version — derived from the most recent git tag (vX.Y.Z format)
# ---------------------------------------------------------------------------
GIT_TAG       := $(shell git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0")
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
EMBEDDED_DIR  := $(BUILD_DIR)/embedded
FRONTEND_DIR  := frontend

LDFLAGS := -X main.version=$(VERSION_FULL) \
           -X main.commit=$(GIT_COMMIT) \
           -X main.buildDate=$(BUILD_DATE)

# Host platform detection
HOST_OS   := $(shell go env GOOS 2>/dev/null || echo linux)
HOST_ARCH := $(shell go env GOARCH 2>/dev/null || echo amd64)

# Container image
REGISTRY   := ghcr.io
IMAGE_NAME := $(REGISTRY)/trickyearlobe-chef/chef-migration-metrics
IMAGE_TAG  := $(VERSION_FULL)

# Ruby build image for embedded environment
RUBY_BUILD_IMAGE := ruby:3.1-bookworm
EMBEDDED_PREFIX  := /opt/chef-migration-metrics/embedded

# nFPM
NFPM := $(shell command -v nfpm 2>/dev/null)

# Chef credentials for functional testing
CHEF_CREDENTIALS_FILE ?= $(HOME)/.chef/credentials
CHEF_CONFIG_RB        ?= $(HOME)/.chef/config.rb
CHEF_PROFILE          ?= $(shell cat $(HOME)/.chef/context 2>/dev/null || echo "default")
FUNCTIONAL_TEST_TAGS  := functional

# Helm chart
HELM_CHART_DIR := deploy/helm/chef-migration-metrics

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

.PHONY: build-all
build-all: build-linux-amd64 build-linux-arm64 ## Cross-compile for all supported platforms

.PHONY: build-frontend
build-frontend: ## Build the React SPA frontend (creates placeholder dist/ if npm unavailable)
	@if [ -d "$(FRONTEND_DIR)" ] && [ -f "$(FRONTEND_DIR)/package.json" ] && command -v npm >/dev/null 2>&1; then \
		echo "$(GREEN)Building frontend...$(RESET)"; \
		cd $(FRONTEND_DIR) && npm ci --prefer-offline && npm run build; \
	else \
		echo "$(YELLOW)npm not found or frontend/ missing — creating placeholder dist/$(RESET)"; \
	fi
	@# Ensure dist/ always exists with at least a placeholder index.html so
	@# that the go:embed directive in frontend/embed.go succeeds at compile time.
	@mkdir -p $(FRONTEND_DIR)/dist
	@if [ ! -f "$(FRONTEND_DIR)/dist/index.html" ]; then \
		echo '<!DOCTYPE html><html lang="en"><head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1.0"><title>Chef Migration Metrics</title><style>body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;display:flex;align-items:center;justify-content:center;min-height:100vh;margin:0;background:#f9fafb;color:#374151}.p{text-align:center;max-width:480px;padding:2rem}h1{font-size:1.25rem;margin-bottom:.5rem}p{color:#6b7280;font-size:.875rem;line-height:1.5}code{background:#f3f4f6;padding:.15em .4em;border-radius:4px;font-size:.8125rem}</style></head><body><div class="p"><h1>Frontend Not Built</h1><p>Build the React SPA: <code>cd frontend &amp;&amp; npm ci &amp;&amp; npm run build</code></p><p>API available at <a href="/api/v1/health">/api/v1/health</a></p></div></body></html>' \
		> "$(FRONTEND_DIR)/dist/index.html"; \
	fi

# ---------------------------------------------------------------------------
# Embedded Ruby Environment
# ---------------------------------------------------------------------------

.PHONY: build-embedded
build-embedded: ## Build embedded Ruby environment for the host architecture
	@$(MAKE) _build-embedded EMBED_PLATFORM=linux/$(HOST_ARCH)

.PHONY: build-embedded-amd64
build-embedded-amd64: ## Build embedded Ruby environment for linux/amd64
	@$(MAKE) _build-embedded EMBED_PLATFORM=linux/amd64

.PHONY: build-embedded-arm64
build-embedded-arm64: ## Build embedded Ruby environment for linux/arm64
	@$(MAKE) _build-embedded EMBED_PLATFORM=linux/arm64

.PHONY: _build-embedded
_build-embedded:
	@echo "$(GREEN)Building embedded Ruby environment ($(EMBED_PLATFORM))...$(RESET)"
	@mkdir -p $(EMBEDDED_DIR)
	docker run --rm --platform $(EMBED_PLATFORM) \
		-v "$(CURDIR)/$(EMBEDDED_DIR):/output" \
		$(RUBY_BUILD_IMAGE) bash -c ' \
			set -euo pipefail && \
			export GEM_HOME=$(EMBEDDED_PREFIX)/lib/ruby/gems/3.1.0 && \
			export GEM_PATH=$$GEM_HOME && \
			mkdir -p $$GEM_HOME && \
			gem install --no-document ffi:1.16.3 && \
			gem install --no-document \
				cookstyle:7.32.8 \
				test-kitchen:3.9.1 && \
			gem install --no-document \
				inspec-bin:5.24.7 && \
			gem install --no-document \
				kitchen-inspec:3.1.0 \
				kitchen-vagrant:2.2.0 \
				kitchen-ec2:3.22.1 \
				kitchen-azurerm:1.13.6 \
				kitchen-google:2.6.1 \
				kitchen-hyperv:0.10.3 \
				kitchen-vcenter:2.12.2 \
				kitchen-vra:3.3.3 \
				kitchen-openstack:6.2.1 \
				kitchen-digitalocean:0.16.1 && \
			gem install --no-document specific_install && \
			gem specific_install -l https://github.com/Stromweld/kitchen-dokken.git -b main && \
			gem install --no-document --force \
				busser:0.8.0 \
				busser-serverspec:0.6.3 \
				busser-bats:0.5.0 && \
			mkdir -p $(EMBEDDED_PREFIX)/bin && \
			cp $$(which ruby) $(EMBEDDED_PREFIX)/bin/ruby && \
			for cmd in cookstyle kitchen inspec; do \
				printf "#!/opt/chef-migration-metrics/embedded/bin/ruby\n" > $(EMBEDDED_PREFIX)/bin/$$cmd && \
				cat $$(gem environment gemdir)/bin/$$cmd >> $(EMBEDDED_PREFIX)/bin/$$cmd && \
				chmod 0755 $(EMBEDDED_PREFIX)/bin/$$cmd; \
			done && \
			mkdir -p $(EMBEDDED_PREFIX)/lib && \
			cp -a /usr/local/lib/libruby* $(EMBEDDED_PREFIX)/lib/ 2>/dev/null || true && \
			cp -a /usr/local/lib/ruby $(EMBEDDED_PREFIX)/lib/ruby/ 2>/dev/null || true && \
			cp -a $(EMBEDDED_PREFIX)/* /output/ \
		'
	@echo "$(GREEN)Embedded environment: $(EMBEDDED_DIR)/$(RESET)"

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
		cd $(FRONTEND_DIR) && npm ci --prefer-offline && npm test -- --coverage; \
	else \
		echo "$(YELLOW)Skipping frontend tests — $(FRONTEND_DIR)/ not found$(RESET)"; \
	fi

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
lint: lint-go lint-frontend lint-helm ## Run all linters

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

.PHONY: lint-helm
lint-helm: ## Run helm lint on the Helm chart
	@if [ -d "$(HELM_CHART_DIR)" ]; then \
		echo "$(GREEN)Running helm lint...$(RESET)"; \
		helm lint $(HELM_CHART_DIR); \
	else \
		echo "$(YELLOW)Skipping helm lint — $(HELM_CHART_DIR)/ not found$(RESET)"; \
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
# Packaging
# =============================================================================

.PHONY: package-docker
package-docker: ## Build the multi-stage container image
	@echo "$(GREEN)Building container image $(IMAGE_NAME):$(IMAGE_TAG)...$(RESET)"
	docker build \
		--build-arg VERSION=$(VERSION_FULL) \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-t $(IMAGE_NAME):$(IMAGE_TAG) \
		-t $(IMAGE_NAME):$(GIT_COMMIT_SHORT) \
		-t $(IMAGE_NAME):latest \
		.
	@echo "$(GREEN)Image: $(IMAGE_NAME):$(IMAGE_TAG)$(RESET)"

.PHONY: package-docker-multiarch
package-docker-multiarch: ## Build multi-arch container image (amd64 + arm64)
	@echo "$(GREEN)Building multi-arch container image $(IMAGE_NAME):$(IMAGE_TAG)...$(RESET)"
	docker buildx build \
		--platform linux/amd64,linux/arm64 \
		--build-arg VERSION=$(VERSION_FULL) \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-t $(IMAGE_NAME):$(IMAGE_TAG) \
		-t $(IMAGE_NAME):$(GIT_COMMIT_SHORT) \
		-t $(IMAGE_NAME):latest \
		.
	@echo "$(GREEN)Multi-arch image: $(IMAGE_NAME):$(IMAGE_TAG)$(RESET)"

.PHONY: _check-nfpm
_check-nfpm:
	@if [ -z "$(NFPM)" ]; then \
		echo "$(RED)ERROR: nfpm not found. Install it with:$(RESET)" >&2; \
		echo "  go install github.com/goreleaser/nfpm/v2/cmd/nfpm@latest" >&2; \
		exit 1; \
	fi

.PHONY: package-rpm
package-rpm: _check-nfpm build build-embedded ## Build RPM package
	@echo "$(GREEN)Building RPM package...$(RESET)"
	VERSION=$(VERSION) ARCH=$(HOST_ARCH) nfpm package --packager rpm --target $(BUILD_DIR)/
	@echo "$(GREEN)RPM: $(BUILD_DIR)/*.rpm$(RESET)"

.PHONY: package-deb
package-deb: _check-nfpm build build-embedded ## Build DEB package
	@echo "$(GREEN)Building DEB package...$(RESET)"
	VERSION=$(VERSION) ARCH=$(HOST_ARCH) nfpm package --packager deb --target $(BUILD_DIR)/
	@echo "$(GREEN)DEB: $(BUILD_DIR)/*.deb$(RESET)"

.PHONY: package-all
package-all: package-rpm package-deb package-docker ## Build RPM, DEB, and container image

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
	@echo "$(GREEN)Pushing tag $(LATEST_TAG) to origin...$(RESET)"
	git push origin "$(LATEST_TAG)"
	@echo "$(GREEN)Tag pushed. Release workflow should start shortly.$(RESET)"

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
# Docker Compose (local development stack)
# =============================================================================

.PHONY: compose-up
compose-up: ## Start the local Docker Compose stack (app + PostgreSQL)
	@echo "$(GREEN)Starting Docker Compose stack...$(RESET)"
	docker compose -f deploy/docker-compose/docker-compose.yml up -d --build

.PHONY: compose-down
compose-down: ## Stop the local Docker Compose stack
	docker compose -f deploy/docker-compose/docker-compose.yml down

.PHONY: compose-down-volumes
compose-down-volumes: ## Stop the stack and remove all volumes
	docker compose -f deploy/docker-compose/docker-compose.yml down -v

.PHONY: compose-logs
compose-logs: ## Tail logs from the Docker Compose stack
	docker compose -f deploy/docker-compose/docker-compose.yml logs -f

.PHONY: compose-ps
compose-ps: ## Show status of Docker Compose services
	docker compose -f deploy/docker-compose/docker-compose.yml ps

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
# Helm
# =============================================================================

.PHONY: helm-deps
helm-deps: ## Build Helm chart dependencies
	@echo "$(GREEN)Building Helm chart dependencies...$(RESET)"
	helm dependency build $(HELM_CHART_DIR)

.PHONY: helm-template
helm-template: ## Render Helm chart templates locally
	helm template chef-migration-metrics $(HELM_CHART_DIR)

.PHONY: helm-package
helm-package: ## Package the Helm chart
	@echo "$(GREEN)Packaging Helm chart...$(RESET)"
	helm package $(HELM_CHART_DIR) --destination $(BUILD_DIR)/

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
ci: deps fmt vet lint test-all build ## Run the full CI pipeline locally (deps, fmt, vet, lint, test, build)
	@echo ""
	@echo "$(GREEN)$(BOLD)CI pipeline passed.$(RESET)"
