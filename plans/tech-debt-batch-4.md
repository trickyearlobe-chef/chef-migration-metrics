# Plan: Tech Debt Batch 4

## Goal

Resolve 5 tech debt items (B3, B8, B10, B1, P4) in a single branch.

## Specs to Read

- `todo-tech-debt.md` (already read)

## Items

### B3 — Deduplicate node snapshot filter query builders

`node_snapshot_filter.go` has `buildNodeSnapshotFilterQuery` (L83–244) and
`buildNodeSnapshotFilterParts` (L500–582) with ~80% duplicated CTE + WHERE
logic. Refactor so `buildNodeSnapshotFilterQuery` calls
`buildNodeSnapshotFilterParts` internally for the shared CTE/WHERE/args,
then adds its own SELECT, sort, and pagination. Existing tests in
`node_snapshot_filter_test.go` must all pass unchanged.

### B8 — Deduplicate log entry filter building

`log_entries.go` has `ListLogEntries` (L287–369) and `CountLogEntries`
(L373–423) with identical WHERE clause construction. Extract a
`buildLogEntryFilterQuery` helper returning `(where string, args []any)`.
Both methods call the helper then prepend their own SELECT. Add tests for
the new helper.

### B10 — SQL push-down for collection runs

`handleCollectionRuns` in `handle_logs.go` loads ALL runs across all orgs
into memory then paginates. Add a `ListCollectionRunsFiltered` datastore
method with optional org/status filters, ORDER BY, LIMIT, OFFSET, and a
COUNT. Add it to `DataStore` interface + mock. Rewrite handler to use it.
Add tests for the new handler path.

### B1 — Fix N+1 readiness queries in web handlers

`handleNodes` and `handleNodesWithOwnerFilter` each loop over page nodes
calling `ListNodeReadinessByNodeName` per node. Add a bulk
`BulkListNodeReadinessByNodeNames` datastore method that takes a slice of
`(organisationID, nodeName)` pairs and returns all matching readiness
records in one query. Add to `DataStore` interface + mock. Update both
handlers. Add tests.

### P4 — Decompose `main.go` `run()` function

Split 968-line `run()` into named phases: `setupCLI`, `setupLogger`,
`setupDatabase`, `setupAuth`, `setupSecrets`, `setupCollector`,
`setupHTTPServer`, `runServer`. Each returns its outputs or an error. `run()`
becomes a sequencer calling each phase. No behaviour change — pure refactor.

## Order

1. **B3** — no dependencies, one file, tests exist
2. **B8** — same pattern as B3, adjacent file
3. **B10** — depends on B8 pattern being established
4. **B1** — independent, new datastore method + handler change
5. **P4** — independent, large but mechanical refactor

## Acceptance Criteria

- All existing tests pass after each commit
- No new test failures introduced
- `go build ./...` clean
- `go vet ./...` clean
- Tech debt list updated (5 items removed)