# Cop Classification & Analysis — Component Specification

> **TL;DR** — Classify CookStyle cops by their actual migration impact
> (Blocker / Review / Noise) so the signal is a *reliable* indicator: **reds mean
> "we know it must be fixed", Review means "operator must decide", Noise means
> "provably harmless".** No bucket presents a guess as knowledge.

> **Reliability model (v2 — trustworthy reds).** This revises the earlier
> auto-seed/severity-fallback model. Two invariants drive it:
> 1. **Single target.** There is exactly one active target Chef version. Cops are
>    classified per-cop, not per-target. (The old per-target machinery is removed
>    — see [cookstyle-reliability plan].)
> 2. **Asymmetric confidence.** A wrong Blocker wastes effort (visible,
>    recoverable); a wrong Noise *hides a real blocker* (silent, dangerous). So
>    Noise needs a higher bar than Blocker, and anything uncertain falls to
>    **Review**, never to Noise and never to a severity-derived red.

## Overview

CookStyle severity levels (`convention`, `refactor`, `warning`, `error`, `fatal`) do not reliably indicate whether a cop is a migration blocker. `Chef/Deprecations/NodeSet` (removed in Chef 14 — hard crash) and `Chef/Deprecations/DependsPoise` (unmaintained but still works) both fire at `warning`. Severity is therefore never a source of a migration verdict — it is the signal this feature exists to *replace*.

This specification provides:

1. **Cop classification** — per-cop Blocker / Review / Noise for the active target
2. **Trustworthy Blocker sources** — verified removal knowledge, operator confirmation, or a custom/manual cop (a blocker by intent)
3. **Review as a worklist** — the honest default for migration-relevant-but-unproven cops; the operator triages each into Blocker-or-clear
4. **Structural Noise only** — a cop is Noise only for a positive structural reason (cosmetic RuboCop department, or test/CI-tooling-only), never as a fallback
5. **Custom cop definitions** — patterns not yet in cookstyle (e.g. `nil.=~` removal in Ruby 3); these resolve as Blocker by intent
6. **Curation linter** — CI cross-check that curated removal data agrees with the shipped cop descriptions and flags stale entries
7. **Cop analysis view** — per-cop aggregation showing affected cookbooks, fix effort, and classification with visible provenance

## Classification Levels

| Level | Meaning | Visual | Rollup |
|-------|---------|--------|--------|
| **Blocker** | We *know* it must be fixed for the target | 🔴 | Blocks |
| **Review** | Migration-relevant but unproven — operator must decide (absorbs the old "Unclassified") | 🟠 | Does not block; a worklist item |
| **Noise** | *Provably* harmless (cosmetic department or test/CI tooling only) | ⚪ | Does not block |

There is no separate "Unclassified" level: an unresolved cop *is* a Review item ("not yet reviewed"). A cop file that will not parse (`fatal`) is surfaced by a **separate "won't parse — fix first" flag**, not folded into a classification Blocker.

## Classification Resolution

For a given cop (against the single active target), classification resolves in priority order. **Every source is a positive statement of knowledge; the default is Review, never a severity-derived red.**

1. **Operator override** (stored in DB) — highest priority; the operator's confirmed verdict.
2. **Custom/manual cop** → **Blocker**. A cop hand-defined in a migration tool is a blocker by intent.
3. **Verified removal** → **Blocker**. A curated `RemovedIn` for the cop (`RemovedIn ≤ target`). Curated removal is human-asserted knowledge; the linter cross-checks it against the cop description and flags disagreements/staleness, but does not auto-demote.
4. **Structural Noise** → **Noise**, only from a positive structural reason (longest match wins):
   - Cosmetic RuboCop departments: `Style/`, `Layout/`, `Chef/Style/` — non-functional *by RuboCop's own taxonomy*.
   - Test/CI-tooling-only cops (ChefSpec, Foodcritic, Delivery, Librarian/Berks) — cannot affect production convergence.
5. **Review** (default) — everything else, including all `Chef/Deprecations/*`, `Chef/Correctness/*`, `Lint/*`, and any cop with no positive Blocker/Noise reason. Honest "unproven — operator decides".

Removed from the old model: the `RemovedIn`-auto-seed-as-primary, the curated *exact/prefix classification* defaults that guessed Review/Noise for whole namespaces without a structural reason, and the **Unclassified→severity→Blocked fallback** entirely.

