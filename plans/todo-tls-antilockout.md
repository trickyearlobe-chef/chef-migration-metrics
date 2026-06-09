# TODO — TLS anti-lockout gaps

Surfaced 2026-06-08 by a customer mTLS lockout (`ERR_BAD_SSL_CLIENT_AUTH_CERT`)
caused by setting `server.tls.ca_path`. UI clarity for that field is done
(see git history / `fix/tls-capath-mtls-ui-clarity`).

The mTLS escape hatch and the missing-file fail-open (formerly items 1 and 2)
are resolved by `fix/tls-mtls-failopen-escape-hatch`: `validateTLSStatic` is now
structural-only, so a missing/moved cert/key/CA file no longer aborts startup —
the listener falls open to plain HTTP (`tls.md` § 2.4). For DB-stored TLS material
(`cert_source: db`, ACME state, `ca_path`) on-host file removal no longer applies;
the supported recovery is the **repair CLI** (`tls reset` / `tls clear-ca`,
delivered in TLS Chunk 3a, `cmd/.../tlsrepair.go`) — see `tls.md` § 6.3. The
remaining gap:

## 1. `CHEF_MIGRATION_METRICS_SERVER_TLS_MODE` ignored on DB path (documented limitation)
`applyEnvOverrides` (`internal/config/config.go`) runs only on the YAML `ParseRaw`
path, not in `configstore.AssembleConfig`. So the documented env override (§ 2.5)
silently does nothing when config is DB-managed. Per the TLS plan decision this is
**no longer load-bearing** — the repair CLI is the escape hatch, not this env var.
Leave as a documented limitation in `tls.md` § 2.5 unless a future need arises to
apply env overrides after assembly.
