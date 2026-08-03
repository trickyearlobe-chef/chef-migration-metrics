-- SPDX-License-Identifier: Apache-2.0

-- git_kitchen_results_active fixed its column list when it was created, so the
-- `SELECT gkr.*` it was defined with did not pick up failure_kind. Every TK
-- rollup reads this view, so it has to be refreshed before the column is of any
-- use. Recreating rather than replacing: CREATE OR REPLACE VIEW is fussy about
-- column lists, and there is nothing depending on this view to preserve.
DROP VIEW IF EXISTS git_kitchen_results_active;

CREATE VIEW git_kitchen_results_active AS
SELECT gkr.*
FROM git_kitchen_results gkr
WHERE NOT EXISTS (
    SELECT 1 FROM kitchen_instance_exclusions kie
    WHERE kie.git_repo_name = gkr.git_repo_name
      AND kie.suite_name = gkr.suite_name
      AND kie.platform_name = gkr.platform_name
);

-- Mirrors tkstatus.ClassifyFailure. The Go function is authoritative for new
-- rows; this exists to classify rows captured before the column, and the two
-- are pinned together by a functional test that runs both over the same
-- outputs. Change them together.
--
-- network_timeout has no stored flag of its own: it is a timeout with no sign
-- Chef ever started, which is exactly how the executor derives it live.
CREATE OR REPLACE FUNCTION gkr_failure_kind(
    output TEXT, passed BOOLEAN, timed_out BOOLEAN
) RETURNS TEXT AS $$
    SELECT CASE
        WHEN passed IS TRUE THEN ''
        WHEN timed_out AND COALESCE(output, '') !~* '(Converging [0-9]+ resource|\* [^[:space:]]+\[.*\] action |Recipe: |Starting Chef (Infra )?Client|resolving cookbooks)' THEN 'network_timeout'
        WHEN timed_out THEN 'timeout'
        WHEN COALESCE(output, '') LIKE '%#create action%' THEN 'create_failed'
        WHEN COALESCE(output, '') LIKE '%#converge action%' THEN 'converge_failed'
        WHEN COALESCE(output, '') LIKE '%#verify action%' THEN 'verify_failed'
        WHEN COALESCE(output, '') LIKE '%#destroy action%' THEN 'destroy_failed'
        WHEN COALESCE(output, '') !~* '(Converging [0-9]+ resource|\* [^[:space:]]+\[.*\] action |Recipe: |Starting Chef (Infra )?Client|resolving cookbooks)' THEN 'no_converge'
        ELSE 'unknown'
    END
$$ LANGUAGE SQL IMMUTABLE;
