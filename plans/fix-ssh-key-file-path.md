# Fix: SSH Key File Path for Test Kitchen Transport

## Goal

Test Kitchen's SSH transport `ssh_key` expects a **file path**, not inline PEM content. Currently the overlay generators inject raw PEM content via ERB env var expansion, causing SSH auth failure ("Operation not supported by device").

## Specs to Read

- `.claude/specifications/test-kitchen-drivers.md` (transport credential model)

## Steps

1. Add `WriteSSHKeyFiles` helper in `internal/analysis/kitchen_credentials.go`
   - Takes resolved credential env vars, identifies `CMM_TK_KEY_*` entries
   - Writes each PEM to a temp file (mode 0600)
   - Returns map of `CMM_TK_KEY_PATH_<NAME>` → file path, plus cleanup func
2. Update overlay generators to reference `CMM_TK_KEY_PATH_*` instead of `CMM_TK_KEY_*`:
   - `internal/analysis/kitchen.go` — `buildOverlay`
   - `internal/nodekitchen/config_gen.go` — `GenerateOverlay`
   - `internal/batch/kitchen_runner.go` — `buildInstanceOverlay`
3. Update callers to call `WriteSSHKeyFiles` after credential resolution and inject path env vars:
   - `internal/analysis/kitchen.go` — `testOne`
   - `internal/nodekitchen/runner.go` — `Run`
   - `internal/batch/kitchen_runner.go` — `RunInstance`
4. Update tests for all changed functions
5. Delete this plan

## Acceptance Criteria

- Overlay YAML has `ssh_key: <%= ENV['CMM_TK_KEY_PATH_ALMA10'] %>` (path, not content)
- Temp key files are mode 0600 and cleaned up after the run
- All existing tests pass; new tests cover the helper and overlay change