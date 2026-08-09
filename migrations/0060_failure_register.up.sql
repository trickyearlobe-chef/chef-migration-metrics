-- The failure register: a person's verdict on whether a cookbook actually
-- works on the target version, recorded with a reason and read back every
-- morning.
--
-- It exists because the automated signals are wrong in both directions —
-- CookStyle marks cookbooks blocked that demonstrably run fine, and Test
-- Kitchen reports the test environment falling over as a cookbook that does
-- not work. Behaviour is journeys/human-verdict.md.

CREATE TABLE failure_register_entries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- The subject is the repo: that is where a fix is made and re-released.
    --
    -- Deliberately NOT a foreign key to git_repos. Its primary key is
    -- (name, git_repo_url) and repo URLs are discovered and volatile — the
    -- collector re-points a clone at a higher-preference base by writing a new
    -- row and deleting the old one. A foreign key would cascade a person's
    -- verdict away on a re-hosting. The name is the stable part, so the name
    -- is what this keys on.
    git_repo_name TEXT NOT NULL,

    -- The label, because standup says "cookbook" while looking at repo-level
    -- work. Never a version: several versions are in use at once and the
    -- failure is discussed version-agnostically.
    cookbook_name TEXT NOT NULL,

    -- Two-sided. 'broken' records a failure nothing detected; 'not_broken'
    -- overrules a wrong automated verdict. Both are the same act — a person
    -- overruling a machine, with evidence.
    verdict TEXT NOT NULL CHECK (verdict IN ('broken', 'not_broken')),

    -- Mandatory. A verdict with no reason is an opinion, and it will be
    -- overturned by the next person who disagrees. The reason is what makes it
    -- survive, and what lets a later reader judge whether it still holds.
    reason TEXT NOT NULL CHECK (btrim(reason) <> ''),

    -- Optional but expected: the stacktrace, the run that failed, or the fleet
    -- observation that contradicts the scan.
    --
    -- Unbounded text. Storing it is not a problem; a btree entry is capped at
    -- roughly a third of a page, which a few tens of lines of trace already
    -- exceeds, and the failure mode is a hard write error rather than
    -- slowness. This has caused a production outage in this project before.
    -- NEVER add an index or a unique constraint over this column. If repeat
    -- occurrences ever need recognising as the same failure, hash a bounded,
    -- canonicalised projection and index that instead.
    evidence TEXT,

    -- What journey 4 asks for beyond the verdict. All optional: a failure is
    -- worth recording the moment it is seen, before anybody knows what to do
    -- about it. Requiring a plan up front means failures go unrecorded.
    diagnosis   TEXT,
    plan        TEXT,
    target_date DATE,

    -- Who is on it: an owner, or a reference to work tracked in another
    -- system. Where work is tracked elsewhere this holds the URL or ticket
    -- number; CMM does not read or write the system behind it.
    --
    -- No separate 'user' kind. Everything person-shaped is an owner, and other
    -- identities — including one sourced from SAML — reach an owner through an
    -- alias; the signed-in CMM user resolves the same way, which is what makes
    -- "what's mine" filtering possible. A second kind for people would be a
    -- second identity space for the same thing.
    holder_type TEXT CHECK (holder_type IN ('owner', 'ticket')),
    holder_ref  TEXT,

    -- Lifecycle. Resolution is recorded, not deleted: the standup view needs
    -- the direction of travel, which is unavailable if resolved entries
    -- vanish. 'superseded' is what a reversal leaves behind — verdicts are
    -- superseded, never silently replaced.
    status TEXT NOT NULL DEFAULT 'open'
        CHECK (status IN ('open', 'resolved', 'superseded')),

    raised_by TEXT        NOT NULL,
    raised_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    resolved_by     TEXT,
    resolved_at     TIMESTAMPTZ,
    resolution_note TEXT,

    -- The reversal that replaced this verdict, so the disagreement stays
    -- readable rather than being overwritten.
    superseded_by UUID REFERENCES failure_register_entries (id) ON DELETE SET NULL,

    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- A resolved entry says who resolved it and when. Without this the
    -- direction of travel is computed from rows that may carry no date.
    CONSTRAINT chk_failure_register_resolved
        CHECK (status <> 'resolved' OR (resolved_at IS NOT NULL AND resolved_by IS NOT NULL)),

    -- A holder is a type and a reference together, or neither.
    CONSTRAINT chk_failure_register_holder
        CHECK ((holder_type IS NULL AND holder_ref IS NULL)
            OR (holder_type IS NOT NULL AND btrim(COALESCE(holder_ref, '')) <> ''))
);

-- At most one open verdict per repo. A second verdict on a repo that already
-- has one is a reversal: the first moves to 'superseded' and points at the
-- second. Resolved and superseded rows are unconstrained, which is what lets
-- the history accumulate.
CREATE UNIQUE INDEX idx_failure_register_one_open_per_repo
    ON failure_register_entries (git_repo_name)
    WHERE status = 'open';

-- The standup view: what is open, most recently raised first.
CREATE INDEX idx_failure_register_open
    ON failure_register_entries (status, raised_at DESC);

-- The whole history for one repo, which is where a reader sees that a scan
-- called it incompatible and a person overruled it.
CREATE INDEX idx_failure_register_repo
    ON failure_register_entries (git_repo_name, raised_at DESC);

-- Direction of travel, and the accuracy report: how many entries were raised
-- and resolved in a window, split by which way the verdict went.
CREATE INDEX idx_failure_register_resolved_at
    ON failure_register_entries (resolved_at DESC)
    WHERE resolved_at IS NOT NULL;

-- 'failure_recorded', 'failure_revised', 'failure_reversed' and
-- 'failure_resolved' join the audit actions. The action column carries no
-- CHECK constraint (removed deliberately in an earlier change), so nothing
-- has to be widened here — this comment records the vocabulary.
