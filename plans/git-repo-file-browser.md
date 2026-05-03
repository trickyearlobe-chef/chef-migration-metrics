# Git Repo File Browser & Kitchen Filter

## Goal

Add two features to help investigate repos detected as having no kitchen:

1. **Kitchen filter on git list page** — FilterMultiCheckbox for "has_test_suite" (Yes/No) so users can quickly find repos without kitchen configs.
2. **File browser tab on git detail page** — New "Files" tab showing the repo's file tree with click-to-expand directories and click-to-view file contents (binary-safe).

## Specs to Read

- `.claude/specifications/web-api.md` (endpoint patterns)
- `.claude/specifications/project-conventions.md` (Go/frontend conventions)

## Steps

### 1. Backend: Add `has_test_suite` filter to git repos list

- `internal/webapi/handle_git_repos.go` — parse `has_test_suite` query param ("yes"/"no"), filter repos accordingly.
- `internal/webapi/handle_git_repos_test.go` — add test for the new filter.
- `frontend/src/api/client.ts` — add `has_test_suite` to `GitRepoFilterQuery`.

### 2. Frontend: Add kitchen filter to GitReposPage

- `frontend/src/pages/GitReposPage.tsx` — add FilterMultiCheckbox for "Kitchen" with Yes/No options.

### 3. Backend: Add file tree endpoint

- New handler: `GET /api/v1/git-repos/:name/files?path=` — returns directory listing (entries with name, type, size).
- Security: Validate path stays within repo dir (no traversal).
- Register route in router.

### 4. Backend: Add file content endpoint

- New handler: `GET /api/v1/git-repos/:name/files/content?path=` — returns file content.
- Binary detection: check for null bytes in first 512 bytes. If binary, return base64-encoded with `encoding: "base64"`. Otherwise return raw text with `encoding: "text"`.
- Size limit: cap at 1MB, return error for larger files.

### 5. Frontend: Add Files tab to GitRepoDetailPage

- Add tab navigation (existing content becomes "Overview" tab).
- "Files" tab shows tree component: expandable directories, clickable files.
- File viewer: syntax highlighting for text, "Binary file" message for base64 content.

## Acceptance Criteria

- Filtering git repos list by "Kitchen: No" shows only repos without test suite.
- Git detail page has a "Files" tab showing the repo's file hierarchy.
- Clicking a file shows its content (text rendered, binary shows message).
- Backend tests pass for new filter and file endpoints.
- Path traversal is prevented in file endpoints.
