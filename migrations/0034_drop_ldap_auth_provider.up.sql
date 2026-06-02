-- Migration 0034: Remove 'ldap' from auth_provider CHECK constraints.
-- LDAP authentication is not being implemented; local and SAML are supported.

ALTER TABLE users
    DROP CONSTRAINT IF EXISTS chk_users_auth_provider,
    ADD CONSTRAINT chk_users_auth_provider
        CHECK (auth_provider IN ('local', 'saml'));

ALTER TABLE sessions
    DROP CONSTRAINT IF EXISTS chk_sessions_auth_provider,
    ADD CONSTRAINT chk_sessions_auth_provider
        CHECK (auth_provider IN ('local', 'saml'));
