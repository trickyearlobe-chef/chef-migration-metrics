# Tech Debt

Status key: [ ] Not started | [~] In progress | [x] Done

---

## Architecture — Duplicated Derived Calculations

- [ ] **Client-side filtering/sorting/dashboarding is fragile** — derived values like blast radius scores, complexity calculations, and TK pass/fail/partial statuses are computed independently in multiple places (API handlers, frontend sort comparators, dashboard aggregation, export formatters). When the calculation logic drifts between copies, filters disagree with dashboards and sort order doesn't match displayed values. **Strategic fix:** push derived calculations into the database as materialised columns or summary tables, computed once at collection time. API surfaces then filter/sort/aggregate on pre-computed values rather than re-deriving them. This also enables server-side pagination with correct sort order and eliminates the class of paging bugs caused by client-side recomputation. Relates to the platform filter tree multiselect item (server-side group filtering). **Note (Phase 2 caption):** adding `platform_caption` introduces a 4th input to `ResolveInfo` that is re-derived at 6 call sites — increases urgency of materialising platform display/group into DB columns.

## Frontend — Large Files

- [ ] `NodeDetailPage.tsx` (~1136 lines) contains 10+ sub-components — extract `DiskSpacePanel`, `CookbookCompatibilityTable`, `ReadinessCard`, `ReadinessSection`, `InfoCard` into separate files.
- [ ] `StatusCards.tsx` (~860 lines) — 7 cards each repeat the same fetch-load-error pattern; extract a `useFetch<T>` hook to eliminate boilerplate.

## Frontend — Inconsistency

- [ ] `RemediationPage.tsx` uses hand-rolled sort logic (L67–68, L151–164) instead of `useSort` hook + `SortableColumnHeader` that every other sortable page uses.
- [ ] `DownloadStatusBadge` (CookbooksPage) and `CloneStatusBadge` (GitReposPage) are near-identical — unify into a shared component.
- [ ] **Platform filter is flat multiselect** — should be a tree-based multiselect allowing selection at group level (e.g. "RHEL 8") or individual version level (e.g. "RHEL 8.10"). Requires deciding whether group expansion happens client-side or server-side (API accepts `group_key` filter and resolves in SQL). Server-side is preferred to avoid paging instability. Part of a broader design around server-side vs client-side data processing.

## Secrets — config_store Master Key Rotation

- [ ] When `CMM_CREDENTIAL_ENCRYPTION_KEY` is rotated, entries in `config_store` (including all credentials) are **not** re-encrypted under the new key. The old `rotateSecrets` function only operated on the now-dropped `credentials` table. **Strategic fix:** implement `Store.RotateKey(ctx, oldKey []byte)` that re-encrypts every `config_store` row under the new derived key within a single transaction.

## Backend — Code Smells

