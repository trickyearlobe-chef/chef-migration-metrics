# Task Summary: ToDo Audit + Logging Package Implementation

**Date:** 2025
**Components:** `.claude/specifications/todo/` (audit), `internal/logging/` (new), `internal/datastore/log_entries.go` (new), `migrations/0002_*` (new)
**Files modified:** see list at bottom

---

## Context

Two tasks were completed in this session:

1. **ToDo audit** — The master `ToDo.md` and several component todo files were stale. The Data Collection, Configuration, Specification, and Testing sections in the master file had not been updated to reflect work done in previous sessions (chefapi client, configuration package, datastore/main.go). The component files `todo/data-collection.md` and `todo/configuration.md` had already been updated by their respective sessions, but the master was lagging.

2. **Logging package implementation** — `internal/logging/` was the next item on the critical path. It was needed by virtually every other package (secrets wiring, collection orchestration, TLS, etc.) and was identified as the recommended next step in multiple prior summaries.

---

## What Was Done

### Part 1: ToDo Audit

Audited all 12 component todo files against the actual codebase. Updated stale items in both the component files and the master `ToDo.md`.

#### Changes to master `ToDo.md`

| Section | Items marked done | Notes |
|---------|-------------------|-------|
| Specification | 1 | `Write database migration files for initial schema` — `migrations/0001_*.sql` exists |
| Data Collection | 22 | Synced with `todo/data-collection.md` which was already updated by chefapi session |
| Configuration | 28 | Synced with `todo/configuration.md` which was already updated by config session. Added `— Partially Done` heading, blockquote with test counts |
| Testing | 4 | Chef API client tests, partial search tests, stale node detection, role graph building (partially) |
| Logging Infrastructure | 8 | All items marked done after implementation (Part 2) |

#### Changes to component todo files

| File | Items marked done | Notes |
|------|-------------------|-------|
| `todo/specification.md` | 1 | Migration files item |
| `todo/testing.md` | 4 | Chef API client, partial search, stale node, role graph (partial) |
| `todo/logging.md` | 8 | All core items done; added "Remaining (wiring)" subsection with 4 items |

### Part 2: Logging Package Implementation

#### `internal/logging/logging.go` (~634 lines)

Core structured logging package:

- **Severity** — `DEBUG`, `INFO`, `WARN`, `ERROR` as typed constants with ordering, `String()`, and `ParseSeverity()` (case-insensitive, trims whitespace, defaults to INFO with error on unknown)
- **Scope** — 9 typed constants matching the spec: `collection_run`, `git_operation`, `test_kitchen_run`, `cookstyle_scan`, `notification_dispatch`, `export_job`, `tls`, `readiness_evaluation`, `startup`. `IsValidScope()` validator.
- **Entry** — Struct with all spec fields (timestamp, severity, scope, message, organisation, cookbook_name, cookbook_version, commit_sha, chef_client_version, process_output, collection_run_id, notification_channel, export_job_id, tls_domain). Custom `MarshalJSON` serialises severity as string. Empty optional fields omitted via `omitempty`.
- **Options** — 9 functional option setters: `WithOrganisation`, `WithCookbook`, `WithCommitSHA`, `WithChefClientVersion`, `WithProcessOutput`, `WithCollectionRunID`, `WithNotificationChannel`, `WithExportJobID`, `WithTLSDomain`. Later options override earlier ones.
- **Writer interface** — `WriteEntry(Entry) error`. All writers must be safe for concurrent use.
- **StdoutWriter** — Mutex-protected, configurable output (`io.Writer`), supports human-readable and JSON formats. Human format: `2025-01-20T12:00:00Z INFO  [startup] message org=value cookbook=name@version`. `NewStdoutWriter()` with `WithOutput()` and `WithJSON()` options.
- **Logger** — Central type with configurable minimum severity, multiple writers (fan-out), injectable clock. `New(Options)` defaults to StdoutWriter and `time.Now`. Methods: `Debug`, `Info`, `Warn`, `Error`, `Logf`. All timestamps stored as UTC. Entries below minimum level silently discarded (returns nil). Writer errors collected but don't block other writers; single error returned directly, multiple errors joined.
- **ScopedLogger** — Convenience wrapper fixing scope and base options. `WithScope(scope, baseOpts...)` returns a `ScopedLogger` with `Debug`, `Info`, `Warn`, `Error` methods that prepend base options before per-call options. `Scope()` accessor.
- **MemoryWriter** — Thread-safe in-memory writer for testing. `Entries()` returns a copy. `Len()`, `Reset()`.
- **ErrorWriter** — Always-failing writer for testing error paths.

