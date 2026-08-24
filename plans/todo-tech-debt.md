# Tech Debt

## CookStyle — static coverage of Ruby removals is incomplete (invisible blockers)

- [ ] **Enable the disabled removal cops we control.** If the app owns the cookstyle
  config it runs (`internal/analysis/cookstyle_invocation.go` /
  `cookstyle_config_isolation`), enable `Lint/UriEscapeUnescape` (a real removed-API
  cop that ships off) and add its `copmapping.go` Blocker entry — turns a proven crash
  from invisible into a Blocker. Verify no false-positive blast radius first.

## Backup — Restore Exits 0 But Unit Is `Restart=on-failure`

- [ ] **Post-restore restart may not happen.** `executeRestore` (`internal/webapi/handle_admin_backups.go:288-292`) terminates the process with `exitFn(0)` and restore assumes "systemd/supervisor restarts app". But the packaged unit (`deploy/pkg/chef-migration-metrics.service`) sets `Restart=on-failure`, which does **not** restart on a clean exit 0 — so after a restore the service would stay down until manually started. The Apply & Restart action deliberately exits non-zero (`exitCodeRestart=2`) so the existing unit restarts it. **Strategic fix:** make restore use the same non-zero restart exit code (or otherwise guarantee a restart under the shipped unit), and unify the "exit-to-restart" path so restart and restore agree.

## Secrets — config_store Master Key Rotation

- [ ] When `CMM_CREDENTIAL_ENCRYPTION_KEY` is rotated, entries in `config_store` (including all credentials) are **not** re-encrypted under the new key. The old `rotateSecrets` function only operated on the now-dropped `credentials` table. **Strategic fix:** implement `Store.RotateKey(ctx, oldKey []byte)` that re-encrypts every `config_store` row under the new derived key within a single transaction.

## Blocking cookbooks

- [ ] **BlockingCookbooks paths don't include cookbook→cookbook transitive deps** — `getBlockingCookbooks`/`collectPaths` in `datastore/role_detail.go` only walk role→role and role→cookbook edges, so a cookbook that is incompatible purely because one of *its* dependencies is incompatible won't appear in `BlockingCookbooks` with the correct dependency path. The dependency tree and graph correctly show the full expansion, but the blocking-cookbook path computation remains role-edge-only. **Strategic fix:** extend `collectPaths` to also follow cookbook→cookbook edges using a `cbAdj` map, building paths like `role:web → cookbook:nginx → cookbook:apt`.

## Collector — Cron Schedule `N/step` Not Honoured

- [ ] **A collection schedule of `0/2 * * * *` fires hourly, not every 2 minutes.**
  `parseCronPart` (`internal/collector/scheduler.go`) only applies a `/step` to a
  `*` wildcard or an `a-b` range; for a **literal** base (`0/2`) it truncates to
  `0` and drops the step → the minute field becomes `{0}` → effectively `0 * * * *`
  (hourly). Standard cron treats `0/2` as `0,2,4,…`. Confirmed: the literal branch
  sets the single value and never reads `step`. Workaround: use `*/2`.
  **Fix:** honour `N/step` in `parseCronPart` — start at N, step by the step — plus
  a unit test for `0/2`, `5/10`.

## Cookbook Remediation Route Ignores Organisation

- [ ] **`handle_cookbook_remediation.go` resolves a cookbook by name+version and never reads `organisation`**, though `server_cookbook_cookstyle_results` is keyed by `(organisation_name, cookbook_name, cookbook_version, target_chef_version)`. The same name+version in two organisations is therefore ambiguous on `/cookbooks/:name/:version/remediation` — whichever row the query returns first wins, silently. **Strategic fix:** thread `organisation` through the route and the query. Another instance of the cross-view record-selection family — the fix is to make the selection explicit, not to derive it.

## Event Ingest Endpoint Is Unauthenticated (MVP)

- [ ] **`POST /api/v1/ingest` accepts any POST and ignores `Authorization`.** Deliberate
  MVP choice: adding auth on deployed chef-clients / the Chef Server proxy / Automate's
  Data Feed destination needs change control, deferred to prove value first. Ingest is off
  unless somebody enables it, so this reaches only deployments that turned it on. Where it is
  on it is a real exposure — an unauthenticated ingress can be flooded or fed spoofed run data, and
  it is CMM's first inbound endpoint (everything else is pull-only). **Proper fix:** a
  shared bearer token or basic-auth secret validated at the handler, stored in the
  encrypted config store, with the value handed to the producer side. Automate's Data
  Feed already sends basic-auth creds we currently ignore, so that shape needs no client
  change; the node/proxy shapes do. Revisit before any non-lab exposure.

## Frontend — a failed fetch renders as "there are none"

**12 sites, listed below.** The pattern is
`.catch(() => setSomething([]))`: a request that fails leaves the view showing an empty list,
which is indistinguishable on screen from a list that is genuinely empty and means the
opposite thing. Six of them are the Nodes page's own filter dropdowns, so a role or platform
catalogue that fails to load tells an operator there are no roles.

**The fix is not a rewrite.** `OwnerFilter` already does it correctly: keep the empty list for
rendering, set a `loadFailed` flag, and say "could not load" instead of "none". Each site is a
few lines, and each one is independently shippable.

- [ ] Sites: `NodesPage` (policy names, policy groups, environments, roles, platforms),
  `RunEventsPage` (organisations, versions), `RemediationPage` (complexity labels),
  `OwnershipMappedImport` (saved mappings), `OwnerDuplicatesPage` (rejected pairs),
  `AdminTestKitchenPage` (mapping status), `CookbookDetailPage` (platform coverage — this
  one also needs the API to tell "not evaluated" from "no such cookbook").
