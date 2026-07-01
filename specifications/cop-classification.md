# Cop Classification & Analysis — Component Specification

> **TL;DR** — A system for classifying CookStyle cops by their actual migration
> impact (Blocker / Review / Noise), with auto-seeding from `RemovedIn` data, operator overrides, custom cop definitions for gaps in cookstyle, and a cop-centric analysis UI that answers "what must I fix to migrate?"

## Overview

CookStyle severity levels (`convention`, `refactor`, `warning`, `error`, `fatal`) do not reliably indicate whether a cop is a migration blocker. The `Chef/Deprecations/NodeSet` cop (removed in Chef 13 — hard crash) and `Chef/Deprecations/DependsPoise` (unmaintained but still works) both fire at `warning`. Operators need to know which cops actually block their migration.

This specification adds:

1. **Cop classification** — per-cop, per-target-version labels (Blocker / Review / Noise)
2. **Auto-seeding** — cops with `RemovedIn ≤ target_version` are auto-classified as Blocker
3. **Operator overrides** — reclassify any cop via the UI
4. **Custom cop definitions** — define patterns not yet in cookstyle (e.g. `nil.=~` removal in Ruby 3)
5. **Cop analysis view** — per-cop aggregation showing affected cookbooks, fix effort, and classification
6. **Updated pass/fail** — classification-aware failure determination replaces pure severity rules

## Classification Levels

| Level | Meaning | Visual | Pass/Fail |
|-------|---------|--------|-----------|
| **Blocker** | Will crash or silently produce wrong results on target version | 🔴 | Fails cookbook |
| **Review** | Likely problematic — operator should investigate | 🟠 | Does not fail (advisory) |
| **Noise** | Tooling-only, style, or harmless on target version | ⚪ | Does not fail |
| **Unclassified** | Not yet reviewed — falls back to severity rules | ❓ | Severity fallback |

## Classification Resolution

For a given cop + target version, classification is resolved in priority order:

1. **Operator override** (stored in DB) — highest priority
2. **Auto-seed: `RemovedIn ≤ target_version`** — from cop mapping table
3. **Curated exact default** (shipped) — a specific named cop
4. **Curated prefix/department default** (shipped) — longest matching namespace
   (`Chef/Style/`, `Style/`, `Layout/` → Noise); matches cops we never enumerated,
   so new cosmetic cops classify with no code change
5. **Unclassified** — fallback to severity-based failure rules

### Pass/Fail Determination

A cookbook **fails** if it has any offense where:
- Classification = Blocker, OR
- Classification = Unclassified AND severity ∈ configured failure severities (existing failure rules as fallback)

This preserves backward compatibility: until operators classify cops, the existing severity-based rules still apply.

### CookStyle Rollup Status

Binary pass/fail hides advisory work: a repo whose only issues are Review-level
cops is neither "ready" nor "broken". The classification-derived **CookStyle
rollup status** is the canonical per-cookbook / per-repo / per-node verdict used by
every surface (list, summary card, dashboard compatibility cards, detail header,
node readiness, exports, trends), replacing the old
compatible/incompatible/passed/failed wording:

| Status | Visual | Condition |
|--------|--------|-----------|
| **Ready** | 🟢 | Scan exists; no blockers and no review-level offenses (clean, or only Noise / non-failing Unclassified) |
| **Needs review** | 🟠 | No blockers, but ≥1 Review offense |
| **Blocked** | 🔴 | ≥1 Blocker offense, OR ≥1 Unclassified offense that triggers the severity failure rules |
| **Untested** | ⚪ | No CookStyle scan result for this unit + target |

This is the **CookStyle signal only**. Test Kitchen remains a separate badge
(passed/failed/partial/untested) — the two signals are never merged into one
verdict (see [dual-compatibility-signals.md](dual-compatibility-signals.md)).

Invariants:
- **Single source of truth.** Status (and complexity) are derived once, by
  `(offenses + resolved classification) → status`, and materialised. Every read
  path consumes the materialised value; the cop-analysis view and offense-group
  badges resolve from the same classification — the surfaces must never disagree
  (this incoherence is the bug this revision fixes).
