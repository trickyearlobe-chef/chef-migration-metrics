# Dynamic Kitchen Worker Scaling

## Goal
Allow the kitchen queue manager to adjust worker count at runtime when concurrency settings change via the admin UI.

## Specs to read
- `.claude/specifications/project-conventions.md` (Go patterns)

## Steps
1. Add `SetWorkerCount(n int)` to `kitchenqueue.Manager` with scale-up (spawn) and scale-down (exit) logic
2. Wire it in `putAdminConfigConcurrency` — after config reload, call `kitchenQueue.SetWorkerCount()`
3. Write tests for scale-up and scale-down
4. Rebuild and verify

## Acceptance Criteria
- Changing "Test Kitchen Run Workers" in admin UI adjusts active workers without restart
- Scale-down: excess workers exit after their current poll cycle
- Scale-up: new workers spawn immediately
- Duration polling fix (already committed) takes effect after rebuild
