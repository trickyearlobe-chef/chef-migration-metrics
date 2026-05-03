# Fix: Ownership Always Enabled

## Goal

Remove the `Ownership.Enabled` config flag. Ownership (and committer collection) should always be active — the flag adds complexity without value.

## Specs to Read

- `.claude/specifications/todo-tech-debt.md` (the item being fixed)

## Steps

1. Remove `Enabled` field from `OwnershipConfig` struct
2. Remove env var handling for `CMM_OWNERSHIP_ENABLED`
3. Remove `if !c.Ownership.Enabled { return }` guard in `validateOwnership`
4. Remove `ownershipEnabled` parameter from `fetchGitCookbooks` — always extract committers
5. Remove `r.cfg.Ownership.Enabled` checks in webapi handlers — ownership endpoints always available
6. Update tests that set `cfg.Ownership.Enabled = true/false`
7. Remove diagnostic config output of `enabled` field
8. Run tests, verify all pass

## Acceptance Criteria

- `Ownership.Enabled` field no longer exists
- Committers always collected during git fetch
- Ownership API endpoints always available
- Owner filtering still works (gated by filter being active, not config flag)
- All tests pass
