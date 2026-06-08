# Active Plan — Server/TLS config: safe-apply, recovery, editable bind

Goal: make the HTTP listener fully configurable from the UI without the ability
to lock yourself out. Driven by three decisions:
1. Bad TLS at startup → **fall back to plain HTTP + loud banner**, never crash-loop.
2. `port` / `listen_address` become **DB-managed and UI-editable** (restart-required).
3. Config needing a restart gets an **"Apply & Restart"** admin action (graceful
   self-exit; external supervisor restarts the process).

Key facts (verified 2026-06-08): `port`/`listen_address` are bootstrap-only —
`setupConfigStore` carries them (+`Datastore.URL`) over from YAML, the rest comes
from the DB (`config_store` key `server.tls`, encrypted) (`main.go:668-671`). Bad
static certs hard-fail the listener at *restart* → `run()` exits 1 (no env escape
hatch; only recovery is deleting the `server.tls` DB row). Specs to change (signed
off): `tls.md` §2.4 fail-fast → fail-open; `configuration-schema-server.md` (port/
listen_address now DB/UI); `configuration-live-reload.md` (restart-required set).
Predecessor: Batch UX done; sweep-status item back in the todo backlog.

## Chunk 1 — Preflight validation on save  [DONE]

Done (`bc0c657` code, `a5455d0` specs): `tls.ValidateStaticPair` (cert/key/CA
load, shared with startup); `PUT /admin/config/server` rejects (422, no persist)
unusable static certs and `http_redirect_port == listen port`. Test-bind on a
changed port deferred to Chunk 3 (port not UI-editable until then).

## Chunk 2 — Startup TLS fallback + degraded banner  [DONE]

Done: static-mode listener-setup failure now fails open — `main.go`
`degradeToPlainHTTP`/`servePlainHTTP` log ERROR, set a shared
`webapi.TLSStatusHolder` (`{degraded,reason}`), and serve plain HTTP on the
configured `listen_address:port` instead of exiting. Public DB-free endpoint
`GET /api/v1/server/tls-status`. Frontend `TLSDegradedBanner` polls it and shows
a red "running INSECURE" banner globally (AppLayout) + inline on AdminServerPage.
Spec `tls.md` §2.4 rewritten fail-fast → fail-open and §5.3 status/recovery added.
TDD: webapi endpoint (holder/public/degraded), cmd fallback (degraded+serves),
frontend banner (healthy/degraded/error).

## Chunk 3 — Editable port/listen_address (depends on Chunk 2)  [DONE]

Done: new `server.listen` config-store section (`configstore.KeyServerListen` +
`ServerListenSection`); `AssembleConfig`/`ConfigToSections` round-trip it and
`HasKey` distinguishes DB-sourced from absent. `main.go` captures the bootstrap
listen target, sources listen from DB when present, and `servePlainHTTP` now
pre-binds with a candidate fallback (configured → bootstrap → 0.0.0.0:8080),
flagging degraded (Chunk 2 holder) when it falls back. `ConfigHolder.Reload`
sources listen from DB too. `PUT /admin/config/server` validates the port range
and test-binds a changed address/port (`apptls.TestBind`) before persisting
`server.listen`. `AdminServerPage` gained an HTTP Listener section (address +
port, restart-required). Specs `encrypted-config-store.md` +
`configuration-schema-server.md` updated. TDD across all five layers.

## Chunk 4 — Apply & Restart action (depends on Chunk 1)

Scope: `internal/webapi` new `POST /admin/restart` (admin-only, graceful exit),
`AdminServerPage.tsx` (button + reconnect UX), spec `configuration-live-reload.md`.
- Endpoint triggers graceful shutdown then exits with a supervisor-restart code.
- Frontend "Apply & Restart" after a successful save; shows "restarting…" and
  polls health until back. Gated on the save having passed preflight.
- TDD (backend handler auth + shutdown trigger; frontend button calls endpoint + reconnect).
Acceptance: one click applies a restart-required change end to end.

## Chunk 5 — Privileged-port binding (443/80): packaging + dev (pairs with Chunk 3)

Scope: `deploy/pkg/chef-migration-metrics.service`, `deploy/pkg/scripts/postinstall.sh`,
`Makefile` (`run`/new target), specs `packaging.md` + `tls.md` (a "binding low ports" note).
Context: service runs non-root (`User=chef-migration-metrics`) with
`NoNewPrivileges=true`; that combination silently ignores `setcap` file caps, so
the binary cannot bind 443 today.
- systemd unit: add `AmbientCapabilities=CAP_NET_BIND_SERVICE` +
  `CapabilityBoundingSet=CAP_NET_BIND_SERVICE` (compatible with `NoNewPrivileges=true`;
  ambient caps are granted by systemd and survive the drop to the service user).
- SELinux: 80/443 are already `http_port_t` so they bind under the default
  targeted policy. For a non-standard low/custom port, document/emit
  `semanage port -a -t http_port_t -p tcp <port>`. Add a startup check that, on a
  bind-permission denial, logs a precise remediation (capability vs SELinux label)
  — reuses Chunk 2's degraded path rather than crashing.
- `make run`: keep default 8080; add a `run-privileged` (or doc) path that does
  `sudo setcap cap_net_bind_service=+ep $(BINARY)` before running (works in dev —
  no `NoNewPrivileges` there).
- Tests: unit-test the bind-denial remediation message; verify the unit file
  parses (systemd-analyze verify if available in CI) — no privileged bind in CI.
Acceptance: a packaged install can serve on 443 as the non-root service; a clear,
actionable message when the OS blocks the bind.

## Chunk 6 — Export SAML SP metadata to XML (independent)

Scope: `frontend/src/pages/AdminAuthPage.tsx` + test, spec `auth.md` (done).
Backend already serves it: `GET /api/v1/auth/saml/metadata` → `HandleMetadata`
→ `provider.Metadata()` (`application/samlmetadata+xml`, XML declaration
prepended); wired only when SAML is configured, else `501`.
- Add an "Export SP Metadata (XML)" button to the SAML section that fetches the
  endpoint and saves the response as `sp-metadata.xml` (blob download).
- Show/enable only when a SAML provider is configured; surface a clear message
  on `501`/error rather than downloading an error body.
- TDD: button visible when SAML configured + triggers fetch/download; hidden/
  disabled otherwise; error path shows a message.
Acceptance: an admin can download the live SP metadata XML to hand to the IdP.

## Notes
- Order: 1 → 2 → 3 → 4 (2 before 3; 1 before 4). Chunk 6 is independent. Chunk 5 pairs with Chunk 3
  (editable port) and can land alongside it. Each chunk = one session; update spec
  first, then TDD.
- Out of scope: changing ACME-mode behaviour; multi-replica restart coordination;
  containerised (Docker) capability config beyond a doc note.
- Capability vs SELinux are distinct layers: `CAP_NET_BIND_SERVICE` grants the
  privilege to bind <1024; SELinux port labels (`http_port_t`) separately permit
  the bind. Both must be satisfied on enforcing RHEL for a non-standard port.
