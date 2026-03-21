-- ---------------------------------------------------------------------------
-- 0010_drop_notification_history
-- ---------------------------------------------------------------------------
-- Drop the notification_history table. This table was created in the
-- initial schema (migration 0001) but has no Go code that reads from or
-- writes to it — no struct, no repository method, no API handler, and no
-- test references it. The specification's to-do list explicitly marks
-- notification persistence as unimplemented.
--
-- Dropping it eliminates an unused table and its four indexes from the
-- schema, reducing maintenance overhead.
-- ---------------------------------------------------------------------------

DROP TABLE IF EXISTS notification_history;
