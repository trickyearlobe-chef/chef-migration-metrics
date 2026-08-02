# Event Ingest — ToDo

MVP merged to `main` and released (**v2.18.4**): `converge_runs` ingest sink
(`POST /api/v1/ingest`), resilient normaliser (node-direct / `run_converge` proxy /
Automate Data Feed), partitioned store + retention ticker, Node Detail Runs tab, and the
top-level **Run events** view (list + node detail + export, feature-gated). Below =
post-MVP follow-ups only. Spec: `specifications/event-ingest.md`.

## Why this matters at the customer (2026-08-02)

Context from the product owner, recorded so it is not rediscovered.

The customer runs a **daily speculative converge against the new Chef version**, installed
side by side with the current one. A run stops at the first failure, so the blockers in a
runlist are enumerated **one per cycle** — three blocking cookbooks take three days to
find. The blockers themselves are stacktraces in that converge history.

**CMM does not have that history there.** It goes to a third-party analytics platform
consuming the Automate event feed; a parallel feed into CMM is being negotiated and the
obstacle is political, not technical. In the lab there is no equivalent platform, so the
Data Feed path stands in for it — which is why lab captures are Data Feed shaped.

**What the run data would buy, and what it would not.** Static readiness already holds
*every* blocking cookbook per node, not just the first, so predicting the set does not need
run data at all. What run data buys is **grading that prediction**: each failure either
lands on a cookbook the static analysis named or on one it missed, and the misses measure
how far the prediction can be trusted before work is dispatched from it.

## Open

- [ ] **CC19 target-version failing-nodes preset mode.** The distinct-node rollup
  (`converge_run_filter.go:164`, `ListConvergeRunNodesFiltered`) and the
  `chef_version ∧ status=failure` discriminator are built and covered
  (`converge_runs_functional_test.go:254`), and `RunEventsPage.tsx:39` defaults status to
  `failure`. Missing: a wired "prospective target-version" preset — `chefVersion` defaults
  to empty (`RunEventsPage.tsx:41`) and `useTargetChefVersion` (exists) is NOT used on the
  page, so the target version is a manual dropdown pick. Auto-populate the target version
  and name the mode. Open Qs (from prior design notes): latest-run vs ever-failed;
  per-node columns; relationship to the static readiness signal (feeding it in is
  out-of-scope for this preset).
- [ ] **Live-fidelity validation of Data Feed + proxy shapes.** Node-direct is proven
  live. Data Feed timestamps + list-cookbooks were corrected against real captures, but
  `testdata/event-ingest/README.md` still flags all fixtures as authored reconstructions.
  Chef Server proxy shares the node-direct `ShapeConverge` code path (differs only by
  data, e.g. `chef_server_fqdn`) → low risk. Validate Data Feed against a full live
  capture — `chef-load` on `dev.home.arpa`; needs an Automate admin token.
- [ ] **Depsolve / attributes-only gap.** `run_start` and `datafeed_attributes_only`
  shapes are accepted-not-persisted (`normalise.go:297`). Depsolve / missing-cookbook
  failures arrive attributes-only, so they are ingested but invisible. Decide whether/how
  to surface them.

  **Sharpened 2026-08-02 — this is the gap that decides whether ingest can name a
  blocker.** Fixtures show a converge failure carrying the failed resource's cookbook and
  recipe on **both** the Automate and raw node/proxy shapes, so attribution does not depend
  on the transport. What carries nothing is the attributes-only delivery, and that is a
  property of the *failure*: a run that dies before it converges has declared no resources,
  so on any transport there is nothing to attribute a cookbook to.

  That is exactly the first-wave upgrade blocker — a depsolve failure, or a cookbook using
  an API removed in the new version and failing at compile time before a resource exists.
  For those the cookbook name is only in the **backtrace**, which is captured and bounded
  but never parsed. So the open question is not whether telemetry arrives; it is whether
  the blocker in it can be named.

- [ ] **Force one 19.x compile-time / depsolve failure in the lab and look at what
  arrives.** Settles the item above with evidence rather than reasoning, and is worth doing
  before a parallel feed is negotiated rather than after — if a delivered failure cannot be
  attributed to a cookbook, winning the negotiation buys less than it appears to.
- [ ] **Lab cleanup.** The lab node still has `data_collector.rb` + `ssl_verify_mode
  :verify_none` set from MVP bring-up. Revert once validation is complete.

## Tracked as tech debt (see `plans/todo-tech-debt.md`)

- Unauthenticated `POST /api/v1/ingest` (`handle_ingest.go:23`, deliberate MVP shortcut).
- Event-ingest admin settings parked on the Collection page (`AdminCollectionPage`).
- One unmappable record (missing `end_time`) drops the WHOLE ingest body
  (`handle_ingest.go:78`) — reconsider per-record tolerance vs all-or-nothing.
