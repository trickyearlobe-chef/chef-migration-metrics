# Logging Infrastructure — ToDo

Status key: [ ] Not started | [~] In progress | [x] Done

---

- [x] Implement structured logging with consistent severity levels (`DEBUG`, `INFO`, `WARN`, `ERROR`) — `internal/logging/logging.go` with `Severity` type, `Logger`, `ScopedLogger`, `Entry`, functional options; 93 tests in `logging_test.go`
- [x] Include contextual metadata in each log entry (timestamp, severity, organisation, cookbook name, commit SHA as applicable) — `Entry` struct with all spec fields, functional `Option` setters (`WithOrganisation`, `WithCookbook`, `WithCommitSHA`, etc.)
- [x] Persist log entries to the datastore — `DBWriter` in `internal/logging/db_writer.go` via `DBInserter` interface; `internal/datastore/log_entries.go` with `InsertLogEntry`, `BulkInsertLogEntries`, `ListLogEntries`, `CountLogEntries`, `PurgeLogEntriesBefore`, `PurgeLogEntriesOlderThanDays`; migration `0002_log_entries_extra_columns` adds `notification_channel`, `export_job_id`, `tls_domain` columns
- [x] Capture stdout/stderr from external processes and associate with the relevant log scope — `WithProcessOutput` option stores combined stdout/stderr in `process_output` field, persisted to `log_entries.process_output` column
- [x] Implement log retention period configuration and automated purge of expired logs — `PurgeLogEntriesOlderThanDays(retentionDays)` and `PurgeLogEntriesBefore(cutoff)` methods on datastore; config already supports `logging.retention_days` (default 90)
- [x] Implement log level configuration to control minimum persisted severity — `Logger` accepts configurable `Level` (minimum severity); entries below minimum are silently discarded; config already supports `logging.level` (default INFO)
- [x] Implement `notification_dispatch` log scope for notification delivery logging — `ScopeNotificationDispatch` constant with `WithNotificationChannel` option
- [x] Implement `export_job` log scope for export operation logging — `ScopeExportJob` constant with `WithExportJobID` option

> **Package summary:** `internal/logging/` — 2 production files (`logging.go`, `db_writer.go`), 1 test file (`logging_test.go`, 93 tests). `internal/datastore/log_entries.go` — log entry repository with insert, bulk insert, filtered list, count, purge, and per-collection-run queries (62 datastore tests). Migration `0002_log_entries_extra_columns` adds 3 columns to `log_entries` table.

### Remaining (wiring into application)

- [x] Replace `log.Printf` calls in `cmd/chef-migration-metrics/main.go` with the structured `Logger` — `main.go` uses structured `Logger` throughout startup, config, DB, secrets, and collection; only remaining `log.Printf` is the intentional DB-writer error fallback (cannot use structured logger there without infinite loop)
- [x] Wire `DBWriter` into the logger at startup (after DB connection is established) using `DatastoreAdapter` — `main.go` creates `DatastoreAdapter` wrapping `db.InsertLogEntry`, builds `DBWriter` with `WithContext` and `WithOnError`, and re-creates the logger with both `StdoutWriter` and `DBWriter`
- [x] Call `PurgeLogEntriesOlderThanDays` during each collection run cycle — `Collector.Run()` in `internal/collector/collector.go` calls `PurgeLogEntriesOlderThanDays` at the end of each run when `cfg.Logging.RetentionDays > 0`
- [x] Add `tls` log scope entries when TLS subsystem is implemented — `ScopeTLS` constant (`"tls"`) defined in `internal/logging/logging.go` with `WithTLSDomain` option and `TLSDomain` field on `Entry`; registered in `validScopes` map; tested in `TestIsValidScope`, `TestScope_StringValues`, and `TestOptions_WithTLSDomain` in `logging_test.go`; migration `0002_log_entries_extra_columns` adds `tls_domain` column to `log_entries` table; scope is ready for use when TLS subsystem is built