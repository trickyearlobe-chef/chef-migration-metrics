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

## Chunk 4 — Apply & Restart action (depends on Chunk 1)  [DONE]

Done: `POST /api/v1/admin/restart` (admin-only, `handleAdminRestart`) returns
202 then signals a `restartFunc` (wired via `WithRestartTrigger`). `main.go`
adds a buffered `restartCh`; `awaitShutdown` selects on it, drains gracefully
(scheduler/kitchen queue/HTTP), and returns `exitCodeRestart` (=2, non-zero) so
systemd `Restart=on-failure` starts a fresh process — a clean SIGTERM still
exits 0 and is not restarted. 503 when no trigger wired. Frontend: amber
"Apply & Restart" button on `AdminServerPage` (shown when restart pending,
enabled only when not dirty) → `restartServer()` then `waitForServerHealthy()`
polls `/health` until back, then reloads. New api: `restartServer`,
`waitForServerHealthy`. Spec `configuration-live-reload.md` gained
§ Restart-Required Settings + § Apply & Restart. TDD across handler (503/405/
202+trigger), `awaitShutdown` restart-code, and the page (button visibility/
enablement, trigger+poll, error path).

## Chunk 5 — Privileged-port binding (443/80): packaging + dev  [DONE]

Done: systemd unit (`deploy/pkg/chef-migration-metrics.service`, packaged for
both RPM/DEB via `nfpm.yaml`) gained `AmbientCapabilities=CAP_NET_BIND_SERVICE` +
`CapabilityBoundingSet=CAP_NET_BIND_SERVICE` so the non-root service can bind
80/443 (survives the privilege drop, unlike `setcap` under `NoNewPrivileges`).
New `apptls.BindPermissionRemediation(addr, port, err)` returns capability +
SELinux remediation guidance on an EACCES bind denial and "" otherwise;
`servePlainHTTP` logs it on each failed candidate (reuses Chunk 2's degraded
fallback — no crash). `make run-privileged` does `sudo setcap
cap_net_bind_service=+ep` then runs (dev). Specs `tls.md` § 1.4 (binding low
ports) + `packaging-rpm-deb.md` § 2.5 (capability directives). TDD: remediation
message (EACCES/wrapped/other), `TestBind` happy/in-use, unit-file directives +
optional `systemd-analyze verify` (skipped without the binary).

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