- Unclassified offenses that severity-fail map to **Blocked** (conservative —
  anything that fails today stays red until a human classifies it).
- The boolean `passed` field is retained for backward-compat = `status not in
  {Blocked}` (Untested has `passed = false`/null per existing semantics). New code
  reads `status`; `passed` is a derived convenience.

The scan pipeline MUST derive status via classification (the resolver), not via
severity rules alone; severity rules remain only the Unclassified fallback.

### Complexity Weighting by Classification

Complexity scoring MUST weight offenses by their resolved classification, so an
advisory-only repo does not score as "high":

- **Blocker** offenses dominate the score (highest weight).
- **Review** offenses contribute a low weight (advisory).
- **Noise** offenses contribute ~0.
- **Unclassified** offenses keep the existing category weights (deprecation /
  correctness / manual-fix) as the fallback.

The double-counting that produces today's inflated scores (the same offense
counted as both a deprecation *and* a manual fix) is removed: each offense
contributes once, via its classification.

## Data Model

### Cop Classifications Table

```sql
CREATE TABLE cop_classifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    cop_name TEXT NOT NULL,
    target_chef_version TEXT NOT NULL,
    classification TEXT NOT NULL CHECK (classification IN ('blocker', 'review', 'noise')),
    reason TEXT,
    created_by TEXT,
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now(),
    UNIQUE (cop_name, target_chef_version)
);
```

### Custom Cop Definitions Table

```sql
CREATE TABLE custom_cop_definitions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    cop_name TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL,
    pattern_type TEXT NOT NULL CHECK (pattern_type IN ('regex', 'literal')),
    pattern TEXT NOT NULL,
    file_glob TEXT DEFAULT '*.rb',
    target_chef_version_min TEXT,
    removed_in TEXT,
    classification TEXT DEFAULT 'blocker' CHECK (classification IN ('blocker', 'review', 'noise')),
    enabled BOOLEAN DEFAULT true,
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now()
);
```

Custom cops are scanned at analysis time (alongside cookstyle) and produce offenses in the same format. They appear in the classification UI like any other cop.

## Curated Defaults

Shipped as compiled Go data (like `embeddedCopMappings`). Covers well-known behaviour-change cops not captured by `RemovedIn`:

| Cop | Default Classification | Reason |
|-----|----------------------|--------|
| `Lint/DeprecatedClassMethods` | Blocker (target ≥ 18) | `File.exists?` removed in Ruby 3 |
| `Chef/Deprecations/LogResourceNotifications` | Blocker (target ≥ 16) | Notifications silently stop firing |
| `Chef/Deprecations/WindowsFeatureServermanagercmd` | Blocker (target ≥ 14) | Install method silently ignored |
| `Chef/Deprecations/HWRPWithoutUnifiedTrue` | Review (target ≥ 18) | Required for Chef 18 unified mode |
| `Chef/Deprecations/ResourceWithoutUnifiedTrue` | Review (target ≥ 18) | Required for Chef 18 unified mode |
| `Chef/Deprecations/ChefSpecLegacyRunner` | Noise | Test tooling only |
| `Chef/Deprecations/FoodcriticTesting` | Noise | Test tooling only |
| `Chef/Deprecations/LibrarianChefspec` | Noise | Test tooling only |
| `Chef/Deprecations/Delivery` | Noise | CI tooling only |
| `Chef/Deprecations/DependsPoise` | Noise | Still works, just unmaintained |

## Data Provenance & Durability (decisions)

Records where each input comes from and how the system avoids rotting as
cookstyle evolves. Agreed 2026-07-01.

**Static (compiled Go, hand-maintained):** the `RemovedIn`/description mapping
table (`embeddedCopMappings`) and the curated exact + prefix defaults. **Dynamic
(runtime):** operator overrides + custom cops (DB), scan offences + severity
(from running the binary), failure rules (config store).

