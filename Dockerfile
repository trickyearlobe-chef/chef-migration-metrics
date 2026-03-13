# =============================================================================
# Chef Migration Metrics — Dockerfile
# =============================================================================
# Multi-stage build:
#   1. Build    — golang + node to compile the static Go binary with embedded
#                 frontend SPA assets.
#   2. Runtime  — debian:bookworm-slim with the static binary and git.
#
# CookStyle, Test Kitchen, and InSpec are NOT included in this image.
# For Kubernetes deployments, enable the Chef Workstation init container in the
# Helm chart (chefWorkstation.enabled=true) which copies the tools into a
# shared volume at pod startup.
#
# For standalone Docker usage, mount a Chef Workstation installation into the
# container or run with --volumes-from a chef/chef-workstation container:
#
#   docker run -v /opt/chef-workstation/bin:/opt/chef-tools/bin:ro \
#              -v /opt/chef-workstation/embedded:/opt/chef-tools/embedded:ro \
#              chef-migration-metrics:latest
#
# Build:
#   docker build \
#     --build-arg VERSION=1.2.3 \
#     --build-arg GIT_COMMIT=$(git rev-parse HEAD) \
#     --build-arg BUILD_DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ) \
#     -t chef-migration-metrics:latest .
# =============================================================================

# ---------------------------------------------------------------------------
# Stage 1 — Build (Go binary + frontend assets)
# ---------------------------------------------------------------------------
FROM golang:1.25-bookworm AS builder

WORKDIR /src

# Cache Go module downloads before copying the full source tree.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build the React frontend. The frontend/dist directory MUST exist before
# "go build" because frontend/embed.go uses //go:embed all:dist to bake
# the SPA assets into the binary. If Node.js is unavailable or the build
# fails, a minimal placeholder index.html is created so the embed directive
# succeeds and the binary serves a fallback page.
ARG NODE_MAJOR=20
RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates curl gnupg && \
    mkdir -p /etc/apt/keyrings && \
    curl -fsSL https://deb.nodesource.com/gpgkey/nodesource-repo.gpg.key \
        | gpg --dearmor -o /etc/apt/keyrings/nodesource.gpg && \
    echo "deb [signed-by=/etc/apt/keyrings/nodesource.gpg] https://deb.nodesource.com/node_${NODE_MAJOR}.x nodistro main" \
        > /etc/apt/sources.list.d/nodesource.list && \
    apt-get update && \
    apt-get install -y --no-install-recommends nodejs && \
    rm -rf /var/lib/apt/lists/*

RUN if [ -d "frontend" ] && [ -f "frontend/package.json" ]; then \
        cd frontend && npm ci --prefer-offline && npm run build; \
    else \
        echo "INFO: frontend/ not found — skipping SPA build"; \
    fi && \
    mkdir -p frontend/dist && \
    if [ ! -f "frontend/dist/index.html" ]; then \
        echo '<!DOCTYPE html><html lang="en"><head><meta charset="UTF-8"><title>Chef Migration Metrics</title></head><body><p>Frontend not built. API at <a href="/api/v1/health">/api/v1/health</a></p></body></html>' \
            > frontend/dist/index.html; \
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

# ---------------------------------------------------------------------------
# Stage 2 — Runtime (slim)
# ---------------------------------------------------------------------------
FROM debian:bookworm-slim

# Re-declare build args for LABEL instructions.
ARG VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_DATE=unknown

# OCI-standard image labels.
LABEL org.opencontainers.image.title="chef-migration-metrics"
LABEL org.opencontainers.image.description="Tool for planning and tracking Chef Client upgrade projects"
LABEL org.opencontainers.image.version="${VERSION}"
LABEL org.opencontainers.image.revision="${GIT_COMMIT}"
LABEL org.opencontainers.image.created="${BUILD_DATE}"
LABEL org.opencontainers.image.source="https://github.com/trickyearlobe-chef/chef-migration-metrics"
LABEL org.opencontainers.image.licenses="Apache-2.0"
LABEL org.opencontainers.image.vendor="Chef Migration Metrics Project"

# Install only the minimal runtime dependencies.
#   ca-certificates  — HTTPS connections (Chef API, ACME, etc.)
#   git              — clone/pull cookbook repositories
#   openssh-client   — SSH-based git remotes
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        ca-certificates \
        git \
        openssh-client \
    && rm -rf /var/lib/apt/lists/*

# Create a non-root service account — matches the system user created by
# the RPM/DEB preinstall scripts for filesystem-layout parity.
RUN groupadd -r chef-migration-metrics && \
    useradd -r -g chef-migration-metrics \
        -d /var/lib/chef-migration-metrics \
        -s /usr/sbin/nologin \
        chef-migration-metrics

# Filesystem layout matching native packages (RPM/DEB).
RUN mkdir -p \
        /etc/chef-migration-metrics/keys \
        /etc/chef-migration-metrics/tls \
        /var/lib/chef-migration-metrics \
        /var/lib/chef-migration-metrics/acme \
        /var/log/chef-migration-metrics \
        /opt/chef-tools/bin && \
    chown -R chef-migration-metrics:chef-migration-metrics \
        /etc/chef-migration-metrics \
        /var/lib/chef-migration-metrics \
        /var/log/chef-migration-metrics \
        /opt/chef-tools

# Copy the static Go binary from the build stage.
COPY --from=builder /build/chef-migration-metrics /usr/bin/chef-migration-metrics

# Switch to the non-root service user for runtime.
USER chef-migration-metrics
WORKDIR /var/lib/chef-migration-metrics

EXPOSE 8080

# Health check using the built-in healthcheck subcommand.
HEALTHCHECK --interval=30s --timeout=5s --start-period=60s --retries=3 \
    CMD ["/usr/bin/chef-migration-metrics", "healthcheck"]

ENTRYPOINT ["/usr/bin/chef-migration-metrics"]
CMD ["--config", "/etc/chef-migration-metrics/config.yml"]
