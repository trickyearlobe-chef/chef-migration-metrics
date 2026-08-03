-- A git_repo assignment is keyed by the repo name, not its URL.
--
-- The committers panel wrote the git URL as the assignment's key while every
-- screen that lists repos reads ownership by name, so a repo somebody had just
-- claimed went on reading as unowned — and appeared in the unowned filter, the
-- list migration work is dispatched from. The writer now records the name; this
-- repairs the rows it already wrote.
--
-- Only rows whose key matches a known repo URL are rewritten. A key matching
-- nothing is left exactly as it is: it names a repo this instance has never
-- collected, and guessing at it would turn a visible oddity into a silent wrong
-- answer.
--
-- The unique index is (owner_name, entity_type, entity_key, org), so the
-- rewrite can collide two ways. Both are resolved by dropping the redundant
-- row before the update, never by dropping an owner: every owner surviving here
-- already has the same repo under its name.

-- 1. A name-keyed row for this owner and repo already exists — the import wrote
--    one, the panel wrote another. The URL-keyed row adds nothing.
DELETE FROM ownership_assignments dup
USING git_repos gr, ownership_assignments keep
WHERE dup.entity_type = 'git_repo'
  AND dup.entity_key = gr.git_repo_url
  AND keep.entity_type = 'git_repo'
  AND keep.entity_key = gr.name
  AND keep.owner_name = dup.owner_name
  AND keep.organisation_name IS NOT DISTINCT FROM dup.organisation_name;

-- 2. Two URL-keyed rows for the same owner that resolve to one name — the same
--    repo tracked at two URLs. Keep the oldest; the rest would collide.
DELETE FROM ownership_assignments oa
WHERE oa.id IN (
    SELECT id FROM (
        SELECT oa2.id,
               ROW_NUMBER() OVER (
                   PARTITION BY oa2.owner_name, gr.name,
                                COALESCE(oa2.organisation_name, '__none__')
                   ORDER BY oa2.id
               ) AS rn
        FROM ownership_assignments oa2
        JOIN git_repos gr ON gr.git_repo_url = oa2.entity_key
        WHERE oa2.entity_type = 'git_repo'
    ) ranked
    WHERE ranked.rn > 1
);

-- 3. Rewrite what is left.
UPDATE ownership_assignments oa
SET entity_key = gr.name,
    updated_at = now()
FROM git_repos gr
WHERE oa.entity_type = 'git_repo'
  AND oa.entity_key = gr.git_repo_url;