**cookstyle does NOT expose `RemovedIn`.** `cookstyle --show-cops` gives
`Enabled`/`Severity`/`Description`/`VersionAdded` — but `VersionAdded` is the
*gem* version, and the Chef-Client removal version exists only in free-text
`Description`. So the Chef removal signal is inherently curated; no machine source
supplies it. This is accepted, not a gap to close.

**Custom cops classify via severity, not the resolver.** A custom cop's DB
`classification` sets its offence *severity* (`blocker→error`, `review→warning`,
else `convention`); the resolver returns Unclassified for `Custom/*` (creating one
writes no override row), so it blocks via the severity/failure-rule path, not the
classification path.

Durability principles (avoid silent obsolescence — the named tables must not be
load-bearing for *unknown* cops):

1. **Department-default classification.** Unknown cops resolve by namespace, never
   to invisible-Unclassified: `Chef/Deprecations/*`, `Chef/Correctness/*` default
   to **Review**; cosmetic namespaces to **Noise**. The `RemovedIn`/curated exacts
   become *upgrades* (promote a specific cop to Blocker), not the primary signal —
   so a brand-new cop cookstyle ships is at-least-Review automatically.
2. **Live inventory + drift report.** `cookstyle --show-cops` is the authoritative
   list of cops *this* binary has (cached at startup / on upgrade). Cross-referenced
   against the static tables it surfaces **stale** entries (mapped cops the binary no
   longer emits) and **coverage gaps** (`Chef/*` cops with no classification), turning
   silent drift into an admin worklist. Generic-Ruby cops (Style/Layout/Lint/…) stay
   out of the default view — only ~30% (`Chef/*`) are migration-relevant.
3. **Seed static defaults into the DB.** The `RemovedIn`/curated tables seed
   editable DB rows (operator overrides already layer on top), so updating migration
   knowledge is a data edit, not a recompile. The Go tables are the initial seed only.

## API

### GET /api/v1/cookstyle/cops

Returns all known cops (from scan results + mapping + custom definitions) with their resolved classification for the given target version.

#### Query Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `target_chef_version` | string | Required. Determines classification resolution. |
| `source` | string | `server` or `git` — which results to aggregate |
| `classification` | string | Filter: `blocker`, `review`, `noise`, `unclassified` |
| `sort` | string | `cookbooks_affected`, `offence_count`, `cop_name` |
| `sort_dir` | string | `asc` or `desc` |

#### Response

```json
{
  "summary": {
    "blocker_cops": 8,
    "blocker_cookbooks": 47,
    "review_cops": 5,
    "review_cookbooks": 23,
    "noise_cops": 31,
    "unclassified_cops": 12
  },
  "data": [
    {
      "cop_name": "Lint/DeprecatedClassMethods",
      "description": "Checks for deprecated class method calls (File.exists? → File.exist?)",
      "category": "Lint",
      "severity": "warning",
      "classification": "blocker",
      "classification_source": "curated_default",
      "removed_in": null,
      "introduced_in": null,
      "migration_url": null,
      "cookbooks_affected": 34,
      "total_offences": 89,
      "auto_correctable_pct": 100,
      "unblocks": 12,
      "is_custom": false
    }
  ],
  "pagination": { "page": 1, "per_page": 50, "total_items": 56, "total_pages": 2 }
}
```

Fields:
- `classification_source` — how classification was determined: `operator_override`, `removed_in`, `curated_default`, `unclassified`
- `unblocks` — cookbooks that would pass if this cop alone were resolved (only meaningful for blockers)
- `auto_correctable_pct` — percentage of offences cookstyle can auto-fix
- `is_custom` — true if this is a custom-defined cop

### GET /api/v1/cookstyle/cops/:cop_name/cookbooks

Returns the list of cookbooks affected by a specific cop.

#### Response

```json
{
  "cop_name": "Lint/DeprecatedClassMethods",
  "data": [
    {
      "source": "server",
      "name": "example-cookbook",
      "version": "1.2.3",
      "organisation": "acme",
      "offence_count": 5,
      "auto_correctable": 5,
      "would_pass_without": true
    }
  ],
  "pagination": { ... }
}
```

