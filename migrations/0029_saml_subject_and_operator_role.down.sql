-- Revert saml_subject column and operator role.

ALTER TABLE users ALTER COLUMN password_hash SET NOT NULL;

ALTER TABLE users DROP CONSTRAINT chk_users_role;
ALTER TABLE users ADD CONSTRAINT chk_users_role CHECK (role IN ('admin', 'viewer'));

DROP INDEX IF EXISTS idx_users_saml_subject;

ALTER TABLE users DROP COLUMN IF EXISTS saml_subject;
