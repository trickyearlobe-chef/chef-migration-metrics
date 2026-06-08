# Configuration — Live Reload Requirement

**All configuration changes made via the admin API (config store) MUST take effect immediately without requiring an application restart.** This applies to all config-store-backed settings including:

- Backup schedule (cron expression, enabled/disabled)
- Target Chef version
- Collection schedule
- Git base URLs
- Kitchen settings
- Any other admin-configurable value

Components that consume config-store values must either:
1. Read the current value from the config store on each use (pull), OR
2. Subscribe to config-change notifications and update their state (push)

Approach (1) is preferred for simplicity unless the component has long-lived state (e.g. a cron scheduler that needs to reschedule its next tick).

Requiring an application restart to pick up config changes is a **bug**.

## Restart-Required Settings (the exception)

A small set of settings cannot be applied to a running process and are
**restart-required** by design. These bind OS resources or initialise
process-wide state at startup:

- `server.listen_address` / `server.port` (the bound listener)
- `server.tls.*` (the active listener and certificate)
- `server.websocket.*`
- `server.graceful_shutdown_seconds`
- `auth.*`

PUT responses for these sections set `restart_required: true` (see
`encrypted-config-store.md`). The change is persisted to the config store but
does not take effect until the process restarts.

## Apply & Restart Action

To apply a restart-required change without shell access, an admin can trigger a
restart from the UI.

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
