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

## Chunk H1 — `graceful_shutdown_seconds` → applied [low risk, self-contained]
- Read the shutdown timeout from the holder at shutdown time (not boot).
- Scope: `main.go` shutdown path (`awaitShutdown`/graceful drain).
- TDD: a unit seam that resolves the timeout live. Acceptance: change applies
  without restart; section reports applied for a graceful-only change.

## Chunk H2 — listener ownership + `listen_address`/`port` rebind [architectural core]
- Extract listener ownership so the bound listener(s) can be closed/rebound:
  plain HTTP (`servePlainHTTP`) and static TLS (`apptls.Listener`). The TLS
  `Listener` already takes a pre-bound listener via `SetHTTPSListener` — lean on
  that. Build a server controller holding the current listener + a `Rebind`.
- `WithListenerRebinder` option + server config applier (listener granularity);
  make the server PUT diff-aware (listen key changed → rebind → report listener).
- Bind-new-first/keep-old/rollback; surface bind errors on the save (500 + msg).
- TDD: rebind across ephemeral ports (`:0`), assert old served until new up, and
  a forced-bind-failure keeps the old listener + returns the error. `-race`.
- Acceptance: changing listen_address/port applies live (plain + static TLS); no
  restart; in-flight requests on the old listener drain.

## Chunk H3 — `server.websocket.*` → subsystem [hub rebuild]
- Rebuild the websocket hub in place from live config (max_connections, buffers,
  timeouts, ping/pong). Assess hub ownership + safe swap with active connections.
- TDD: hub rebuild applies new limits; existing connections handled gracefully.

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
