# Git Repo File Browser — Component Specification

## Purpose

The Kitchen Config Analysis shows ~35 cookbooks "without TK" but provides no way to investigate *why* — users cannot easily see what's actually in those repos. This feature addresses that gap by:

1. Adding a **Kitchen filter** on the git repos list page so users can quickly isolate the repos that have no test suite.
2. Adding a **file browser** on the git repo detail page so users can explore a repo's contents directly in the UI without needing shell access to the server's git clone directory.

Together these let an operator filter to "Kitchen: No", click into a repo, switch to the Files tab, and immediately see what files exist — helping them understand whether a kitchen config is genuinely missing, misnamed, or the repo isn't a cookbook at all.

---

## Kitchen Filter (List Page)

### API Contract

**Endpoint:** `GET /api/v1/git-repos`

**New query parameter:**

| Param | Values | Behaviour |
|-------|--------|-----------|
| `has_test_suite` | `yes`, `no`, or `yes,no` | Filters repos by their `has_test_suite` boolean field. Comma-separated values are supported. When both `yes` and `no` are selected, no filtering is applied (equivalent to omitting the parameter). |

### Frontend Behaviour

- A `FilterMultiCheckbox` labelled "Kitchen" with options "Yes" and "No".
- Selecting "No" shows only repos where the collector found no kitchen config (no `.kitchen.yml`, `kitchen.yml`, `test/`, or `spec/` directory).
- Filter state participates in the existing clear-all, pagination reset, and URL search-param workflows.

---

## File Tree Endpoint

### `GET /api/v1/git-repos/:name/files`

Lists directory entries within the repo's local git clone.

**Query parameters:**

| Param | Default | Description |
|-------|---------|-------------|
| `path` | `.` | Relative path within the repo clone to list. |

**Response (200):**

```json
[
  { "name": "recipes", "type": "dir" },
  { "name": "metadata.rb", "type": "file", "size": 234 }
]
```

- `type` is `"dir"` or `"file"`.
- `size` is present only for files (bytes).
- The `.git` directory is always excluded from listings.

**Error responses:**

| Code | Condition |
|------|-----------|
| 400 | Path traversal attempt, `.git` access, or invalid repo name |
| 404 | Repo clone not found or path does not exist |
| 503 | Git clone directory not configured |

---

## File Content Endpoint

### `GET /api/v1/git-repos/:name/files/content`

Returns the content of a single file from the repo clone.

**Query parameters:**

| Param | Required | Description |
|-------|----------|-------------|
| `path` | Yes | Relative path to the file within the repo. |

**Response (200):**

```json
{
  "path": "recipes/default.rb",
  "encoding": "text",
  "content": "log 'hello'\n",
  "size": 12
}
```

- `encoding` is `"text"` for text files or `"base64"` for binary files.
- Binary detection: a file is considered binary if any null byte (`0x00`) exists in the first 512 bytes.
- `size` is the raw byte length (before any base64 encoding).

**Error responses:**

| Code | Condition |
|------|-----------|
| 400 | Missing path, path traversal, `.git` access, symlink, non-regular file, or directory path |
| 404 | File not found or repo clone missing |
| 413 | File exceeds 1 MB size limit |
| 503 | Git clone directory not configured |

---

## Security Constraints

All file-serving endpoints enforce:

1. **Repo name validation** — must be a single clean path component (`filepath.Base(name) == name`). Rejects traversal via the name segment.
2. **Path traversal prevention** — `filepath.Clean` + `filepath.Rel` containment check. The resolved path must remain within the repo directory.
3. **Symlink resolution** — after logical path validation, both the repo root and target are resolved with `EvalSymlinks` and containment is re-verified. Symlinks that escape the repo boundary are rejected.
4. **`.git` exclusion** — any path segment equal to `.git` is rejected for both list and content operations.
5. **Non-regular file rejection** — only regular files (not devices, sockets, pipes) are served by the content endpoint.
6. **Size cap** — files larger than 1 MB are rejected with HTTP 413 before reading content into memory.

---

## Frontend Behaviour (Detail Page)

- The git repo detail page gains tab navigation: **Overview** | **Files**.
- Overview tab contains all existing content (cookstyle results, complexity, kitchen instances, committers link).
- Files tab presents a two-panel layout:
  - **Left panel:** directory listing. Clicking a directory navigates into it. A ".. (parent)" entry navigates up. Files are clickable to load content.
  - **Right panel:** file viewer. Text files render in a `<pre><code>` block. Binary files show a "Binary file — content cannot be displayed as text" message. File path and size are shown in the header.
- The Files tab loads the root directory on first activation and retains navigation state within the tab session.

---

## Data Source

File content is served directly from the local filesystem clone at `cfg.Storage.GitCookbookDir/<repo-name>/`. This directory is populated by the git collector during collection cycles. If clone status is `failed` or `pending`, the Files tab will show a "clone not found" error from the API.
