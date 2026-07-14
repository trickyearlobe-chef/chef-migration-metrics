-- Saved filters: a named, owned, view-scoped set of filter query params.
--
-- The payload (filters) is the view's existing request contract — the same
-- query params the view's URL and its <x>FilterFromValues parser already speak,
-- stored as param -> list of values. One table serves every list view; the
-- filter vocabulary stays owned by the view's own code and is enforced in
-- internal/webapi/saved_filter_params.go (keep the view_name CHECK below in
-- step with savedFilterVocabulary there).
--
-- Ownership is anchored on username, the established anchor (auth.SessionInfo
-- carries Username, not a user id). A surrogate id is used rather than the
-- natural (owner, view, name) key because a saved filter can be renamed and the
-- API needs a stable handle across a rename.
CREATE TABLE saved_filters (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_username TEXT        NOT NULL,
    view_name      TEXT        NOT NULL,
    name           TEXT        NOT NULL,
    filters        JSONB       NOT NULL DEFAULT '{}',
    shared         BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT fk_saved_filters_owner
        FOREIGN KEY (owner_username) REFERENCES users (username) ON DELETE CASCADE,

    CONSTRAINT uq_saved_filters_owner_view_name
        UNIQUE (owner_username, view_name, name),

    CONSTRAINT chk_saved_filters_view_name
        CHECK (view_name IN ('nodes', 'roles', 'cookbooks', 'git-repos')),

    CONSTRAINT chk_saved_filters_name
        CHECK (name <> '')
);

-- Listing is always "visible to this user on this view": own filters, plus the
-- shared ones.
CREATE INDEX idx_saved_filters_owner_view ON saved_filters (owner_username, view_name);
CREATE INDEX idx_saved_filters_shared_view ON saved_filters (view_name) WHERE shared;
