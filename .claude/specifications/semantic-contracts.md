# Semantic Contracts — Derived Metric Definitions

Canonical definitions for all derived metrics. Each metric has exactly one authoritative calculation path. Any code computing these values MUST conform to the contract here.

## 1. Staleness Tier

**Package:** `internal/staleness`

**Inputs:** `ohaiTime time.Time`, `now time.Time`, `Thresholds{WarningHours, CriticalDays}`

**Contract:**
- `ohaiTime.IsZero()` → `critical`
- `age >= CriticalDays * 24h` → `critical`
- `age >= WarningHours * 1h` → `warning`
- else → `fresh`

**Backward compat:** `IsStaleCompat(tier) = tier != fresh`

**Canonical function:** `staleness.ComputeTier`

**Consumers:** node list API, collector summaries, readiness evaluator, exports.

**Discrepancy (resolved by this contract):** None currently — already centralised.

---

## 2. Test Kitchen Status

**Package:** `internal/tkstatus`

**Inputs:** `passed int`, `failed int`

**Contract:**
- `passed > 0 && failed > 0` → `"partial"`
- `failed > 0` → `"failed"`
- `passed > 0` → `"passed"`
- else → `""` (no data)

**Canonical function:** `tkstatus.ComputeTKStatus`

**Consumers:** readiness evaluator, complexity scoring, git repo handlers, dashboard compatibility, role detail, cookbook remediation.

**Discrepancy (resolved by this contract):** All callers MUST use `tkstatus.ComputeTKStatus` or `Counts.Status()`. No inline re-implementation. Fixed: `webapi/handle_kitchen_batches.go` was inlining the logic — now uses canonical function.

---

## 3. Complexity Score

**Package:** `internal/remediation`

**Inputs:** `ComplexityInput{Cookstyle CookstyleOffenseSummary, TestKitchen TKStatus, Blast BlastRadius}`

**Contract:**
```
score = ErrorFatalCount * 5
      + DeprecationCount * 3
      + CorrectnessCount * 3
      + ManualFixCount * 4
      + ModernizeCount * 1
      + tkPenalty
```

Where `tkPenalty`:
- TK status `"failed"` → 20
- TK status `"partial"` → 10
- else → 0

Blast radius is NOT part of the score. It is persisted alongside.

**Label mapping:**
- 0 → `none`
- 1–10 → `low`
- 11–30 → `medium`
- 31–60 → `high`
- 61+ → `critical`

**Canonical functions:** `remediation.ComputeComplexityScore`, `remediation.ScoreToLabel`

**Consumers:** server cookbook scoring, git repo scoring, remediation priority API, cookbook detail API, dashboard compatibility.

**Discrepancy (resolved by this contract):** None — already centralised in pure functions.

---

## 4. Blast Radius

**Package:** `internal/remediation` (cookbook), `internal/datastore` (role)

### 4a. Cookbook Blast Radius

**Inputs:** cookbook usage summaries, role counts per cookbook

**Contract:**
- `AffectedNodeCount` = count of distinct nodes running this cookbook (from `ListCookbookUsageSummaries`)
- `AffectedPolicyCount` = count of distinct policy names referencing this cookbook
- `AffectedRoleCount` = count of distinct roles including this cookbook (from `CountRolesPerCookbook`)
- Aggregated per cookbook NAME across all versions and organisations

**Canonical function:** `remediation.loadBlastRadii`

### 4b. Role Blast Radius

**Inputs:** `node_snapshots` table

**Contract:**
- Count distinct `(organisation_name, node_name)` for nodes whose roles contain the target role
- Break down by: overall, per-org, per-environment, per-platform

**Canonical function:** `datastore.getRoleBlastRadius`

**Discrepancy (noted):** Cookbook blast radius aggregates across orgs by cookbook name. Role blast radius is inherently multi-org aware. These are different dimensions — both are correct for their context.

---

## 5. Node Readiness

**Package:** `internal/analysis`

**Inputs per node:** node snapshot, staleness tier, cookbook list, cookstyle/TK results (via cache), min free disk MB

**Contract:**
- `IsReady = AllCookbooksCompatible AND SufficientDiskSpace`
- `AllCookbooksCompatible = len(blockingCookbooks) == 0`
- `SufficientDiskSpace`: nil (unknown) if stale, else `availableDiskMB >= minFreeDiskMB`
- Stale nodes: `SufficientDiskSpace = nil`, `StaleData = true`
- `BlockingCookbooks`: list of cookbooks with at least one incompatible verdict from any source

