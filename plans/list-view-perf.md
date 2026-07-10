# List-View / node_snapshots Performance — Investigation & Delivery

Process: **diagnose definitively → write up problems → design proposal → TDD
build.** No fix is decided. Materialisation / indexing are *candidate* designs to
be evaluated in Phase 3, NOT conclusions. These are large changes; no winging.

**Agreed delivery order:**
1. **Fix roles (P2)** — diagnosis considered closed (root cause proven; see Phase 1
   findings). Proceed to write-up → design proposal → TDD.
2. **Data-layer query diagnostics** — per-method query stats + slow-query capture +
   admin EXPLAIN (run-twice, show cold+warm). Own spec/plan. Redact arg values
   (customer data → Splunk exposure); admin-only; SELECT-only ANALYZE guard.
3. **node_snapshots big problem (P1 coverage + P3)** — tackle WITH the tooling from
   step 2 giving real per-query diags (esp. to identify P3's caller).

## Evidence status (honest)

- **P1 coverage hotspot — PROVEN + plan captured.** Unindexed `cookbooks ? $1` full scan;
  **5.6M calls / 3.1B ms**, 45% cache hit; **10.6M `node_snapshots` seq scans**;
  no cookbooks index (DDL + Index Usage + seq_scan agree). Driver: per-org
  coverage loop (`collector.go:1476`) over all cloned repos. EXPLAIN now confirms the
  plan (Seq Scan, 119,073 rows removed by filter — see EXPLAIN evidence). Open: exact
  call cadence/trigger; whether the candidate fixes fully remove it.
- **P2 roles — FIXED + CLOSED.** Materialised `role_summary` (migration 0049); read path
  rolls up `role_summary`. Confirmed sub-second at customer scale via the v2.16.1 admin
  EXPLAIN runner. Residual cost is the `COUNT(*) OVER()` full-count + external-merge sort —
  same class as the node-list P3 mechanism, so folded into the P1/P3 fix directions.
- **P3 — `node_snapshots` full-row fetches — IDENTIFIED via EXPLAIN.** It is the
  **node-list heavy-JSON path**, not a single-node fetch: full Seq Scan over ~119k rows +
  a `COUNT(*) OVER()` WindowAgg that materialises **all** rows before `LIMIT 50` + a
  heavy-row (width≈1796) top-N sort that **spills to temp** (~47k temp reads). The
  single-node full-row fetch is indexed and <1 ms, so it is ruled out. See EXPLAIN evidence.

## Customer-scale EXPLAIN evidence

Captured on the customer DB via the v2.16.1 admin EXPLAIN runner (ANALYZE + run-twice,
target 19.3.15, ~119,142 `node_snapshots`). Identifiers redacted; only plan shapes,
cardinalities, timings, and buffer/temp stats recorded.

- **Node list (heavy JSON), LIMIT 50 — 606 ms cold / 602 ms warm.**
  `Limit → Sort (top-N heapsort, node_name) → WindowAgg → Seq Scan on node_snapshots (~119,142 rows)`;
  width=1796; buffers shared read≈75,596, **temp read≈47,777 written≈24,100**. Mechanism:
  `COUNT(*) OVER()` materialises all rows before the LIMIT; heavy 1796-byte rows blow the
  sort's work_mem → large temp spill.
- **Node list (light), LIMIT 50 — 445 ms cold / 430 ms warm.** Same shape, width=746,
  temp read≈18,075 written≈9,213. The full Seq Scan + WindowAgg over 119k rows is a ~440 ms
  floor even without heavy JSONB; the heavy columns add ~165 ms + ~2.6× the temp spill.
- **Cookbook coverage containment (`cookbooks ? <name>`) — 712 ms cold / 735 ms warm (P1).**
  `Sort → GroupAggregate → Sort → Seq Scan on node_snapshots, Filter: cookbooks ? $1, Rows Removed by Filter: 119,073` (kept ~69); buffers shared read≈70,532; no index on `cookbooks`.
- **Single node full row (by org+name) — 0.5 ms cold / 0.02 ms warm.**
  `Limit → Index Scan using idx_node_snapshots_org_name_collected`. Already fast — **rules
  out** single-node fetch as P3's slow caller.
- **Distinct node roles — 305 ms cold / 305 ms warm.**
  `Unique → Gather Merge (2w) → Sort → HashAggregate → Nested Loop → Parallel Seq Scan on
  node_snapshots (Filter jsonb_typeof(roles)='array') → Function Scan jsonb_array_elements_text (loops≈118,438)`.
  Full parallel seq scan + roles unnest across all nodes.

**Cross-cutting:** warm ≈ cold everywhere (606→602, 445→430, 712→735, 305→305) — these are
full-scan / temp-spill bound, **not** cache-warmup; each reads ≈70–76k shared blocks (repeated
full reads of `node_snapshots`). (The customer's earlier ~6 s vs 606 ms here likely reflects a
deeper page/OFFSET, concurrent load, or colder cache — same mechanism.)

**Fix directions to evaluate (Phase 3):**
- **List queries (P3):** remove the `COUNT(*) OVER()` full-materialisation (separate/estimated
  count), and/or a deferred-heavy-columns pattern — sort+page on a cheap indexed projection
  (PK/node_name), then fetch heavy JSONB only for the 50 rows on the page → sort width collapses,
  temp spill disappears. Consider an index supporting the default `node_name` sort.
- **Coverage (P1):** GIN index on `node_snapshots.cookbooks` (jsonb_path_ops) → `?` index scan.
- **Distinct roles:** materialise distinct roles (or reuse `role_summary`) if on a hot path.

## Phase 1 — Definitive diagnosis (evidence, not inference)

Constraint: customer is VDI/screenshot-only. Per CLAUDE.md ("look at data in the
docker DB for evidence"), primary method = **reproduce at representative scale
locally**, cross-check against customer screenshots.

- Build a representative dataset in the docker dev DB matching
  [[customer-db-scale]] cardinalities (~120k `node_snapshots`, ~96k
  `role_dependencies` edges, 3 orgs, realistic per-node cookbooks/roles arrays and
  role→role nesting depth). Decide: new synthetic generator vs extend existing
  fixtures.
- **Roles:** pin the exact slow request(s) (which `sort`/`tk_status`/filters gave
  28 s). `EXPLAIN (ANALYZE, BUFFERS)` each slow-path variant — sort=node_count,
  sort=incompatible, `tk_status` filter, compat filter — plus the fast path.
  Attribute wall-time per CTE/step; identify the dominant cost. Confirm or refute
  each hypothesis.
- **Coverage (light — already proven):** confirm call driver/cadence; verify a
  candidate one-pass query returns identical `cookbook_platform_coverage` rows.
- **P3:** capture one 6 s plan; identify the caller by matching normalized SQL.
- Output: raw plans + an attribution table. **No design work in this phase.**

## Phase 1 findings — roles sort=node_count (customer, target 19.3.15)

`EXPLAIN (ANALYZE, BUFFERS)` on the reconstructed slow-path query.
**Execution 12,632 ms** — reproduces p50. Attribution:

- **`node_count` over all roles — DOMINANT (~7 s).** `node_counts` = Nested Loop,
  **37,453 GIN containment probes** on `node_snapshots` (1/role). Index
  (`idx_node_snapshots_roles_gin`) IS used, index scan instant (0.005 ms) — cost is
  the **Bitmap Heap Scan: 581,075 disk blocks read** (Heap Blocks exact=821,876,
  ~62% uncached) fetching org+node for **920,892 role-node pairs**, then an
  Incremental Sort that spills.
- **`role_compat` transitive expansion — ~4.4 s.** Materialises **1,994,430 rows**
  (transitive cookbook×org joined to cookstyle) + **external-merge Sort 160 MB**.
- **`transitive_deps` recursion — 219 ms, negligible.** Refutes "recursion is the
  cost".
- `work_mem` too small: 3 external-merge sorts spill (160 / 6.6 / 4.7 MB).

Design implications (evidence-based):
- Both derived aggregates run over ALL ~37k roles before `LIMIT` (inherent to
  derived-field sort) → fast-path seeding cannot apply here.
- `node_count` (version-independent, biggest): candidate = ONE `unnest(roles)`+
  `GROUP BY` pass over `node_snapshots` (replaces 37k random probes + 581k
  scattered reads with 1 seq scan).
- `role_compat`: candidate = per-role compat counts materialised for active target
  (kills 2M-row expansion + disk sort).
- Recursion needs no work.

`GetRoleTKStatuses` (tk_status path 2nd query, all roles) EXPLAIN: **891 ms** —
cheap, NOT the tail driver (refutes the tk-status-is-the-28s hypothesis).

Summary-cache hypothesis for the p95 — **REFUTED.** App test: `tk_status=passed`
filter, let it stabilise, re-applied within the 60 s TTL (warm summary cache) →
**still ~25 s**. So `GetRoleCompatSummary` is NOT the tail driver.

**tk_status path (~25 s) is intrinsic.** With `sort=name` + tk filter, `Limit=0`
forces the fast path to seed `buildRoleFilterQuery` with ALL ~37k roles. That
seeded query: (a) same `node_count` + `role_compat` root cost as the 12.6 s query,
plus (b) seeded-only overhead — 37k-element `ANY()` anchor, no `LIMIT` (returns
all 37k rows), and **`ORDER BY array_position($seed::text[], role_name)` at
`role_filter.go:303` = O(N²)** over the 37k seed. Exact split of (b) not yet
EXPLAINed; materialisation eliminates the whole expansion so it may not matter.

**Root cause (all slow roles paths):** derived aggregates — `node_count`
(containment over 2 GB `node_snapshots`) + `role_compat` (2M-row transitive
expansion) — computed for all ~37k roles at query time. Proven by the 12.6 s plan.

## Phase 2 — Problem write-up

One concise `problems` doc: for P1/P2/P3, the *proven* mechanism, dominant cost
with plan evidence, scale-dependence, and blast radius. Supersedes all inference.

## Phase 3 — Design proposal

Per proven problem: options + trade-offs, chosen with evidence. Candidates to
*evaluate* (not pre-commit): roles — materialised `role_summary` / node→role
index / query rewrite (kill all-rows tk path) / precompute-on-collection;
coverage — one-pass query + hoist loop vs GIN index (insert write-cost tradeoff).
Produce a decision record + list spec deltas (`roles.md`, `analysis.md`) — specs
edited only after approval.

## Phase 4 — TDD implementation

Per approved design; chunked; tests first, run after each change; branch per
chunk; acceptance criteria set in Phase 3.

## Immediate unblockers (need from user)

- Exact roles request params that produced the 28 s reading.
- OK to build a synthetic representative-scale seed in the docker dev DB for local
  `EXPLAIN ANALYZE` — or do you prefer capturing `EXPLAIN` from the customer box
  via screenshot?
