-- Single active target: cop classification overrides are keyed by cop_name only
-- (the per-target dimension is removed). De-duplicate any existing per-target
-- rows first, keeping the most recently updated row per cop, then drop the
-- target column (CASCADE removes the composite unique constraint and the
-- target index) and re-key uniqueness on cop_name.

DELETE FROM cop_classifications a
USING cop_classifications b
WHERE a.cop_name = b.cop_name
  AND (a.updated_at < b.updated_at
       OR (a.updated_at = b.updated_at AND a.id < b.id));

ALTER TABLE cop_classifications DROP COLUMN target_chef_version CASCADE;

ALTER TABLE cop_classifications ADD CONSTRAINT cop_classifications_cop_name_key UNIQUE (cop_name);
