-- Dropping this table loses every operator decision about what is cookbook code
-- and returns every scan to the curated seed list alone. Cookbooks excluded by
-- an operator-only pattern will start blocking again.
DROP TABLE IF EXISTS scan_scope_exclusions;
