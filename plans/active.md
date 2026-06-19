# Active Plan

One chunk: UI revamp follow-up cleanup [current]. SAML follow-ups parked below.

Admin-status endpoint + frontend Status tab are DONE/merged (commits up to 61ab9be).

Orphan-sweep ticker wiring DONE on `feature/orphan-sweep-ticker` (2026-06-19,
pending review/merge): `StartSweepTicker` made dynamic (live params + hypervisor
factory each tick via closures), wired in main.go outside the kitchen-binary gate,
synchronous `stop()` in `awaitShutdown`. Folder-scoping deferred per owner decision
— scheduled real-destroy is prefix+age scoped with a logged caveat; tracked in
`plans/todo-tech-debt.md` § "Scheduled Orphan Sweep Has No Folder Scoping" (known
spec-acceptance divergence, reconcile spec when the filter lands).

## Chunk 2 — UI revamp follow-up cleanup [CURRENT]

UI Revamp Phase 1 is DONE in main (audited 2026-06-19; nav restructure + polish all
verified). Two divergences from the original plan remain — captured in
`plans/todo-ui-polish.md` § "Follow-up cleanup". This chunk = **decide how to
refactor**, not a prescribed change. Relevant now because it touches the System
Health hub we just extended with the Status tab (`/admin/system-stats`).

Open the todo, then decide per item: (a) System Health sub-tab structure — actual
`Overview|Performance|Status` vs planned `Overview|API|Database|Actions`; is
"Actions" staying top-level intended? (b) 5 orphaned-but-live Kitchen sub-routes
(no nav link, no redirect) — mirror the `/admin/performance`→hub redirect pattern,
or leave as deep links. Reconcile the plan/roadmap so the divergence is recorded,
not silent (CLAUDE.md: never silently diverge from a spec).

## Parked — SAML config follow-ups (lower priority)

- Warn when a SAML provider has empty `username_attr` (transient-NameID footgun;
  breaks login anchoring + ownership matching — `plans/todo-ownership.md`).
- Turn the local-user username collision (`ErrAlreadyExists` → opaque 500) into a
  clear, actionable message.
