# Configuration — ToDo

Status key: [ ] Not started | [~] In progress | [x] Done

---

### TLS and Certificate Management

- [ ] Implement ACME client integration (CertMagic or `autocert` — CertMagic recommended)
- [ ] Implement ACME HTTP-01 challenge handler on the redirect listener
- [ ] Implement ACME TLS-ALPN-01 challenge handler on the main HTTPS listener
- [ ] Implement ACME DNS-01 challenge support with pluggable DNS provider interface
- [ ] Implement DNS-01 provider: Amazon Route 53
- [ ] Implement DNS-01 provider: Cloudflare
- [ ] Implement DNS-01 provider: Google Cloud DNS
- [ ] Implement DNS-01 provider: Azure DNS
- [ ] Implement DNS-01 provider: RFC 2136 (Dynamic DNS / TSIG)
- [ ] Implement ACME certificate storage to `acme.storage_path` with correct permissions (0700/0600)
- [ ] Implement automatic certificate renewal before expiry (`renew_before_days`)
- [ ] Implement exponential backoff on ACME renewal failure (1h → 24h cap)
- [ ] Log ACME certificate obtained/renewed at `INFO`, renewal failure at `ERROR`
- [ ] Log `WARN` when certificate is within 7 days of expiry and renewal has not succeeded
- [ ] Send `certificate_expiry_warning` notification event when certificate is near expiry
- [ ] Implement `agree_to_tos` gate — refuse to start in ACME mode unless `true`
- [ ] Log `WARN` when ACME staging CA URL is detected
- [ ] Implement multi-replica coordination for ACME via file-based locking in `storage_path`
- [ ] Implement OCSP stapling for ACME-obtained certificates
- [ ] Implement backward compatibility: treat `tls.enabled: true` as `mode: static` with deprecation warning
- [ ] Validate all ACME settings on startup (domains, email, agree_to_tos, storage_path, challenge, dns_provider)
- [ ] Validate `http_redirect_port` is set when `challenge: http-01`
- [ ] Update `healthcheck` CLI subcommand to support HTTPS with `--insecure` flag for TLS skip-verify
- [ ] Add TLS-related entries to the logging specification (`tls` log scope)
- [ ] Add `certificate_expiry_warning` to the notification events list