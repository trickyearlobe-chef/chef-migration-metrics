-- SPDX-License-Identifier: Apache-2.0
-- Migration 0012 (down): Drop kitchen analysis tables

DROP TABLE IF EXISTS kitchen_discovered_platforms;
DROP INDEX IF EXISTS idx_kitchen_analysis_driver;
DROP TABLE IF EXISTS kitchen_analysis_results;
