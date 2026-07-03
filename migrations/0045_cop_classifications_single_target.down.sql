-- Reverse the single-target re-key. Structural reversal only: the per-target
-- rows removed by the up migration's de-duplication cannot be restored.

ALTER TABLE cop_classifications DROP CONSTRAINT IF EXISTS cop_classifications_cop_name_key;

ALTER TABLE cop_classifications ADD COLUMN target_chef_version TEXT NOT NULL DEFAULT '';
ALTER TABLE cop_classifications ALTER COLUMN target_chef_version DROP DEFAULT;

ALTER TABLE cop_classifications
    ADD CONSTRAINT cop_classifications_cop_name_target_chef_version_key UNIQUE (cop_name, target_chef_version);

CREATE INDEX idx_cop_classifications_target ON cop_classifications (target_chef_version);
