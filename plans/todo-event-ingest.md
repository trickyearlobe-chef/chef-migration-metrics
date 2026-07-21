# Event Ingest — ToDo

MVP merged to `main` and released (**v2.18.4**): `converge_runs` ingest sink
(`POST /api/v1/ingest`), resilient normaliser (node-direct / `run_converge` proxy /
Automate Data Feed), partitioned store + retention ticker, Node Detail Runs tab, and the
top-level **Run events** view (list + node detail + export, feature-gated). Below =
post-MVP follow-ups only. Spec: `specifications/event-ingest.md`.

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
- [ ] **Lab cleanup.** The lab node still has `data_collector.rb` + `ssl_verify_mode
  :verify_none` set from MVP bring-up. Revert once validation is complete.

## Tracked as tech debt (see `plans/todo-tech-debt.md`)

- Unauthenticated `POST /api/v1/ingest` (`handle_ingest.go:23`, deliberate MVP shortcut).
- Event-ingest admin settings parked on the Collection page (`AdminCollectionPage`).
- One unmappable record (missing `end_time`) drops the WHOLE ingest body
  (`handle_ingest.go:78`) — reconsider per-record tolerance vs all-or-nothing.
