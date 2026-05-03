# Tech Debt

Status key: [ ] Not started | [~] In progress | [x] Done

---

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

## Database

- [ ] Migrations 0001–0009 establish natural composite keys; migrations 0013–0016 reintroduce UUID PKs for `vm_tracking`, `node_kitchen_runs`, `kitchen_batches`, `git_kitchen_results`. This is a deliberate choice (these tables model ephemeral operational records, not domain entities) but should be documented in `project-conventions.md` under a "Primary Key Strategy" section.
- [ ] **Roles compat summary not pre-computed** — `GetRoleCompatSummary` runs a full recursive CTE over all roles on every cache miss (60s TTL). At 67k+ roles this is slow. **Strategic fix:** write `(org, target_chef_version, compatible, incompatible, untested, total)` rows to a `role_compat_summary` table at the end of each collection run. The summary bar and compat-filter fast path read from that table (O(1)) instead of re-expanding the dep graph. The dashboard cookbook-compatibility card has the same problem.
- [ ] **Roles list slow for non-name sort fields** — sorting by `node_count` or `incompatible_cookbook_count` still uses the single-query slow path (full recursive CTE over all roles before sorting). **Strategic fix:** store pre-computed node counts and compat counts per role in a summary table (same as above), enabling O(1) sorts.

## Test Kitchen — Driver-Specific Suite Failures

- [ ] `kubernetes-cluster` git repo: `ha-cluster-k8s135-cp1` suite hardcodes `control_plane_endpoint: "192.168.56.10:6443"` in `kitchen.yml`, which depends on a Vagrant `private_network` interface. On non-Vagrant drivers (e.g. Proxmox), that IP is never created — kubeadm starts on the Proxmox-assigned IP but times out trying to reach the hardcoded endpoint. This is a suite-level incompatibility with non-Vagrant drivers, not a cookbook bug. **Strategic fix:** detect or flag suites that reference driver-specific networking (e.g. `192.168.56.x`) and either skip them or allow per-suite driver overrides in the kitchen overlay.

## Kitchen Queue — Live Output Streaming

- [ ] The kitchen queue shows output only after a run completes. True live streaming during execution would require: (a) an SSE endpoint per queue item, (b) a ring buffer in the executor to capture output lines as they arrive, (c) frontend `EventSource` subscription. Deferred because the project has no existing SSE infrastructure and the post-completion output (already available via `GET /kitchen/queue/:id`) covers 90% of the use case.

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

## Phasing Notes

These are not debt — they are deliberate holds awaiting prerequisites.

- SAML authentication endpoints return 501 — waiting for customer environment access to test.
- Notification subsystem (`internal/notify/`) not yet implemented — entire feature deferred.
- TLS ACME mode logged as "not yet implemented" in `main.go`.