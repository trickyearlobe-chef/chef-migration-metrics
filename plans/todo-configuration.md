# Configuration — ToDo

### Configuration UI Gaps

Settings in `specifications/configuration.md` that have **no UI** and are currently file-only:

- [ ] **Concurrency**: add `test_kitchen_run` worker spinner to Concurrency page (spec: `concurrency.test_kitchen_run`, default 4 — missing from UI, 6/7 workers shown)
- [ ] **Elasticsearch export**: add Elasticsearch page or section to Exports page — `elasticsearch.enabled`, `elasticsearch.output_directory`, `elasticsearch.retention_hours`
- [x] **Upgrade Readiness**: add disk space config to Collection/Readiness page — `readiness.install_path_linux`, `readiness.install_path_windows`, `readiness.install_size_mb_linux`, `readiness.install_size_mb_windows`, `readiness.min_remaining_free_percent`; install path fields must show a prominent warning about non-default path risks (cookbook assumptions, Windows knife bootstrap config dir issue)

Settings intentionally file-only (no UI needed):
- `frontend.base_path` — deployment-time reverse proxy config

`server.listen_address`/`server.port` are now DB-managed and UI-editable
(restart-required, bind-failure fallback) — see Chunk 3 in `plans/active.md`.

---

### TLS and Certificate Management

Done — TLS-in-DB / CSR / ACME branch (`feature/tls-db-certs-csr-acme`, Chunks 7–10).
ACME runs on `x/crypto/acme` directly (CertMagic/lego deliberately rejected,
`tls-acme.md` § 3.1); all state is DB-backed (no `storage_path`). HTTP-01 +
Route 53 DNS-01 solvers, renewal with 1h→24h backoff + 7-day expiry WARN,
`agree_to_tos` gate, staging-CA WARN, `tls.enabled`→`mode` deprecation, and
startup validation all shipped. See `plans/tls-db-certs-csr-acme.md`.
