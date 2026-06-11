# Configuration — ToDo

### Compatibility Guardrails (read before implementing)

The fixes below MUST NOT break existing deployments. Hard rules:

1. **No config-item renames or removals.** YAML keys, config-store `Key*` constants,
   struct `yaml`/`json` tags, and env-var names stay byte-identical. Existing config
   files and DB-stored sections must keep parsing. Changes are read/apply plumbing,
   additive UI for existing fields, and additive DB columns only.
2. **Preserve exact values when centralising defaults.** Dedup-to-a-const must keep the
   identical value. Landmines: the `CMM_CREDENTIAL_ENCRYPTION_KEY` env-var *name string*
   (wrong value ⇒ stored secrets won't decrypt); default literals (`exports` dir, port
   `8080`, readiness `3072`/`6144` sentinels). Any *actual* default-value change is a
   separate, explicitly-flagged decision — never a side effect of refactoring.
3. **Keep legacy aliases.** `tls.enabled`→`mode`, legacy `test_kitchen_timeout_minutes`,
   `readiness.min_free_disk_mb` back-compat shims stay. Do not delete in a cleanup pass.
4. **`restart_required` value flips are the one intended contract change.** The applier
   inversion + pessimistic default makes some sections report `true` where they returned
   `false`. This is correct (honest reporting), but it changes the "Apply & Restart" UX
   and breaks contract tests pinning specific values (e.g. `handle_admin_config_test.go:255`).
   Update those assertions deliberately, not reflexively.

### Restart / Reload Audit (2026-06-11)

Policy (`configuration-live-reload.md`): everything live-reloads except a small
by-design allowlist; needing a restart elsewhere is a **bug**.

**"Restart" ≠ full process restart.** The granularity needed differs per setting.
Only socket/TLS binds need the supervisor to re-exec the process; most "restart"
items just need the owning subsystem to re-read and re-apply (rebind the
listener, reschedule the cron, swap the log level, resize a pool). Reload
granularities:
- **applied** — read live per request (`r.liveConfig()`); nothing to re-apply
- **subsystem** — owning component re-applies in place (reschedule, SetLevel, resize)
- **listener** — rebind the HTTP/TLS listener only; rest of the process stays up
- **process** — supervisor re-exec (re-reads persisted config); last resort

#### Structural fix — derive `restart_required`, don't declare it

Root cause of bucket 2 below: `restart_required` is a **hardcoded literal** passed
by each web handler (`storeAdminConfigSection(..., restartRequired bool)`,
`handle_admin_config.go:397`, written at `:428`) — decoupled from whether any
subsystem actually applies the change. The `postReload ...func(ctx) error` hooks
(`onOrganisationsChanged`, kitchen `SetWorkerCount`) already *are* subsystem-owned
apply points, but they return only `error` and run regardless of the flag, so the
claim and the behaviour drift. That drift IS the bug class.

**Invert it: the subsystem owns the call; the flag is computed from the result.**

- [ ] Evolve `postReload` into an applier the subsystem registers per section:
  `type Applier func(ctx) (ApplyResult, error)` returning `ApplyResult{ Reload Granularity }`.
- [ ] `storeAdminConfigSection`: persist → reload holder → run appliers →
  `restartRequired := result.Reload == ReloadProcess`. Never pass the bool in.
- [ ] **Default for a section with no applier = `process`** (pessimistic). Today's
  default `false` *lies*; a pessimistic default at worst over-prompts a restart —
  honest, never silently wrong. Adding a live applier flips the flag to `false`
  automatically, so it can never drift again.
- [ ] Appliers report `listener`/`subsystem` where applicable, so the API surfaces
  the real granularity (e.g. a `port` change rebinds the listener and reports
  `listener`, not `process`). The web layer becomes a dumb relay.

Do the trivial `r.cfg`→`r.liveConfig()` handler swaps (the `applied`-granularity
items below) first — they're independent and unblock honest `false` immediately.

#### Bucket 1 — restart-required by design (API returns `restart_required:true`)

| Setting | Min granularity actually needed | Note |
|---|---|---|
| `server.listen_address`, `server.port` | listener | rebind listener; full re-exec not strictly required |
| `server.tls.mode` / paths / `min_version` / mTLS CA | listener | cert *material* (db source + ACME) already hot-swaps in place |
| `server.websocket.*` | subsystem | hub rebuildable without process restart |
| `server.graceful_shutdown_seconds` | applied | only read at shutdown; could read live |
| `auth.*` | subsystem | auth chain rebuildable |

These are `process` today only because no applier exists yet. Under the inverted
model each becomes a candidate applier returning `listener`/`subsystem`; until
then they correctly default to `process`. No action required to stay correct —
they're honest. Listed as downgrade candidates, not bugs.

#### Bucket 2 — declared live but silently needs restart (BUGS — violate policy)

