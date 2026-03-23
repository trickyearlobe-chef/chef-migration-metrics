# Specification: CookStyle Exit Code Error Handling

## Problem

CookStyle (built on RuboCop) uses three exit codes:

| Exit Code | RuboCop Constant     | Meaning |
|-----------|----------------------|---------|
| 0         | `STATUS_SUCCESS`     | Scan completed, no offences |
| 1         | `STATUS_OFFENSES`    | Scan completed, offences found |
| 2         | `STATUS_ERROR`       | Scan failed — config error, Ruby exception, load error |

The current code in `executeCommand` treats **all** non-zero exit codes as
"normal" (offences found) and returns `nil` for the error:

```
if err != nil && cmd.ProcessState != nil && exitCode != 0 {
    return stdoutBuf.String(), stderrBuf.String(), exitCode, nil
}
```

This means exit code 2 (CookStyle crashed) is indistinguishable from exit
code 1 (offences found). The scan functions `scanOneServerCookbook` and
`scanOneGitRepo` then attempt to parse stdout as JSON. Depending on what
CookStyle printed before crashing, two bad outcomes occur:

1. **JSON parsing fails** → result persisted with `Passed: false` (zero
   value) → cookbook appears **incompatible** when CookStyle just crashed.

2. **Valid JSON was emitted before crash** → result persisted with
   `Passed: true` and zero offences → cookbook appears **compatible** when
   CookStyle actually errored.

Common causes of exit code 2 include invalid `.rubocop.yaml` in the
cookbook (references unknown cops, invalid YAML syntax, incompatible
configuration options).

Additionally, server cookbooks are immutable — the scanner skips any
cookbook that already has a result. So a cookbook that errored on a previous
scan will **never be re-scanned**, even if the underlying problem (e.g. a
bundled CookStyle version update that fixes the config issue) is resolved.

## Solution

### 1. Add `error_message` column to CookStyle result tables

Add a nullable `TEXT` column `error_message` to both
`server_cookbook_cookstyle_results` and `git_repo_cookstyle_results`.

Semantics:

| `passed` | `error_message` | Meaning |
|----------|----------------|---------|
| `true`   | `NULL`/empty   | Scan completed successfully — compatible |
| `false`  | `NULL`/empty   | Scan completed, error-severity offences — incompatible |
| `false`  | non-empty      | Scan errored (exit code ≥ 2) — not a real result |

This is backward-compatible: existing code that only checks `Passed`
continues to work (error results have `Passed = false`, which is the safe
default). New code checks `ErrorMessage` to distinguish "incompatible"
from "errored".

### 2. Detect exit code ≥ 2 in scan functions

In `scanOneServerCookbook` and `scanOneGitRepo`, after step 4 (execution
failure handling) and before step 5 (JSON parsing), add a check:

```
if exitCode >= 2 {
    sr.Error = fmt.Errorf("cookstyle error (exit %d): %s", exitCode, strings.TrimSpace(stderr))
    sr.ErrorMessage = fmt.Sprintf("CookStyle error (exit %d): %s", exitCode, strings.TrimSpace(stderr))
    log.Warn(...)
    persist(ctx, sr)
    return sr
}
```

This prevents JSON parsing of crash output and records the error message.

The `CookstyleScanResult` struct gains an `ErrorMessage string` field
that flows through to the persist functions.

### 3. Re-scan cookbooks with error results

The scanner currently skips server cookbooks when any result exists:

```
existing, err := s.db.GetServerCookbookCookstyleResult(ctx, sc.ID, targetChefVersion)
if err == nil && existing != nil {
    sr.Skipped = true
    return sr
}
```

Change this to only skip when the existing result has no error:

```
if err == nil && existing != nil && existing.ErrorMessage == "" {
    sr.Skipped = true
    return sr
}
```

Cookbooks with error results will be re-scanned on the next collection
run, allowing recovery when CookStyle is updated or config issues are
fixed externally.

Git repo cookbooks are already re-scanned on every commit (keyed by
commit SHA), so no change is needed there. However, the same error
detection logic applies — they should record the error message rather
than a false incompatible result.

### 4. Readiness evaluator: treat error results as untested

In the readiness evaluator's `checkCookbookCompatibility`, results with
a non-empty `ErrorMessage` should be skipped — they are not evidence of
compatibility or incompatibility. The cookbook is effectively untested.

The cache-based lookups in `readiness.go` check `Passed` on the cached
result structs. Add a check: if `ErrorMessage != ""`, skip the result
(do not set `anyTested = true`).

### 5. Dashboard and list handlers: count errors separately

The dashboard compatibility handler currently counts results as either
compatible (`Passed = true`) or incompatible (`Passed = false`). Add an
"error" count for results where `ErrorMessage != ""`. These should not
inflate the incompatible count.

The cookbook list handler should show "error" as a distinct compatibility
status alongside "compatible", "incompatible", and "untested".

## Migration

File: `migrations/0005_cookstyle_error_message.up.sql`

```
ALTER TABLE server_cookbook_cookstyle_results
    ADD COLUMN error_message TEXT;

ALTER TABLE git_repo_cookstyle_results
    ADD COLUMN error_message TEXT;
```

File: `migrations/0005_cookstyle_error_message.down.sql`

```
ALTER TABLE git_repo_cookstyle_results
    DROP COLUMN IF EXISTS error_message;

ALTER TABLE server_cookbook_cookstyle_results
    DROP COLUMN IF EXISTS error_message;
```

## Changes by File

### Migration
| File | Change |
|------|--------|
| `migrations/0005_cookstyle_error_message.up.sql` | Add `error_message TEXT` to both tables |
| `migrations/0005_cookstyle_error_message.down.sql` | Drop column |

### Datastore
| File | Change |
|------|--------|
| `internal/datastore/server_cookbook_cookstyle_results.go` | Add `ErrorMessage` to struct and params, update all column lists, scan helpers, and upsert query |
| `internal/datastore/git_repo_cookstyle_results.go` | Add `ErrorMessage` to struct and params, update all column lists, scan helpers, and upsert query |

### Analysis
| File | Change |
|------|--------|
| `internal/analysis/cookstyle.go` | Add `ErrorMessage` field to `CookstyleScanResult`, detect exit code ≥ 2 in `scanOneServerCookbook` and `scanOneGitRepo`, pass error message through persist functions, change skip logic to re-scan error results |
| `internal/analysis/readiness.go` | Skip results with non-empty `ErrorMessage` in `checkCookbookCompatibility` |

### Web API
| File | Change |
|------|--------|
| `internal/webapi/handle_dashboard.go` | Count error results separately in compatibility handlers |
| `internal/webapi/handle_cookbooks.go` | Map error results to "error" compatibility status |

### Frontend
| File | Change |
|------|--------|
| Frontend cookbook/dashboard components | Display "error" state distinctly from "incompatible" |

## Test Changes

### New tests
- `TestScanOneNoDB_ExitCode2_ErrorMessage` — exit code 2 sets ErrorMessage, Passed=false
- `TestScanOneNoDB_ExitCode2_NoJsonParsing` — exit code 2 does not attempt JSON parse
- `TestScanOneNoDB_ExitCode1_NoErrorMessage` — exit code 1 (normal offences) has empty ErrorMessage
- `TestScanOneServerCookbook_SkipsOnlyCleanResults` — existing result with error is re-scanned
- `TestCheckCookbookCompatibility_ErrorResultTreatedAsUntested` — readiness evaluator skips error results

### Updated tests
- Existing scan helper tests updated for new `ErrorMessage` field in struct
- Existing upsert tests updated for new column