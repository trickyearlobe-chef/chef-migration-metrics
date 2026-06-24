# CookStyle Failure Rules — Component Specification

> **TL;DR** — Configurable per-namespace severity thresholds that determine whether a CookStyle scan result is "passed" or "failed". Includes named presets (strict/default/relaxed) and auto re-scoring of stored results when rules change. The default preset preserves current behaviour: fail on `error` or `fatal` severity regardless of namespace.

## Overview

Currently `Passed` is determined at scan time by a hardcoded rule: any offense with severity `error` or `fatal` causes failure. Operators cannot tune which cop namespaces or severity levels constitute a failure.

This specification adds a configurable **failure rules matrix** — a map of cop namespace prefixes to severity lists. An offense triggers failure if its namespace's configured severities include its severity level. A catch-all (`*`) rule covers cops not matching any explicit namespace.

## Configuration Shape

```yaml
analysis_tools:
  cookstyle_failure_preset: "default"  # "strict" | "default" | "relaxed"
  cookstyle_failure_rules: {}          # explicit overrides (takes precedence over preset)
```

When `cookstyle_failure_rules` is non-empty, it overrides the preset entirely. When empty/omitted, the named preset supplies the rules.

### Presets

| Preset | Rules |
|--------|-------|
| `strict` | `{"Chef/Deprecations/": ["warning","error","fatal"], "Chef/Correctness/": ["warning","error","fatal"], "*": ["error","fatal"]}` |
| `default` | `{"*": ["error","fatal"]}` |
| `relaxed` | `{"Chef/Deprecations/": ["error","fatal"], "Chef/Correctness/": ["error","fatal"], "Chef/Style/": [], "Chef/Modernize/": [], "*": []}` |

- **strict** — Fails on warnings in Deprecations/Correctness (catches issues that may not crash but will break on upgrade), plus errors/fatal everywhere else.
- **default** — Current behaviour: fail only on error/fatal regardless of namespace.
- **relaxed** — Only Deprecations and Correctness errors cause failure; Style/Modernize/other cops never fail.

### Explicit Rules

```yaml
cookstyle_failure_rules:
  "Chef/Deprecations/": ["warning", "error", "fatal"]
  "Chef/Correctness/":  ["error", "fatal"]
  "Chef/Style/":        []
  "Chef/Modernize/":    []
  "*":                  ["error", "fatal"]
```

Keys are cop namespace prefixes (matched against the `cop_name` field using `strings.HasPrefix`). Values are lists of severity strings that trigger failure for that namespace. Empty list = never fails.

The `*` key is the catch-all for cops not matching any other prefix. If `*` is omitted, unmatched cops never trigger failure.

### Valid Severities

`convention`, `refactor`, `warning`, `error`, `fatal` (the five RuboCop severity levels).

## Evaluation Algorithm

For a set of offenses and a rules map:

1. For each offense, find the matching rule:
   - Try each explicit namespace prefix (longest-prefix-first match)
   - If no prefix matches, use `*` (if present)
   - If no rule matches at all, the offense cannot trigger failure
2. Check if the offense's severity is in the matched rule's severity list
3. Result: `passed = true` if no offense triggered failure

### Longest-Prefix Match

Rules are sorted by key length descending before evaluation. This allows both broad (`Chef/Deprecations/`) and narrow (`Chef/Deprecations/ResourceWithoutUnifiedTrue`) rules to coexist — the narrower rule wins.

## Auto Re-Score on Config Change

When the effective failure rules change (preset change or explicit rules edit):

1. The config applier detects the rules have changed (compare serialised rules to previous)
2. A lightweight re-score runs in-process:
   - Load all `server_cookbook_cookstyle_results` and `git_repo_cookstyle_results` rows where `offences` is non-null
   - For each, deserialise the stored offenses, apply the new rules, compute `passed`
   - Batch-update rows where `passed` has changed
3. Trigger materialised-status recomputation for affected git repos (existing `RecomputeGitRepoStatus`)
4. Log the re-score outcome: `N results re-scored, M verdicts changed`

### Performance Characteristics

- Typical dataset: hundreds to low-thousands of cookstyle results
- Each re-score is a JSON unmarshal + iterate offenses — sub-millisecond per result
- Total re-score: tens of milliseconds for typical deployments
- Runs synchronously in the config applier (blocks the save response briefly)

