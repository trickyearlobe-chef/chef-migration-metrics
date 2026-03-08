# CookStyle Namespace Prefixes and Target Version Fix

**Date:** 2026-03-08
**Component:** Analysis pipeline (CookStyle scanning, autocorrect, complexity scoring, cop mappings)
**Branch:** `fix/cookstyle-namespace-and-target-version`

---

## Context

After clicking "Rescan CookStyle" on the chef-client cookbook, the result showed **Failed** with **0 offenses listed**. Investigation revealed two root-cause bugs that combined to make CookStyle scanning completely non-functional when a target Chef version was configured.

## Root Causes

### 1. Invalid `--target-chef-version` CLI flag

`buildCookstyleArgs()` passed `--target-chef-version 18.0` on the CLI, but CookStyle does not support this flag. CookStyle exited with code 2 and the message `invalid option: --target-chef-version`. Because the executor treats non-zero exits as normal (CookStyle exits non-zero when offenses are found), the code fell through to JSON parsing, which failed on the error text. The result was persisted with `Passed=false` and 0 offenses — exactly matching the reported symptom.

The correct mechanism is to set `AllCops.TargetChefVersion` in a `.rubocop.yml` configuration file and point CookStyle at it with `--config`.

### 2. Wrong cop namespace prefixes

The code used `ChefDeprecations/`, `ChefCorrectness/`, `ChefStyle/`, `ChefModernize/` as cop name prefixes, but CookStyle 7.32.8 actually emits `Chef/Deprecations/`, `Chef/Correctness/`, `Chef/Style/`, `Chef/Modernize/` (with a `Chef/` prefix and `/` separator). This caused:

- The `--only ChefDeprecations,ChefCorrectness` filter to match zero cops (silent no-op)
- Offense classification (`isDeprecation`, `isCorrectness`, etc.) to never match
- Cop mapping lookups (`LookupCop`) to always return nil (no remediation guidance)

## What Was Done

### Sidecar `.rubocop_cmm.yml` approach

Instead of writing into the cookbook's `.rubocop.yml` (which would clobber the cookbook's own configuration), we now write a sidecar file named `.rubocop_cmm.yml` into the cookbook directory and pass `--config <path>` to CookStyle.

- **If the cookbook has an existing `.rubocop.yml`**: the sidecar uses `inherit_from: .rubocop.yml` to preserve all cookbook settings (excludes, custom cops, `require: cookstyle`, etc.)
- **If no cookbook config exists**: the sidecar includes `require: - cookstyle` so that the `TargetChefVersion` AllCops parameter is registered by CookStyle's extensions

This approach was validated against both server-sourced cookbooks (no `.rubocop.yml`) and git-sourced cookbooks (with `.rubocop.yml` containing `require: cookstyle`).

### Namespace prefix corrections

All cop namespace prefixes were updated across the entire codebase:

| Old prefix | New prefix |
|---|---|
| `ChefDeprecations/` | `Chef/Deprecations/` |
| `ChefCorrectness/` | `Chef/Correctness/` |
| `ChefStyle/` | `Chef/Style/` |
| `ChefModernize/` | `Chef/Modernize/` |

This includes the `--only` filter values, classification helper constants, and all 78 entries in the embedded cop mapping table.

## Final State

- `go test ./...` — all 15 packages pass (0 failures)
- `make build` — compiles and frontend builds cleanly
- `npm audit` — 0 vulnerabilities
- Manually verified with `cookstyle --format json --config <sidecar> --only Chef/Deprecations,Chef/Correctness` against both the chef-client (server) and logrotate (git) cookbooks — offenses are correctly detected and classified

## Known Gaps

- The `TargetChefVersion` setting may behave differently across CookStyle versions. Tested with CookStyle 7.32.8 / RuboCop 1.25.1.
- The sidecar `.rubocop_cmm.yml` file is left behind in the cookbook directory after scanning. It is small and deterministic, so this is harmless, but a cleanup step could be added if desired.

## Files Modified

**Production code:**
- `internal/analysis/cookstyle.go` — fixed namespace constants, replaced `--target-chef-version` CLI flag with sidecar `.rubocop_cmm.yml` + `--config`, added `writeCookstyleTargetConfig()` with `inherit_from` / `require: cookstyle` logic
- `internal/remediation/autocorrect.go` — same sidecar approach for autocorrect args, fixed `--only` filter
- `internal/remediation/complexity.go` — fixed namespace constants and comments
- `internal/remediation/copmapping.go` — updated all 78 cop name entries and section comments
- `internal/webapi/handle_cookbook_remediation.go` — fixed cop name in code comment

**Test code:**
- `internal/analysis/cookstyle_test.go` — updated namespace strings, added temp dir usage for sidecar tests, added tests for `inherit_from` vs `require: cookstyle` paths
- `internal/remediation/autocorrect_test.go` — same sidecar test updates, added `WithCookbookConfig` test variant
- `internal/remediation/complexity_test.go` — updated namespace strings in classification tests
- `internal/remediation/copmapping_test.go` — updated cop name strings in lookup and validation tests
- `internal/webapi/handle_cookbook_remediation_test.go` — updated cop name strings in response assertions
- `internal/logging/logging_test.go` — updated cop name string in process output test

**Documentation:**
- `.claude/summaries/2026-03-08-22-44-cookstyle-namespace-and-target-version.md` — this file
- `.claude/Structure.md` — added summary entry

## Recommended Next Steps

1. **Run `make run` and trigger a collection cycle** to verify the chef-client cookbook now produces correct CookStyle results with offenses listed. The rescan button should invalidate and the next cycle should populate real offense data.
2. Refer to the previous summary (`2026-03-08-13-25-cookstyle-json-and-build-fixes.md`) for the broader project roadmap.