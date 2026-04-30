-- Copyright 2025 Chef Migration Metrics Authors
-- SPDX-License-Identifier: Apache-2.0

-- Drop the FK from organisations before dropping the credentials table.
ALTER TABLE organisations
    DROP CONSTRAINT IF EXISTS fk_organisations_credential;

DROP TABLE IF EXISTS credentials;
