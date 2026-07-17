# Proposal: Chef Automate Data Feed Event Ingest

**Status: FOR DISCUSSION.** Not approved to build. The transport question is now
**empirically settled** (lab measurement 2026-07-16, Automate build 4.13.x); the
sections below the line remain the original exploration/context for the *whether*.

## Transport — empirically validated (2026-07-16)

The customer runs **Automate as both Chef server and dashboard**, so the transport is
settled: **CMM is a passive Automate Data Feed webhook destination** — no client changes,
no proxy, no separate Chef server, one Automate→CMM connection. Measured Data Feed
behaviour (drives the design):

- **Native when Automate is the Chef server.** Nodes registered to Automate's embedded
  Chef server reach ingest with NO client-side `data_collector` config; the Data Feed
  then POSTs to CMM (gzip + NDJSON, basic auth, batched by `node_batch_size`, driven by
  `feed_interval`).
- **Emits only nodes with new run data** — an idle interval sends nothing (even with
  `updated_nodes_only=false`). Not a periodic full-state re-push.
- **Record = `node` + `client_run` + `attributes`.** `client_run` is the run
  (`client_run.id` = per-run UUID; status, run_list, cookbooks, resources, error,
  chef_version); `attributes` is the full node tree (~100 KB, the bulk). Extract
  `client_run`, discard `attributes` (~5× shrink). Dedup/key the table on `client_run.id`.
- **Coalescing is per-`feed_interval`**: two runs in one window → the latest only (earlier
  dropped); runs in separate windows → all delivered. **Tunable** — set `feed_interval`
  below the node run-cadence (~2 h) → effectively every run from the one transport, so it
  serves BOTH failure-isolation AND the special-job blind spot.
- **Failure isolation confirmed live:** a converge failure delivers `status=failure` +
  `error{class,message,backtrace}` + the failing resource — exactly the payload Automate's
  own filtering can't isolate at scale (the primary value).
- **Gap (still needs the wrapper):** resolution/missing-cookbook and install/prereq aborts
  arrive **attributes-only, no `client_run`** → invisible via the feed. Job 3
  (install/prereq failures) still needs a cron-wrapper/exit-code POST.

**MVP:** passive Data Feed sink → gunzip/NDJSON receiver (cap records-per-body, one txn
per body, respond 200–204) → extract `client_run`, drop `attributes` → rolling 2-day
partitioned `converge_runs` keyed on `client_run.id` → a new panel on `NodeDetailPage`
(recent runs: status, error, failing cookbook·recipe, chef version). Throwaway: delete
the panel + table + Data Feed destination. Pilot small (short retention / one org) first.

This supersedes the raw-firehose sizing and the attribute-stamp / Server-proxy transport
options below — kept for rationale only.

---

## Why (value)

- **Closes the active-only test blind spot.** Testing is scoped to active
  (node-object-in-use) cookbooks. Recurring non-primary "special job" runlists
  (patching, audit, DR, decommission) pull in cookbooks that are in no primary
  runlist → marked unused/inactive → never CookStyle/TK tested → target-version
  compat unknown → fleet can read "ready" while a real workload breaks on migration
  day. Event ingest surfaces *observed real usage* → those cookbooks enter test
  scope. This is a **correctness** gap, not cosmetics.
- **Real target-version converge results per node.** From daily speculative runs.
  Field knowledge: run pass/fail is a **live-telemetry-only** signal (NOT
  recoverable from logs/gather-bundles), so ingest is the *only* reliable source.

## Reframe — it's failure telemetry, not a firehose (per speculative-run model)

Speculative runs already exist: a per-node **cron** runs the CC19 binary (hab path)
against the **default runlist** and writes a **result file** (install + CC19 outcome).

The current signal transport is **unreliable, not "free."** The result reaches us
only via a fragile multi-hop chain: [cron writes file] → [a SUBSEQUENT full **CC16**
run reads the file, analyses it, writes node **attributes**] → [CMM pulls the node
object]. Long lag AND a hard dependency on a **healthy CC16 run** to carry the
signal — but CC16 runs are themselves failing on broken cookbooks, so the signal is
least reliable exactly when it matters. (Exact file-write/pickup mechanism is NOT
fully understood — characterise before designing.)