**Canonical function:** `analysis.evaluateOne` (private), exposed via `ReadinessEvaluator.EvaluateOrganisation`

**Persisted columns:** `is_ready`, `all_cookbooks_compatible`, `sufficient_disk_space`, `blocking_cookbooks` (JSONB), `stale_data`, `cookstyle_status`, `kitchen_status`

---

## 6. Cookstyle Status (per node)

**Package:** `internal/analysis` (write-time), `internal/webapi` (read-time)

**Inputs:** `StaleData bool`, `AllCookbooksCompatible bool`, `BlockingCookbooks []BlockingEntry`

**Contract:**
- Stale → `"unknown"`
- `AllCookbooksCompatible && len(blocking) == 0` → `"passed"`
- Any blocking cookbook with a cookstyle verdict of `"incompatible"` → `"failed"`
- Blocking exists but ALL are TK-only failures (no cookstyle verdicts at all) → `"passed"`
- Blocking with cookstyle verdicts but all are `"compatible"` → `"passed"`
- Otherwise → `"unknown"`

**Discrepancy (to fix in Phase 2):** This logic exists in TWO places:
1. `internal/analysis/readiness.go` — computed at evaluation time and persisted
2. `internal/webapi/check_status.go:deriveCookstyleStatus` — re-derived from persisted data at read time

Both implement the same contract but independently. Phase 2 will eliminate the read-time re-derivation by serving the persisted value directly.

---

## 7. Kitchen Status (per node)

**Package:** `internal/analysis` (write-time), `internal/webapi` (read-time)

**Inputs:** `StaleData bool`, `AllCookbooksCompatible bool`, `BlockingCookbooks []BlockingEntry`

**Contract:**
- Stale → `"unknown"`
- `AllCookbooksCompatible && len(blocking) == 0` → `"passed"`
- Any blocking cookbook with TK verdict `"incompatible"` → `"failed"`
- Some blocking tested (TK verdicts exist) + some not tested → `"partial"`
- All blocking lack TK results → `"unknown"`
- Otherwise → `"unknown"`

**Discrepancy (to fix in Phase 2):** Same as cookstyle — two implementations (analysis write-time, webapi read-time). Phase 2 will serve persisted values.

---

## 8. Cross-Org Aggregation

**Contract:**
- Cookbook compatibility: aggregated across all orgs, deduplicated by cookbook name per target version
- Role detail: multi-org aware for blast radius; dependency/compat lookup anchored to first org found
- Readiness evaluation: org-scoped for server cookbooks, global for git repos

**Discrepancy (to fix in Phase 2):** Role dependency/compat lookup uses first org only. If a role exists in multiple orgs with different cookbook versions, only the first org's data is checked. Phase 2 should evaluate per-org and merge results.

---

## 9. Collection Run Gating

**Problem:** Metric snapshots and trend data currently include partial collection runs. A collection that fails mid-way still writes partial results, which can cause trend graphs to show misleading dips.

**Contract (to implement in Phase 2):**
- Each collection run gets a unique `run_id`
- Metric snapshots reference the `run_id` that produced them
- Only snapshots from COMPLETE collection runs are included in trend queries
- Partial runs are flagged and excluded from aggregation

---

## Audit Matrix

| Metric | Canonical Location | Duplicate Sites | Phase 2 Action |
|--------|-------------------|-----------------|----------------|
| Staleness | `staleness.ComputeTier` | None | — |
| TK Status | `tkstatus.ComputeTKStatus` | None (all callers use it) | — |
| Complexity | `remediation.ComputeComplexityScore` | None | — |
| Blast Radius | `remediation.loadBlastRadii` | Role: `datastore.getRoleBlastRadius` | Document as separate dimensions |
| Readiness | `analysis.evaluateOne` | None | — |
| Cookstyle Status | `analysis.readiness` | `webapi.deriveCookstyleStatus` | Serve persisted value |
| Kitchen Status | `analysis.readiness` | `webapi.deriveKitchenStatus` | Serve persisted value |
| Cross-Org | Multiple | Role detail first-org | Per-org evaluation |
| Collection Gating | Not implemented | — | Add run_id + completeness flag |