- [ ] `DataStore` interface has 138+ methods (`webapi/store.go`) — consider splitting into domain-specific sub-interfaces (nodes, cookbooks, kitchen, auth, config, etc.) composed into the full interface.
- [ ] **SAML commit re-introduced `Ownership.Enabled` gate** — commit `fc9f511` removed the `Enabled` flag to make ownership always active, but the SAML commit (`e6c4ff1`) re-added `r.cfg.Ownership.Enabled` checks in `handle_ownership.go` (`requireOwnership` helper, `resolveOwnershipFilter`). Since `Enabled` defaults to `false`, ownership appears disabled unless manually configured. **Strategic fix:** remove `Ownership.Enabled` field from `OwnershipConfig` again, remove `requireOwnership()` helper and all `r.cfg.Ownership.Enabled` checks from `handle_ownership.go`, and remove the env-var override at `config.go:1177`. Ownership should always be active.
- [ ] **`target_chef_versions` is a list but only one target is ever active** — config stores `TargetChefVersions []string` and code picks the highest via `config.HighestVersion()`. This is confusing and error-prone (users may add multiple values thinking they'll all be tested). **Strategic fix:** change config to `target_chef_version: "18.5.0"` (scalar string), update `config.Config`, admin API PUT/GET, frontend admin page, and all call sites that index into the slice. Remove `HighestVersion()` helper.

## Database

- [ ] Migrations 0001–0009 establish natural composite keys; migrations 0013–0016 reintroduce UUID PKs for `vm_tracking`, `node_kitchen_runs`, `kitchen_batches`, `git_kitchen_results`. This is a deliberate choice (these tables model ephemeral operational records, not domain entities) but should be documented in `project-conventions.md` under a "Primary Key Strategy" section.
- [ ] **Roles compat summary not pre-computed** — `GetRoleCompatSummary` runs a full recursive CTE over all roles on every cache miss (60s TTL). At 67k+ roles this is slow. **Strategic fix:** write `(org, target_chef_version, compatible, incompatible, untested, total)` rows to a `role_compat_summary` table at the end of each collection run. The summary bar and compat-filter fast path read from that table (O(1)) instead of re-expanding the dep graph. The dashboard cookbook-compatibility card has the same problem.
- [ ] **Roles list slow for non-name sort fields** — sorting by `node_count` or `incompatible_cookbook_count` still uses the single-query slow path (full recursive CTE over all roles before sorting). **Strategic fix:** store pre-computed node counts and compat counts per role in a summary table (same as above), enabling O(1) sorts.
- [ ] **Cross-org aggregations may produce incorrect counts** — API responses for roles and cookbooks aggregate entities across organisations (e.g. `RoleDetail.Organisations []string`). If downstream logic counts array lengths to derive totals (e.g. "number of affected roles"), it may be counting orgs-per-entity rather than entities. **Investigate during Phase 1 (Semantic Contracts):** trace each metric calculation back to its source query and verify that cross-org grouping does not inflate or deflate counts. Particularly check `affected_role_count`, `affected_node_count`, and any dashboard card that sums across the org dimension.
- [ ] **`git_repo_name` + `git_repo_url` composite FK repeated across many tables** — `git_repo_complexity`, `git_repo_cookstyle_results`, `git_repo_autocorrect_previews`, `kitchen_analysis_results`, `git_kitchen_results`, `kitchen_instance_exclusions` all carry both columns as composite FK to `git_repos`. This bloats storage and complicates joins. **Strategic fix:** consider a surrogate `git_repo_id` PK on `git_repos` with child tables referencing it.
- [ ] **`collection_run_org` denormalised across tables** — appears in `node_snapshots`, `metric_snapshots`, `cookbook_usage_analysis`, `log_entries`. Purpose unclear — may be stale from an earlier design. **Investigate:** determine if this is still needed or can be removed.
- [ ] **Organisation as outermost container not enforced consistently** — in Chef's domain model, org contains nodes/cookbooks/roles. Some API responses present entities role-first or cookbook-first with `organisations: [...]` arrays, inverting the containment. Review during Phase 1 whether the data model should enforce org-first hierarchy more strictly.

## Test Kitchen — Driver-Specific Suite Failures

- [ ] `kubernetes-cluster` git repo: `ha-cluster-k8s135-cp1` suite hardcodes `control_plane_endpoint: "192.168.56.10:6443"` in `kitchen.yml`, which depends on a Vagrant `private_network` interface. On non-Vagrant drivers (e.g. Proxmox), that IP is never created — kubeadm starts on the Proxmox-assigned IP but times out trying to reach the hardcoded endpoint. This is a suite-level incompatibility with non-Vagrant drivers, not a cookbook bug. **Strategic fix:** detect or flag suites that reference driver-specific networking (e.g. `192.168.56.x`) and either skip them or allow per-suite driver overrides in the kitchen overlay.

## Test Kitchen — Pre-Converge Scripts

- [ ] **Some git repos contain shell scripts (e.g. `users.sh`) that must be executed on the VM before converge** — these set up prerequisite state (users, groups, packages) that the cookbook expects to exist. Without them, converge fails for the wrong reason. **Strategic fix:** add a configurable pre-converge hook mechanism — detect scripts matching a naming convention or path (e.g. `scripts/pre-converge/*.sh`, or a top-level `users.sh`), upload and execute them on the VM before the Chef run. Could be a per-repo config item specifying which scripts to run and in what order.

## Test Kitchen — IP-Release pre_destroy Hook (Unvalidated Spike)

- [ ] **The opt-in IP-release `pre_destroy` hook is shipped but empirically unvalidated** — `gitkitchen/overlay.go` injects an OS-family DHCP-release command (`linuxIPReleaseCommand` / `windowsIPReleaseCommand`) when an image sets `release_ip_on_destroy`. It is failure-isolated (always `exit 0`, detached, stdio redirected) so it cannot fail a run, but whether it actually releases the lease before hypervisor power-off is only verifiable on the customer OS mix. **Before relying on it:** validate per platform that (a) the DHCPRELEASE leaves the guest ahead of destroy, and (b) the run result is unchanged on a simulated hook failure. Until validated, the VM start-rate limiter remains the sole pool-exhaustion guarantee. Known limitations to revisit:
  - **No `sudo`** — the Linux command runs the release binaries directly, so it is a no-op on images whose transport user is non-root with passwordless sudo. Add a tolerant `sudo -n` prefix once an image needs it.
  - **Composition is `pre_destroy`-only and primary-file-only** — `readExistingPreDestroy` reads only the cookbook's primary `.kitchen.yml` (via `DiscoverKitchenFiles`), not variant files or driver-specific overlays. If CMM ever injects a second lifecycle phase, the no-clobber composition must be generalised beyond the single hard-coded phase.
  - **Detach-vs-power-off race** — detaching to survive a severed transport races the hypervisor destroy; the release packet must leave first. Only empirically tunable, no code guarantee.

## Kitchen Queue — Live Output Streaming

- [ ] The kitchen queue shows output only after a run completes. True live streaming during execution would require: (a) an SSE endpoint per queue item, (b) a ring buffer in the executor to capture output lines as they arrive, (c) frontend `EventSource` subscription. Deferred because the project has no existing SSE infrastructure and the post-completion output (already available via `GET /kitchen/queue/:id`) covers 90% of the use case.

## Kitchen — Proxmox VMID Race Condition (Upstream)

- [ ] **Proxmox `nextid` API has no reservation mechanism** — `GET /cluster/nextid` returns the lowest free VMID but doesn't reserve it. When multiple clients clone concurrently, they can receive the same VMID, causing lock timeouts and one process inadvertently destroying another's VM. **Upstream ticket:** Bugzilla #7553 filed requesting atomic VMID allocation. **Workarounds implemented in kitchen-proxmox (branch `fix/lock-timeout-retry`):**
  - `vmid_conflict?` detection in `allocate_and_clone`: retries clone with a fresh VMID when the clone POST returns "already exists", "unable to create VM", or "can't lock file" (immediate 500 from Proxmox).
  - `wait_for_task` exit status check: raises `ApiError` when a clone task completes with a non-OK exit status (e.g. lock timeout discovered only after task polling). Previously the driver silently treated failed tasks as successful.
  - `vmid_race_lost?` detection in `clone_and_start`: if configure or start fails with "already running", "hotplug problem", "does not exist", or "can't lock file", the driver abandons the VMID (does NOT destroy — it belongs to another process), clears Kitchen state, and retries the full create sequence with a new VMID.
  - Exponential backoff with jitter between retries (up to `clone_retries`, default 5).
  - **Strategic fix (if upstream resolves):** remove retry logic and use atomic VMID allocation or omit `newid` from clone request. Alternative client-side mitigation: use random VMIDs in a high range (900000–999999) to reduce collision probability.

### Empirical Findings (2026-05-04)

Tested against live Proxmox VE cluster (2 nodes). Key findings:

1. **`nextid` is a validator, not an allocator** — `GET /cluster/nextid?vmid=X` checks if X is free and returns it or errors. Without `vmid` param it scans sequentially from 100. No reservation occurs either way. Multiple concurrent calls return the same value.

2. **`newid` is mandatory on clone** — Proxmox will not auto-assign a VMID. The client must always pick one, making the race unavoidable at the API level.

3. **Two concurrent clone POSTs to the same VMID are both accepted** (HTTP 200 with valid task UPIDs). The race resolves at the filesystem lock level:
   - **Full clone** (~15-20s): loser gets `exitstatus: "can't lock file '/var/lock/qemu-server/lock-<vmid>.conf' - got timeout"` — only discoverable by polling task status.
   - **Linked clone** (<1s): loser gets `exitstatus: "unable to create VM <vmid>: config file already exists"` — faster rejection due to shorter clone time.

4. **No ownership boundary** — the losing caller can still `start`, `stop`, or `destroy` the VMID that was cloned by the winner. There is no concept of "this task created this VM".

5. **Random high-range strategy works** — 3 concurrent full clones to pre-selected unique VMIDs (900001-900003) all succeeded without conflict. VMID range is 100–999,999,999. Using `rand(900000..999999)` gives ~0.003% collision probability with 10 concurrent clients.

6. **Recommended approach**: generate a random VMID in a high range, validate with `nextid?vmid=X`, clone, poll task `exitstatus`, retry on conflict. The retry logic in `fix/lock-timeout-retry` covers this but should be updated to use random high-range VMIDs as the primary allocation strategy rather than relying on sequential `nextid`.

## Kitchen — Cloud Driver Orphan Detection

- [ ] The orphan sweep relies on VM naming conventions (embedded timestamp) and Proxmox uptime as fallback. Cloud drivers (EC2, GCE, Azure) name instances differently and don't expose uptime in the same way. **Strategic fix:** Use cloud-native tagging (e.g. `cmm-created-at: <timestamp>` tag on EC2 instances) and query by tag for orphan detection. Each cloud driver would need a sweep adapter. Only needed when Test Kitchen is used with cloud drivers at scale.

## Hypervisor — Split REST/SOAP APIs for vCenter

- [ ] `VCenterClient` uses two different API transports: govmomi (SOAP/PropertyCollector) for `ListTemplates` and the vSphere REST API (`/api/vcenter/vm`) for `ListManagedVMs` and `DestroyVM`. This works but means two auth sessions, two TLS connections, and two code paths to maintain. **Strategic fix:** migrate all vCenter operations to govmomi so there is a single SOAP session — use `object.VirtualMachine.Destroy` and `PowerOff` for VM cleanup, and Finder queries for managed VM listing. Remove the REST client entirely.

## Node Kitchen — Supplemental Data Sources

- [ ] Node kitchen runs currently execute against the node object alone. Real cookbook convergence typically requires supplemental data that comes from other Chef sources: environment attributes, role attributes, data bags, and Chef Vault items. Without these, test runs may silently succeed (missing data causes cookbooks to skip blocks or use defaults) or fail for the wrong reasons. **Strategic fix:** design a data-injection layer for node kitchen runs that can pull or mock environment/role attributes, data bag items, and vault secrets — either by fetching them from the live Chef server at run time, by allowing per-node or per-org overrides to be stored in CMM, or by generating a synthetic node JSON that merges all attribute sources before converging. Needs a solid design plan before implementation.

## Kitchen — Integration Testing with InSpec

- [ ] Git cookbooks may contain InSpec profiles or existing Test Kitchen verifier configs that test individual cookbook behaviour. There is an unaddressed gap for *integration* tests that verify multiple cookbooks converge correctly together on a single node — i.e. the full runlist plays nice end-to-end. Additionally, InSpec-based verification (as a Kitchen verifier) is not yet wired into the git kitchen pipeline. **Strategic fix:** (a) detect and surface InSpec profiles present in git repos; (b) support InSpec as a verifier option alongside the existing verifiers in the git kitchen pipeline; (c) define a mechanism for composing multi-cookbook integration suites that reflect real-world runlists, potentially derived from existing node data. Needs a solid design plan before implementation.

## Security

- [ ] **Content-Security-Policy not set** — `X-Frame-Options`, `X-Content-Type-Options`, and `Referrer-Policy` are now set by `SecurityHeadersMiddleware`. CSP is deferred because the React frontend uses runtime-computed inline `style={{}}` props (progress bars, data-driven colours) that require `style-src 'unsafe-inline'`, making a strict CSP a non-trivial frontend refactoring effort. **Strategic fix:** convert dynamic inline styles to CSS custom properties (`--foo: value`) set via JS, then remove `unsafe-inline` from CSP.
- [ ] **MD5 checksums in chefapi** — `internal/chefapi/client.go` uses `crypto/md5` for cookbook checksum verification when the Chef server returns MD5 hashes (Chef protocol requirement). MD5 is cryptographically broken for collision resistance. Risk is low (verifying server-returned data over an already-authenticated channel) but should be documented as protocol-forced. **Strategic fix:** upgrade Chef API version in requests to prefer SHA-256 checksums and only fall back to MD5 for older server versions.
- [ ] **BlockingCookbooks paths don't include cookbook→cookbook transitive deps** — `getBlockingCookbooks`/`collectPaths` in `datastore/role_detail.go` only walk role→role and role→cookbook edges, so a cookbook that is incompatible purely because one of *its* dependencies is incompatible won't appear in `BlockingCookbooks` with the correct dependency path. The dependency tree and graph correctly show the full expansion (fixed in `fix/roles-bugs`), but the blocking-cookbook path computation remains role-edge-only. **Strategic fix:** extend `collectPaths` to also follow cookbook→cookbook edges using a `cbAdj` map, building paths like `role:web → cookbook:nginx → cookbook:apt`.

## Ownership — Committer-to-Owner Email Mapping

- [ ] `GetOwnerEmailsForGitRepo` marks a committer as `is_owner` by matching the committer's `author_email` against the owner's single `contact_email`. When two committer emails map to the same `owner_name` (e.g. `user@example` and `user@example.com` both produce owner_name `user`), only the first email is stored as `contact_email`. The second committer never shows as "Owner" in the UI despite sharing the same owner identity. **Strategic fix:** either (a) store multiple contact emails per owner (many-to-one), or (b) match `is_owner` by owner_name derivation (email prefix) rather than exact contact_email comparison.
- [ ] **`maintainer_email` from `metadata.rb` not used for ownership** — cookbook metadata contains `maintainer` and `maintainer_email` fields explicitly declared by the author. This is a stronger ownership signal than git commit email heuristics. We store `maintainer` in `server_cookbooks` but not `maintainer_email`. **Strategic fix:** collect `maintainer_email`, use it as an ownership auto-assignment source (`assignment_source: "metadata"`, `confidence: "definitive"`). Also available in git repo `metadata.rb`/`metadata.json`.

## Git — Committers Not Populated

- [x] **Git repo committers no longer being collected** — Root cause: committer extraction was gated behind `Ownership.Enabled` which defaulted to `false` with no UI toggle. Fixed by removing the flag entirely — ownership (and committer collection) is now always active.

## Backup — Scheduled Cron Not Firing at Customer

- [ ] **Backup cron schedule not triggering** — customer has `0 2 * * *` configured with "Enable scheduled backups" checked, but no scheduled backups are being created. Manual "Create Backup Now" works (2.3 GB backup succeeded 21/05/2026). Only one backup exists, implying cron has never fired since deployment. **Root cause (suspected):** config was changed via admin UI but the app was not restarted; the backup scheduler goroutine reads config only at startup and does not re-read on config-store changes. **Strategic fix:** the backup scheduler must subscribe to config-store change notifications (or re-read config on each tick) so that enable/disable and schedule changes take effect immediately. See configuration spec § Live Reload Requirement.

## Backup — No Log Scope Filter in UI

- [ ] **No way to filter logs to show backup activity** — the log UI has no scope/category filter to isolate backup-related log entries. The backup service logs using `ScopeBackup` but users cannot filter by scope in the logs page. **Strategic fix:** add a scope filter dropdown to the log viewer UI (selecting from known scopes: backup, collection, analysis, kitchen, etc.) and a corresponding `?scope=backup` query param on the log entries API.

## UI — Misleading Exclusion Tooltip

- [ ] **"Known to be incompatible" tooltip is hardcoded regardless of actual exclusion reason** — `StatusBadge.tsx` displays the tooltip "Known to be incompatible with the target Chef version" for all excluded/skipped kitchen instances. However, exclusions can be created for many reasons (EOL platform, no hypervisor image, not deployed there, licensing cost, flaky infra, irrelevant suite). The `kitchen_instance_exclusions` table has a `reason` field that captures the actual motivation. **Strategic fix:** pass the exclusion `reason` through to the frontend and display it in the tooltip (e.g. "Excluded: Platform is EOL and being decommissioned"). Fall back to a generic "Excluded from testing" message when `reason` is empty, rather than claiming incompatibility.

## UI — Redundant Target Version Selector

- [ ] **Target version dropdown is dead weight** — the project uses a single-target model where changing the target invalidates all previous analysis (cookstyle, TK, readiness) and the startup reconciliation purges live-state data for old versions. This means there's only ever one version of live data. Despite this, the UI shows a `<select>` dropdown (via `GlobalFilterContext.targetChefVersion`) populated from `GET /filters/target-chef-versions`. Since the config list always has exactly one entry, the dropdown shows a single option that cannot be changed. On the dashboard trends page, the selected version isn't even passed to the API — the backend iterates all configured versions internally. **Strategic fix:** hide the selector entirely when `targetVersions.length <= 1` (in `AppLayout.tsx` where the global filter bar is rendered). Longer term, align with the scalar config change (`target_chef_version: string` instead of `target_chef_versions: []string`) tracked above under "Backend — Code Smells".

## Specifications — Stub / Incomplete

- [ ] **`specifications/datastore.md` is a stub** — the datastore is referenced by ~10 other specs but has no full prose specification; the authoritative schema currently lives only in `migrations/*.up.sql`. A stub was added (during the spec-split/link-fix work) so cross-spec links resolve and the LLM is oriented. **Fix:** write the full datastore spec — table definitions and relationships, data-access patterns per consuming component (collector, analysis, web API, ownership), and retention/snapshot behaviour — using the migrations as the source of truth.

## Phasing Notes

These are not debt — they are deliberate holds awaiting prerequisites.

- SAML authentication endpoints return 501 — waiting for customer environment access to test.
- Notification subsystem (`internal/notify/`) not yet implemented — entire feature deferred.
- TLS ACME mode logged as "not yet implemented" in `main.go`.