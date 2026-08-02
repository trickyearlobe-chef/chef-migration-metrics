-- Saying "these two are not the same person", durably.
--
-- The duplicates view offered a merge and nothing else, so a pair somebody had
-- looked at and rejected stayed on the list. It could not be worked down to
-- nothing, which is the only state that makes a list like this worth opening.
--
-- This lives outside owner_duplicate_candidates because the scan deletes and
-- rebuilds that table on every run — a dismissal recorded there would be
-- swept away and the pair would return, which is precisely the complaint.

CREATE TABLE owner_duplicate_dismissals (
    -- Ordered the same way the candidate table orders a pair, so a dismissal
    -- matches however the scan happens to encounter the two.
    owner_a TEXT NOT NULL REFERENCES owners (name) ON DELETE CASCADE,
    owner_b TEXT NOT NULL REFERENCES owners (name) ON DELETE CASCADE,

    -- Why they are not the same person. Optional: a rejection is useful even
    -- unexplained, and demanding a reason is how dismissals stop being
    -- recorded at all. Unlike a failure register verdict, this one only ever
    -- removes a suggestion — it cannot make anybody act on the wrong thing.
    reason TEXT,

    dismissed_by TEXT        NOT NULL,
    dismissed_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    PRIMARY KEY (owner_a, owner_b),
    CONSTRAINT chk_owner_duplicate_dismissal_order CHECK (owner_a < owner_b)
);

-- The cascade is the point of the foreign keys: merging one of the two away,
-- or deleting an owner, takes the dismissal with it. A pair that no longer
-- exists should not go on suppressing anything.
