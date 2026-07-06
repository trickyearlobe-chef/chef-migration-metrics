# Node Tags — Component Specification

> Ingest Chef node tags, store them per node snapshot, and expose them as a node-list filter dimension: three pinned migration-phase tags (`prepare`, `upgrade`, `rollback`) plus searchable long-tail discovery of any tag ever collected.

## Overview

Chef node tags are a normal node attribute (`node['tags']`) — an array of strings set via `knife tag`, the `tag`/`tagged?` recipe helpers, or `chef_tag`. This tool does not currently fetch, store, or filter on them. This component adds tags end-to-end so operators can slice the node list by migration phase and by any other tag present in the fleet.

Modelled on the existing **roles** dimension: roles are also an array on `node_snapshots`, already served as a distinct-value facet and matched with array-overlap filtering. Tags are "roles, but for the `tags` array."

### Why

The migration workflow tags nodes by phase — `prepare`, `upgrade`, `rollback`. Operators need to filter the node list to a phase cohort, and to discover/filter on other operational tags without knowing the full set in advance.

### Data Source

- Chef node attribute `normal.tags` — an array of strings. Fetched via Chef partial search alongside the other node attributes the collector already requests.

### Invariants

- **`null` vs `[]`**: a node that never had tags set returns `null` from Chef, not `[]`. Ingestion MUST coalesce `null` to an empty array. An empty array means "collected, no tags"; it is never null in storage.
- **Case-sensitive**: tag values are stored and matched exactly as Chef returns them. No normalisation (no lowercasing, no trimming) — matches Chef search semantics.
- **De-duplication**: a node's stored tag set is de-duplicated (Chef may present duplicates); order is not significant.
- **Reflects collected snapshots only**: the tag catalogue and all counts derive from ingested `node_snapshots`, not from a Chef-server-wide query. An absent tag means "not seen in any collected snapshot", which is not the same as "not present in Chef." This distinction MUST be stated in the UI (see Facet) so a short list is not read as "the fleet has no tags."

## Ingestion

The collector adds `tags` to the node attributes it fetches from Chef and writes the coalesced, de-duplicated array to each node snapshot.

- Attribute fetch: add `tags` to the node partial-search attribute set. Contract: the collector's Chef attribute list (`internal/chefapi/client.go`, `NodeSearchAttributes`) and node accessor (`NodeData`).
- Snapshot assembly: populate the new tags field in the snapshot insert params (`internal/collector/collector.go`, alongside `custom_attributes`).

Tags are stored as a dedicated column, NOT folded into the `custom_attributes` CMDB JSONB — tags are a first-class filter dimension needing fast array-overlap and distinct-facet queries, and mixing them with ownership metadata muddies both.

## Storage

A dedicated tags array column on `node_snapshots` with a GIN index for array-containment/overlap queries.

- New migration under `migrations/` adding the column + GIN index. Column shape (`TEXT[]`) and the snapshot row contract are owned by the datastore layer (`internal/datastore`, `node_snapshots` insert params); this spec pins the invariants above, not the DDL.

## Filtering

Adds `tags` as a node-list filter. Multi-select with **OR** semantics: a node matches if its tag array overlaps any selected tag (array-overlap, not contains-all).

- **Semantics**: OR (union). Rationale — the three pinned tags are migration phases and a node is effectively in one phase at a time, so "`prepare` or `rollback`" is the meaningful query; "`prepare` and `rollback`" is not. AND (intersection) semantics are out of scope for v1; revisit only if intersecting phase tags with non-phase tags (`prod` AND `upgrade`) becomes a real need.
- **API**: new `tags` query parameter on `GET /api/v1/nodes` (repeatable/multi-value), parsed in the node filter builder (`nodeSnapshotFilterFromValues`, `internal/webapi/handle_nodes.go`). New `Tags []string` field on `NodeSnapshotFilter` (`internal/datastore/node_snapshot_filter.go`) with array-overlap SQL, mirroring the roles multi-select path.

### Tags vs migration_state

Node tags and the tool's existing `migration_state` dimension are independent and MUST NOT be assumed to agree:

