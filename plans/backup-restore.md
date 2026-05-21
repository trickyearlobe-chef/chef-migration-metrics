# Backup/Restore Implementation

## Goal

Implement database backup and restore with UI-triggered operations, scheduled snapshots, and generation management.

## Specs

- `.claude/specifications/backup-restore.md`
- `.claude/specifications/project-conventions.md`

## Steps

1. Write migration 0031 (backups table)
2. Add BackupConfig to config package
3. Create `internal/backup/` package — types, manifest, pgtools
4. Write tests for manifest read/write
5. Write tests for pg_dump/pg_restore execution (mocked)
6. Write tests for backup orchestration (create, list, delete, prune)
7. Implement backup package
8. Add datastore layer for backups table
9. Write handler tests
10. Implement API handlers
11. Wire routes in router
12. Add scheduled backup goroutine
13. Integration: verify with `make test`

## Acceptance Criteria

- `pg_dump -Fc` creates backup with sidecar manifest
- Restore acquires lock, verifies checksum, runs pg_restore, exits process
- List reconciles filesystem with DB
- Prune keeps N most recent
- Scheduled backup runs on interval with advisory lock
- All existing tests pass + new unit tests
- Credentials never in CLI args (env vars only)
