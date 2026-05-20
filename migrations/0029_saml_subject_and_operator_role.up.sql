-- Add saml_subject column for federated identity keying and expand
-- the role check constraint to include the new 'operator' role.

ALTER TABLE users ADD COLUMN saml_subject TEXT;

-- Unique index: only one user per SAML subject (non-null values only).
CREATE UNIQUE INDEX idx_users_saml_subject ON users (saml_subject) WHERE saml_subject IS NOT NULL;

-- Expand role constraint to include 'operator'.
ALTER TABLE users DROP CONSTRAINT chk_users_role;
ALTER TABLE users ADD CONSTRAINT chk_users_role CHECK (role IN ('admin', 'operator', 'viewer'));

-- Allow empty password_hash for SAML-only users.
ALTER TABLE users ALTER COLUMN password_hash DROP NOT NULL;
