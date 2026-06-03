# Plan: Disk Space Analysis (Phase 2)

## Goal

Replace the single `min_free_disk_mb` readiness config with per-platform install sizes, configurable install paths, and a dual threshold (absolute + percentage). Add a UI settings page.

## Specs

- `specifications/configuration.md` — Upgrade Readiness section
- `specifications/analysis.md` — Disk Space Evaluation section

## Steps

1. Backend config: expand `ReadinessConfig` struct with new fields and defaults
2. Backend evaluator: update `ReadinessEvaluator` to accept new config, use per-platform sizes, configurable paths, and dual threshold
3. Backend API: add `/api/v1/admin/config/readiness` GET/PUT endpoint
4. Frontend: add Readiness settings page with path warning
5. Frontend: wire into nav (Settings sub-group)
6. Tests: unit tests for dual threshold, per-platform logic, config API
7. Update `plans/todo-configuration.md` when done

## Acceptance Criteria

- Config YAML supports: `install_path_linux`, `install_path_windows`, `install_size_mb_linux`, `install_size_mb_windows`, `min_remaining_free_percent`
- Old `min_free_disk_mb` still works as a backward-compatible fallback (deprecated)
- Disk eval uses dual threshold: absolute size + remaining-free percentage
- `determineInstallPath` reads from config, not hardcoded
- UI page shows all 5 fields with prominent path warning
- All existing tests still pass