API returns `restart_required:false` but a boot-captured consumer no-ops the save.
Each fix = register the applier named below (granularity in brackets); the flag
then derives correctly with no literal to maintain.

- [ ] `logging.level` — logger `level` is immutable (`logging.go:462,480`); no `SetLevel`. **Applier [subsystem]:** live level (e.g. `slog.LevelVar`/setter).
- [ ] `collection.schedule` — cron parsed once; scheduler has no reschedule (`collector/scheduler.go`). **Applier [subsystem]:** add `Reschedule`.
- [ ] `collection.stale_node_warning_hours` / `stale_node_critical_days` — read from static `r.cfg` (`handle_nodes.go:52,53,144,215`). **Fix [applied]:** `r.cfg`→`r.liveConfig()`.
- [ ] `backup.schedule`, `backup.enabled` — `cronExpr` fixed in `NewScheduler` (`backup/scheduler.go:25`); no applier. **Applier [subsystem]:** reschedule/start/stop.
- [ ] `exports.async_threshold` / `max_rows` / `retention_hours` / `output_directory` — static `r.cfg` (`handle_exports.go:103,108,146,206`); cleanup dir captured `main.go:1059`. **Fix [applied] + applier [subsystem]:** handler swap + re-point cleanup ticker.
- [ ] `concurrency.cookstyle_scan`, `concurrency.readiness_evaluation` — baked into the scanner/evaluator analysis components at boot (`setupCollector`, `main.go:880–931`); the collector's run-start refresh does NOT reach them. **Applier [subsystem]:** resize live, mirroring kitchen `SetWorkerCount`. (`concurrency.cookbook_download` likewise snapshot at `main.go:1278` for the node-kitchen factory.)
  - NOT restart: `concurrency.organisation_collection` / `node_page_fetching` are read from `c.cfg` *during* a run (`collector.go:605`, `node_metrics_snapshot.go:258`) and the collector refreshes `c.cfg = c.configFn()` at each run start (`collector.go:573`) → live at next run. (LSP xref correction to the earlier "all concurrency = restart" claim.)

Other static-`r.cfg` read sites to convert to `liveConfig()` when touching their
sections: `handle_system_health.go:24`, `handle_performance.go:75`,
`handle_admin_users.go:122,302`, `handle_saml_admin.go:55`, `handle_git_repos.go`,
`handle_git_repo_files.go`, `handle_cookbook_reset_git.go`, `handle_auth.go:74,86`
(the last few are bucket-1 sections, so consistent today).

#### Bucket 3 — genuinely live (working)

`organisations`, `target_chef_versions`, `git_base_urls`,
`analysis_tools.test_kitchen.*`, `readiness.*` thresholds, kitchen worker pool.

The **collector** uses a valid pull-per-run model: `effectiveConfig()` (`collector.go:189`)
and the run-start `c.cfg = c.configFn()` (`collector.go:573`) refresh its snapshot
each run, so everything it reads mid-run (org collection / node-page concurrency,
git URLs, target versions, in-collection stale thresholds) is live at the next run.
Not a restart surface — the cadence (`collection.schedule`, owned by the separate
`Scheduler`) is the only collector-adjacent restart item.

#### LSP cross-check (2026-06-11) — verification pass

Type-aware `findReferences` (not grep) on the config providers:
- `ConfigHolder.Get` — non-test live readers are only `main.go` (6 wiring sites) + `router.go liveConfig`.
- `Router.liveConfig` — 41 refs / 18 handler files (the handlers that read live).
- `Router.cfg` (static snapshot) — 37 refs; the non-test read sites match the
  grep audit **exactly** (nothing missed, nothing spurious). Bug surface fully enumerated.
- `Collector.cfg` vs `configFn` — surfaced the pull-per-run model above and the
  concurrency split (correction folded into Bucket 2).
- Frontend `GlobalFilterContext` — see staleness item under Multiple Sources of Truth.

#### Bucket 4 — no API/UI handler at all (can't change at runtime; see UI gaps below)

`storage`, `elasticsearch`, `frontend`, `ownership` (config section),
`system_health`, `performance`, `datastore`, `credential_encryption_key_env`.
`system_health` and `performance` have store-key constants but nothing writes them.

---

### Configuration UI Gaps (verified at frontend layer, 2026-06-11)

UI coverage is broad — Collection, Concurrency (all 6 workers), WebSocket (all 6),
TLS/ACME, Readiness, Auth, Exports, Logging, Server are all **complete**. Real gaps:

**API exists but no UI control** (settable via raw API only):
- [ ] `backup.pg_dump_path`, `backup.pg_restore_path` — in PUT payload (`frontend/src/api/backup.ts:15-16`) but no input rendered in `AdminBackupPage.tsx` (only enabled/max_generations/schedule/dir).

