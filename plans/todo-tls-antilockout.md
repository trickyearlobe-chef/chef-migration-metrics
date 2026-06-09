# TODO — TLS anti-lockout gaps

Surfaced 2026-06-08 by a customer mTLS lockout (`ERR_BAD_SSL_CLIENT_AUTH_CERT`)
caused by setting `server.tls.ca_path`. UI clarity for that field is done
(see git history / `fix/tls-capath-mtls-ui-clarity`).

The mTLS escape hatch and the missing-file fail-open (formerly items 1 and 2)
are resolved by `fix/tls-mtls-failopen-escape-hatch`: `validateTLSStatic` is now
structural-only, so a missing/moved cert/key/CA file no longer aborts startup —
the listener falls open to plain HTTP (`tls.md` § 2.4) and on-host file removal
is the supported mTLS-lockout recovery (`tls.md` § 5.3). The remaining gap:

## 1. `CHEF_MIGRATION_METRICS_SERVER_TLS_MODE` ignored on DB path
`applyEnvOverrides` (`internal/config/config.go`) runs only on the YAML `ParseRaw`
path, not in `configstore.AssembleConfig`. So the documented env override (§ 2.5)
silently does nothing when config is DB-managed. Lower priority now that on-host
file removal is the primary recovery path. Decide: apply env overrides after
assembly, or document the limitation in `tls.md` § 2.5.
