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

## 2. "443 lifeboat" — health-driven port move (future TLS refinement)

Idea (raised 2026-06-10, ACME confirmed working: self-signed first → real cert
promoted in place). When TLS is healthy, serve HTTPS on 443 and redirect the
original port (e.g. 8080) → 443; when TLS is broken/reset, keep serving on the
original port so the operator always has a known lifeboat URL. **443 is the happy
path, the original port is the lifeboat.**

Reuses: the HTTP→HTTPS redirect primitive (`NewChallengeRedirectServer`,
`http_redirect_port`) and the fail-open status `kind`/degraded signal.

Open design tensions to resolve in the spec before building:
- **Privilege:** binding 443 needs `CAP_NET_BIND_SERVICE`/root; many deployments
  are unprivileged. Must degrade via the existing bind-failure fallback, not
  assume 443 is bindable.
- **Hot rebind:** `server.port` is restart-required today. Health-driven port
  moves need a graceful listener swap (new engineering, not just a redirect).
- **Flapping/predictability:** a port that hops 443↔8080 on transient TLS blips
  is its own lockout/confusion vector — needs hysteresis + a loud status surface.
- **Overlap with same-port fail-open:** today broken TLS stays on the *same* port
  with a self-signed cert. Decide whether port-movement replaces or complements
  that (doing both silently is confusing).

Conventional half (enable TLS → 443 + redirect old port) is low-risk; the
automatic health-driven port-flip is the spicy half. New TLS follow-on, own spec
(touches `tls.md`); does not block the shipped same-port fail-open.
