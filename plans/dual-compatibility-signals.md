# Dual Compatibility Signals

## Goal

Show CookStyle and Test Kitchen as separate signals across all views. CookStyle = low-confidence linting. TK = high-confidence for passes.

## Specs to Read

- `.claude/specifications/dual-compatibility-signals.md`
- `.claude/specifications/roles.md` (role detail/list structure)
- `.claude/specifications/web-api.md` (API contracts)

## Steps

1. Backend: source lookup + TK status queries for role chain nodes
2. Frontend: add CS/TK badge variants to StatusBadge
3. Frontend: dependency tree with source icons + dual badges
4. Backend: add TK status to role list endpoint
5. Frontend: TK column on role list
6. Backend: add TK status to cookbook list endpoint
7. Frontend: TK column on cookbook list
8. Frontend: role detail summary cards with TK counts

## Acceptance Criteria

- Dependency tree shows source icon (📦/🔀/📦🔀) per cookbook
- Dependency tree shows separate CS and TK badges
- Role list has TK status column
- Cookbook list has TK status column for cookbooks with git repos
- Role detail summary includes TK counts
- All existing tests pass + new tests for added functionality
- No changes to dashboard or node views (already dual-signal)