### PUT /api/v1/cookstyle/cops/:cop_name/classification

Set or update the classification for a cop at a given target version.

#### Request Body

```json
{
  "target_chef_version": "18.5.0",
  "classification": "blocker",
  "reason": "File.exists? removed in Ruby 3, crashes at runtime on Chef 18+"
}
```

### CRUD /api/v1/cookstyle/custom-cops

Standard CRUD for custom cop definitions. See data model above.

## Custom Cop Scanning

Custom cops are simple pattern matchers run during analysis alongside cookstyle:

- **regex** — Ruby regex applied line-by-line to cookbook source files
- **literal** — exact string match (faster, simpler)

The `file_glob` field limits which files are scanned (default `*.rb`).

Example custom cop for `nil.=~` removal:

```json
{
  "cop_name": "Custom/Ruby3/NilRegexpMatch",
  "description": "The =~ method on nil was removed in Ruby 3. Code that relies on nil =~ /pattern/ returning nil will raise NoMethodError.",
  "pattern_type": "regex",
  "pattern": "=~",
  "file_glob": "*.rb",
  "target_chef_version_min": "18.0",
  "removed_in": "18.0",
  "classification": "blocker"
}
```

Custom cop offenses are stored in the same `offences` JSONB as cookstyle results, with `cop_name` prefixed `Custom/` to distinguish them.

## Frontend

### Cop Analysis Page

New tab on the Remediation page: **Priority | Cop Analysis**

(Replaces the previously planned "CookStyle Violations" flat-list tab.)

#### Layout

1. **Summary cards** — Blocker cops / Review cops / Noise cops / Unclassified, with cookbook counts
2. **Classification filter** — toggle which levels to show (default: Blockers only)
3. **Cop table** — one row per cop, grouped by classification level:
   - Cop name (with link to drill-down)
   - Classification badge (🔴/🟠/⚪/❓) with source tooltip
   - `RemovedIn` version (if known)
   - Severity (from cookstyle)
   - Cookbooks affected (count)
   - Total offences
   - Auto-correctable %
   - Unblocks count (blocker cops only)
4. **Drill-down panel** — click a cop → slide-out or expand showing affected cookbooks with links to remediation

#### Interactions

- Click classification badge → dropdown to reclassify (with reason field)
- Reclassification takes effect immediately (triggers re-evaluation of affected cookbooks)
- "Show unclassified" filter → review cops that need classification

### Cop Management Page

Admin page: **Admin → CookStyle → Cop Classification**

Three sections:

1. **Classifications** — searchable list of **all** known cops (curated defaults +
   `RemovedIn` mappings + scanned + custom), with a target-version selector, the
   resolved classification + its source (operator_override / removed_in /
   curated_default / unclassified), and per-cop override (with reason). Curated
   defaults are visible as the seed; overrides layer on top. This is the missing
   surface — today reclassification is only reachable inline from the Cop Analysis
   drill-down.
2. **Custom Cops** — CRUD for custom cop definitions (name, pattern, target version, classification)
3. **Fallback rules** — the existing severity-based "Failure Rules" grid, reframed
   and labelled as applying **only to unclassified cops** (de-emphasised / below
   classification). Not removed — it remains the Unclassified fallback.

### Updated Cookbook/Git Repo Detail Views

Existing remediation detail pages are reorganised to answer "what must I fix?"
at a glance:

- **Verdict headline** at the top — plain-language bottom line derived from the
  CookStyle rollup status, e.g. "✓ No blockers for Chef 19 — 1 item to review
  before migrating" or "🔴 2 blockers must be fixed for Chef 19". This is the first
  thing the user reads.
- **Rollup status badge** in the header (🟢/🟠/🔴) replacing the binary
  CookStyle Passed/Failed badge, consistent with the list and summary card.