### Pass/Fail Determination

A cookbook is **Blocked** iff it has at least one **Blocker** offense. Review and Noise never block. Severity plays no part in the verdict.

Separately, a cookbook that **won't parse** (a `fatal`/parse-failure offense) is flagged "won't parse — fix first" — a data-quality signal distinct from the migration classification, surfaced but not counted as a classification Blocker.

### CookStyle Rollup Status

Binary pass/fail hides advisory work: a repo whose only issues are Review-level
cops is neither "ready" nor "broken". The classification-derived **CookStyle
rollup status** is the canonical per-cookbook / per-repo / per-node verdict used by
every surface (list, summary card, dashboard compatibility cards, detail header,
node readiness, exports, trends), replacing the old
compatible/incompatible/passed/failed wording:

| Status | Visual | Condition |
|--------|--------|-----------|
| **Ready** | 🟢 | Scan exists; no Blocker and no Review offenses (clean, or only Noise) |
| **Needs review** | 🟠 | No Blocker, but ≥1 Review offense |
| **Blocked** | 🔴 | ≥1 Blocker offense (verified removal, operator, or custom cop). Severity is never a source. |
| **Untested** | ⚪ | No CookStyle scan result for this unit |

This is the **CookStyle signal only**. Test Kitchen remains a separate badge
(passed/failed/partial/untested) — the two signals are never merged into one
verdict (see [dual-compatibility-signals.md](dual-compatibility-signals.md)).

Invariants:
- **Single source of truth.** Status (and complexity) are derived once, by
  `(offenses + resolved classification) → status`, and materialised. Every read
  path consumes the materialised value; the cop-analysis view and offense-group
  badges resolve from the same classification — the surfaces must never disagree.
- **Only knowledge produces red.** Blocked requires a Blocker offense from a
  positive source; there is no severity-derived red. An operator who does nothing
  sees an honest "N items to review", not a false alarm.
- The boolean `passed` field is retained for backward-compat = `status not in
  {Blocked}` (Untested has `passed = false`/null per existing semantics). New code
  reads `status`; `passed` is a derived convenience.
- A "won't parse" (fatal/parse-failure) flag is carried alongside status, not
  inside it.

### Complexity Weighting by Classification

Complexity scoring MUST weight offenses by their resolved classification, so an
advisory-only repo does not score as "high":

- **Blocker** offenses dominate the score (highest weight).
- **Review** offenses contribute a low weight (advisory).
- **Noise** offenses contribute ~0.

Each offense contributes exactly once, via its classification (the old
double-counting — the same offense counted as both a deprecation *and* a manual
fix — is removed). With no Unclassified level, there is no severity-category
fallback weight.

## Data Model

### Cop Classifications Table

Operator overrides are keyed by `cop_name` only — there is a single active target
(the per-target column and key are removed):

