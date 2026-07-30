# CookStyle Violations Browser — Component Specification

> **⚠️ SUPERSEDED** — This spec's flat-list approach has been replaced by the
> cop-centric classification system in
> [cop-classification.md](cop-classification.md). The API endpoint (chunk 1)
> remains useful as a data source; the frontend tab (chunk 2) will be replaced
> by the Cop Analysis view.

> **TL;DR** — A filterable list view of cookbooks with CookStyle violations, with filters for namespace (category), severity, cop name, and pass/fail status. Reuses existing per-cookbook cookstyle result data; no new aggregation endpoint required.

## Overview

Operators need to answer questions like "which cookbooks have deprecation errors?" or "how many cookbooks trigger Chef/Correctness/InvalidDefaultAction?" Currently this requires clicking into each cookbook's detail page individually. This specification adds a violations browser that presents a flat, filterable cookbook list with offense summary counts, linked to existing remediation detail pages.

## Design Decision

A flat filterable list was chosen over a collapsible tree (category → severity → cop → cookbooks) because:

- Reuses existing per-cookbook queries — no new server-side aggregation endpoint
- Offense data is small per cookbook (already stored as JSONB) — filtering is fast client-side
- Avoids deep nesting (4 namespaces × 5 severities × ~50 cops × N cookbooks)
- The existing remediation pages already provide cop-grouped detail within a cookbook

## Data Source

The view consumes the same data as existing cookbook/git-repo detail pages:

- `server_cookbook_cookstyle_results` — per-org cookbook scan results
- `git_repo_cookstyle_results` — per-git-repo scan results

Each row contains:

- `passed` (boolean) — back-compat convenience: `cookstyle_status != blocked`
- `offence_count`, `deprecation_count`, `correctness_count` — pre-computed counts
- `offences` (JSONB) — the persisted offence array. Shape not restated here. Note it is the *enriched* shape, not raw CookStyle output: it carries `correctable` (the cop's static capability) and not `corrected`, which is only meaningful for a correcting run.
- `error_message` — non-null when scan was inconclusive

## API

### New Endpoint

`GET /api/v1/cookstyle/violations`

Returns a paginated list of cookbook cookstyle results with offense summaries.

#### Query Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `source` | string | `server` (default) or `git` — which result table to query |
| `target_chef_version` | string | Filter by target Chef version (required) |
| `status` | string | `failed`, `passed`, `error`, or omit for all |
| `namespace` | string | Filter: only include results with offenses matching this namespace prefix (e.g. `Chef/Deprecations/`) |
| `severity` | string | Filter: only include results with offenses at this severity level |
| `cop` | string | Filter: only include results with offenses from this cop name |
| `page` | int | Page number (default 1) |
| `per_page` | int | Page size (default 50, max 200) |
| `sort` | string | `name` (default), `offence_count`, `deprecation_count` |
| `sort_dir` | string | `asc` (default) or `desc` |

#### Response

```json
{
  "items": [
    {
      "source": "server",
      "name": "example-cookbook",
      "version": "1.2.3",
      "organisation": "example-org",
      "target_chef_version": "18.0",
      "passed": false,
      "offence_count": 12,
      "deprecation_count": 5,
      "correctness_count": 2,
      "scanned_at": "2026-06-20T10:00:00Z",
      "namespace_counts": {
        "Chef/Deprecations/": 5,
        "Chef/Correctness/": 2,
        "Chef/Style/": 3,
        "Chef/Modernize/": 2
      },
      "severity_counts": {
        "warning": 4,
        "error": 7,
        "fatal": 1
      },
      "top_cops": ["Chef/Deprecations/NodeSet", "Chef/Correctness/InvalidDefaultAction"]
    }
  ],
  "total": 142,
  "page": 1,
  "per_page": 50
}
```

### Implementation Strategy

The `namespace_counts`, `severity_counts`, and `top_cops` fields are derived **server-side** by deserialising the stored `offences` JSONB per row. This is lightweight (sub-millisecond per row for typical offense arrays of 5–50 items) and avoids sending the full offense payload to the client.

Server-side filtering by namespace/severity/cop uses SQL `WHERE EXISTS` with JSONB containment or array element checks, keeping the query efficient without a full table scan of JSONB contents:

```sql
WHERE EXISTS (
  SELECT 1 FROM jsonb_array_elements(offences) AS f,
               jsonb_array_elements(f->'offenses') AS o
  WHERE o->>'cop_name' LIKE $1 || '%'
)
```

For `target_chef_version` (required), the existing index on `target_chef_version` keeps the base query fast.

## Frontend

### Page Location

Added as a **"CookStyle Violations"** tab on the existing Remediation page (`/remediation`). The existing priority view becomes the default tab; the violations browser is the second tab. A source toggle (Server Cookbooks / Git Repos) switches between the two result tables within the tab. Filter state is persisted in URL query parameters.

### Layout

1. **Filter bar** — horizontal row of filter controls:
   - Target version dropdown (required, populated from configured target versions)
   - Source toggle: Server Cookbooks / Git Repos
   - Namespace multi-select: Chef/Deprecations, Chef/Correctness, Chef/Style, Chef/Modernize
   - Severity multi-select: convention, refactor, warning, error, fatal
   - Cop name typeahead (free text, matches `cop_name` prefix)
   - Status filter: All / Failed / Passed / Scan Error

2. **Results table** — sortable columns:
   - Cookbook/Repo Name (link to detail page)
   - Version / Organisation
   - Status badge (Passed/Failed/Error)
   - Offence Count
   - Deprecation Count
   - Top Cops (first 2–3, truncated with "+N more")

3. **Pagination** — standard server-side pagination

### Interaction

- Changing any filter immediately reloads the list (debounced for typeahead)
- Clicking a cookbook name navigates to the existing detail page
- A "View Remediation" link on each row navigates to the existing remediation page with `target_chef_version` pre-set
- Filter state is persisted in URL query parameters for shareability

## Performance

- Typical dataset: hundreds to low-thousands of cookstyle results per target version
- The `target_chef_version` filter (required) indexes down to a manageable subset
- JSONB filtering is lightweight for this cardinality — no materialised views needed
- Response includes summary counts, not full offense arrays — payload stays small (~200 bytes per item)
- Server-side pagination caps response size

## Edge Cases

- No target versions configured: show a message linking to the target versions admin page
- No results for selected filters: empty state with "No violations match the current filters"
- Scan errors (non-null `error_message`): shown with an "Error" badge, no offense counts
- Cookbooks with zero offenses but `passed=false` (legacy rows predating the
  classification-derived verdict): shown as "Failed" with 0 offenses

## Related

- [cop-classification.md](cop-classification.md) — Classification is the only source of a verdict
- [analysis.md](analysis.md) — CookStyle invocation and output parsing
- [server-side-pagination.md](server-side-pagination.md) — Pagination conventions
