# Active Plan

Single source of truth for what is in flight. **Read this first at session start.**

Only work that is in flight or next lives here. Everything else is in a `todo-*.md`
backlog — do not re-summarise it here; the duplication is what makes this file stale.

## Branch map (2026-08-02)

`main` — v2.18.11 tagged and pushed. Collection is hourly. No open branches.

## NOW — CC19 target-version failing-nodes preset (`plans/todo-event-ingest.md`)

Rollup and filters are built and tested. `useTargetChefVersion`
(`frontend/src/hooks/useTargetChefVersion.ts:31`) is already wired into `GitReposPage`,
`NodesPage`, `RoleDetailPage` and `CookbooksPage`, but not `RunEventsPage`. This is the
wiring — copy an existing call site, not new behaviour.

## NEXT — Dependabot triage

12 vulnerabilities on the default branch (5 high, 5 moderate, 2 low) as of 2026-08-02,
up from 10 (3 high) on 2026-07-30. Our local gates are clean for a reason, not by
contradiction: govulncheck covers reachable Go code, Trivy covers
`frontend/package-lock.json` at MEDIUM+ with suppressions, and Dependabot covers every
manifest. Triage for reachability first — do not blanket-bump.

## Release preconditions (the bump target runs no tests)

1. `make ci` and `make vuln-go` must pass first — `bump-patch-push` does not depend on `ci`.
2. Local `make ci` needs `TRIVY_SKIP_DB_UPDATE=true`: the 1.2GB Trivy DB pull from
   `mirror.gcr.io` stalls mid-transfer (registry handshake is fine; the blob does not
   move). GitHub CI pulls it fresh, so the gap closes on push.
3. The push is a human step — CLAUDE.md forbids remotes. An assistant may bump and tag
   locally, and may push only with explicit, per-action authorisation.

## Parked — do not propose picking these up

- **Collector streaming** — shared collection path, silent-corruption failure modes,
  conflicts with runtime-observability Chunk 4.
- **Collection history** (`plans/collection-history.md`) and the early `completed_at`
  stamp that rides with it — complex and risky for a small gain; the duration question is
  answerable from logs today.
- **Spec/Plan Drift Control** (`plans/spec-drift-control.md`).
- **SAML config follow-ups** — empty `username_attr` warning; local-user username
  collision returning an opaque 500.
