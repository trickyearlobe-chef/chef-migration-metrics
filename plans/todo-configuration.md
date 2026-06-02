# Configuration — ToDo

### Configuration UI Gaps

Settings in `specifications/configuration.md` that have **no UI** and are currently file-only:

- [ ] **Concurrency**: add `test_kitchen_run` worker spinner to Concurrency page (spec: `concurrency.test_kitchen_run`, default 4 — missing from UI, 6/7 workers shown)
- [ ] **Elasticsearch export**: add Elasticsearch page or section to Exports page — `elasticsearch.enabled`, `elasticsearch.output_directory`, `elasticsearch.retention_hours`
- [ ] **Upgrade Readiness**: add `readiness.min_free_disk_mb` to Collection page (minimum free disk for Habitat bundle upgrade check)

Settings intentionally file-only (no UI needed):
- `server.listen_address`, `server.port` — require restart; changing via UI would disconnect the session
- `frontend.base_path` — deployment-time reverse proxy config

---

### TLS and Certificate Management

- [ ] Implement ACME client integration (CertMagic recommended)
- [ ] Implement ACME HTTP-01 challenge handler on the redirect listener
- [ ] Implement ACME DNS-01 challenge support with Route 53 provider
- [ ] Implement ACME certificate storage to `acme.storage_path` with correct permissions (0700/0600)
- [ ] Implement automatic certificate renewal before expiry (`renew_before_days`)
- [ ] Implement exponential backoff on ACME renewal failure (1h → 24h cap)
- [ ] Log ACME certificate obtained/renewed at `INFO`, renewal failure at `ERROR`
- [ ] Log `WARN` when certificate is within 7 days of expiry and renewal has not succeeded
- [ ] Implement `agree_to_tos` gate — refuse to start in ACME mode unless `true`
- [ ] Log `WARN` when ACME staging CA URL is detected
- [ ] Implement backward compatibility: treat `tls.enabled: true` as `mode: static` with deprecation warning
- [ ] Validate all ACME settings on startup (domains, email, agree_to_tos, storage_path, challenge, dns_provider)
- [ ] Validate `http_redirect_port` is set when `challenge: http-01`
