# Tech Debt

Status key: [ ] Not started | [~] In progress | [x] Done

---

## Frontend — Large Files

- [ ] `NodeDetailPage.tsx` (~1136 lines) contains 10+ sub-components — extract `DiskSpacePanel`, `CookbookCompatibilityTable`, `ReadinessCard`, `ReadinessSection`, `InfoCard` into separate files.
- [ ] `StatusCards.tsx` (~860 lines) — 7 cards each repeat the same fetch-load-error pattern; extract a `useFetch<T>` hook to eliminate boilerplate.

## Frontend — Inconsistency

- [ ] `RemediationPage.tsx` uses hand-rolled sort logic (L67–68, L151–164) instead of `useSort` hook + `SortableColumnHeader` that every other sortable page uses.
- [ ] `DownloadStatusBadge` (CookbooksPage) and `CloneStatusBadge` (GitReposPage) are near-identical — unify into a shared component.

## Secrets — config_store Master Key Rotation

- [ ] When `CMM_CREDENTIAL_ENCRYPTION_KEY` is rotated, entries in `config_store` (including all credentials) are **not** re-encrypted under the new key. The old `rotateSecrets` function only operated on the now-dropped `credentials` table. **Strategic fix:** implement `Store.RotateKey(ctx, oldKey []byte)` that re-encrypts every `config_store` row under the new derived key within a single transaction.

## Backend — Code Smells

- [ ] `DataStore` interface has 138+ methods (`webapi/store.go`) — consider splitting into domain-specific sub-interfaces (nodes, cookbooks, kitchen, auth, config, etc.) composed into the full interface.

## Database

- [ ] Migrations 0001–0009 establish natural composite keys; migrations 0013–0016 reintroduce UUID PKs for `vm_tracking`, `node_kitchen_runs`, `kitchen_batches`, `git_kitchen_results`. This is a deliberate choice (these tables model ephemeral operational records, not domain entities) but should be documented in `project-conventions.md` under a "Primary Key Strategy" section.

## Test Kitchen — Driver-Specific Suite Failures

- [ ] `kubernetes-cluster` git repo: `ha-cluster-k8s135-cp1` suite hardcodes `control_plane_endpoint: "192.168.56.10:6443"` in `kitchen.yml`, which depends on a Vagrant `private_network` interface. On non-Vagrant drivers (e.g. Proxmox), that IP is never created — kubeadm starts on the Proxmox-assigned IP but times out trying to reach the hardcoded endpoint. This is a suite-level incompatibility with non-Vagrant drivers, not a cookbook bug. **Strategic fix:** detect or flag suites that reference driver-specific networking (e.g. `192.168.56.x`) and either skip them or allow per-suite driver overrides in the kitchen overlay.

## Kitchen Queue — Live Output Streaming

- [ ] The kitchen queue shows output only after a run completes. True live streaming during execution would require: (a) an SSE endpoint per queue item, (b) a ring buffer in the executor to capture output lines as they arrive, (c) frontend `EventSource` subscription. Deferred because the project has no existing SSE infrastructure and the post-completion output (already available via `GET /kitchen/queue/:id`) covers 90% of the use case.

## Kitchen — Cloud Driver Orphan Detection

- [ ] The orphan sweep relies on VM naming conventions (embedded timestamp) and Proxmox uptime as fallback. Cloud drivers (EC2, GCE, Azure) name instances differently and don't expose uptime in the same way. **Strategic fix:** Use cloud-native tagging (e.g. `cmm-created-at: <timestamp>` tag on EC2 instances) and query by tag for orphan detection. Each cloud driver would need a sweep adapter. Only needed when Test Kitchen is used with cloud drivers at scale.

## Phasing Notes

These are not debt — they are deliberate holds awaiting prerequisites.

- SAML authentication endpoints return 501 — waiting for customer environment access to test.
- Notification subsystem (`internal/notify/`) not yet implemented — entire feature deferred.
- TLS ACME mode logged as "not yet implemented" in `main.go`.