**Neither API nor UI** (bootstrap/YAML-only — decide per section whether that's intended):
- [ ] **Elasticsearch** (`enabled`, `output_directory`, `retention_hours`) — add to Exports page (was already on backlog).
- [ ] **Ownership config** (`enabled`, `audit_log.retention_days`, `auto_rules`) — owner *data* has rich UI; the config *knobs* have none. `auto_rules` editor absent entirely.
- [ ] **SystemHealth** (`disk_paths`, `pause_collection_on_critical`, thresholds) — returned read-only by GET `/admin/system-health`, never editable.
- [ ] **Performance** (`enabled`, `pprof_enabled`, `window_seconds`) — read-only dashboard only.
- `storage.*`, `datastore.*` — intentionally bootstrap-only (paths/DSN). No action.
- `frontend.base_path` — intentionally file-only (reverse-proxy deploy config). No action.

Stale item removed: the old "missing `test_kitchen_run` worker spinner (6/7 shown)"
is obsolete — all 6 `concurrency.*` workers are present; TK concurrency is
`analysis_tools.test_kitchen.max_concurrent_vms` (resized live), not a worker field.

`server.listen_address`/`server.port` are DB-managed and UI-editable (`AdminServerPage.tsx`).

---

### Multiple Sources of Truth (2026-06-11)

Same logical value living in 2+ places that can drift. Ranked.

- [ ] **`r.cfg` snapshot vs `liveConfig()`** (HIGH) — same root cause as restart Bucket 2
  above; the inverted-applier fix + handler swaps resolve it. Don't fix twice.
- [ ] **`server.listen` carry-over duplicated** (HIGH) — "DB wins unless absent, else
  carry bootstrap" logic copied in `main.go:722-730` and `reloader.go:91-98`; must stay
  in lockstep by hand. **Fix:** extract one shared helper.
- [ ] **`organisations` config vs DB `organisations` table** (MEDIUM) — identity in both;
  `SSLVerify`/key only in config, matched by name in two duplicated loops
  (`collector.go:1556`, `main.go:1237`). API-created orgs have **no source of truth for
  SSLVerify** (silently defaults `true`). **Fix:** dedupe the match into one helper; decide
  whether SSLVerify/key belong on the org table.
- [ ] **`CMM_CREDENTIAL_ENCRYPTION_KEY` default env name in 3 places** (MEDIUM) —
  `config.go:846`, `main.go:629`, `assembly.go:328`. If the default changes, `main.go`'s
  fallback looks up the **old** env var → secrets loading breaks. **Fix:** one exported const.
- [ ] **`exports.output_directory` default literal in 3 places** (MEDIUM) — `config.go:971`,
  `main.go:1061`, `handle_exports.go:208`. **Fix:** one const; consumers reference it.
- [ ] **`server.port` default `8080` in 3 places** (MEDIUM) — `config.go:1007`, `:1674`,
  `tls/autohttps.go:37` re-implements zero→8080. **Fix:** one const.
- [ ] **`performance.window_seconds`** (LOW) — reported from stale `r.cfg` (`handle_performance.go:75`)
  vs recorder window fixed at boot. Consistent today (no live PUT); fix with the perf applier.
- [ ] **`system_health.disk_paths` derived-once** (LOW) — defaulted from storage dirs at
  default-time; if storage dirs change later, health watches the old paths. **Fix:** derive live.
- [ ] **Redundant `300` perf-window default** (LOW) — defaulted twice in `setDefaults`
  (`config.go:1057` and `:1128`). Dead block — delete.

- [ ] **Frontend: `targetVersions` cached on mount** (LOW) — `GlobalFilterContext`
  fetches the version list once with `[]` deps (`context/GlobalFilterContext.tsx:119-155`,
  "intentionally run only on mount"); `useTargetChefVersion` wraps it. After an admin
  edits `target_chef_versions`, every page's version dropdown is stale until a full
  reload. **Fix:** refetch on focus/interval, or invalidate after the target-versions save.

Handled aliases (not bugs, noted): `tls.enabled`→`mode` (`resolveTLSMode`),
legacy `test_kitchen_timeout_minutes`→nested, `readiness.min_free_disk_mb`→`install_size_mb_*`.
Caveat: the readiness back-compat keys off default sentinels `3072`/`6144` (`config.go:955,958`)
— if those defaults change, the migration mis-fires. Couple it to the same const, not a literal.

---

### TLS and Certificate Management

Done — TLS-in-DB / CSR / ACME branch (`feature/tls-db-certs-csr-acme`, Chunks 7–10).
ACME runs on `x/crypto/acme` directly (CertMagic/lego deliberately rejected,
`tls-acme.md` § 3.1); all state is DB-backed (no `storage_path`). HTTP-01 +
Route 53 DNS-01 solvers, renewal with 1h→24h backoff + 7-day expiry WARN,
`agree_to_tos` gate, staging-CA WARN, `tls.enabled`→`mode` deprecation, and
startup validation all shipped. See `plans/tls-db-certs-csr-acme.md`.
