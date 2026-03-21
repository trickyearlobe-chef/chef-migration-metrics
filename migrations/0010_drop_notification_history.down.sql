-- ---------------------------------------------------------------------------
-- 0010_drop_notification_history (down) — recreate the table
-- ---------------------------------------------------------------------------
-- Restores the notification_history table exactly as it was defined in
-- migration 0001_initial_schema.up.sql.
-- ---------------------------------------------------------------------------

CREATE TABLE notification_history (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    channel_name   TEXT        NOT NULL,
    channel_type   TEXT        NOT NULL,
    event_type     TEXT        NOT NULL,
    summary        TEXT        NOT NULL,
    payload        JSONB       NOT NULL,
    status         TEXT        NOT NULL,
    error_message  TEXT,
    retry_count    INTEGER     NOT NULL DEFAULT 0,
    sent_at        TIMESTAMPTZ NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT chk_notification_history_channel_type CHECK (
        channel_type IN ('webhook', 'email')
    ),
    CONSTRAINT chk_notification_history_event_type CHECK (
        event_type IN (
            'cookbook_status_change',
            'readiness_milestone',
            'new_incompatible_cookbook',
            'collection_failure',
            'stale_node_threshold_exceeded'
        )
    ),
    CONSTRAINT chk_notification_history_status CHECK (
        status IN ('sent', 'failed', 'retrying')
    )
);

CREATE INDEX idx_notification_history_event_type ON notification_history (event_type);
CREATE INDEX idx_notification_history_channel_name ON notification_history (channel_name);
CREATE INDEX idx_notification_history_status ON notification_history (status);
CREATE INDEX idx_notification_history_sent_at ON notification_history (sent_at);
