-- =============================================================================
-- Migration 0048: Node Snapshot Tags
-- =============================================================================
-- Adds Chef node tags (normal.tags) as a first-class filter dimension on
-- node_snapshots. Tags are stored as a native TEXT[] — not folded into the
-- custom_attributes JSONB — so array-overlap filtering and distinct-value
-- facets are fast and type-safe.
--
-- TEXT[] (not JSONB like roles) is deliberate: the tags filter uses OR /
-- array-overlap semantics, which map directly to the Postgres && operator on
-- a GIN-indexed text array. See: specifications/node-tags.md
-- =============================================================================

-- Native text array. NULL is possible for pre-migration rows; the ingestion
-- path always writes a (possibly empty) array, so new rows are never NULL.
-- "collected, no tags" is the empty array '{}', distinct from NULL.
ALTER TABLE node_snapshots
    ADD COLUMN IF NOT EXISTS tags TEXT[];

-- GIN index with the default array_ops operator class supports the && (overlap)
-- operator used by the OR multi-select filter:
--   tags && ARRAY['prepare','upgrade']::text[]
-- and the unnest()-based distinct-value facet.
CREATE INDEX IF NOT EXISTS idx_node_snapshots_tags_gin
    ON node_snapshots USING GIN (tags);
