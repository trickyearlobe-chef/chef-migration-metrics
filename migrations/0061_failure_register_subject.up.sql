-- A verdict can be about a cookbook that has no git repo.
--
-- The register was keyed on the repo because that is where a fix is made and
-- re-released. But a cookbook can be entirely real, deployed and broken with
-- no repo collected for it — and those are more likely to be unowned and
-- untested, not less. Refusing to record them excluded exactly the population
-- the register exists to catch.
--
-- The column is renamed rather than reused: storing a cookbook name in a
-- column called git_repo_name is the kind of misleading shape that produces a
-- wrong answer years later, and the register has never shipped, so this is the
-- cheapest it will ever be.

ALTER TABLE failure_register_entries
    RENAME COLUMN git_repo_name TO subject_name;

-- What kind of thing subject_name names. 'git_repo' for everything recorded
-- so far, which is what the default backfills.
ALTER TABLE failure_register_entries
    ADD COLUMN subject_type TEXT NOT NULL DEFAULT 'git_repo'
        CHECK (subject_type IN ('git_repo', 'cookbook'));

-- The indexes survive the rename but keep their old names, which would read as
-- though they were still about repos.
ALTER INDEX idx_failure_register_one_open_per_repo
    RENAME TO idx_failure_register_one_open_per_subject;
ALTER INDEX idx_failure_register_repo
    RENAME TO idx_failure_register_subject;

-- Uniqueness stays on the name alone, deliberately NOT on (subject_type,
-- subject_name). Where there is one cookbook per repo, a repo and a cookbook of
-- the same name are the same thing seen from two sides; keying on the pair would
-- allow two standing verdicts about it that could disagree. One standing verdict
-- per thing, however it was picked.
