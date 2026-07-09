# Roles List Performance — Problem & Design Proposal

**Status: APPROVED 2026-07-07** — locked-in approach (Candidate A + B quick wins).
Next step on resume: `roles.md` spec edit (pending explicit go-ahead) → TDD.
Evidence: `EXPLAIN ANALYZE` on the customer box (target 19.3.15) + app-level
timing. Full diagnosis trail in `plans/list-view-perf.md`.

## Problem (proven)

Every non-trivial roles request computes derived aggregates over **all ~37k
roles at query time**, because sorting/filtering by a derived field (`node_count`,
`incompatible_cookbook_count`, `tk_status`) can't paginate before the value
exists. Two dominant costs, both over the full role set:

- **`node_count` — ~7 s.** 37,453 GIN containment probes on the 2 GB
  `node_snapshots` (one per role). Index is used and instant; cost is the **Bitmap
  Heap Scan reading 581,075 disk blocks** (~62% uncached) to fetch org+node for
  ~920k role-node matches, then a spilling sort.
- **`role_compat` — ~4.4 s.** Transitive cookbook expansion materialises
  **1,994,430 rows** + an **external-merge Sort of 160 MB** on disk.

Measured totals: **p50 12.6 s** (sort=node_count), **~25 s** (tk_status filter).

Not significant (measured, ruled out): recursion (~220 ms), `GetRoleTKStatuses`
(891 ms), summary-bar cold cache (refuted — warm re-test still 25 s).

Secondary, tk_status-path-specific: `Limit=0` seeds the "fast path" with all 37k
roles → `ORDER BY array_position($seed::text[], role_name)` at `role_filter.go:303`
is **O(N²)** over the 37k seed, plus a no-`LIMIT` 37k-row return.

Root cause is common to all slow paths: **derived aggregates computed for every
role on every request.**

## Candidate designs

### A — Materialised per-role summary table (recommended)

Mirror the proven `git_repos` materialised-column pattern
(`git_repo_status_recompute.go`). New `role_summary` table, grain
**(organisation_name, role_name)** (orgs=3, so rollup is free).

- Version-independent columns (always): `node_count`, `direct_cookbook_count`,
  `transitive_cookbook_count`.
- Active-target columns (like git_repos): `compatible/incompatible/untested_count`,
  `compatibility_status`, `tk_status`.
- `node_count` recompute = ONE `unnest(roles)` + `GROUP BY` pass over
  `node_snapshots` (replaces 37k probes + 581k random reads with 1 seq scan).
- Recompute fns (single-role / bulk / reset-on-target-change) fired on: collection
  completion, role_dependency change, cookstyle result change, kitchen result
  change, target-version change. Same trigger points as git_repos.
- List query reads `role_summary`: every sort/filter → indexed column read. Drop
  the seeded expansion and the in-memory tk pagination in `handle_roles.go`.
- Pros: removes the root cause for ALL paths; every slow path → indexed read;
  precedent exists; matches roles.md's own future-work note (§163).
- Cons: most code (table + recompute wiring); staleness window source→recompute;
  needs consistency contract tests; target-dependent columns reset+recompute on
  target change.

### B — Targeted fixes, no table

- Kill the O(N²) `array_position` ordering (use `WITH ORDINALITY` / join) — cheap.
- Raise `work_mem` for these queries → avoids the 160 MB disk sort — cheap.
- `node_count` / `role_compat` still computed over all roles unless precomputed.
- Pros: small, some are quick wins. Cons: does NOT fix the root cause (the 37k-role
  aggregation remains); piecemeal.

### C — Precompute-on-collection only

Same table as A but populated only at collection end (no per-change recompute).
Simpler wiring; stale between collection runs. Viable if collection is frequent.

## Recommendation

**A**, plus fold in B's two independent low-risk wins (kill O(N²) ordering; tune
`work_mem`) since they help regardless and are cheap. A is the only option that
removes the root cause for every slow path and matches existing precedent.

## Open design decisions (resolve before TDD)

- **`is_stale`:** `node_count` from non-stale snapshots only? (Likely yes — may
  also fix a latent over-count in today's CTE.)
- **Multi-target:** materialise active target only (git_repos approach) — confirm
  the roles LIST is always single-target (detail page shows multi-version, but
  that's a different endpoint).
- **Recompute strategy — RESOLVED by precedent.** The app already fans CookStyle
  changes out to derived views via `cookstyle_propagation.go`
  (`RecomputeAllGitRepoCookstyleStatus` + node readiness re-eval). Roles are just
  missing from that fan-out. So: structural columns (`node_count`, cookbook counts)
  at collection; compat/tk columns hooked into the SAME `cookstyle_propagation`
  path (+ TK-change + target-change). Collection-only (C) would make roles the one
  view that ignores rescans — inconsistent with nodes AND cookbooks. Remaining
  sub-question: incremental per-(org,role) vs bulk recompute inside that hook.
- **Consistency:** contract test asserting materialised == live derivation after
  each trigger.

## Spec delta (needs approval to edit)

- `roles.md` Performance Architecture §141–165: replace the future
  `role_compat_summary` note with the `role_summary` design + remove the
  tech-debt defer. No other spec touched.

## Then (Phase 4)

TDD, chunked: (1) migration + `role_summary` schema; (2) recompute fns + triggers
with consistency tests; (3) query rewrite + drop in-memory tk pagination; (4) the
two B quick-wins. Tests first, run after each change, branch per chunk.
