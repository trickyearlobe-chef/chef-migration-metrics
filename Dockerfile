# =============================================================================
# Chef Migration Metrics — Multi-Stage Dockerfile
# =============================================================================
# Two-stage build:
#   1. Build       — ruby:3.1-bookworm with Go toolchain added. Compiles the
#                    static Go binary AND installs Ruby gems into a self-
#                    contained embedded prefix. Because Ruby is the base image,
#                    the interpreter, shared libraries, stdlib, and default gems
#                    all live at their compiled-in paths — no path hacks needed
#                    during the build itself.
#   2. Runtime     — debian:bookworm-slim with the static binary, the embedded
#                    Ruby tree, git, a non-root user, and HEALTHCHECK.
#
# Gem versions are pinned to match Chef Workstation 25.13.7 — the canonical
# source is components/gems/Gemfile.lock in the chef/chef-workstation repo:
#   https://github.com/chef/chef-workstation/blob/main/components/gems/Gemfile.lock
#
# Build:
#   docker build \
#     --build-arg VERSION=1.2.3 \
#     --build-arg GIT_COMMIT=$(git rev-parse HEAD) \
#     --build-arg BUILD_DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ) \
#     -t chef-migration-metrics:latest .
#
# See: .claude/specifications/packaging/Specification.md §§ 1, 4
# =============================================================================

# ---------------------------------------------------------------------------
# Stage 1 — Unified build (Ruby base + Go toolchain)
# ---------------------------------------------------------------------------
# Ruby 3.1 is used to match Chef Workstation 25.13.7 which ships Ruby 3.1.7.
# Using 3.2+ causes gem conflicts — nokogiri >= 1.19.1 requires Ruby >= 3.2
# and Chef Workstation caps it at < 1.19.1. The ffi gem is capped at <= 1.16.3
# across the Chef ecosystem because mixlib-log requires ffi < 1.17.0.
FROM ruby:3.1-bookworm AS builder

# Install the Go toolchain. The official tarball is ~70 MB and needs no
# package manager integration — we only use it during the build.
ARG GO_VERSION=1.24.4
RUN arch="$(dpkg --print-architecture)" && \
    case "${arch}" in \
        amd64) goarch=amd64 ;; \
        arm64) goarch=arm64 ;; \
        *)     echo "Unsupported arch: ${arch}" >&2; exit 1 ;; \
    esac && \
    curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${goarch}.tar.gz" \
        | tar -C /usr/local -xz
ENV PATH="/usr/local/go/bin:/go/bin:${PATH}"
ENV GOPATH="/go"

# --- Go binary build ---

WORKDIR /src

