# Active — server live listener rebind

Branch: `refactor/config-live-reload`. Spec: `configuration-live-reload.md`
(Listener-Rebind section, updated). Goal: apply `server.*` changes in place
instead of a process re-exec — re-exec is unavailable on unsupervised hosts and
kills in-flight goroutines (test-kitchen runs, collection runs); bind-new-first
makes a rebind strictly safer (a failed bind is a no-op).

Bucket 2 (the restart bugs) is already closed (chunks A–G). This is the Bucket 1
`server.*` downgrade-candidate work, full scope (incl. TLS mode/ACME), approved
2026-06-12.

## Core protocol (all rebind chunks)

**Bind-new-first / keep-old / rollback:** bind the new listener(s) before
retiring the old; only once the new is serving, drain+close the old. On any bind
failure, keep the old serving and return the error to the operator (e.g. "address
already in use" → another process holds the port). Preflight: validate
bindability, but prefer bind-and-keep over test-bind-release (no release race).

**Diff-aware server flag:** the server PUT bundles listen/tls/websocket/graceful.
The handler must detect which sub-keys actually changed and report the worst
granularity *of the changed set* (so a graceful-only change reports applied, a
port change reports listener). Introduced in H2, extended per chunk.

**Seam:** a server controller (new `internal/serverctl` or in `main.go`) owns the
live listener(s) and exposes `Rebind(newServerCfg) (ReloadGranularity, error)`.
Wired to webapi via `WithListenerRebinder`; the server config applier calls it.

## Chunk H1 — `graceful_shutdown_seconds` → applied [DONE]
- `resolveShutdownTimeout()` reads the drain budget live from the holder at
  shutdown time (fallback to boot cfg; ≤0 → 15s); `awaitShutdown` uses it.
- Server PUT is now diff-aware (minimal): `serverReloadGranularity` diffs the
  submitted vs pre-save live sections; graceful-only change → applied (no
  restart), any listen/tls/websocket/trusted_proxy change → process (pessimistic
  until H2–H4). Reports `reload`/`restart_required` like the generic handler.
- Tech debt logged: dead `apptls.ListenerConfig.GracefulShutdownTimeout` field.
- H2 extends the diff: listen key → listener granularity + wire the rebinder.

## Chunk H2 — listener ownership + `listen_address`/`port` rebind [DONE]
- `internal/serverctl.Controller`: owns the live `Instance` (Addr+Shutdown),
  `Rebind(addr,port)` binds-new-first via an injected `BuildFunc`, swaps current,
  drains the old in the background (live graceful budget). No-op on unchanged
  target; bind failure keeps the old serving + returns the error.
- webapi `ListenerRebindHolder` (func-based, no import cycle) +
  `WithListenerRebinder`; `ErrNoListenerRebinder` sentinel. Wired up front like
  `tlsReload`.
- Server PUT diff-aware: listen key changed → call rebinder. Success → report
  `listener` (no restart); no rebinder wired → `process` (restart_required);
  bind error → 500 (old keeps serving). `serverReloadGranularity` now takes the
  resolved listen granularity; static map keeps tls/websocket/trusted_proxy.
- main: holder created in `setupAndServeHTTP`; plain-`off` mode adopts a plain
  controller, healthy static-TLS-on-configured-port adopts a TLS controller
  (BuildFunc rebuilds `apptls.Listener` at the new target, re-points `tlsReload`
  + re-arms cert watch; db source refetches cert from store so a prior hot-swap
  is preserved).
- **Scope deferred to H4** (restart_required until then — holder simply not
  adopted, so the no-rebinder path applies): active auto-443 lifeboat
  (`https443Ln != nil`), ACME mode, and the degraded self-signed/plain
  fallbacks. These need the full listener-topology rebuild that H4 owns.
- TDD: serverctl protocol (rebind across ephemeral ports, old drained, forced
  bind-failure keeps old); handler (listener/process/500); main (plain rebind +
  bind-failure). All under `-race`.

## Chunk H3 — `server.websocket.*` → subsystem [hub rebuild] [DONE]
- `EventHub.Reconfigure(max, buf)`: max_connections/send_buffer_size now atomic
  and reconfigured live. Lowering max never evicts existing clients (new
  registrations rejected until count drops below the new ceiling); buffer change
  sizes only clients registered after the call. Non-positive → unchanged.
- Handler timeouts (write/ping/pong) pulled live via `WithWebSocketConfigFunc`
  (router closes over `liveConfig()`); resolved once per connection, so existing
  connections keep their started values — graceful.
- Server PUT diff-aware: websocket key changed → `r.hub.Reconfigure(...)` from the
  reloaded live config (defaulted) → reports `subsystem` (no restart). The hub is
  router-owned, so no holder/seam is needed and it always applies (never
  restart_required). `serverKeyGranularity[websocket]` flipped process→subsystem.
- main: hub now created with `WithMaxConnections/WithSendBufferSize` from
  `server.websocket.*` (was ignoring config at boot — pre-existing gap, fixed).
- TDD: hub reconfigure (live max/buf, non-positive no-op, lowered-max keeps
  existing + rejects new, buffer applies to new only); handler resolveConfig
  (live/static/default); handler PUT → subsystem + hub reconfigured. All `-race`.

## Chunk H4 — TLS mode transitions + ACME port rebind [largest / riskiest]
- Live `off`/`static`/`acme` transitions and ACME-mode port changes: rebuild the
  listener topology in place (HTTPS listener + port-80 challenge/redirect +
  renewer lifecycle) using bind-new-first. Cert *material* hot-swap already done.
- Riskiest: ACME owns port 80, issuance may be in flight; coordinate the
  challenge server + renewer cancel/restart. Likely sub-chunks (static↔off first,
  then acme).
- TDD: mode transition rebinds without dropping the process; ACME challenge stays
  reachable across a port change.

## Notes / cross-branch
- `auth.*` is listed in this spec's rebind section as subsystem; on this branch
  it is NOT yet implemented (done on `fix/saml-config-ux`). At merge, reconcile —
  the SAML branch must also flip `configuration-live-reload.md` §auth.
- Each chunk = one session (start fresh; read only this plan + the touched code).
