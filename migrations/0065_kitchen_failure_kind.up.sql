-- SPDX-License-Identifier: Apache-2.0

-- Why a Test Kitchen run failed, not just that it did.
--
-- A failed run recorded nothing but `passed = false`, so a cookbook that will
-- not converge and a lab that could not build a VM to converge on were the
-- same fact. Readiness treats any Test Kitchen failure as incompatible, over-
-- riding a CookStyle pass, so lab failures blocked real nodes.
--
-- New results are classified in Go (tkstatus.ClassifyFailure). This column,
-- the view refresh in 0066 and the backfill in 0067 exist because results were
-- captured before it, and their output already says which phase failed.
--
-- Three migrations rather than one: a migration file is executed as a single
-- batch, which is parsed before any of it runs, so nothing in this file can
-- refer to the column it adds.
ALTER TABLE git_kitchen_results
    ADD COLUMN IF NOT EXISTS failure_kind TEXT NOT NULL DEFAULT '';
