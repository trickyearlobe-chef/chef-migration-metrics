-- SPDX-License-Identifier: Apache-2.0

-- 0066's down script restores the view first; it names this column explicitly,
-- so dropping it before that would fail.
ALTER TABLE git_kitchen_results DROP COLUMN IF EXISTS failure_kind;
