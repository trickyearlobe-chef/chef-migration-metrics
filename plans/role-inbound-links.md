# Role Inbound Links & Summary Bar Click-to-Filter

## Goal

Wire up inbound links to role pages from existing views, and make the summary bar segments clickable to pre-set the compatibility filter.

## Specs to Read

- `.claude/specifications/roles.md` § Navigation → Inbound Links, § Summary Bar

## Steps

1. `DependencyGraphPage.tsx` — convert role `<span>` to `<Link>` in 6 locations:
   - `SelectedNodePanel` → Role Dependencies list
   - `SelectedNodePanel` → Depended-on-by list (role items)
   - `SelectedNodePanel` → Add "View Role Details" button (alongside existing cookbook button)
   - `TableRow` → Role Name column
   - `TableRow` → Expanded row role dependency pills
   - `TableRow` → Mini dependency pills (role type)
2. `NodeDetailPage.tsx` — convert role badge `<span>` to `<Link to="/roles/:name">` in the Roles section
3. `RolesPage.tsx` — make summary bar segments clickable to set `compatibilityStatus` filter
4. Add/update frontend tests for the new links
5. Run all tests, verify clean

## Acceptance Criteria

- Role names in dependency graph (graph view panel + table view) link to `/roles/:name`
- Role badges on node detail page link to `/roles/:name`
- Clicking a summary bar segment on the roles list page sets the compatibility filter
- All existing + new tests pass