-- Migration 0034 (down): Restore 'ldap' in auth_provider CHECK constraints.

ALTER TABLE users
    DROP CONSTRAINT IF EXISTS chk_users_auth_provider,
    ADD CONSTRAINT chk_users_auth_provider
        CHECK (auth_provider IN ('local', 'ldap', 'saml'));

ALTER TABLE sessions
    DROP CONSTRAINT IF EXISTS chk_sessions_auth_provider,
    ADD CONSTRAINT chk_sessions_auth_provider
        CHECK (auth_provider IN ('local', 'ldap', 'saml'));
