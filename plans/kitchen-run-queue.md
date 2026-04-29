# Plan: Kitchen Run Queue

## Goal

Replace fire-and-forget goroutines with a DB-backed queue + worker pool for all TK execution.

## Specs

- `.claude/specifications/kitchen-run-queue.md`
- `.claude/specifications/bulk-kitchen-scanning.md` (existing batch context)

## Steps

1. DB migration: create `kitchen_run_queue` table with indexes and partial unique constraints
2. Create `internal/kitchenqueue` package: queue manager, worker pool, claim/complete logic
3. Refactor `handleGitKitchenRun` to enqueue instead of spawning goroutine
4. Refactor `handleGitKitchenRunAll` to enqueue all mapped instances
5. Refactor `handleNodeKitchenTrigger` to enqueue instead of spawning goroutine
6. Refactor batch execution to enqueue items with batch_id
7. Add `GET /kitchen/queue` and `DELETE /kitchen/queue/:id` endpoints
8. Wire `Concurrency.TestKitchenRun` config to worker pool size
9. Add startup recovery (mark stale `running` items as `interrupted`)
10. Add WS events for queue state changes
11. Frontend: fetch queue state in GitKitchenSection, disable buttons accordingly
12. Frontend: subscribe to queue WS events for live updates
13. Remove redundant `max_concurrent_vms` from TK config (keep per-batch cap)
14. Tests for queue package, handler changes, frontend

## Acceptance Criteria

- Single run, run-all, and batch all go through queue
- Concurrency bounded by `Concurrency.TestKitchenRun` (verified with test)
- Duplicate requests for same instance are idempotent
- Ad-hoc runs get priority over batch runs
- Run buttons disabled for queued/running instances
- Graceful shutdown drains running items
- Queue survives app restart (DB-backed)