- `tags` is ground truth **observed on the node** in Chef.
- `migration_state` is what the **tool tracks/derives**.

Keep them as separate filters. A mismatch (node tagged `rollback` but tracked as `upgrade`) is a useful **signal**, not a bug to reconcile.

## Facet (Filter Options)

New distinct-value endpoint `GET /api/v1/filters/tags`, following the existing filter-option pattern (`internal/webapi/handle_filters.go`, registered in `router.go`). Backed by the existing distinct-over-array facet query used for roles (`ListDistinctNodeValues` / roles-style distinct with `DistinctValueOpts{SearchPrefix, Limit}`).

- **Query parameters**: `organisation` (optional, scopes to one or more orgs), `q`/prefix (optional, server-side prefix filter for typeahead), and a server-enforced result cap.
- **Scalability**: the endpoint returns a bounded, prefix-filtered, count-ranked page — never the full set. Cardinality (3 tags or 3,000) does not change client behaviour; the list is bounded server-side.
- **Response**: standard `{"data": [...]}` envelope. See [web-api-filters.md](web-api-filters.md) for the shape.

## Frontend

Tags filter on the node list (`frontend/src/pages/NodesPage.tsx`), composed from existing reusable filter components.

- **Pinned quick toggles**: `prepare`, `upgrade`, `rollback` always visible as one-click OR toggles above the search.
- **Searchable long tail**: a typeahead (`FilterTypeAhead`) backed by `filters/tags` prefix search for any other tag; selected tags shown as chips.
- **Empty/short list caption**: when the catalogue is empty or short, show a caption clarifying it reflects collected snapshots, not a Chef-wide tag list (see Invariants).
- **API client**: add `fetchFilterTags` and a `tags` field on the node filter query type (`frontend/src/api.ts`).

The three pinned values are UI defaults, not hardcoded server logic — they are ordinary entries in the dynamically-discovered catalogue, pinned for convenience.

## Exports

Tags appear in the node export. The export reuses the same node datastore query and filter set (including the new `tags` filter), so a filtered export already scopes correctly; this section covers the added output column.

- Add a `tags` column to the node export's single source of truth for headers/keys (`nodeExportColumns`, `internal/webapi/export_nodes.go`). The exported snapshot row must carry the tags array (`internal/datastore/node_snapshot_export.go`).
- **CSV**: the array is joined into one cell with the same delimiter convention used for the other array columns in the export (e.g. `run_list`/roles). An empty array renders as an empty cell.
- **JSON**: emitted as a JSON array of strings (`[]` when none).
- Exports are synchronous streaming downloads over the filtered, keyset-paginated query; tags ride along per row with no separate lookup.

## Acceptance Criteria

- A node's `normal.tags` from Chef is collected and stored; `null` is coalesced to `[]`; duplicates removed.
- `GET /api/v1/nodes?tags=upgrade` returns exactly the nodes whose tag array contains `upgrade`; multiple `tags` params union (OR).
- `GET /api/v1/filters/tags` returns distinct collected tags, prefix-filterable via `q`, capped, org-scopable.
- Node list shows `prepare`/`upgrade`/`rollback` as pinned toggles and a working typeahead for other tags.
- A fleet with hundreds of distinct tags does not degrade the filter UI (bounded server responses).
- Tags and `migration_state` remain independent filters.
- The `nodes` export (CSV and JSON) includes a `tags` column; it honours the `tags` filter and array-joins (CSV) / array-emits (JSON) each node's tags, empty when none.

## Related Specifications

- [Roles](roles.md) — the array-dimension pattern this mirrors (distinct facet, array-overlap filter, node blast-radius).
- [Web API — Filters](web-api-filters.md) — filter-option endpoint conventions; `filters/tags` documented there.
- [Web API — Nodes](web-api-nodes.md) — node-list query parameters and response shape.
- [Data Collection](data-collection.md) — node attribute collection from Chef.
- [Datastore](datastore.md) — `node_snapshots` schema and access layer.
- [Filter UX Overhaul](filter-ux-overhaul.md) — dashboard/node filtering UX patterns.
