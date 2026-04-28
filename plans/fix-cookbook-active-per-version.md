# Fix: cookbook active status per version

## Goal

Mark only the specific cookbook versions actually referenced by non-stale nodes as active, not all versions of any active cookbook name.

## Specs to read

- `.claude/specifications/web-api.md` (cookbooks endpoint)

## Steps

1. Change `MarkServerCookbooksActiveForOrg` to accept `map[string]map[string]bool` (name→versions) instead of `[]string` (names)
2. Update SQL to match on `(name, version)` pairs rather than just `name`
3. Update collector to pass name+version pairs from `activeCookbookNames`
4. Update mock interface
5. Write/update tests
6. Verify existing tests pass

## Acceptance criteria

- Only cookbook versions actually in a node's expanded run list are marked active
- Versions of the same cookbook not used by any node are inactive
- Existing tests pass
