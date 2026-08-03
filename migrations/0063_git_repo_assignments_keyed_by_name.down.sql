-- Puts the git URL back as the key for git_repo assignments.
--
-- This is not a true inverse and cannot be: rows the up-migration deleted as
-- redundant duplicates are gone, and a name-keyed row the import wrote is
-- indistinguishable here from one the up-migration rewrote — so this rewrites
-- both. Only run it to get an older binary reading its own key form again.

UPDATE ownership_assignments oa
SET entity_key = gr.git_repo_url,
    updated_at = now()
FROM git_repos gr
WHERE oa.entity_type = 'git_repo'
  AND oa.entity_key = gr.name;
