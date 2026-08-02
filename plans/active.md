# Active Plan

Single source of truth for what is in flight. **Read this first at session start.**

Only work that is in flight or next lives here. Everything else is in a `todo-*.md`
backlog — do not re-summarise it here; the duplication is what makes this file stale.

## Branch map (2026-08-02)

`main` — v2.18.11 tagged and pushed. Collection is hourly.

`feature/owner-ingest-discovery` — the ownership MVP. **Long-lived and deliberately
unmerged; do not report it as stale or propose merging it.** All chunks land on this one
branch. Read the two gates in `plans/ownership-work-attribution.md` § How to work before
doing anything on it.

## NOW — the ownership MVP (`plans/ownership-work-attribution.md`)

Work order and journeys live in that plan; per-chunk scope lives in
`plans/todo-ownership.md`. Do not re-plan either.

**Chunk 1, owner ingest — done and reviewed by the product owner, 2026-08-02.** Behaviour is
`specifications/ownership-intake.md`. Two decisions departed from the written plan and were
confirmed in review: unresolved people are created rather than rejected, and a fuzzy
candidate no longer rejects the row. Both stay.

**Chunk 2, identity and alias management — built, awaiting the product owner's review.** Three
strands landed: aliases editable inline on the owner's own page; a standing
possible-duplicate-owners view at `/ownership/duplicates`; and a merge action folding one owner
into another, which moves the aliases and keeps the folded-away name reachable so a correction
survives a re-ingest. Three decisions to confirm in review: the duplicate report compares owner
names as well as alias values (so an owner with no alias is still visible); the committer-assign
path now records the commit address as a `git_email` alias; and the duplicate list is built by
an explicit scan rather than computed per request — comparing everything with everything does
not finish, see the gotcha of that name.

**Chunk 3, node matching, is next — do not start it until chunk 2 is reviewed.** Then git repo
matching.

## QUEUED behind the ownership MVP

- **CC19 target-version failing-nodes preset** (`plans/todo-event-ingest.md`) — wiring
  `useTargetChefVersion` into `RunEventsPage`; copy an existing call site, not new behaviour.
- **General audit log** (`plans/todo-audit.md`, spec `specifications/audit-log.md`) — who
  changed config, who triggered a rescan. Proposed, not started.
- **Dependabot triage** — below.

## Dependabot triage

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
