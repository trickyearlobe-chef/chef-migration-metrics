-- Migration 0010: Create runtime settings table for UI-managed configuration
--
-- Stores configuration overrides that can be managed through the admin UI.
-- Settings stored here take precedence over config.yml values.
-- See test-kitchen-config-ui.md.

CREATE TABLE IF NOT EXISTS runtime_settings (
    key         TEXT        NOT NULL,
    value       JSONB       NOT NULL DEFAULT '{}',
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_by  TEXT        NOT NULL DEFAULT '',

    PRIMARY KEY (key)
);

COMMENT ON TABLE runtime_settings IS
    'UI-managed configuration overrides. DB values take precedence over config.yml.';
COMMENT ON COLUMN runtime_settings.key IS
    'Setting key (e.g. test_kitchen).';
COMMENT ON COLUMN runtime_settings.value IS
    'Setting value as JSONB. Structure depends on the key.';
COMMENT ON COLUMN runtime_settings.updated_by IS
    'Username of the admin who last saved this setting.';