#### `internal/logging/db_writer.go` (~271 lines)

Database persistence bridge:

- **DBInserter interface** — `InsertLogEntry(ctx, LogEntryParams) (string, error)`. Decouples logging from the datastore package (no import dependency in either direction).
- **LogEntryParams** — Mirrors `datastore.InsertLogEntryParams` without importing it.
- **DBWriter** — Implements `Writer`. Converts `Entry` to `LogEntryParams` (severity to string, scope to string) and calls `DBInserter.InsertLogEntry()`. On failure: swallows error (returns nil to logger so stdout still works), calls optional `OnError` callback. Configurable context and error callback via `WithContext()` and `WithOnError()` options. `SetOnError()` for post-construction changes (mutex-protected).
- **DatastoreAdapter** — Wraps a function `func(ctx, LogEntryParams) (string, error)` to implement `DBInserter`. Used in `main.go` to bridge `datastore.DB.InsertLogEntry()` without the logging package importing datastore. Includes usage example in godoc.
- **FailingDBInserter** — Test double that always returns an error.
- **RecordingDBInserter** — Test double that records all entries with sequential IDs. Thread-safe. `Entries()`, `Len()`, `Reset()`.

#### `internal/datastore/log_entries.go` (~661 lines)

Log entry repository:

- **LogEntry** type — Matches `log_entries` table (16 columns). `MarshalJSON` with `omitempty` for optional fields.
- **InsertLogEntryParams** + `validateLogEntryParams()` — Validates required fields (timestamp non-zero, severity in {DEBUG,INFO,WARN,ERROR}, scope non-empty, message non-empty).
- **InsertLogEntry** — Single row insert with RETURNING. Uses `nullString`/`nullStringUUID` for optional fields.
- **BulkInsertLogEntries** — Prepared statement within a transaction. Returns count. Validates each entry; stops on first validation error.
- **GetLogEntry** — By UUID.
- **ListLogEntries** — Dynamic query builder with `LogEntryFilter`: scope, severity (exact), min_severity (IN array), organisation, cookbook_name, collection_run_id, since/until timestamps, limit/offset. Results ordered by timestamp DESC.
- **CountLogEntries** — Same filter logic, returns count (ignores limit/offset).
- **ListLogEntriesByCollectionRun** — All entries for a run, ordered ASC (chronological).
- **PurgeLogEntriesBefore** — Delete by timestamp cutoff. Validates non-zero time.
- **PurgeLogEntriesOlderThanDays** — Convenience wrapper computing cutoff from retention days. Validates > 0.
- **DeleteLogEntriesByCollectionRun** — Delete by collection run ID.
- **Helpers** — `nullStringUUID()`, `stringArray` with `Value()` for PostgreSQL array literals, `joinQuoted()`, `scanLogEntry()` (from `*sql.Row`), `scanLogEntryRow()` (from `*sql.Rows`).
- **minSeverityValues** / **severityOrdinal** — Converts min severity string to list of valid values for `ANY()` queries.

#### `migrations/0002_log_entries_extra_columns.up.sql`

Adds 3 columns to `log_entries` table that were specified in the logging spec but omitted from the initial schema:
- `notification_channel TEXT`
- `export_job_id TEXT`
- `tls_domain TEXT`

Down migration (`0002_log_entries_extra_columns.down.sql`) drops them with `IF EXISTS`.

