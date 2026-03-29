# Plan — Tech Debt Batch 6 (Final Cleanup)

## Goal

Resolve remaining tech debt items except B0 (XL), B4a, B5, and F6.

## Completed

| ID | Summary | Commit |
|----|---------|--------|
| P8 | Re-enable errcheck linter, fix 50 violations + 7 govet/staticcheck | ✅ |
| P1 | Generate CHANGELOG.md from 46 git tags | ✅ |
| F4 | Extract FilterInput/FilterSelect/FilterCombobox to shared component | ✅ |
| F7 | Install Vitest + Testing Library, add 39 tests (semver, useSort, FilterInputs) | ✅ |

## Deferred

| ID | Reason |
|----|--------|
| F6 | Large page file split — moderate risk, low urgency, no functional impact |
| B4a | Readiness trend snapshots — needs collector + handler changes, warrants own spec |
| B5 | Datastore tests — requires live Postgres, best done as functional tests |
| B0 | UUIDs→natural keys — XL, requires own specification and phased migration |

## Acceptance criteria

- [x] `golangci-lint run ./...` clean (errcheck re-enabled)
- [x] `go test ./...` all green
- [x] `go build ./...` clean
- [x] `npm run build` clean
- [x] `npm test` runs 39 real tests and passes
- [x] `CHANGELOG.md` exists with entries for all 46 tags
- [x] Tech debt list updated