- **Collapsible category sections** — offense groups grouped under **Blockers /
  Review / Noise / Unclassified**, each section header showing a count
  (e.g. "Blockers (0)", "Review (1)"). Blockers expanded by default; the rest
  collapsed and visually de-emphasised. An explicit "Blockers (0)" header reads
  as reassurance, not breakage.
- Within each section, offense groups keep their classification badge,
  `RemovedIn` inline where available, and per-cop counts.
- The classification filter is retained; an empty result shows a real
  empty-state ("No blocker-level cops for this target — items below are
  advisory"), never a blank list.

### Updated List & Summary Surfaces

The git-repo / cookbook lists and the per-item summary cards show the **CookStyle
rollup status** badge (🟢 Ready / 🟠 Needs review / 🔴 Blocked / ⚪ Untested) instead
of a binary pass/fail, sourced from the same derivation as the detail header. The
summary card replaces the bare "complexity N" number with the classification-
weighted score plus its label (e.g. "low") so the figure is interpretable.

### Updated Remediation Priority View

The existing priority table gains:

- "Blocker offences" column — count of offences from Blocker-classified cops
- Complexity scoring updated to weight Blocker cops higher
- Filter: "Only show cookbooks with blockers"

## Re-evaluation & Propagation

Any **criteria change** MUST trigger a scoped recompute through the derivation
graph. Criteria changes are: operator override (PUT/DELETE classification),
failure-rule edit, custom-cop add/edit, curated-default update (app upgrade), and
target-version addition.

The recompute closure (invalidate only what is downstream — see
`plans/cookstyle-status-consistency.md` for the full table):

1. Re-derive **status + weighted complexity** for every cookstyle result
   containing the affected cop(s), per affected target — re-resolution only, no
   rescan (custom-cop *definition* changes are the exception: they require a
   rescan because they change which offenses exist).
2. Update `status`/`passed` + `complexity_score` in
   `server_cookbook_cookstyle_results` / `git_repo_cookstyle_results`.
3. Recompute git-repo compatibility status (CS ⊕ TK).
4. Recompute **readiness** for nodes whose run-list includes an affected cookbook.
   A run-list change alone re-rolls only that node's readiness; a readiness-config
   change re-rolls all nodes.
5. Record a criteria-change **audit event** (who/what/when) so a step in any trend
   is explainable.
6. Emit a WebSocket event so open UI pages refresh.

Reclassification is cheap (re-resolve ~tens of thousands of results in memory; no
rescan), so full current-state recompute on every criteria change is affordable.

### History

Past trend points are **not** retroactively recomputed — the raw offense-level
inputs were never retained (snapshots store rolled-up aggregates only), so past
points are unrecoverable and stay frozen. Going forward, a change-deduped per-scan
offense **fingerprint** history (cop_name + count + severity + correctable per
result per scan; appended only when it differs from the prior scan) makes trends
recomputable under current criteria for data captured after it ships. See
[enriched-metric-snapshots.md](enriched-metric-snapshots.md).

## Migration from Failure Rules

The existing failure rules system (`cookstyle_failure_preset`, `cookstyle_failure_rules`) continues to function as the fallback for unclassified cops. Over time, as operators classify cops, the severity-based rules become less relevant.

No breaking change: existing behaviour is preserved. The classification system is additive.

## Performance

- Cop aggregation query groups offences by `cop_name` across all results for a target version — same cardinality as the existing violations endpoint
- Classification lookup is O(1) per cop (in-memory map from DB + curated defaults)
- Custom cop scanning adds per-file regex matching during analysis — bounded by file count × pattern count
- Re-evaluation on classification change is bounded by cookbooks with that cop in their offences

## Related

- [cookstyle-failure-rules.md](cookstyle-failure-rules.md) — Existing severity-based pass/fail (becomes fallback)
- [cookstyle-violations-browser.md](cookstyle-violations-browser.md) — Superseded by this spec's Cop Analysis view
- [analysis.md](analysis.md) — CookStyle invocation and output parsing (extended for custom cops)
- `internal/remediation/copmapping.go` — Embedded cop mapping with `RemovedIn` data