So the gap is **failure data AND a reliable transport**, for two run types:

- **Speculative CC19 converge failure** — why a node won't converge on 19
  (cookbook compat / resource errors). Gets far enough to likely emit data_collector
  events.
- **CC19 install run failure** — the prereq step that installs the CC19 hab package
  **hard-aborts** on disk space / prereqs. HIGHEST actionable value (easy fixes
  unblock nodes), BUT may abort *before* emitting any data_collector event →
  **capturability risk**; may need the cron wrapper to POST exit code/stderr instead
  of relying on run_converge.

Implications:
- **Volume collapses.** Ingest FAILURES only, from the speculative/install cron
  (point only those runs at us) — not the 2 h production firehose. ~thousands/day,
  not ~17/sec. The write-load / firehose concern largely evaporates.
- **No declaration shortcut for this stream.** Failure telemetry is live-only; D
  (declared runlists) does NOT provide it. This stream genuinely needs ingest (or the
  manual collection customers do today).
- **Separate from the special-job blind spot.** Speculative runs use the DEFAULT
  runlist, so they don't discover special-job cookbooks. Two independent value
  streams — failure-telemetry (this) is the strong, cheap case; special-job discovery
  is the separate, weaker/optional one.

**Transport constraint — Chef Server is the only universally-reachable hub.** Many
nodes have NO direct network path to CMM (DMZ / cross-BU firewalls) → wrapper→CMM
direct POST is out for them. This is *why* today's signal routes through node
attributes (CMM pulls Chef Server; every node can reach Chef Server). So keep that
path, but fix the fragile step. Preferred:
- **Wrapper stamps the result directly onto its node object via the Chef Server API**
  (node's own key — same object chef-client saves each run), decoupled from any later
  CC16 converge. Carries failure detail (status + error tail). CMM reads it via the
  node-object pull it already does. No ingest endpoint, no new network paths.
- Richer alt: **Chef Server as data_collector proxy → CMM** (needs only Server→CMM
  reachability + config; the Nuclia doc confirms the proxy path emits identical
  events). More moving parts than the attribute stamp, BUT gives **resource-level
  failure detail** the stamp can't.

**Configurable ingest filter (makes ingest DB-safe).** CMM already has the
speculative pass/fail boolean, so ingest's job is the *detail*. To avoid hammering
the DB, ingest applies a live-config filter:
- **Drop policy** — by source / org / environment / **status** (default: keep
  **failures only**). Turns the firehose into a trickle.
- **Whitelist** — by runlist / role / cookbook — force-ingest even successes for
  named things (e.g. confirm the **install job** succeeded via its cookbook/role).
- Runs at CMM ingest (fed by the Server proxy). Volume becomes tiny + tunable.
- Whitelisted ingest is telemetry only — never writes node primary associations.

Two transports, different strengths: **attribute stamp** = universal (all DMZ nodes),
summary-only, minimal; **proxy + filtered ingest** = resource-level detail, tunable,
needs proxy config + reachability. Possibly hybrid (stamp everywhere; proxy-ingest
where reachable).

## What (scope sketch)

- Stand up our own data_collector endpoint. chef-client posts schema-identical
  `run_start`/`run_converge` to any collector (verified via `entity_uuid` — our
  field-knowledge doc), so nodes/speculative runs POST to us; **no Automate needed**.
- **Extract-and-discard** at ingest: keep run_id, node, entity_uuid, target version,
  runlist-context, status, timestamps, and (on failure) failed resource + error.
  Drop the ~400 KB payload.
- **Store:** append-only, **time-partitioned** `converge_runs` (+ failures), TTL
  24–48 h, drop old partitions cheaply.
- **Feed the used/tested catalog:** observed cookbooks → used(observed)/active → test
  scope. (Optional) ingest-enriched readiness: node ready only if ALL its runlist
  contexts converge on target.
- **Node detail:** recent runs per runlist-context; expand override runlists on
  demand (reuse the roles expansion engine).

## Invariants (non-negotiable)

- **Node objects are the SOLE source of per-node primary associations** (used/unused,
  blast radius). Events NEVER write them.
- **Event runlists are per-run / observed-usage facts only.** Special-job cookbooks
  join the catalog-level "must test" set (tagged observed), without entering any
  node's primary associations.
- **Optional per customer.** Nothing structural breaks without ingest.

## Firehose sizing (Automate's own benchmarks)

- 120k nodes / 2 h ≈ **~17 runs/sec** avg, bursty; ~**400 KB**/run → ~6–7 MB/s →
  ~0.5 TB/day raw if unfiltered → extract-and-discard is mandatory.
- Automate's design is our precedent AND our warning: message **buffer/queue** in
  front of ingest ("queue is full" errors), **concurrency rate-limiter** (max 960),
  **>4 MB payloads rejected**. We must replicate: receiver → buffer → batched write;
  bound concurrency; cap payload; 200 the POST even when discarding.

## Costs / risks (the "are we sure?" side)

- **Sustained write load** on an already write-heavy DB (collection + firehose).
  Re-weights the roles A-vs-D and coverage-index decisions toward write-frugality.
- **New always-on ingress service** — HTTP endpoint, token auth, TLS, buffering,
  backpressure, retention/partition maintenance. Real operational surface vs today's
  pull-only (no inbound).
- **Security/PII** — converge payloads carry node attributes, possibly secrets in
  resource content; customer logs go to a shared Splunk. Extract minimally, NEVER
  log raw payloads, keep customer-side.
- **Customer must reconfigure** clients to point data_collector at us (opt-in); some
  won't. Requires chef-client ≥ 12.12.15.
- **Speculative-run execution model is a bigger unknown than the ingest** — how are
  target-version runs actually driven across 120k nodes (why-run? isolated
  converge?)? May dwarf this work.

## Alternatives (decide between these, don't assume the firehose)

- **A — full firehose ingest** (this proposal).
- **B — failures + target-only ingest.** Ingest only failed runs and speculative
  (target-version) runs — far lower volume, still closes most of the blind spot +
  gives target pass/fail. Likely the sweet spot.
- **C — periodic batch import.** Customer exports run data; we batch-load. No
  always-on service; staler, more manual.
- **D — declared special-job runlists (no ingest).** A UI where the customer
  *declares* their special-job runlists; we expand + test those cookbooks. Closes the
  blind spot with **no firehose at all** — automatic *discovery* traded for manual
  *declaration*.

The core decision axis: **automatic discovery (A/B firehose) vs declaration (D).** The
blind-spot value may be capturable far more cheaply than a firehose.

## Open questions to settle before committing

- Override/special-job **detection & grouping**: infer by comparing event run_list to
  node's primary? group key = job name / tag / policy_group vs runlist signature?
  (verify `run_converge` fields against the Nuclia event-format doc).
- **Readiness scope**: display-only vs ingest-enriched (all runlist contexts pass).
- **Discovery vs declaration** (A/B vs D) — the biggest fork.
- **Speculative-run execution model** at fleet scale — feasibility unknown.
- **Write-budget impact** alongside collection + the perf fixes.

## Recommendation

Updated post-measurement. The transport is the **Automate Data Feed** (see validated
section) — a single passive webhook destination, no client/proxy changes on the
customer's Automate-as-Chef-server topology. The feasibility gate is answered: **converge
failures DO emit a fully usable record** (status + error + backtrace + failing resource);
**install/prereq/depsolve aborts do NOT** (attributes-only, no `client_run`) → that class
still needs the cron-wrapper/exit-code POST. Special-job/blind-spot discovery no longer
needs a separate declaration mechanism — it **falls out of tuning `feed_interval` below
run cadence** (every run delivered). Recommend: build the Data Feed failure-telemetry MVP
(sink → extract `client_run` → node-detail panel), pilot small, and treat the
install/prereq wrapper as a separate follow-on for the attributes-only gap.
