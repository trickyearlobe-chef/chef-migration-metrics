# TODO — TLS anti-lockout gaps

Surfaced 2026-06-08 by a customer mTLS lockout (`ERR_BAD_SSL_CLIENT_AUTH_CERT`)
caused by setting `server.tls.ca_path`. UI clarity for that field is done
(see git history / `fix/tls-capath-mtls-ui-clarity`). The deeper gaps below remain.

## 1. mTLS has no anti-lockout escape hatch (highest priority)
A non-empty `ca_path` sets `RequireAndVerifyClientCert` (`internal/tls/certmanager.go:164`).
The listener builds fine, so the § 2.4 fail-open never triggers — one config field can
lock out the entire org, including admins, with no recovery except deleting the
`server.tls` row in `config_store` and restarting.
Options to evaluate: exempt a recovery/login path or `localhost` from client-cert
requirement; a startup env-var/flag that forces `mode: off`; loud confirmation on save.
Spec: `tls.md` — would need a new section; ask before editing the spec.

## 2. Missing cert files crash startup instead of failing open
`validateTLSStatic` (`internal/config/config.go:1655`) `os.Stat`s cert/key/ca paths and
makes a missing file a FATAL validation error during DB assembly (`main.go:689`),
aborting before `degradeToPlainHTTP` (`main.go:1420`) can run. Violates `tls.md` § 2.4
("MUST NOT exit … triggers when certificate files change on disk underneath an
already-running deployment"). Fix: file presence/readability/PEM-validity belongs to the
listener (fail-open), not config validation. Add a failing test: static mode + missing
cert files should boot to degraded plain HTTP, not exit 1. Keep save-time preflight
(§ 2.6) intact.

## 3. `CHEF_MIGRATION_METRICS_SERVER_TLS_MODE` ignored on DB path
`applyEnvOverrides` (`internal/config/config.go:1138`) runs only on the YAML `ParseRaw`
path, not in `configstore.AssembleConfig`. So the documented env override (§ 2.5) silently
does nothing when config is DB-managed — removing another escape hatch. Decide: apply env
overrides after assembly, or document the limitation.
