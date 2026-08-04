# Backup & Restore

Safety net for database rollback. Must be UI-triggered because customer CLI access is restricted (VDI only).

## Scope

- On-demand and scheduled PostgreSQL backups using `pg_dump -Fc`
- Restore via `pg_restore --clean --if-exists` with maintenance-mode semantics
- Generation management (retain N most recent, auto-prune)
- Admin-only API and UI page

## Architecture

### Storage

- Backup files stored in `<data_dir>/backups/`
- Each backup has a sidecar manifest: `<backup_id>.json`
- Filesystem is authoritative for available backups; DB table is a cache/index
- On startup or list, reconcile DB with filesystem

### Backup File

- Format: `pg_dump -Fc` (compressed custom format)
- Filename: `<backup_id>.dump`
- Permissions: `0600` on file, `0700` on directory
- Written atomically: write to `.tmp` suffix, rename on success
- SHA-256 checksum stored in manifest

### Sidecar Manifest (`<backup_id>.json`)

```json
{
  "id": "uuid",
  "filename": "<id>.dump",
  "size_bytes": 123456,
  "sha256": "hex-string",
  "created_at": "RFC3339",
  "app_version": "x.y.z",
  "schema_version": 30,
  "pg_server_version": "17.x",
  "pg_dump_version": "17.x",
  "status": "succeeded",
  "error": "",
  "initiated_by": "user@example.com"
}
```

### Credentials

- Never pass DB URL as CLI argument
- Use environment variables: `PGHOST`, `PGPORT`, `PGUSER`, `PGDATABASE`, `PGPASSWORD`
- Parse connection string from app config to extract components

## Operations

### Create Backup

- Async: returns 202 with backup ID immediately
- Runs `pg_dump -Fc` via `exec.CommandContext` with timeout
- Preflight: check disk has >2× estimated dump size free (or >500MB minimum)
- On success: rename temp file, write manifest, update DB record
- On failure: remove temp file, write manifest with error, update DB record
- Prevent concurrent backup/restore (advisory lock or in-process mutex)

### Restore

- Maintenance-mode operation:
  1. Set maintenance mode flag (atomic bool on Router)
  2. Broadcast WebSocket `maintenance` event to connected clients
  3. Stop background workers via restore hook (collection scheduler, kitchen queue, backup scheduler, export cleanup)
  4. Brief pause for in-flight DB operations to drain
  5. Verify checksum of backup file
  6. Run `pg_restore --clean --if-exists --single-transaction --exit-on-error -d <dbname>`
  7. On success: broadcast completion event, terminate process (systemd/supervisor restarts app, auto-migrates)
  8. On failure: log error, clear maintenance mode, broadcast failure event, resume normal operation
- During maintenance mode: all `/api/` routes return 503 except `/api/v1/health`, `/api/v1/version`, and `/api/v1/admin/backups/status`
- Frontend assets continue to be served normally
- Requires confirmation body: `{"confirm": "RESTORE"}`
- Returns 202 with status tracking

### List

- Read manifests from filesystem
- Reconcile: remove DB records for missing files, add records for orphaned manifests
- Return sorted by created_at descending

### Delete

- Remove dump file + manifest from filesystem
- Remove DB record
- Cannot delete a backup that is currently being restored

### Prune

- Keep N most recent successful backups (configurable, default 7)
- Run after each successful backup creation
- Do not prune backups with status `running` or `restoring`

### Scheduled Backup

- Background goroutine with configurable interval (default 24h)
- Uses advisory lock to prevent duplicate runs across instances
- Disabled by default unless backup dir is valid and writable
- On failure: log error, continue (do not crash app)
- **Config changes (enable/disable, schedule interval) take effect immediately** — the scheduler must re-read or be notified when config store is updated. No app restart required.

## Configuration (Config Store)

Stored in the encrypted config store under key `backup`:

```json
{
  "enabled": false,
  "dir": "",
  "max_generations": 7,
  "schedule_interval": "24h",
  "pg_dump_path": "",
  "pg_restore_path": ""
}
```

Only `backup.dir` might need to be in YAML if the backup directory must be
known before DB connectivity — but since the default derives from `data_dir`
(which is already in the config store), this is fine in the config store.

## API Endpoints

All admin-only (`/api/v1/admin/backups`).

| Method | Path | Action | Response |
|--------|------|--------|----------|
| GET | /api/v1/admin/backups | List all backups | 200 + array |
| POST | /api/v1/admin/backups | Create backup | 202 + `{id, status}` |
| GET | /api/v1/admin/backups/{id} | Get backup detail | 200 + manifest |
| DELETE | /api/v1/admin/backups/{id} | Delete backup | 204 |
| POST | /api/v1/admin/backups/{id}/restore | Restore from backup | 202 + `{status}` |
| GET | /api/v1/admin/backups/status | Current job status | 200 + job info |

### Error Responses

- 404: Backup not found
- 409: Another backup/restore operation in progress
- 422: Insufficient disk space / checksum mismatch / confirmation missing
- 503: Restore in progress (for normal endpoints)

## Health Check Integration

- Report `pg_dump`/`pg_restore` availability and version in system health
- Warn if tools are missing or version mismatches server

## Package Layout

- `internal/backup/` — core backup/restore/prune logic
- `internal/backup/backup.go` — types and orchestration
- `internal/backup/pgtools.go` — pg_dump/pg_restore execution
- `internal/backup/manifest.go` — sidecar manifest read/write
- `internal/backup/scheduler.go` — scheduled backup goroutine
- `internal/webapi/handle_admin_backups.go` — HTTP handlers

## Migration

- `0031_backups.up.sql`: Create `backups` table (cache/index only)
  - `id UUID PRIMARY KEY DEFAULT gen_random_uuid()`
  - `filename TEXT NOT NULL`
  - `size_bytes BIGINT`
  - `sha256 TEXT`
  - `status TEXT NOT NULL DEFAULT 'pending'`
  - `error TEXT`
  - `app_version TEXT`
  - `schema_version INTEGER`
  - `pg_server_version TEXT`
  - `pg_dump_version TEXT`
  - `initiated_by TEXT`
  - `created_at TIMESTAMPTZ NOT NULL DEFAULT now()`
  - `completed_at TIMESTAMPTZ`

## Constraints

- Must work on macOS (dev with Docker DB) and RHEL 8/9 (production)
- Large DB (~70k nodes) — backup may take minutes; must not block HTTP
- Single app instance (no HA concerns currently)
