# Project Completion Estimate

**Date:** 2026-03-08  
**Component:** All — project-wide completion analysis  
**Status:** Estimation only — no code changes

---

## Context

Performed a full audit of all todo files, the progress table (`ToDo.md`), the most recent summary (`2026-03-08-01-48-data-exports-wiring.md`), and the project structure to estimate remaining work.

## What Was Done

Analysed every todo file under `.claude/specifications/todo/` and cross-referenced with implemented code to produce a thread-by-thread completion estimate.

## Current State

- **445 / 679 tasks done (65%)**
- **234 remaining tasks** across 8 work areas
- All tests passing (1,666+ tests across 11 packages, 0 failures)
- `go build ./...` passes
- No new code changes in this session

## Thread Estimate: ~35–40 More Threads

### 1. Visualisation — Remaining Items (19 tasks) → ~5 threads

| Area | Remaining Tasks | Est. Threads |
|------|:-:|:-:|
| Dashboard perf testing | 1 | ½ |
| Dependency graph (colour-code by compat, filter by compat, lazy loading, link roles→nodes) | 4 | 1 |
| Remediation (auto-correct "preview only" notice) | 1 | ¼ |
| **Notifications** (webhook, email, triggers, filtering, history, retry) | 11 | **3** |
| Historical trending (store snapshots) | 1 | ½ |
| Log viewer (retention purge) | 1 | ¼ |

### 2. Configuration — ACME/TLS (25 tasks) → ~4 threads

All remaining items are ACME certificate management: CertMagic integration, HTTP-01/TLS-ALPN-01/DNS-01 challenges, 5 DNS providers (Route 53, Cloudflare, Google Cloud DNS, Azure DNS, RFC 2136), renewal, OCSP stapling, backward compat, healthcheck CLI, notification events, logging scope.

### 3. Secrets Storage (65 tasks remaining) → ~5–6 threads

Web API integration (admin credential endpoints), consumer integration (chefapi resolver wiring, auth provider, notify), config integration (Helm chart, credential references), system status endpoint, packaging (deploy/pkg, Helm, Docker Compose), documentation, live credential testing, DBCredentialStore functional tests, TLS key/directory permission checks.

### 4. Authentication & Authorisation (5 tasks) → ~3 threads

Local auth, LDAP, SAML, RBAC, credential safety. Greenfield component — the 5 checkboxes are high-level; actual implementation is substantially more work than 5 items suggest.

### 5. Packaging & Deployment (64 tasks remaining) → ~10–11 threads

| Area | Remaining Tasks | Est. Threads |
|------|:-:|:-:|
| Embedded Ruby environment | 10 | 2 |
| RPM package (nfpm, systemd, scripts) | 8 | 1–2 |
| DEB package | 3 | ½ |
| Container image (build/test/tag) | 3 | ½ |
| Docker Compose verification | 3 | ½ |
| ELK stack verification | 5 | 1 |
| **Helm chart** (entire chart from scratch) | 30 | **4–5** |

### 6. Testing Backlog (29 tasks remaining) → ~4–5 threads

Unit tests (stale cookbooks, blast radius, CookStyle profiles, dependency traversal, notifications, exports, ES/NDJSON), integration tests (7), E2E test (1), embedded tool verification (4), package verification (5).

### 7. Data Collection (2 tasks remaining) → ~1 thread

Checkpoint/resume for failed jobs, and dashboard display of failed cookbook downloads.

### 8. Documentation (25 tasks — all 0%) → ~3–4 threads

Complete user and developer documentation covering installation, configuration, auth setup, Policyfile support, stale detection, remediation, dependency graph, exports, notifications, confidence indicators, complexity scoring, embedded tools, Elasticsearch/ELK, and contributing guidelines.

### Summary Table

| Area | Est. Threads |
|------|:-:|
| Visualisation (notifications, graph polish, log retention) | 5 |
| Configuration (ACME/TLS) | 4 |
| Secrets Storage (wiring, consumers, packaging) | 5–6 |
| Authentication & Authorisation | 3 |
| Packaging & Deployment (embedded Ruby, RPM/DEB, Helm) | 10–11 |
| Testing Backlog | 4–5 |
| Data Collection (2 remaining) | 1 |
| Documentation | 3–4 |
| **Total** | **~35–39** |

### Key Assumptions

- Each thread is scoped to fit within the ~80k token budget per `Claude.md` rules.
- The Helm chart and Notifications are the biggest individual chunks.
- Auth is greenfield and inherently complex (LDAP/SAML integration), so it takes more threads than the 5 checkbox items suggest.
- Packaging tasks often require iterative testing (build → verify → fix), which inflates thread count.
- Some threads will naturally overlap (e.g., secrets wiring depends on auth, notifications depend on config).

## Known Gaps

- This is an estimate only — actual thread counts will vary based on complexity discovered during implementation.
- No dependency ordering analysis was performed (e.g., auth must exist before secrets consumer wiring).
- Thread estimates assume focused single-area work per thread; cross-cutting threads may be more efficient.

## Files Modified

- None (estimation only)

## Recommended Next Steps

1. **Notifications feature** (~3 threads, ~40k tokens each). Biggest remaining visualisation gap. Start with webhook dispatch, then email, then triggers/filtering/history. Read: `visualisation/Specification.md` § Notifications, `configuration/Specification.md` § Notifications, `internal/notify/` (existing package), `todo/visualisation.md` § Notifications.

2. **Auth foundation** (~3 threads, ~40k tokens each). Greenfield but blocks secrets consumer wiring and admin endpoints. Start with local auth + session management, then RBAC middleware, then LDAP/SAML. Read: `auth/Specification.md`, `todo/auth.md`.

3. **Helm chart** (~4–5 threads, ~40k tokens each). Largest single packaging item. Start with Chart.yaml + values.yaml + core templates, then ingress/HPA/PVC, then PostgreSQL subchart, then TLS support. Read: `packaging/Specification.md` § Helm Chart, `todo/packaging.md` § Helm Chart.

4. **Testing backlog** (~4–5 threads, ~30k tokens each). Can be interleaved with feature work. Prioritise unit tests for untested logic (stale cookbooks, blast radius, dependency traversal) before integration tests. Read: `todo/testing.md`.

5. **ACME/TLS** (~4 threads, ~40k tokens each). Self-contained; no dependencies on other incomplete areas. Start with CertMagic integration + HTTP-01, then DNS-01 providers, then OCSP/renewal/coordination. Read: `tls/Specification.md`, `todo/configuration.md` § ACME items.