# Cache module downloads before copying the full source tree.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build the React frontend if the directory exists.
# The frontend is optional during early development — the build succeeds
# without it and the Go binary simply serves an empty SPA shell.
RUN if [ -d "frontend" ] && [ -f "frontend/package.json" ]; then \
        apt-get update && \
        apt-get install -y --no-install-recommends nodejs npm && \
        rm -rf /var/lib/apt/lists/* && \
        cd frontend && npm ci --prefer-offline && npm run build; \
    else \
        echo "INFO: frontend/ not found — skipping SPA build"; \
    fi

# Build arguments for version injection via -ldflags.
ARG VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_DATE=unknown

# Produce a fully static binary — no CGO, no dynamic linking.
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "\
        -s -w \
        -X main.version=${VERSION} \
        -X main.commit=${GIT_COMMIT} \
        -X main.buildDate=${BUILD_DATE}" \
    -o /build/chef-migration-metrics \
    ./cmd/chef-migration-metrics

# --- Embedded Ruby environment build ---

# Install gems into an isolated prefix. GEM_HOME controls where `gem install`
# writes; we point it at the embedded tree so everything lands there.
ENV EMBEDDED_PREFIX=/opt/chef-migration-metrics/embedded
ENV GEM_HOME=${EMBEDDED_PREFIX}/lib/ruby/gems/3.1.0
ENV GEM_PATH=${GEM_HOME}

# ---------------------------------------------------------------------------
# Gem version pins — Chef Workstation 25.13.7
# ---------------------------------------------------------------------------
# All versions below are taken from the "One True Source" of shipped gems:
#   https://github.com/chef/chef-workstation/blob/main/components/gems/Gemfile.lock
#
# Key constraints from the Chef ecosystem:
#   - ffi <= 1.16.3       (mixlib-log requires ffi < 1.17.0)
#   - nokogiri < 1.19.1   (1.19.1+ requires Ruby >= 3.2, we use 3.1)
#   - rubocop = 1.25.1    (cookstyle 7.32.8 hard-pins this exact version)
#   - train-core = 3.16.1 (inspec-core and kitchen drivers depend on this)
#
# Gems are installed in dependency order to minimise resolution conflicts.
# The ffi pin is installed first as a floor/ceiling constraint that all
# subsequent gems must respect.
#
# Note: kitchen-dokken — Chef Workstation 25.x uses a temporary fork from
# github.com/Stromweld/kitchen-dokken (v2.22.2). We install from that repo
# directly to match their shipped version.
#
# Busser and busser-* gems — Chef Workstation does not ship these, and
# busser 0.8.0 has a hard dependency on thor <= 0.19.0 which conflicts with
# thor 1.4.0 required by test-kitchen, inspec, cookstyle, etc. However,
# older cookbook repos still use busser-serverspec and busser-bats test
# suites. We install them with --force to override the thor conflict.
# This is safe because busser uses thor only for its own CLI parsing, and
# during Test Kitchen runs the busser verifier plugin manages busser's
# lifecycle internally without invoking the busser CLI directly.
#
# kitchen-vsphere is NOT installed. Chef Workstation ships kitchen-vcenter
# (2.12.2) which is the modern VMware vSphere driver using the REST API.
# The old kitchen-vsphere gem requires --force due to winrm conflicts with
# chef-winrm and is effectively unmaintained.
# ---------------------------------------------------------------------------

# Some gems contain C extensions (ffi, nokogiri, bcrypt_pbkdf, etc.).
# ruby:3.1-bookworm ships with gcc, make, and the necessary -dev headers
# so native extensions compile here and the resulting .so files are copied
# into the slim runtime stage — build tools stay behind.

# Install git — needed for gems installed from git sources (kitchen-dokken).
RUN apt-get update && \
    apt-get install -y --no-install-recommends git && \
    rm -rf /var/lib/apt/lists/*

RUN mkdir -p "${GEM_HOME}" && \
    echo "--- Phase 1: Pin ffi to prevent version drift ---" && \
    gem install --no-document ffi:1.16.3 && \
    \
    echo "--- Phase 2: Core tools ---" && \
    gem install --no-document \
        cookstyle:7.32.8 \
        test-kitchen:3.9.1 && \
    \
    echo "--- Phase 3: InSpec (verifier) ---" && \
    gem install --no-document \
        inspec-bin:5.24.7 && \
    \
    echo "--- Phase 4: kitchen-inspec verifier ---" && \
    gem install --no-document \
        kitchen-inspec:3.1.0 && \
    \
    echo "--- Phase 5: Kitchen drivers (rubygems releases) ---" && \
    gem install --no-document \
        kitchen-vagrant:2.2.0 \
        kitchen-ec2:3.22.1 \
        kitchen-azurerm:1.13.6 \
        kitchen-google:2.6.1 \
        kitchen-hyperv:0.10.3 \
        kitchen-vcenter:2.12.2 \
        kitchen-vra:3.3.3 \
        kitchen-openstack:6.2.1 \
        kitchen-digitalocean:0.16.1 && \
    \
    echo "--- Phase 6: kitchen-dokken from Stromweld fork (matches CW 25.x) ---" && \
    gem install --no-document specific_install && \
    gem specific_install -l https://github.com/Stromweld/kitchen-dokken.git -b main && \
    \
    echo "--- Phase 7: Busser (legacy verifier for older cookbook repos) ---" && \
    echo "busser 0.8.0 requires thor <= 0.19.0 which conflicts with thor 1.4.0" && \
    echo "needed by test-kitchen/inspec/cookstyle. --force is safe here because" && \
    echo "busser uses thor only for its CLI; TK manages busser internally." && \
    gem install --no-document --force \
        busser:0.8.0 \
        busser-serverspec:0.6.3 \
        busser-bats:0.5.0

# Copy the Ruby interpreter into the embedded prefix.
RUN mkdir -p ${EMBEDDED_PREFIX}/bin && \
    cp "$(which ruby)" ${EMBEDDED_PREFIX}/bin/ruby

# Copy the Ruby shared libraries into the embedded prefix.
RUN mkdir -p ${EMBEDDED_PREFIX}/lib && \
    cp -a /usr/local/lib/libruby* ${EMBEDDED_PREFIX}/lib/ 2>/dev/null || true

# Copy the complete Ruby library tree (stdlib, C extensions, default-gem
# specs) into the embedded prefix. Because we started from the ruby image,
# everything under /usr/local/lib/ruby/ is already internally consistent —
# arch-specific paths, rbconfig, default gem specs all match.
RUN cp -a /usr/local/lib/ruby ${EMBEDDED_PREFIX}/lib/ruby

# Detect the arch triplet (aarch64-linux, x86_64-linux, etc.) and write an
# env.sh that any shell wrapper can source. This is determined once at build
# time so it works correctly for both amd64 and arm64 images.
#
# Directory layout after the `cp -a /usr/local/lib/ruby` step:
#   embedded/lib/ruby/ruby/3.1.0/          — stdlib (rubygems.rb, etc.)
#   embedded/lib/ruby/ruby/3.1.0/<arch>/   — arch-specific C extensions + rbconfig
#   embedded/lib/ruby/gems/3.1.0/          — installed gems (from GEM_HOME)
#
# Note the double "ruby/ruby" — `cp -a /usr/local/lib/ruby` copies the
# directory *into* `embedded/lib/ruby/`, producing `embedded/lib/ruby/ruby/`.
# RUBYLIB must point at this actual location.
RUN RUBY_ARCH="$(ls /usr/local/lib/ruby/3.1.0/ \
        | grep -Ev '\.rb$' | grep -E 'linux' | head -1)" && \
    echo "Detected Ruby arch: ${RUBY_ARCH}" && \
    { echo '#!/bin/sh'; \
      echo "export RUBYLIB=${EMBEDDED_PREFIX}/lib/ruby/ruby/3.1.0:${EMBEDDED_PREFIX}/lib/ruby/ruby/3.1.0/${RUBY_ARCH}"; \
      echo "export GEM_HOME=${EMBEDDED_PREFIX}/lib/ruby/gems/3.1.0"; \
      echo "export GEM_PATH=${EMBEDDED_PREFIX}/lib/ruby/gems/3.1.0"; \
    } > ${EMBEDDED_PREFIX}/env.sh && \
    chmod 0755 ${EMBEDDED_PREFIX}/env.sh

# Create shell-wrapper binstubs for each tool. Each wrapper:
#   1. Sources env.sh to set RUBYLIB / GEM_HOME / GEM_PATH
#   2. exec's the embedded ruby interpreter with the gem's real entry-point
# This means the tools work no matter how they're invoked — exec from Go,
# docker run --entrypoint, interactive shell — with zero env pre-requisites.
#
# The raw Ruby gem binstubs already exist at gems/3.1.0/bin/ (placed there by
# `gem install` into GEM_HOME). We just write shell wrappers at embedded/bin/
# that point at them.
RUN for cmd in cookstyle kitchen inspec; do \
        { echo '#!/bin/sh'; \
          echo '. /opt/chef-migration-metrics/embedded/env.sh'; \
          echo "exec /opt/chef-migration-metrics/embedded/bin/ruby /opt/chef-migration-metrics/embedded/lib/ruby/gems/3.1.0/bin/${cmd} \"\$@\""; \
        } > "${EMBEDDED_PREFIX}/bin/${cmd}" && \
        chmod 0755 "${EMBEDDED_PREFIX}/bin/${cmd}"; \
    done

# Quick sanity check — the tools must work inside the build stage itself.
# Verifies core tools, all kitchen drivers, and the inspec verifier.
RUN echo "=== Sanity checks ===" && \
    ${EMBEDDED_PREFIX}/bin/cookstyle --version && \
    ${EMBEDDED_PREFIX}/bin/kitchen    version && \
    ${EMBEDDED_PREFIX}/bin/inspec     version && \
    echo "--- Kitchen drivers ---" && \
    ${EMBEDDED_PREFIX}/bin/ruby -e "require 'kitchen/driver/dokken';       puts 'OK: kitchen-dokken'" && \
    ${EMBEDDED_PREFIX}/bin/ruby -e "require 'kitchen/driver/vagrant';      puts 'OK: kitchen-vagrant'" && \
    ${EMBEDDED_PREFIX}/bin/ruby -e "require 'kitchen/driver/ec2';          puts 'OK: kitchen-ec2'" && \
    ${EMBEDDED_PREFIX}/bin/ruby -e "require 'kitchen/driver/azurerm';      puts 'OK: kitchen-azurerm'" && \
    ${EMBEDDED_PREFIX}/bin/ruby -e "require 'kitchen/driver/gce';          puts 'OK: kitchen-google'" && \
    ${EMBEDDED_PREFIX}/bin/ruby -e "require 'kitchen/driver/hyperv';       puts 'OK: kitchen-hyperv'" && \
    ${EMBEDDED_PREFIX}/bin/ruby -e "require 'kitchen/driver/vcenter';      puts 'OK: kitchen-vcenter'" && \
    ${EMBEDDED_PREFIX}/bin/ruby -e "require 'kitchen/driver/vra';          puts 'OK: kitchen-vra'" && \
    ${EMBEDDED_PREFIX}/bin/ruby -e "require 'kitchen/driver/openstack';    puts 'OK: kitchen-openstack'" && \
    ${EMBEDDED_PREFIX}/bin/ruby -e "require 'kitchen/driver/digitalocean'; puts 'OK: kitchen-digitalocean'" && \
    echo "--- Kitchen verifiers ---" && \
    ${EMBEDDED_PREFIX}/bin/ruby -e "require 'kitchen/verifier/inspec';     puts 'OK: kitchen-inspec verifier'" && \
    echo "--- Busser (legacy verifier) ---" && \
    ${EMBEDDED_PREFIX}/bin/ruby -e "require 'busser';                      puts 'OK: busser'" && \
    ${EMBEDDED_PREFIX}/bin/ruby -e "require 'busser/runner_plugin/serverspec'; puts 'OK: busser-serverspec'" && \
    ${EMBEDDED_PREFIX}/bin/ruby -e "require 'busser/runner_plugin/bats';   puts 'OK: busser-bats'" && \
    echo "--- Key version assertions ---" && \
    ${EMBEDDED_PREFIX}/bin/ruby -e " \
      require 'rubygems'; \
      { \
        'cookstyle'      => '7.32.8', \
        'test-kitchen'   => '3.9.1', \
        'inspec-core'    => '5.24.7', \
        'kitchen-inspec' => '3.1.0', \
        'ffi'            => '1.16.3', \
      }.each do |name, expected| \
        spec = Gem::Specification.find_by_name(name); \
        actual = spec.version.to_s; \
        if actual != expected; \
          abort \"VERSION MISMATCH: #{name} expected #{expected} got #{actual}\"; \
        end; \
        puts \"  #{name} #{actual} ✓\"; \
      end; \
      puts 'All version assertions passed.'; \
    " && \
    echo "=== All sanity checks passed ==="

# ---------------------------------------------------------------------------
# Stage 2 — Runtime (Debian bookworm-slim)
# ---------------------------------------------------------------------------
# debian:bookworm-slim is chosen over scratch/alpine because the application
# shells out to git, cookstyle, and kitchen which require glibc and a shell.
FROM debian:bookworm-slim

# Re-declare build args so they are available for LABEL instructions.
ARG VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_DATE=unknown

# OCI-standard image labels for traceability.
LABEL org.opencontainers.image.title="chef-migration-metrics"
LABEL org.opencontainers.image.description="Tool for planning and tracking Chef Client upgrade projects"
LABEL org.opencontainers.image.version="${VERSION}"
LABEL org.opencontainers.image.revision="${GIT_COMMIT}"
LABEL org.opencontainers.image.created="${BUILD_DATE}"
LABEL org.opencontainers.image.source="https://github.com/trickyearlobe-chef/chef-migration-metrics"
LABEL org.opencontainers.image.licenses="Apache-2.0"
LABEL org.opencontainers.image.vendor="Chef Migration Metrics Project"

# Install only the runtime dependencies the application and embedded Ruby need.
#   ca-certificates  — HTTPS connections (Chef API, ACME, etc.)
#   git              — clone/pull cookbook repositories
#   openssh-client   — SSH-based git remotes
#   libyaml-0-2      — Ruby YAML extension (psych)
#   libffi8          — Ruby FFI gem / native extensions
#   libgmp10         — Ruby bignum / OpenSSL
#   zlib1g           — Ruby zlib extension
#   libxml2          — Nokogiri (inspec dependency)
#   libxslt1.1       — Nokogiri XSLT support
#   libssl3          — OpenSSL (net-ssh, train transports)
#   libgcc-s1        — GCC runtime (native extension .so files)
#   libreadline8     — Ruby readline extension (used by pry/irb in inspec shell)
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        ca-certificates \
        git \
        openssh-client \
        libyaml-0-2 \
        libffi8 \
        libgmp10 \
        zlib1g \
        libxml2 \
        libxslt1.1 \
        libssl3 \
        libgcc-s1 \
        libreadline8 \
    && rm -rf /var/lib/apt/lists/*

# Create a non-root service account — matches the system user created by
# the RPM/DEB preinstall scripts for filesystem-layout parity.
RUN groupadd -r chef-migration-metrics && \
    useradd -r -g chef-migration-metrics \
        -d /var/lib/chef-migration-metrics \
        -s /usr/sbin/nologin \
        chef-migration-metrics

# Filesystem layout matching native packages (RPM/DEB).
#   /etc/chef-migration-metrics/keys/     — Chef API private keys (mounted)
#   /etc/chef-migration-metrics/tls/      — static TLS certs/keys (mounted)
#   /var/lib/chef-migration-metrics/      — git working dirs, runtime data
#   /var/lib/chef-migration-metrics/acme/ — ACME cert storage (persistent vol)
#   /var/log/chef-migration-metrics/      — application log files
RUN mkdir -p \
        /etc/chef-migration-metrics/keys \
        /etc/chef-migration-metrics/tls \
        /var/lib/chef-migration-metrics \
        /var/lib/chef-migration-metrics/acme \
        /var/log/chef-migration-metrics && \
    chown -R chef-migration-metrics:chef-migration-metrics \
        /etc/chef-migration-metrics \
        /var/lib/chef-migration-metrics \
        /var/log/chef-migration-metrics

# Copy the static Go binary from the build stage.
COPY --from=builder /build/chef-migration-metrics /usr/bin/chef-migration-metrics

# Copy the self-contained embedded Ruby tree from the build stage.
# This includes the interpreter, shared libs, stdlib, default gems,
# installed gems (cookstyle, test-kitchen, kitchen-dokken, kitchen-inspec,
# inspec, and all kitchen drivers), env.sh, and shell-wrapper binstubs.
COPY --from=builder /opt/chef-migration-metrics/embedded \
                    /opt/chef-migration-metrics/embedded

# Register the embedded Ruby shared library directory with the dynamic linker
# so the embedded ruby interpreter can find libruby.so without LD_LIBRARY_PATH.
RUN echo "/opt/chef-migration-metrics/embedded/lib" \
        > /etc/ld.so.conf.d/chef-migration-metrics-embedded.conf && \
    ldconfig

# Switch to the non-root service user for runtime.
USER chef-migration-metrics
WORKDIR /var/lib/chef-migration-metrics

EXPOSE 8080

# Health check using the built-in healthcheck subcommand, which performs
# a lightweight HTTP GET against /api/v1/admin/status and exits 0 on success.
HEALTHCHECK --interval=30s --timeout=5s --start-period=60s --retries=3 \
    CMD ["/usr/bin/chef-migration-metrics", "healthcheck"]

ENTRYPOINT ["/usr/bin/chef-migration-metrics"]
CMD ["--config", "/etc/chef-migration-metrics/config.yml"]