```sql
CREATE TABLE cop_classifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    cop_name TEXT NOT NULL UNIQUE,
    classification TEXT NOT NULL CHECK (classification IN ('blocker', 'review', 'noise')),
    reason TEXT,
    created_by TEXT,
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now()
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

Custom cops are scanned at analysis time (alongside cookstyle) and produce offenses in the same format. They resolve as **Blocker** (blocker by intent) and appear in the classification UI like any other cop.

## Blocker & Noise Reference Data

Shipped as compiled Go data. Two disjoint kinds, each with a *positive reason*:

**Verified removals → Blocker** (`RemovedIn` for a specific cop). Human-curated
Chef-Client removal versions; the curation linter cross-checks these against the
shipped cop descriptions:

| Cop | RemovedIn | Reason |
|-----|-----------|--------|
| `Lint/DeprecatedClassMethods` | Ruby 3 / Chef 18 | `File.exists?` removed |
| `Chef/Deprecations/NodeSet` | 14 | `node.set` removed |
| `Chef/Deprecations/WindowsFeatureServermanagercmd` | 15 | install method removed |
| …(see `embeddedCopMappings`, linter-guarded) | | |

**Structural Noise** (positive reason, not a per-cop guess):
- Cosmetic RuboCop departments `Style/`, `Layout/`, `Chef/Style/` — cosmetic by RuboCop taxonomy.
- Test/CI-tooling-only cops (ChefSpec / Foodcritic / Delivery / Librarian).

Everything not covered by these is **Review** by default — no curated
Review/Noise *guesses* for whole namespaces. (Cops like `HWRPWithoutUnifiedTrue`
are simply Review, which is where the default already puts every unproven
`Chef/Deprecations/*` cop.)

## Data Provenance & Durability (decisions)

Records where each input comes from and how the signal stays reliable as cookstyle
evolves. Model agreed 2026-07-03 (trustworthy reds; supersedes the 2026-07-01
auto-seed/DB-seed decisions).

**Static (compiled Go, hand-maintained):** the `RemovedIn` verified-removal table
and the structural-Noise rules. **Dynamic (runtime):** operator overrides + custom
cops (DB), scan offences (from running the binary). **Removed:** the config-store
severity *failure rules* no longer feed the verdict (severity is not a source);
the DB-seeded defaults table (chunk 3) is abandoned.

**cookstyle does NOT reliably expose removal versions.** `--show-cops` gives
`Enabled`/`Description`/`VersionAdded` (the *gem* version, not the Chef-Client
removal), and does not even print default `Severity`. The Chef removal version
exists only in free-text `Description`, and a 2026-07-03 spike showed it is only
~30% cleanly parseable (a third absent, a fifth a deprecation-vs-removal trap). So
removal knowledge stays **curated** — but validated by a linter, not assumed.

**Custom cops are Blockers by intent.** A cop hand-defined in a migration
assessment tool is, by the act of defining it, a declared blocker. It resolves as
Blocker directly (no severity side-channel).

Durability principles:

1. **Review is the honest default.** Any migration-relevant cop we can't *prove*
   is a Blocker or *structurally* show is Noise resolves to Review — a worklist,
   not a guess dressed as a verdict. A brand-new `Chef/*` cop is Review
   automatically; the operator triages it.
2. **Live inventory + drift report.** `cookstyle --show-cops` is the authoritative
   list of cops *this* binary has (cached at startup / on upgrade). Cross-referenced
   against the static tables it surfaces **stale** entries (curated cops the binary no
   longer emits) and **coverage gaps**, turning silent drift into an admin worklist.
3. **Curation linter (CI).** Each curated `RemovedIn` is cross-checked against the
   shipped cop description; disagreements and stale entries (cop absent from the
   binary) fail CI. This is the durability mechanism for the curated Blocker set —
   catching rot at the source instead of moving the data into an editable DB.

## API

> **Single target:** endpoints below no longer take a `target_chef_version`
> parameter or key — resolution is against the one active target. Any remaining
> `target_chef_version` request/response fields are removed. The `cop-defaults`
> endpoints from chunk 3 do not exist.

### GET /api/v1/cookstyle/cops

Returns all known cops (from scan results + mapping + custom definitions) with their resolved classification.

#### Query Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `source` | string | `server` or `git` — which results to aggregate |
| `classification` | string | Filter: `blocker`, `review`, `noise` |
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
    "unclassified_cops": 0
  },
  "data": [
    {
      "cop_name": "Lint/DeprecatedClassMethods",
      "description": "Checks for deprecated class method calls (File.exists? → File.exist?)",
      "category": "Lint",
      "severity": "warning",
      "classification": "blocker",
      "classification_source": "verified_removal",
      "removed_in": "18.0",
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
- `classification_source` — how classification was determined: `operator_override`, `custom_cop`, `verified_removal`, `structural_noise`, `review_default`
- `unblocks` — cookbooks that would pass if this cop alone were resolved (only meaningful for blockers)
- `auto_correctable_pct` — percentage of offences cookstyle can auto-fix
- `is_custom` — true if this is a custom-defined cop
- `unclassified_cops` (summary) — retained for backward compatibility; always `0` (there is no Unclassified level — an unresolved cop is a Review item)

### GET /api/v1/cookstyle/cops/:cop_name/cookbooks

Returns the cookbooks affected by a cop. The response **shape depends on `source`**
because server and git cookbooks have different natural grains (see Cop Analysis
Page):

- **`source=git`** (and the legacy no-source list) — flat, one row per
  `{name, version, org}` (`copCookbookItem`): `source, name, version,
  organisation?, offence_count, auto_correctable, would_pass_without`.
- **`source=server`** — grouped by cookbook name and paginated **by name**
  (`copCookbookGroup`), with `grouped: true`, `version_count`, summed
  `offence_count`/`auto_correctable`, `would_pass_without` (true only if resolving
  the cop clears **every** version), and the per-`{version, org}` detail nested
  under `versions[]`. The pagination total equals the distinct-name count, so it
  matches the header `cookbooks_affected` for the same cop+target (invariant below).

Normative shapes: the Go types `copCookbookItem` / `copCookbookGroup` and their
response wrappers in `internal/webapi/handle_cookstyle_cops.go`.

### PUT /api/v1/cookstyle/cops/:cop_name/classification

Set or update the operator classification for a cop. Single active target — no
target is carried in the body; the active target drives only the re-evaluation
propagation closure.

#### Request Body

```json
{
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

Tabs on the Remediation page: **Priority | Cop Analysis (Server) | Cop Analysis (Git)**

(Replaces the previously planned "CookStyle Violations" flat-list tab, and the
earlier single "Cop Analysis" tab with an All-sources/Server/Git dropdown.)

**Per-source grain.** Server and git cookbooks have different natural grains, so
each has its own tab with `source` fixed (no dropdown): a **server** cookbook has
real multiplicity (many versions across orgs) — headline and drill-down both count
**distinct name**, grouped by name and expandable to `{version, org}` detail; a
**git** repo is **1:1** with a cookbook, so its drill-down is the flat repo list.

**Invariant (shared record selection):** within a tab, the header
`cookbooks_affected` for a cop **equals** its drill-down pagination total. Fixing
`source` per tab is what makes this hold — it removes the old All-sources
double-count (a name in both sources was counted once per source). The legacy deep
link `?tab=cop-analysis` (optionally `&source=git`) migrates to the matching tab.

#### Layout

1. **Summary cards** — Blocker cops / Review cops / Noise cops, with cookbook counts
2. **Classification filter** — toggle which levels to show (default: Blockers only)
3. **Cop table** — one row per cop:
   - Cop name (with link to drill-down)
   - Classification badge (🔴/🟠/⚪/❓) with source tooltip
   - `RemovedIn` version (if known)
   - Cookbooks affected (count)
   - Total offences
   - Auto-correctable %
   - Unblocks count (blocker cops only)
4. **Drill-down panel** — click a cop → expand showing affected cookbooks,
   **paginated** (total surfaced). Server rows group by name and expand to
   version/org detail; git rows are the flat repo list. The panel **resets** when
   the classification filter, sort, or target version changes.

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
custom-cop add/edit, curated-mapping update (app upgrade), and a change to the
single active target version.

The recompute closure (invalidate only what is downstream — see
`plans/cookstyle-status-consistency.md` for the full table):

1. Re-derive **status + weighted complexity** for every cookstyle result
   containing the affected cop(s) — re-resolution only, no rescan (custom-cop
   *definition* changes are the exception: they require a rescan because they
   change which offenses exist).
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

## Retirement of Failure Rules

The severity-based failure-rules system (`cookstyle_failure_preset`,
`cookstyle_failure_rules`) is **retired as a source of the verdict** — severity is
the signal this feature exists to distrust, so it no longer produces reds. The
only migration verdict is classification (Blocker/Review/Noise). A `fatal`/parse
failure is surfaced by the separate "won't parse — fix first" flag. This is a
deliberate behaviour change from the additive-fallback model.

## Performance

- Cop aggregation query groups offences by `cop_name` across all results — same cardinality as before
- Classification lookup is O(1) per cop (in-memory map from DB overrides + curated tables)
- Custom cop scanning adds per-file regex matching during analysis — bounded by file count × pattern count
- Re-evaluation on classification change is bounded by cookbooks with that cop in their offences

## Related

- [cookstyle-failure-rules.md](cookstyle-failure-rules.md) — Severity pass/fail, now **retired** as a verdict source
- [cookstyle-violations-browser.md](cookstyle-violations-browser.md) — Superseded by this spec's Cop Analysis view
- [analysis.md](analysis.md) — CookStyle invocation and output parsing (extended for custom cops)
- `internal/remediation/copmapping.go` — Embedded cop mapping with `RemovedIn` data
