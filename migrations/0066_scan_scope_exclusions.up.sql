-- Scan scope exclusions: which files an operator asserts a converge never executes.
--
-- The curated seed list lives in code (analysis.DefaultScanScopeExclusions) and
-- reaches files with predictable names. This table is the operator's half: it
-- adds patterns the seed cannot name — most importantly a script that only runs
-- because a build job invokes it, which sits at a different path in every
-- estate — and it overturns seeded patterns the operator disagrees with.
--
-- excluded = FALSE is a deliberate row, not an absent one: it records that a
-- seeded pattern is WRONG here (this really is code that runs), which is a
-- decision somebody made and should be able to see and reverse.
--
-- reason is NOT NULL and enforced non-empty because an exclusion without a
-- recorded reason is how the blocked list gets made to look good.

CREATE TABLE scan_scope_exclusions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    pattern TEXT NOT NULL UNIQUE,
    excluded BOOLEAN NOT NULL DEFAULT TRUE,
    reason TEXT NOT NULL CHECK (btrim(reason) <> ''),
    created_by TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