---

## Test Coverage

| Package | Tests | New | Notes |
|---------|-------|-----|-------|
| `internal/logging/` | 133 | 133 | All new — severity, scope, entry, options, writers, logger, scoped logger, DB writer, integration scenarios |
| `internal/datastore/` | 137 | 92 | 45 existing + 92 new for log entry validation, severity ordinal, minSeverityValues, nullStringUUID, stringArray, LogEntry marshal, filter struct, purge validation, empty ID checks |

**Total project tests: 815** (was 627).

All packages pass:
```
go test ./... -count=1
ok  chefapi      9.6s
ok  config       1.1s
ok  datastore    1.5s
ok  logging      1.3s
ok  secrets      2.1s
```

`go build ./...` and `go vet ./...` both clean.

---

## Final State

### What's Done

- **Logging package is fully implemented** — `Logger`, `ScopedLogger`, `Entry`, all 9 functional options, `StdoutWriter` (human + JSON), `DBWriter` with error swallowing and callback, `MemoryWriter`, `ErrorWriter`, `DatastoreAdapter`, test doubles.
- **Datastore log entry repository is fully implemented** — insert, bulk insert, filtered list, count, purge, per-run queries with comprehensive validation.
- **Database migration** adds the 3 missing columns to `log_entries`.
- **All ToDo files are current** — no known stale items remain across all 12 component files and the master.

### What's Not Done (remaining wiring)

1. **Replace `log.Printf` in `main.go`** — Currently uses stdlib `log.Printf`. Should be replaced with the structured `Logger` once we decide on the injection pattern (pass by argument vs. context vs. global).
2. **Wire `DBWriter` at startup** — After DB connection, create a `DatastoreAdapter` and `DBWriter`, attach to the logger alongside `StdoutWriter`.
3. **Call `PurgeLogEntriesOlderThanDays`** — Should be called during each collection run cycle using `config.Logging.RetentionDays`.
4. **TLS log scope entries** — Will be added when the TLS subsystem is implemented.
5. **Functional DB tests** — The log entry SQL paths need `//go:build functional` tests against real PostgreSQL (same gap as all other datastore repositories).

---

## Files Modified

### Production code (new)
- `internal/logging/logging.go`
- `internal/logging/db_writer.go`
- `internal/datastore/log_entries.go`
- `migrations/0002_log_entries_extra_columns.up.sql`
- `migrations/0002_log_entries_extra_columns.down.sql`

### Tests (new)
- `internal/logging/logging_test.go`

### Tests (modified)
- `internal/datastore/datastore_test.go` — added 92 tests for log entry validation, helpers, and type methods

### ToDo / documentation (modified)
- `.claude/specifications/ToDo.md` — updated Specification, Data Collection, Configuration, Testing, and Logging Infrastructure sections
- `.claude/specifications/todo/specification.md` — marked migration files item done
- `.claude/specifications/todo/testing.md` — marked 4 items done
- `.claude/specifications/todo/logging.md` — marked all 8 items done, added remaining wiring subsection
- `.claude/Structure.md` — added log_entries.go to datastore listing, expanded logging/ listing, added summary entry
- `.claude/summaries/2025-logging-and-todo-audit.md` — this file

---

## Recommended Next Work

1. **Wire logging into `main.go`** — Replace `log.Printf` with the structured `Logger`. Attach both `StdoutWriter` and `DBWriter` (after DB connect). This is small and closes the gap between the library and the running application.

2. **Startup validation + secrets rotation wiring** — With logging now available, the remaining secrets-storage items (startup key validation, `RotateMasterKey` wiring with INFO/ERROR logging) can be completed. This would fully close out the `internal/secrets/` component.

3. **Data collection orchestrator** (`internal/collector/`) — The Chef API client, datastore, config, and logging are all in place. Building the collection orchestrator (multi-org parallel collection, background scheduling, persistence) is the next critical-path item that turns the existing libraries into an actual working data pipeline.