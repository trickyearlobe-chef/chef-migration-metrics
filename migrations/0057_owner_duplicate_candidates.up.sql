-- Possible-duplicate owners, computed by a bounded scan and stored.
--
-- Comparing every owner with every other one live does not scale: a trigram
-- sweep over a catalogue of ten thousand owners takes minutes, because owner
-- names cluster hard (people share surnames, and committer-derived names are
-- email localparts). The scan is therefore bounded to the nearest few
-- candidates per owner, run on demand, and its result read from here.

-- GiST supports nearest-neighbour ordering (the <-> operator), which is what
-- makes the scan bounded: the index returns the closest k directly instead of
-- every row above a similarity floor. The existing GIN indexes stay — they
-- serve the % and similarity() lookups used by alias suggestion.
CREATE INDEX IF NOT EXISTS idx_owners_name_gist_trgm
    ON owners USING gist (name gist_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_owner_aliases_value_gist_trgm
    ON owner_aliases USING gist (alias_value gist_trgm_ops);

CREATE TABLE owner_duplicate_candidates (
    -- Ordered so a pair is stored once. The foreign keys mean a merged-away
    -- owner takes its pairs with it, so a resolved duplicate leaves the list
    -- without waiting for the next scan.
    owner_a     TEXT        NOT NULL REFERENCES owners(name) ON DELETE CASCADE,
    owner_b     TEXT        NOT NULL REFERENCES owners(name) ON DELETE CASCADE,
    matched_on  TEXT        NOT NULL,
    value_a     TEXT        NOT NULL,
    value_b     TEXT        NOT NULL,
    similarity  REAL        NOT NULL,

    PRIMARY KEY (owner_a, owner_b),
    CONSTRAINT chk_owner_duplicate_order CHECK (owner_a < owner_b),
    CONSTRAINT chk_owner_duplicate_matched_on CHECK (matched_on IN ('name', 'alias'))
);

CREATE INDEX idx_owner_duplicate_candidates_similarity
    ON owner_duplicate_candidates (similarity DESC, owner_a, owner_b);

-- When the scan last ran, and what it found. Kept separately so an empty
-- candidate table can say "nothing looks alike" rather than being mistaken
-- for "never scanned".
CREATE TABLE owner_duplicate_scans (
    only_row    BOOLEAN     PRIMARY KEY DEFAULT TRUE CHECK (only_row),
    scanned_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    pairs_found INTEGER     NOT NULL DEFAULT 0
);
