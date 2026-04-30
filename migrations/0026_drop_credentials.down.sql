-- Copyright 2025 Chef Migration Metrics Authors
-- SPDX-License-Identifier: Apache-2.0

CREATE TABLE IF NOT EXISTS credentials (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name             TEXT        NOT NULL,
    credential_type  TEXT        NOT NULL,
    encrypted_value  TEXT        NOT NULL,
    metadata         JSONB,
    last_rotated_at  TIMESTAMPTZ,
    created_by       TEXT        NOT NULL,
    updated_by       TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_credentials_name UNIQUE (name),
    CONSTRAINT uq_credentials_type_name UNIQUE (credential_type, name),
    CONSTRAINT chk_credentials_type CHECK (
        credential_type IN ('chef_client_key', 'ldap_bind_password', 'smtp_password', 'webhook_url', 'generic')
    )
);

ALTER TABLE organisations
    ADD CONSTRAINT fk_organisations_credential
        FOREIGN KEY (client_key_credential_name) REFERENCES credentials(name) ON DELETE SET NULL;
