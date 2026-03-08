-- =============================================================================
-- Migration 0006: Authentication — Users and Sessions
-- =============================================================================
-- Creates the `users` and `sessions` tables for local authentication,
-- session management, and RBAC. See the datastore specification
-- (tables 17 and 18) and the auth specification for full details.
-- =============================================================================

-- ---------------------------------------------------------------------------
-- 1. users
-- ---------------------------------------------------------------------------
-- Stores local user accounts. LDAP and SAML users are authenticated
-- externally and mapped directly to sessions without a row here.

CREATE TABLE IF NOT EXISTS users (
    id                      UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    username                TEXT        NOT NULL,
    display_name            TEXT,
    email                   TEXT,
    password_hash           TEXT        NOT NULL,
    role                    TEXT        NOT NULL DEFAULT 'viewer',
    auth_provider           TEXT        NOT NULL DEFAULT 'local',
    is_locked               BOOLEAN     NOT NULL DEFAULT FALSE,
    failed_login_attempts   INTEGER     NOT NULL DEFAULT 0,
    last_login_at           TIMESTAMPTZ,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT users_username_unique UNIQUE (username),
    CONSTRAINT users_role_check CHECK (role IN ('admin', 'viewer')),
    CONSTRAINT users_auth_provider_check CHECK (auth_provider IN ('local', 'ldap', 'saml'))
);

CREATE INDEX IF NOT EXISTS idx_users_username ON users (username);
CREATE INDEX IF NOT EXISTS idx_users_auth_provider ON users (auth_provider);

-- ---------------------------------------------------------------------------
-- 2. sessions
-- ---------------------------------------------------------------------------
-- Stores active user sessions. The session UUID doubles as the opaque
-- session token. Expired rows are cleaned up periodically.

CREATE TABLE IF NOT EXISTS sessions (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID,
    username        TEXT        NOT NULL,
    auth_provider   TEXT        NOT NULL,
    role            TEXT        NOT NULL,
    expires_at      TIMESTAMPTZ NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT sessions_role_check CHECK (role IN ('admin', 'viewer')),
    CONSTRAINT sessions_auth_provider_check CHECK (auth_provider IN ('local', 'ldap', 'saml')),
    CONSTRAINT fk_sessions_user_id FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions (user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions (expires_at);
