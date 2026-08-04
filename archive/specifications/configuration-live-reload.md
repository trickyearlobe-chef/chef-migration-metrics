# Configuration — Live Reload Requirement

**All configuration changes made via the admin API (config store) MUST take effect immediately without requiring an application restart.** This applies to all config-store-backed settings including:

- Backup schedule (cron expression, enabled/disabled)
- Target Chef version
- Collection schedule
- Git base URLs
- Kitchen settings
- Organisations — the `organisations` config section is reconciled into the
  operational `organisations` table on save (so the collector, which reads that
  table, sees the change immediately) and a collection is triggered; adding the
  first organisation clears setup mode without a restart
- Any other admin-configurable value

Components that consume config-store values must either:
1. Read the current value from the config store on each use (pull), OR
2. Subscribe to config-change notifications and update their state (push)

Approach (1) is preferred for simplicity unless the component has long-lived state (e.g. a cron scheduler that needs to reschedule its next tick).

Requiring an application restart to pick up config changes is a **bug**.

## Listener-Rebind and Restart-Required Settings

A small set of settings bind OS resources or initialise process-wide state, so
they cannot be applied by reading the live config per request. Where the owning
resource can be re-applied in place, the change takes effect via an **in-process
listener rebind** (no restart). A process restart is only the fallback — used
when an in-place apply is unavailable (no rebinder wired, e.g. running outside
the server process or in tests).

- `server.listen_address` / `server.port` — **listener.** The server rebinds in
  place: it binds the new address **first** and only retires the old listener
  once the new one is serving. If the new bind fails (e.g. the port is already
  held by another process such as nginx), the old listener keeps serving and the
  bind error is returned on the save — nothing is torn down.
- `server.tls.*` — **listener.** Certificate *material* already hot-swaps in
  place (DB source via the admin API, ACME via the renewer). Mode transitions
  (`off`/`static`/`acme`), redirect-port, and mTLS-CA changes rebind the listener
  topology in place using the same bind-new-first protocol, including the ACME
  port-80 challenge/redirect listener.
- `server.websocket.*` — **subsystem.** The websocket hub is rebuilt in place.
- `server.graceful_shutdown_seconds` — **applied.** Only read at shutdown, so the
  live value is consulted then; nothing to re-apply on save.
- `auth.*` — **subsystem.** The auth chain (session/lockout live reads) and the
  SAML provider are rebuilt in place; see [auth.md](auth.md).

PUT responses report the granularity actually needed (`reload`) and set
`restart_required: false` when the change was applied live. The flag is `true`
only when no in-place apply was available, in which case the change is persisted
but takes effect on the next restart.

**Why rebind rather than re-exec.** A process restart is unavailable on hosts
without a supervisor (the process would simply stay down), and it tears down
in-flight goroutines — a running test-kitchen VM run or a mid-flight collection.
An in-process rebind preserves them and surfaces bind errors directly to the
operator. The port-clash risk is identical either way, so re-exec buys nothing on
that front; bind-new-first makes the rebind strictly safer than a re-exec (a
failed bind is a no-op, not an outage).

## Apply & Restart Action (fallback)

When a change could not be applied live (the response set `restart_required:
true` because no rebinder/applier was wired), an admin can trigger a restart from
the UI without shell access. With in-place rebind wired this is rarely needed —
it remains the fallback for unsupervised edge cases.

### `POST /api/v1/admin/restart`

- **Admin-only.** Non-admin sessions are rejected by the standard admin
  middleware. Only `POST` is accepted (`405` otherwise).
- On success returns `202 Accepted` with a short JSON body
  (`{"status":"restarting","message":...}`), then triggers a **graceful
  shutdown** of the running process: the collection scheduler, kitchen queue,
  and HTTP server are drained (honouring `graceful_shutdown_seconds`) exactly as
  for a `SIGTERM`, so in-flight requests — including this one — complete.
- After draining, the process exits with a **non-zero restart exit code**. The
  service supervisor (systemd `Restart=on-failure`) then restarts it, which
  re-reads the persisted config. A clean `SIGTERM`/`systemctl stop` still exits
  `0` and is **not** restarted.
- When no restart trigger is wired (e.g. running outside a supervisor, or in
  tests), the endpoint returns `503 Service Unavailable` and does not exit.

### Frontend behaviour

- The Server & TLS admin page shows an **"Apply & Restart"** button. It is
  enabled only after a save has succeeded (a restart-required change is pending)
  and there are no unsaved edits.
- On click it calls the endpoint, then shows a "Restarting…" state and polls
  `GET /api/v1/health` until the server responds healthy again, at which point
  it clears the pending-restart state.
