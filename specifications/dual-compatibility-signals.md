# Dual Compatibility Signals

## Problem

Compatibility assessment comes from two independent signals with different characteristics:

- **CookStyle** (static analysis) — lints cookbooks for deprecated Chef API usage. Available for server cookbooks and git repos. **Low confidence**: a pass means no deprecated API calls were found, but cannot prove runtime correctness.
- **Test Kitchen** (runtime) — converges and verifies cookbooks on real platforms. Available for git repos only. **High confidence for passes**: a pass proves the cookbook works. **Unreliable for failures**: failures may be infrastructure noise, missing test fixtures, or driver issues rather than real incompatibility.

These signals should remain separate throughout the UI. Users need to see both independently to make informed migration decisions.

## Current State

### Already Dual-Signal

| View | CookStyle | Test Kitchen |
|------|-----------|-------------|
| Dashboard | 2 cards (server + git CS) | 1 card (TK) |
| Node list | CS icon in Checks column | TK icon in Checks column |
| Node detail | Separate verdict rows per source | Separate verdict rows per source |

### CookStyle-Only (Needs TK)

| View | Current | Needed |
|------|---------|--------|
| Role list | Single "Compatibility" column (CS worst-of) | Add TK summary column |
| Role detail — summary cards | CS-based counts only | Add TK summary counts |
| Role detail — dependency tree | CS badge only | Add TK badge + source icon |
| Cookbook list | Single "Compatibility" column (CS) | Add TK column for cookbooks with git repos |

## Design

### Badge Style

Two small coloured badges per cookbook/role where data exists. The CS and TK
badges are **separate signals and are never merged into one verdict** — this spec
is the source of that principle (see also
[cop-classification.md](cop-classification.md)).

- **CS badge**: the 4-state CookStyle rollup status — 🟢 "CS Ready" / 🟠 "CS Needs
  review" / 🔴 "CS Blocked" / ⚪ "CS Untested" (the canonical per-item rollup; see
  the CookStyle Rollup Status table in [cop-classification.md](cop-classification.md))
- **TK badge**: green "TK ✓" / red "TK ✗" / orange "TK ~" / grey "TK ?" (passed/failed/partial/untested) — unchanged

TK badge is only shown when a matching git repo exists. Server-only cookbooks show CS badge only.

### Source Indicator (Dependency Tree)

Each cookbook node shows where the cookbook exists:

- 📦 Server cookbook only — CS badge only
- 🔀 Git repo only — CS + TK badges
- 📦🔀 Both — CS + TK badges

### Role List

Add a `tk_status` column alongside `compatibility_status`:

| Role | CS Status | TK Status | Nodes | Cookbooks |
|------|-----------|-----------|-------|-----------|
| web-server | Ready | Passed | 12 | 8 |
| db-server | Blocked | Partial | 4 | 6 |
| monitoring | Needs review | — | 3 | 4 |

TK status for a role = worst-of TK status across its transitive git-repo cookbooks:
- Any failed → "failed"
- Any partial (and none failed) → "partial"
- All passed → "passed"
- No TK data → not shown (dash)

### Role Detail

**Summary cards** — add TK summary alongside CS summary:
- CS: W ready / X needs review / Y blocked / Z untested
- TK: A passed / B failed / C partial / D untested

**Dependency tree** — each cookbook node shows source icon + CS badge + TK badge (when available).

### Cookbook List

Add TK column for server cookbooks that have a matching git repo with TK results. Show "—" for server-only cookbooks with no git repo.

## Backend Changes

### Role Chain Node

```
RoleChainNode {
  name, type, compatibility_status (CS),
  source (server/git/both),
  tk_status (passed/failed/partial/untested or empty),
  children
}
```

### New Queries

- `getServerCookbookNames(ctx, orgs, names)` → set of names that exist in server_cookbooks
- `getGitRepoCookbookNames(ctx, names)` → set of names that exist in git_repos
- `getGitKitchenStatusMap(ctx, names)` → map[name]status read from materialised `git_repos.tk_status`

### Role List Enhancement

Add `tk_status` to role list response. Derived from worst-of TK status across the role's transitive cookbook set (only those with git repos).

### Cookbook List Enhancement

Add `tk_status` to cookbook list response. Join server_cookbooks → git_repos → git_kitchen_results to get TK status for cookbooks that have matching git repos.

## Frontend Changes

### StatusBadge Additions

Add dedicated `cs_ready`, `cs_needs_review`, `cs_blocked`, `cs_untested`, `tk_passed`, `tk_failed`, `tk_partial`, `tk_untested` variants to the StatusBadge component with appropriate colours and labels. The `cs_*` variants map to the 4-state CookStyle rollup (Ready / Needs review / Blocked / Untested); the `tk_*` variants are unchanged.

### Dependency Tree

Render: `{sourceIcon} {name} {CS badge} {TK badge if available}`

### Role List Table

Add TK Status column after Compatibility column.

### Cookbook List Table

Add TK Status column after Compatibility column.

## Implementation Order

1. Backend: source + TK status on role chain nodes (queries)
2. Frontend: StatusBadge variants for CS/TK
3. Frontend: dependency tree with source icons + dual badges
4. Backend: TK status on role list
5. Frontend: TK column on role list
6. Backend: TK status on cookbook list
7. Frontend: TK column on cookbook list
8. Frontend: role detail summary cards with TK counts
