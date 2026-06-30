-- SPDX-License-Identifier: Apache-2.0
-- Migration 0044: Generic marker table for one-time, idempotent Go-side
-- backfills.
--
-- Some data corrections cannot be expressed in SQL — the precise CookStyle
-- rollup status needs the stored offences re-derived through cop classification,
-- which only the Go derivation can evaluate. Such a backfill runs once at boot
-- and must be cheap to skip on every subsequent boot. This table records which
-- named backfills have completed; the boot routine checks for the marker and
-- skips when present. Reusable for any future Go-side backfill.

CREATE TABLE schema_backfills (
    name         TEXT        PRIMARY KEY,
    completed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON TABLE schema_backfills IS
    'Completion markers for one-time, idempotent Go-side data backfills. A row means the named backfill has run; the boot routine skips it thereafter.';
