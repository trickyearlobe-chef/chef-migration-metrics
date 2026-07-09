-- SPDX-License-Identifier: Apache-2.0
-- Rollback migration 0049: drop the role_summary materialised table.

DROP TABLE IF EXISTS role_summary;
