# Kitchen Run Queue

## Goal

All Test Kitchen execution paths share a single DB-backed work queue serviced by a bounded worker pool. This prevents hypervisor saturation, enables deduplication, and gives the UI accurate queued/running state.

## Context

Four dispatch paths exist today:
- **Ad-hoc git kitchen single run** — `POST /kitchen/git/run` (one instance, fire-and-forget goroutine)
- **Run All Suites** — `POST /kitchen/git/run-all` (all mapped instances for one repo)
- **Batch run** — `POST /kitchen/batches/:id/run` (multi-repo, filter-based)
- **Node kitchen run** — `POST /kitchen/node-run` (single node, fire-and-forget goroutine)

None share a concurrency limiter. The `Concurrency.TestKitchenRun` config exists but is not consumed. Each VM uses ~4 CPU + 4GB RAM on Proxmox.

## Design

### Queue Table

```
kitchen_run_queue (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  run_type        TEXT NOT NULL,  -- 'git' or 'node'
  git_repo_name   TEXT,
  git_repo_url    TEXT,
  suite_name      TEXT,
  platform_name   TEXT,
  instance_name   TEXT,
  target_chef_version TEXT NOT NULL,
  head_commit_sha TEXT,
  node_name       TEXT,
  organisation_name TEXT,
  cookbook_source  TEXT,
  batch_id        UUID REFERENCES kitchen_batches(id),
  priority        INT NOT NULL DEFAULT 10,
  status          TEXT NOT NULL DEFAULT 'queued',
  enqueued_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  started_at      TIMESTAMPTZ,
  completed_at    TIMESTAMPTZ,
  error_message   TEXT,
  output          TEXT,
  retry_of        UUID REFERENCES kitchen_run_queue(id)
)
```

`run_type` distinguishes git kitchen runs (cookbook-level testing) from node kitchen runs (node-level convergence). Both consume VMs from the same hypervisor pool and share the worker limit.

Status values: `queued`, `running`, `completed`, `failed`, `cancelled`, `interrupted`.

Index: `(status, priority DESC, enqueued_at ASC)` for efficient dequeue.
Unique partial index (git runs): `(git_repo_name, suite_name, platform_name, target_chef_version, head_commit_sha) WHERE status IN ('queued','running') AND run_type = 'git'` for dedup.
Unique partial index (node runs): `(organisation_name, node_name, target_chef_version) WHERE status IN ('queued','running') AND run_type = 'node'` for dedup.

### Deduplication

Git runs key: `(git_repo_name, suite_name, platform_name, target_chef_version, head_commit_sha)`.
Node runs key: `(organisation_name, node_name, target_chef_version)`.

If an item with the same key is already queued or running, the enqueue returns the existing item (idempotent). Including `head_commit_sha` for git runs ensures a new commit can be retested without being blocked by a stale queued run.

### Priority / Fairness

- Ad-hoc single runs: priority 20 (high)
- Run-all: priority 10 (normal)
- Batch: priority 5 (low)

Workers dequeue by `priority DESC, enqueued_at ASC`. This prevents large batches from starving interactive ad-hoc runs.

### Worker Pool

- Located in `internal/kitchenqueue` package
- Pool size = `Concurrency.TestKitchenRun` (read at startup, configurable via admin API)
- Workers poll DB for next `queued` item using `SELECT ... FOR UPDATE SKIP LOCKED` (prevents races between workers)
- Worker transitions item `queued → running`, executes, then transitions to `completed`/`failed`
- On completion, upserts result into `git_kitchen_results` (as today) and broadcasts WS event

### Execution

Each worker:
1. Claims next item from DB (`UPDATE ... SET status='running' WHERE id = (SELECT ... queued ORDER BY priority DESC, enqueued_at LIMIT 1 FOR UPDATE SKIP LOCKED)`)
2. Branches by `run_type`:
   - **git**: Plans the repo (loads analysis, platform map, exclusions), calls `RunInstance()`
   - **node**: Calls the existing `nodeKitchenRunner.Run()` logic
3. Upserts result into the appropriate results table (`git_kitchen_results` or `node_kitchen_runs`)
4. Marks queue item `completed` or `failed`
5. Broadcasts WS event (`git_kitchen_run_complete` or `node_kitchen_run_complete`)

### Batch Integration

`POST /kitchen/batches/:id/run` transitions batch to `preparing`, resolves repos, enqueues all items with `batch_id` set and priority 5, then transitions to `running`. Batch completion is detected when all items with its `batch_id` reach terminal state.

Per-batch `max_concurrent_vms` is enforced as: at most N items from this batch may be `running` simultaneously. Workers skip batch items that would exceed the batch cap (move to next in queue).

### Cancellation

