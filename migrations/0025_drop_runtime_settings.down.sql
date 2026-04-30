-- Migration 0025 (down): Recreate the legacy runtime_settings table.
--
-- This restores the schema for rollback purposes. Any data that was in
-- runtime_settings prior to 0025 will not be restored — it was already
-- migrated to config_store by MigrateFromLegacy().

CREATE TABLE IF NOT EXISTS runtime_settings (
    key         TEXT        NOT NULL,
    value       JSONB       NOT NULL DEFAULT '{}',
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_by  TEXT        NOT NULL DEFAULT '',

    PRIMARY KEY (key)
);

COMMENT ON TABLE runtime_settings IS
    'Legacy UI-managed configuration overrides. Replaced by config_store (migration 0011). Restored for rollback only.';
