-- Reverse of 0065. The rejections are a derived statement about the current
-- source data, re-created by the next import, so dropping them loses nothing
-- that cannot be recomputed by running the import again.

DROP TABLE IF EXISTS ownership_import_rejections;
