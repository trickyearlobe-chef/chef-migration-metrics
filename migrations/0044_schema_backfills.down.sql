-- SPDX-License-Identifier: Apache-2.0
-- Down migration 0044: drop the generic backfill marker table.

DROP TABLE IF EXISTS schema_backfills;
