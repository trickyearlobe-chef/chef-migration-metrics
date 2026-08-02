-- Migration 0056: Saved column mappings for discovery-driven ownership import.
--
-- WHY: the fixed-header import requires the source file to already be in CMM's
-- column order. Sources we do not control are not, so the administrator maps
-- their columns onto our fields in the UI. A repeat import must not need
-- re-mapping, so the mapping document is persisted here.
--
-- field_map is stored as a JSON document rather than shredded into rows: it is
-- a nested tagged union (source kind + ordered transform chain), so normalising
-- it would turn every change to the mapping language into a migration.
--
-- source_kind is CHECK-constrained to 'csv' alone. A SQL source is a later
-- change and will widen this constraint deliberately rather than by accident.

CREATE TABLE ownership_import_mappings (
    id          BIGSERIAL PRIMARY KEY,
    name        TEXT NOT NULL,
    source_kind TEXT NOT NULL DEFAULT 'csv',
    delimiter   TEXT NOT NULL DEFAULT ',',
    field_map   JSONB NOT NULL,
    created_by  TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_ownership_import_mapping_name UNIQUE (name),
    CONSTRAINT chk_ownership_import_mapping_source_kind CHECK (source_kind IN ('csv'))
);

CREATE INDEX idx_ownership_import_mappings_name ON ownership_import_mappings (name);