- `DELETE /kitchen/queue/:id` — cancels a queued item (transitions to `cancelled`)
- `POST /kitchen/queue/:id/cancel` — cancels a running item (sends context cancellation to worker, VM destroyed via the run's always-run `destroy` phase or explicit teardown, transitions to `cancelled`)
- Batch cancel — marks all `queued` items for that batch as `cancelled`, sends cancel to running items
- Shutdown — stop dequeuing, drain running items with configurable timeout, mark survivors `interrupted`

### Retry

- `POST /kitchen/queue/:id/retry` — re-enqueues a `failed`, `interrupted`, or `cancelled` item with same parameters and a fresh `enqueued_at`. Original item stays for history; new item references it via `retry_of` column (nullable FK).

### Startup Recovery

On application startup:
1. Items left in `running` status (from a crash) are transitioned to `interrupted`.
2. These are NOT automatically re-enqueued — the VM may still be alive on the hypervisor and the existing orphan sweep (runs every 30 min, age-based) will clean it up.
3. After the sweep age threshold passes (configurable, default 60 min), the orphan sweep destroys any VMs that outlived their expected lifetime.
4. The startup recovery logs each interrupted item with its instance details so operators can manually check Proxmox if needed.
5. A startup orphan sweep is triggered immediately (non-blocking) to accelerate cleanup of crash-orphaned VMs rather than waiting for the next ticker interval.

### Graceful Shutdown

On SIGTERM/SIGINT:
1. Stop accepting new queue items (enqueue returns 503).
2. Stop dequeuing — workers finish their current item but don't pick up new ones.
3. Wait up to `timeout_minutes` (from TK config, default 30) for running items to complete. Kitchen runs always end with a `destroy` phase so the VM is cleaned up as part of normal execution.
4. If timeout expires, mark remaining running items as `interrupted`. The orphan sweep will clean up their VMs on the next run (or the immediate startup sweep on next boot).

### Backpressure

Max queue depth configurable (default 100). Returns `429 Too Many Requests` when saturated.

## API

### Enqueue (replaces direct dispatch)

- `POST /kitchen/git/run` → enqueue single git item (priority 20), return `202 { id, status, position }`
- `POST /kitchen/git/run-all` → enqueue all mapped git instances (priority 10), return `202 { ids[], count }`
- `POST /kitchen/node-run` → enqueue single node item (priority 20), return `202 { id, status, position }`
- Batch run → enqueue via batch lifecycle (priority 5)

### Queue Status

- `GET /kitchen/queue[?repo=<name>&type=git|node&status=queued,running]` → list queue items (filterable)
- `GET /kitchen/queue/:id` → single item detail (includes output if available)
- `GET /kitchen/queue/:id/output` → streaming endpoint (SSE) for live kitchen output
- `DELETE /kitchen/queue/:id` → cancel queued item (alias for cancel)
- `POST /kitchen/queue/:id/cancel` → cancel queued or running item
- `POST /kitchen/queue/:id/retry` → re-enqueue failed/interrupted/cancelled item

### WebSocket Events

- `kitchen_queue_update` — item state change (queued, running, completed, failed, cancelled, interrupted)
- `git_kitchen_run_complete` — unchanged (backward compat)

## Frontend

### GitKitchenSection (repo detail page)

- Fetches queue state for the repo (`GET /kitchen/queue?repo=<name>`)
- "Run" button shows "Queued" (disabled) when instance has a queued item
- "Run" button shows "Running" (disabled) when instance has a running item
- "Run All Suites" button disabled when any instances are queued/running
- Subscribes to `kitchen_queue_update` WS event to update state live

### Queue Panel (repo detail page)

Inline panel below the kitchen instances table showing active queue items for this repo:
- Columns: instance name, status (queued/running/failed/interrupted), enqueued time, started time, duration
- Running items show a "Cancel" button
- Queued items show a "Cancel" button
- Failed/interrupted items show a "Retry" button
- Running items show a collapsible live output viewer (SSE stream from `/kitchen/queue/:id/output`)
- Completed items auto-collapse after 30s, removed after 5 min (or on next page load)
- Empty state: panel hidden when no active/recent items

### Global Queue Page (admin area)

Accessible from admin nav. Shows all queue items across all repos:
- Tabs or filter: All | Queued | Running | Recent (completed/failed last 24h)
- Columns: repo, instance, type (git/node), priority, status, enqueued, started, duration
- Cancel/Retry buttons per-item
- Running items expandable to show live output
- Summary bar: "3 running, 7 queued, 4 workers available"

### Live Output Streaming

- Worker captures kitchen stdout/stderr line-by-line
- Lines appended to a ring buffer (last 500 lines) held in memory per running item
- `GET /kitchen/queue/:id/output` uses SSE (Server-Sent Events) to push new lines
- On completion, final output is persisted to `kitchen_run_queue.output` (TEXT column, nullable)
- Frontend connects SSE when user expands the output viewer, disconnects on collapse
- If the connection drops or the item is already completed, falls back to fetching stored output

### AdminConcurrencyPage

- `test_kitchen_run` value actually controls the worker pool size
- Default synchronized between frontend and backend (4)

## Config

- `Concurrency.TestKitchenRun` — global worker count (default 4)
- Per-batch `max_concurrent_vms` — retained as a per-batch cap within the global pool
- `max_concurrent_vms` in TK config — removed (redundant with global setting)

## Migration Path

- Add `kitchen_run_queue` table (migration 0024)
- Refactor handlers to enqueue instead of direct dispatch
- Existing `scheduler.RunOne`/`RunAll`/`RunBatch` refactored: execution logic stays, orchestration moves to queue workers
- Existing `batch_instances` table may be deprecated in favour of queue items with `batch_id`

## Out of Scope

- Distributed/multi-instance queue (single process for now)
- Queue priority reordering from UI
- SSH/console access to running VMs
