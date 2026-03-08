-- ---------------------------------------------------------------------------
-- 0007 DOWN: Restore NOT NULL DEFAULT FALSE for sufficient_disk_space
-- ---------------------------------------------------------------------------
-- Any existing NULL values must be converted to FALSE before the constraint
-- can be reinstated, otherwise the ALTER will fail.
-- ---------------------------------------------------------------------------

UPDATE node_readiness
   SET sufficient_disk_space = FALSE
 WHERE sufficient_disk_space IS NULL;

ALTER TABLE node_readiness
    ALTER COLUMN sufficient_disk_space SET DEFAULT FALSE,
    ALTER COLUMN sufficient_disk_space SET NOT NULL;
