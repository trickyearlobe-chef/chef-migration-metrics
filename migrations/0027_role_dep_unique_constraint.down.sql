-- Copyright 2025 Chef Migration Metrics Authors
-- SPDX-License-Identifier: Apache-2.0

DROP INDEX IF EXISTS idx_role_dependencies_org_dep_name;

ALTER TABLE role_dependencies
    DROP CONSTRAINT IF EXISTS uq_role_dependencies;