### Edge Cases

- Results with null/empty `offences` column (legacy or crashed scans): `passed` is not changed
- Results with non-empty `error_message`: `passed` is not changed (scan was inconclusive)
- If rules change back to previous values: re-score still runs but produces no updates

## Live Reload

The failure rules are **applied** granularity (no restart required). The config applier:
1. Persists the new config
2. Runs the re-score
3. Returns `restart_required: false`

The scanner reads the effective rules at scan time via a `func() CookstyleFailureRules` provider (same pattern as `WithCookstyleConcurrencyFunc`).

## Config Struct Changes

Two new fields added to `AnalysisToolsConfig` (`internal/config/config.go`):

- `CookstyleFailurePreset string` — yaml key `cookstyle_failure_preset`
- `CookstyleFailureRules map[string][]string` — yaml key `cookstyle_failure_rules`

## Validation Rules

- `cookstyle_failure_preset` must be one of `strict`, `default`, `relaxed` (or empty → `default`)
- `cookstyle_failure_rules` keys must be non-empty strings
- `cookstyle_failure_rules` values must contain only valid severities: `convention`, `refactor`, `warning`, `error`, `fatal`
- At most one `*` key allowed (enforced by map semantics)

## Invariants

- Default preset MUST produce identical pass/fail verdicts to current hardcoded logic
- Changing rules does NOT trigger a full CookStyle re-scan (expensive); only re-evaluation of stored offenses
- The `passed` column in the DB always reflects the currently-active rules (eventually consistent via re-score)
- Node readiness derivation (`check_status.go`) continues to consume the persisted `passed` value — no change needed there

## Call Sites (LSP-verified)

`isErrorOrFatal` (the function to replace with rules-based evaluation):
- `internal/analysis/cookstyle.go:560` — server cookbook scan path
- `internal/analysis/cookstyle.go:701` — git repo scan path

`CookstyleScanResult.Passed` (set from the evaluation, consumed downstream):
- Set at `:543`, `:561` (server), `:684`, `:702` (git)
- Mapped to upsert params at `:879` (server persist), `:921` (git persist)
- Read by `collector/server_cookbook_pipeline.go:355` (maps to DB params)

Downstream DB types (`ServerCookbookCookstyleResult.Passed`, `GitRepoCookstyleResult.Passed`) are read by web handlers but remain unchanged — the re-score updates their DB column directly.

## Admin UI

The failure rules are presented as a checkbox grid on the Analysis Tools admin page.

### Layout

1. **Preset dropdown** — "Strict", "Default", "Relaxed", "Custom". Selecting a preset populates the grid. Any manual tick change switches to "Custom".
2. **Checkbox grid** — one row per namespace, one column per severity:

| Namespace | convention | refactor | warning | error | fatal |
|-----------|:---:|:---:|:---:|:---:|:---:|
| Chef/Deprecations/ | ☐ | ☐ | ☑ | ☑ | ☑ |
| Chef/Correctness/ | ☐ | ☐ | ☐ | ☑ | ☑ |
| Chef/Style/ | ☐ | ☐ | ☐ | ☐ | ☐ |
| Chef/Modernize/ | ☐ | ☐ | ☐ | ☐ | ☐ |
| Other (catch-all) | ☐ | ☐ | ☐ | ☑ | ☑ |

3. **Save** triggers the config PUT, which persists and runs the re-score. The response confirms how many verdicts changed.

### Behaviour

- Columns are ordered by ascending severity (convention → fatal, left to right)
- Ticking a checkbox adds that severity to the namespace's failure list
- The grid always shows all 5 known namespaces (4 Chef/ + catch-all) even if no offenses exist in some
- After save, a brief toast: "Failure rules updated. N cookbook verdicts changed."

## Related

- [analysis-cookstyle.md](analysis-cookstyle.md) — CookStyle invocation and output parsing
- [configuration.md](configuration.md) — Configuration surface area
- [configuration-live-reload.md](configuration-live-reload.md) — Live reload architecture
- [dual-compatibility-signals.md](dual-compatibility-signals.md) — How CookStyle + TK combine into verdicts
