# Kitchen Instance Exclusions — Specification

## TL;DR

Allow operators to manually exclude specific suite+platform instances from Test Kitchen runs on a per-repo basis. Each exclusion records a user-supplied reason, creating an audit trail of known-failing combinations and why they are intentionally skipped.

## Problem

Some kitchen instances fail for reasons unrelated to cookbook quality — e.g. hardcoded Vagrant-specific IPs, driver-specific networking requirements, unsupported OS/hypervisor combinations. Currently the only options are:

1. Let them fail repeatedly (noise, wasted resources)
2. Remove the platform from the global platform map (affects all repos)
3. Mark the platform as `skip=true` globally (same problem)

None of these let an operator say "this specific instance in this specific repo is a known issue because X" without affecting other repos.

## Scope

### In Scope

- Database table for per-repo instance exclusions
- Exclusion check in the planner (new status: `user_excluded`)
- API endpoints: create, list, delete exclusions
- Frontend: view exclusions on git kitchen results page, create exclusion from a failed result, delete exclusion
- Exclusion reason displayed in results/reports

### Out of Scope

- Bulk exclusion import/export
- Auto-detection of which instances should be excluded
- Expiry/review dates on exclusions (can be added later)
- Exclusion patterns (regex matching) — only exact suite+platform per repo

## Database

### New Table: `kitchen_instance_exclusions`

| Column | Type | Nullable | Description |
|--------|------|----------|-------------|
| `id` | UUID | No | Primary key |
| `git_repo_name` | TEXT | No | Repository name |
| `git_repo_url` | TEXT | No | Repository URL |
| `suite_name` | TEXT | No | Kitchen suite name |
| `platform_name` | TEXT | No | Kitchen platform name |
| `reason` | TEXT | No | User-provided explanation |
| `excluded_by` | TEXT | No | Username of the operator |
| `created_at` | TIMESTAMPTZ | No | When the exclusion was created |

**Unique constraint:** `(git_repo_name, git_repo_url, suite_name, platform_name)` — one exclusion per instance per repo.

**Index:** `idx_kie_repo` on `(git_repo_name, git_repo_url)` for per-repo lookups.

Migration number: next available (currently 0018).

## Planner Integration

### New Status

Add `InstanceStatusUserExcluded` to the planner:

```
InstanceStatusUserExcluded InstanceStatus = "user_excluded"
```

### Exclusion Check

`PlanRepo` gains a new parameter: a slice of exclusions for the repo. After the existing exclude/skip/map logic, if an instance matches an exclusion record, it is marked `user_excluded` with the exclusion reason as `StatusReason`.

Precedence (evaluated in order):
1. Suite-level excludes (from kitchen.yml) → `excluded`
2. Platform skip (from platform map) → `skipped`
3. User exclusion (from DB) → `user_excluded`
4. Unmapped platform → `unmapped`
5. Otherwise → `mapped`

User exclusions are checked AFTER suite excludes and platform skip but BEFORE unmapped — this means a user exclusion on an unmapped platform still shows as `unmapped` (the exclusion has no practical effect since it wouldn't run anyway).

### PlanResult Counts

Add `UserExcluded int` to `PlanResult` alongside `Skipped` and `Excluded`.

## API

### GET /api/v1/kitchen/git/exclusions?repo=\<name\>

Returns exclusions for a repo. If `repo` is omitted, returns all exclusions.

Response: `[]KitchenInstanceExclusion`

### POST /api/v1/kitchen/git/exclusions

Creates a new exclusion. Requires admin role.

Request body:

```
{
  "git_repo_name": "kubernetes-cluster",
  "git_repo_url": "https://git.example.com/cookbooks/kubernetes-cluster",
  "suite_name": "ha-cluster-k8s135-cp1",
  "platform_name": "ubuntu-22.04",
  "reason": "Suite hardcodes control_plane_endpoint 192.168.56.10 which requires Vagrant private_network. Not compatible with Proxmox/vSphere single-NIC provisioning."
}
```

Response: `201 Created` with the created exclusion record.

Validation:
- All fields required and non-empty
- Duplicate returns `409 Conflict`

### DELETE /api/v1/kitchen/git/exclusions/\<id\>

Removes an exclusion (re-enables the instance for future runs). Requires admin role.

Response: `204 No Content`

## Frontend

### Git Kitchen Results Page

- Instances with status `user_excluded` display with a distinct badge (e.g. amber "excluded" with tooltip showing reason)
- An "Exclude" action button on failed instances opens a modal/dialog to enter the reason
- Excluded instances show the reason text inline or on hover
- A section/tab showing current exclusions for the repo with a "Remove" button

### Exclusion Dialog

- Pre-fills repo name, suite, and platform from the failed instance
- Text area for reason (required, minimum 10 characters)
- Submit creates via POST, refreshes the instance list

## Behaviour

- Exclusions are evaluated at plan time, not at run time — if you add an exclusion while a run is in progress, it takes effect on the next run
- Removing an exclusion means the instance will be attempted on the next run
- Existing results for an excluded instance are preserved (not deleted) — the historical failure record remains
- The exclusion reason is informational only — it does not feed into readiness scoring

## Error Handling

- Creating an exclusion for a non-existent repo/suite/platform is allowed (the repo might not have been analysed yet, or the kitchen.yml might change)
- Deleting a non-existent exclusion returns `404`
