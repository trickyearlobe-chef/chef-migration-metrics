# Phase 6: Bulk Kitchen Scanning

## Goal
Implement bulk kitchen execution per `.claude/specifications/bulk-kitchen-scanning.md`.

## Specs to read
- `.claude/specifications/bulk-kitchen-scanning.md` (primary)
- `.claude/specifications/kitchen-refactor.md` (context)
- `.claude/specifications/test-kitchen-drivers.md` (driver config)

## Key design decisions (from rubber-duck review)
- Graceful cancellation: stop scheduling new work, let in-flight drain on detached ctx
- Batch-scoped work items: `kitchen_batch_instances` table for accurate progress
- CAS state transitions: `UpdateBatchStatusIfCurrent(id, from, to)` prevents race
- Plan expansion in background goroutine, not request path
- Single target chef version per batch (validated up front)

## Implementation order

### 1. Enabled gate
- Add `IsEnabled()` check to `handleGitKitchenRun` and `handleRunKitchenBatch`
- Return 409 if disabled. Tests for both.

### 2. Batch instances table + CAS transitions
- Migration: `kitchen_batch_instances` table
- `UpdateKitchenBatchStatusIfCurrent(id, from, to)` method
- Datastore CRUD for batch instances

### 3. Scheduler.RunBatch
- New method: `[]*PlanResult` → flat queue with shared semaphore
- Graceful cancellation: in-flight use detached ctx, new scheduling respects cancel
- Progress callback includes repo name

### 4. Batch execution wiring
- Background goroutine: resolve → plan → RunBatch
- Persist batch instances before execution
- runningBatches map, single-running guard, WebSocket events

### 5. Batch cancellation
- Cancel func lookup + call, graceful drain

### 6. Batch progress endpoint
- `GET /kitchen/batches/:id/progress` from batch_instances table

### 7. Restart resilience
- On init, running batches → cancelled

### 8. Network timeout + TimedOut detection
- Executor: context deadline → TimedOut=true
- Scheduler: timed_out + no converge → network_timeout

### 9. Orphan sweep infrastructure
- Folder filter, SweepOrphanVMs, config, manual endpoint, scheduled goroutine

### 10. Frontend updates
- Disable Run when disabled, live progress, sweep button
