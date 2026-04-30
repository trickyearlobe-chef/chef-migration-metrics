-- Copyright 2025 Chef Migration Metrics Authors
-- SPDX-License-Identifier: Apache-2.0

-- Migration 0009 dropped the UNIQUE constraint on role_dependencies without
-- re-adding it under the new natural-key schema. This restores it.
--
-- The implicit B-tree index on (organisation_name, role_name, dependency_type,
-- dependency_name) also provides the (organisation_name, role_name) prefix
-- needed by the recursive CTE join in ListRolesFiltered:
--
--   JOIN role_dependencies rd2
--     ON rd2.organisation_name = td.organisation_name
--    AND rd2.role_name = td.dependency_name
--
-- Without this index the recursive step performs a sequential scan per
-- iteration, making the roles list extremely slow at scale.
--
-- A second index on (organisation_name, dependency_name, dependency_type)
-- is added to cover the reverse lookup used when constructing the
-- transitive dependency graph for a single role.

ALTER TABLE role_dependencies
    ADD CONSTRAINT uq_role_dependencies
        UNIQUE (organisation_name, role_name, dependency_type, dependency_name);

CREATE INDEX idx_role_dependencies_org_dep_name
    ON role_dependencies (organisation_name, dependency_name, dependency_type);
