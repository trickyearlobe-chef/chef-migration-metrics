# Node Kitchen Archive — Component Specification

> **⛔ PARKED** — This feature is on hold. The Test Kitchen MVP (`test-kitchen-mvp.md`) focuses on validating overlay → converge → verify → destroy against real infrastructure (Proxmox, vSphere, EC2). Node Kitchen Archive may become a Test Kitchen plugin later. Do not start implementation work on this spec.

> **Implementation language:** Go. See `../../Claude.md` for language and concurrency rules.

> Component specification for the Node Kitchen Archive feature of Chef Migration Metrics.
> See the [top-level specification](../Specification.md) for project overview and scope.

---

## TL;DR

Adds a per-node API endpoint that generates and serves a downloadable `.tar.gz` archive containing a self-contained [Test Kitchen](https://kitchen.ci/) project. The archive is assembled from the node's collected `run_list`, resolved cookbooks (fetched from git repos and/or the Chef server), associated roles, and Policyfile metadata. The goal is to let an operator download a ready-to-run kitchen project that reproduces the node's Chef configuration locally, enabling hands-on migration testing and debugging outside the automated analysis pipeline.

Related specs: `data-collection.md`, `analysis.md`, `chef-api.md`, `web-api.md`, `configuration.md`, `datastore.md`.

---

## 1. Motivation

The existing analysis pipeline runs CookStyle and Test Kitchen automatically against individual git-sourced cookbooks. However, operators frequently need to:

1. **Test the full convergence** of a specific node's cookbook set together, not each cookbook in isolation.
2. **Debug upgrade failures** by reproducing the node's exact configuration in a local development environment.
3. **Experiment with fixes** — edit cookbooks locally, re-converge, and iterate without pushing changes to git or the Chef server.
4. **Share reproducible test cases** with team members who may not have direct access to the Chef server or the CMM dashboard.

A downloadable Test Kitchen archive solves all four by packaging everything needed into a single portable artifact.

---

## 2. Archive Contents

The archive is a gzip-compressed tar file (`.tar.gz`) with the following layout:

```
<node_name>-kitchen/
├── .kitchen.yml
├── Berksfile                   # (classic nodes only)
├── Policyfile.rb               # (Policyfile nodes only)
├── roles/
│   ├── base.json
│   └── webserver.json
├── data_bags/
│   └── example/
│       ├── item1.json
│       └── item2.json
├── cookbooks/
│   ├── nginx/                  # (git repo — full clone, directly editable)
│   │   ├── .git/
│   │   ├── .kitchen.yml
│   │   ├── metadata.rb
│   │   ├── recipes/
│   │   └── ...
│   ├── apt/                    # (server cookbook — reconstructed from manifest)
│   │   ├── metadata.rb
│   │   ├── recipes/
│   │   └── ...
│   └── ...
├── README.md
└── .gitignore
```

### 2.1 Root Directory

The archive root is a single directory named `<node_name>-kitchen` where `<node_name>` is the Chef node name with any characters unsafe for filesystem paths replaced by underscores.

### 2.2 `.kitchen.yml`

A generated Test Kitchen configuration file. The generation logic must produce a valid, runnable kitchen config:

```yaml
---
driver:
  name: dokken
  privileged: true

transport:
  name: dokken

provisioner:
  name: dokken
  product_name: "<chef or chef-ice>"   # see § 2.2.2
  product_version: "<node_chef_version>"

verifier:
  name: inspec

platforms:
  - name: <platform>-<platform_version>
    driver:
      image: <dokken_image>

suites:
  - name: <node_name>
    run_list:
      - "role[base]"
      - "recipe[nginx::default]"
    attributes: {}
```

#### Generation Rules

| Field | Source | Notes |
|-------|--------|-------|
| `driver.name` | Configurable, default `dokken` | Overridable via `kitchen_archive.default_driver` config |
| `provisioner.product_name` | Derived from effective Chef version (§ 2.2.2) | `chef` for versions < 19, `chef-ice` for versions ≥ 19 |
| `provisioner.product_version` | `node_snapshots.chef_version` | The node's current Chef Client version |
| `platforms[0].name` | `node_snapshots.platform` + `node_snapshots.platform_version` | e.g. `ubuntu-22.04`, `centos-8` |
| `platforms[0].driver.image` | Platform-to-image mapping (§ 2.2.1) | Dokken-compatible Docker image |
| `suites[0].name` | `node_snapshots.node_name` | Sanitised for kitchen suite naming (alphanumeric + hyphens) |
| `suites[0].run_list` | `node_snapshots.run_list` | The node's raw run_list, preserved exactly |

When a `target_chef_version` query parameter is provided, `provisioner.product_version` is set to the target version instead of the node's current version. This lets operators test the node's configuration against the version they plan to upgrade to. The `product_name` is also re-derived from the target version (not the node's current version) so the correct product is always used.

#### 2.2.2 Product Name Mapping

Starting with version 19, Chef Infra Client was rebranded from **Chef** to **Chef ICE**. The provisioner `product_name` must reflect this:

| Effective Chef Version | `product_name` |
|------------------------|----------------|
| < 19.0 | `chef` |
| ≥ 19.0 | `chef-ice` |

The **effective version** is the `target_chef_version` query parameter if provided, otherwise the node's `chef_version` from `node_snapshots`.

Version comparison uses **semver major version parsing** — extract the major version number from the version string (e.g. `18.5.0` → 18, `19.0.0` → 19) and compare against the threshold of 19. If the version string cannot be parsed (e.g. empty or malformed), default to `chef`.

#### 2.2.1 Platform-to-Image Mapping

A configurable map translates Chef platform names to Docker images suitable for `kitchen-dokken`. Default mappings:

| Platform | Image |
|----------|-------|
| `ubuntu` | `dokken/ubuntu-<version>` |
| `debian` | `dokken/debian-<version>` |
| `centos` | `dokken/centos-<version>` |
| `rocky` | `dokken/rockylinux-<version>` |
| `almalinux` | `dokken/almalinux-<version>` |
| `amazonlinux` | `dokken/amazonlinux-<version>` |
| `redhat` | `dokken/centos-<version>` |
| `oracle` | `dokken/oraclelinux-<version>` |
| `suse` | `dokken/opensuse-<version>` |
| `windows` | `dokken/windows-<version>` |
| (fallback) | `dokken/<platform>-<platform_version>` |

Operators can extend or override this mapping via the `kitchen_archive.platform_images` configuration key (see § 7).

### 2.3 Cookbooks

The archive includes **all cookbooks required to converge the node**. This is computed by starting from the node's resolved `cookbooks` attribute (`node_snapshots.cookbooks`) and then **recursively expanding transitive dependencies** declared in each cookbook's metadata. Each cookbook is placed under `cookbooks/<cookbook_name>/`.

#### Dependency Expansion

The node's `cookbooks` map gives the direct set. However, a cookbook's `metadata.rb` (or `metadata.json`) may declare dependencies on other cookbooks that are not explicitly listed in the node's `cookbooks` attribute (e.g. library cookbooks, resource cookbooks). These transitive dependencies must also be included so that the archive converges without external network access.

**Algorithm:**

1. Start with the **seed set** — all cookbook name + version pairs from `node_snapshots.cookbooks`.
2. For each cookbook in the set, look up its `dependencies` JSONB column from `server_cookbooks` (keyed by organisation + cookbook name + version). This contains entries like `{"apt": ">= 2.0", "yum": "~> 5.0"}`.
3. For each dependency name not already in the set, resolve the **best available version** — the version present in `server_cookbooks` for this organisation that satisfies the constraint. If multiple versions satisfy the constraint, prefer the highest version. Add it to the set.
4. Repeat until no new cookbooks are added (fixed-point).
5. If a dependency cannot be resolved (not present in `server_cookbooks` for any satisfying version), record a warning and continue — the cookbook will appear as "unavailable" in the archive.

**Note:** Git-sourced cookbooks do not have a `dependencies` column in `git_repos`. For git cookbooks, the dependency metadata is read from the cookbook's `metadata.rb` or `metadata.json` on disk in the local clone. If parsing fails, fall back to checking `server_cookbooks` for the same cookbook name, since the same cookbook may exist on both the Chef server and in git.

#### Source Priority

For each cookbook name + version in the expanded set:

1. **Git repo** — If a matching entry exists in `git_repos` for this cookbook name, include the **full git clone as-is** (including `.git/` directory). This allows operators to directly edit the cookbook, commit changes, push to the remote, and run `kitchen test` against individual cookbooks within the archive. The git repo is included at HEAD of the default branch.
2. **Chef server** — If no git repo exists, or the git repo fetch fails, download the cookbook version from the Chef server using the existing `chefapi.Client.DownloadFileContent()` mechanism (same as the server cookbook pipeline). The cookbook is reconstructed from the manifest file listing.
3. **Unavailable** — If neither source is available (e.g. download failure, missing repo), include a placeholder directory with a `README.md` explaining the cookbook could not be fetched. The archive generation must **not** fail — partial archives are acceptable.

#### Git Cookbook Inclusion

When including a git repo clone:

- Include the **entire directory as-is**, including `.git/`, `.kitchen.yml`, test suites, and all other files. This preserves the cookbook as a fully functional git working tree that operators can modify, commit, push, and test independently.
- Exclude only `.kitchen.local.yml` (transient overlay generated by CMM's analysis pipeline — not part of the cookbook itself).

### 2.4 Roles

All roles referenced by the node are included as JSON files under `roles/`. This includes:

1. **Direct roles** — Roles from `node_snapshots.roles` (the expanded role list).
2. **Run-list roles** — Roles parsed from `node_snapshots.run_list` using the existing `ParseRunListEntry()` function.
3. **Transitive roles** — Roles referenced by other roles (from the `role_dependencies` table where `dependency_type = 'role'`). The full transitive closure is computed.

Each role is serialised as a JSON file matching the Chef role format:

```json
{
  "name": "webserver",
  "description": "",
  "json_class": "Chef::Role",
  "chef_type": "role",
  "default_attributes": {},
  "override_attributes": {},
  "run_list": ["recipe[nginx::default]", "recipe[base::packages]"],
  "env_run_lists": {}
}
```

Role detail data (run_list, env_run_lists, attributes) must be fetched live from the Chef server API at archive generation time using `GET /organizations/<ORG>/roles/<ROLE_NAME>`. The `role_dependencies` table only stores dependency edges, not the full role content.

If a role cannot be fetched (e.g. it has been deleted from the Chef server since the last collection), a placeholder JSON file is written with a comment in the description field explaining the role data was unavailable.

### 2.5 Berksfile (Classic Nodes)

For nodes that are **not** Policyfile nodes (`policy_name` is empty), generate a `Berksfile` that points to the local `cookbooks/` directory:

```ruby
# Berksfile — generated by Chef Migration Metrics
# This Berksfile resolves all cookbooks from the local cookbooks/ directory.
# Modify paths or add sources as needed for your environment.

source chef_repo: "."

# Alternatively, if you want to resolve from a Chef Supermarket or internal server:
# source "https://supermarket.chef.io"
```

The Berksfile uses the local directory as the source so that kitchen can find the bundled cookbooks without network access.

### 2.6 Policyfile (Policyfile Nodes)

For nodes where `policy_name` is non-empty, generate a `Policyfile.rb`:

```ruby
# Policyfile.rb — generated by Chef Migration Metrics
# Policy: <policy_name>
# Policy Group: <policy_group>

name "<policy_name>"

default_source :chef_repo, "cookbooks/"

run_list <run_list entries as Ruby array>

# Cookbook versions pinned to match the node's resolved set:
cookbook "nginx", path: "cookbooks/nginx"
cookbook "apt", path: "cookbooks/apt"
```

Each cookbook in the node's `cookbooks` attribute gets a `cookbook` line with a `path:` directive pointing to the local copy.

**Note:** A `Policyfile.lock.json` is **not** generated because the lock file format includes content hashes and identifiers that cannot be reliably reconstructed from the data CMM collects. The `README.md` instructs the operator to run `chef install Policyfile.rb` if a lock file is needed.

### 2.7 `data_bags/`

A sample data bag is included to demonstrate the expected directory structure. Operators should replace or extend this with their own data bag items as needed.

The sample data bag is structured as:

```
data_bags/
└── example/
    ├── item1.json
    └── item2.json
```

**`data_bags/example/item1.json`:**

```json
{
  "id": "item1",
  "description": "This is a sample data bag item. Replace or remove this file.",
  "key1": "value1",
  "key2": "value2"
}
```

**`data_bags/example/item2.json`:**

```json
{
  "id": "item2",
  "description": "This is a second sample data bag item showing a different structure.",
  "users": ["alice", "bob"],
  "config": {
    "enabled": true,
    "timeout": 30
  }
}
```

The `README.md` instructs operators to replace the `example` data bag with their own data bags, or add additional data bags alongside it.

### 2.8 `README.md`

A generated README providing context and usage instructions:

```markdown
# Test Kitchen Archive — <node_name>

Generated by Chef Migration Metrics on <timestamp>.

## Node Details

| Property | Value |
|----------|-------|
| Node Name | <node_name> |
| Organisation | <organisation_name> |
| Environment | <chef_environment> |
| Chef Version | <chef_version> |
| Platform | <platform> <platform_version> |
| Policy Name | <policy_name or "N/A"> |
| Policy Group | <policy_group or "N/A"> |
| Stale | <yes/no> |
| Last Check-in | <ohai_time as human-readable> |

## Cookbooks (<count>)

| Cookbook | Version | Source | Included Via |
|---------|---------|--------|--------------|
| nginx | 5.1.0 | git | node run_list |
| apt | 7.4.0 | server | dependency of nginx |
| base | 1.3.2 | unavailable | node run_list |

## Quick Start

### Prerequisites

- Docker (for kitchen-dokken driver)
- Chef Workstation (`kitchen`, `berks` or `chef` CLI)

### Run Converge

```bash
cd <node_name>-kitchen
kitchen converge
```

### Run Full Test

```bash
kitchen test
```

### Clean Up

```bash
kitchen destroy
```

## Notes

- This archive reproduces the node's cookbook set as observed during the
  last collection run. It may not reflect changes made since then.
- Cookbooks sourced from git include the full `.git/` directory. You can
  `cd` into any git-sourced cookbook, make changes, commit, push, and run
  `kitchen test` directly against that individual cookbook.
- The `data_bags/` directory contains a sample data bag (`example/`).
  Replace it with your own data bags before converging, or add
  additional data bags alongside it.
- Cookbooks marked "unavailable" could not be fetched from either git
  or the Chef server. You may need to obtain them manually.
- The cookbook list includes transitive dependencies declared in each
  cookbook's metadata, not just the cookbooks directly in the node's
  run_list.
- <If Policyfile node>: Run `chef install Policyfile.rb` to generate a
  lock file if your workflow requires one.
```

### 2.9 `.gitignore`

A standard `.gitignore` for Test Kitchen projects:

```
.kitchen/
.kitchen.local.yml
Berksfile.lock
Policyfile.lock.json
*.gem
.bundle/
vendor/
```

---

## 3. API Endpoint

### 3.1 Download Node Kitchen Archive

#### `GET /api/v1/nodes/:organisation/:name/kitchen-archive`

**Requires: `viewer` role (or higher).**

Generates and streams a `.tar.gz` archive containing a Test Kitchen project for the specified node.

**Path parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `organisation` | string | Organisation name (URL-encoded if necessary) |
| `name` | string | Node name (URL-encoded if necessary) |

**Query parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `target_chef_version` | string | No | Override the Chef version in `.kitchen.yml`. If omitted, uses the node's current `chef_version`. |

**Response (200):**

```
Content-Type: application/gzip
Content-Disposition: attachment; filename="<node_name>-kitchen.tar.gz"
```

The response body is the raw gzip-compressed tar archive streamed directly to the client.

**Response (404):**

```json
{
  "error": "node not found",
  "detail": "No node snapshot found for organisation 'myorg' node 'web-node-99'."
}
```

**Response (500):**

```json
{
  "error": "archive generation failed",
  "detail": "Failed to fetch cookbook 'nginx' version '5.1.0' from Chef server: connection refused"
}
```

The 500 response is returned only for **fatal** errors that prevent archive generation entirely (e.g. database unavailable). Individual cookbook or role fetch failures produce a partial archive with placeholder entries — they do **not** cause a 500.

### 3.2 WebSocket Event

When archive generation begins and completes, emit events on the existing WebSocket event hub:

| Event | Payload |
|-------|---------|
| `kitchen_archive.started` | `{ "organisation": "...", "node_name": "...", "requested_by": "..." }` |
| `kitchen_archive.completed` | `{ "organisation": "...", "node_name": "...", "size_bytes": 12345, "cookbook_count": 8, "duration_ms": 2500 }` |
| `kitchen_archive.failed` | `{ "organisation": "...", "node_name": "...", "error": "..." }` |

---

## 4. Implementation Design

### 4.1 Package Structure

The archive generation logic lives in a new package: `internal/kitchenarchive/`.

```
internal/kitchenarchive/
├── archive.go          # Core archive builder (tar.gz assembly)
├── archive_test.go     # Unit tests
├── kitchen_yml.go      # .kitchen.yml template generation
├── kitchen_yml_test.go
├── berksfile.go        # Berksfile generation (classic nodes)
├── policyfile.go       # Policyfile.rb generation (Policyfile nodes)
├── readme.go           # README.md generation
├── roles.go            # Role fetching and JSON serialisation
├── roles_test.go
├── cookbooks.go        # Cookbook source resolution and copying
├── cookbooks_test.go
├── platform.go         # Platform-to-Docker-image mapping
└── platform_test.go
```

### 4.2 Core Types

**`Builder`** — The primary type in the package. Assembles a node-specific Test Kitchen archive. Holds references to the datastore, Chef API client, git repo reader, archive-specific configuration, and a logger.

**`Config`** — Archive-specific configuration passed into the builder:

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `DefaultDriver` | string | `"dokken"` | Test Kitchen driver name |
| `PlatformImages` | map of string → string | (see § 2.2.1) | Platform name → Docker image template |
| `GitCookbookDir` | string | (from storage config) | Filesystem path to local git clones |
| `IncludeTestDirs` | bool | `true` | Whether to include `test/` and `spec/` directories from git-sourced cookbooks |

**`ArchiveResult`** — Returned after archive generation with metadata:

| Field | Type | Description |
|-------|------|-------------|
| `SizeBytes` | int64 | Total archive size |
| `CookbookCount` | int | Number of cookbooks included |
| `RoleCount` | int | Number of roles included |
| `Duration` | duration | Wall-clock generation time |
| `Warnings` | string list | Non-fatal issues (missing cookbooks, unresolvable deps, etc.) |

### 4.3 DataStore Interface

The archive builder consumes a `DataStore` interface with the following methods (a subset of the full `datastore.DB`):

| Method | Purpose |
|--------|---------|
| `GetNodeSnapshotByOrgAndName` | Look up the target node |
| `GetOrganisation` / `GetOrganisationByName` | Resolve organisation for Chef API connection |
| `ListGitRepos` | Determine which cookbooks have git sources |
| `ListRoleDependenciesByOrganisation` | Compute transitive role closure |
| `ListServerCookbooksByOrganisation` | Read cookbook dependency metadata for transitive expansion |

### 4.4 ChefAPIClient Interface

The builder requires a Chef API client interface with these methods:

| Method | Purpose |
|--------|---------|
| `GetRoleDetail` | Fetch full role content (run_list, attributes) for inclusion in `roles/` |
| `GetCookbookVersionManifest` | Fetch the file listing for a server-sourced cookbook |
| `DownloadFileContent` | Download individual cookbook files by bookshelf URL |

### 4.5 GitRepoReader Interface

A single-method interface providing read access to locally cloned git cookbook repositories. Given a cookbook name, it returns the local filesystem path of the git clone (or empty string if not available).

### 4.6 Archive Generation Flow

1. **Resolve node** — Look up `node_snapshots` by organisation name + node name. Return 404 if not found.
2. **Parse node data** — Unmarshal the JSONB fields: `cookbooks` (map of name→version), `run_list` (string array), `roles` (string array).
3. **Resolve organisation** — Fetch the `organisations` row to obtain Chef server connection details for live role fetching.
4. **Expand cookbook dependencies** — Starting from the node's `cookbooks` map, load all `server_cookbooks` for the organisation and recursively resolve transitive dependencies from each cookbook's `dependencies` JSONB column. For git-sourced cookbooks, parse `metadata.rb`/`metadata.json` from the local clone. The result is the full expanded cookbook set (see § 2.3 "Dependency Expansion").
5. **Initialise tar writer** — Create a `*tar.Writer` wrapping a `*gzip.Writer` wrapping the HTTP response writer (streaming).
6. **Generate static files** — Write `.kitchen.yml`, `Berksfile` or `Policyfile.rb`, `README.md`, `.gitignore` directly to the tar stream.
7. **Fetch and write roles** — For each role referenced by the node (direct + transitive), fetch from Chef API and write as `roles/<name>.json`. Failures produce placeholder files.
8. **Fetch and write cookbooks** — For each cookbook in the expanded set, copy from git clone (full repo including `.git/`) or download from Chef server and write to `cookbooks/<name>/`. Cookbook fetches should run concurrently (bounded by a configurable worker pool, default: 4 workers). Results are collected and written to the tar stream sequentially (tar format requires sequential writes).
9. **Write `data_bags/` sample** — Add the `data_bags/example/` directory with sample items (see § 2.7).
10. **Finalise** — Close the tar and gzip writers. Emit WebSocket completion event.

#### Concurrency for Cookbook Fetching

Since tar writes must be sequential, cookbook data is fetched concurrently into an in-memory buffer (or temp directory), then written sequentially to the tar stream:

```
┌─────────────────────────────┐
│  Cookbook fetch worker pool  │
│  (bounded, default: 4)      │
│                             │
│  ┌──────┐ ┌──────┐         │
│  │ git  │ │server│ ...      │
│  │ copy │ │ dl   │         │
│  └──┬───┘ └──┬───┘         │
│     │        │              │
│     ▼        ▼              │
│  ┌─────────────────┐       │
│  │ results channel  │       │
│  └────────┬────────┘       │
└───────────┼────────────────┘
            │
            ▼
┌─────────────────────────────┐
│  Sequential tar writer      │
│  (single goroutine)         │
└─────────────────────────────┘
```

Each fetched cookbook is represented as a slice of `tarEntry` structs (header + data) that are sent through a channel to the writer goroutine. This preserves tar's sequential write requirement while parallelising the I/O-bound fetch operations.

### 4.7 Memory Management

For nodes with many large cookbooks, the in-memory approach could consume significant RAM. Mitigations:

1. **Streaming from git clones** — Git cookbook files (including `.git/` pack files) are read from disk and written to the tar stream without buffering the entire cookbook in memory.
2. **Chef server cookbook streaming** — Downloaded files are written to the tar stream as they arrive. The manifest is fetched first (small JSON), then each file is downloaded and streamed individually.
3. **Size limit** — A configurable maximum archive size (`kitchen_archive.max_archive_size_mb`, default: 500 MB) acts as a safety valve. If the running total exceeds this limit, remaining cookbooks are replaced with placeholder entries and a warning is added to the README.

### 4.8 Error Handling

| Failure | Behaviour |
|---------|-----------|
| Node not found | Return HTTP 404 |
| Database error | Return HTTP 500 |
| Chef API authentication failure | Return HTTP 500 with detail |
| Individual cookbook fetch failure (git or server) | Include placeholder, add warning to README, continue |
| Individual role fetch failure | Include placeholder JSON, add warning to README, continue |
| Archive size limit exceeded | Stop adding cookbooks, add warning to README, finalise archive |
| Client disconnects mid-stream | Log at WARN, clean up resources |

---

## 5. Web API Handler

The handler is registered in `internal/webapi/router.go` on the authenticated router group at the path defined in § 3.1.

The handler:

1. Extracts path parameters and validates them.
2. Reads the optional `target_chef_version` query parameter.
3. Constructs a `kitchenarchive.Builder` with the appropriate dependencies.
4. Sets response headers (`Content-Type`, `Content-Disposition`).
5. Calls `builder.Generate(ctx, w)` where `w` is the `http.ResponseWriter`.
6. Emits WebSocket events on start/completion/failure.

### 5.1 Response Headers

```
Content-Type: application/gzip
Content-Disposition: attachment; filename="web-node-01-kitchen.tar.gz"
Cache-Control: no-store
```

`Content-Length` is **not** set because the archive is streamed (size unknown at response start). Clients must handle chunked transfer encoding.

---

## 6. Frontend Integration

### 6.1 Node Detail Page

Add a **"Download Kitchen Archive"** button to the Node Detail page (`NodeDetailPage.tsx`), positioned alongside existing action buttons.

The button triggers a browser download via:

```
GET /api/v1/nodes/<organisation>/<name>/kitchen-archive
```

If the dashboard has a target Chef version selector active, the selected version is appended as `?target_chef_version=<version>`.

#### Button States

| State | Display |
|-------|---------|
| Idle | "Download Kitchen Archive" with a download icon |
| Downloading | Spinner with "Generating archive..." |
| Complete | Brief "Downloaded!" flash, then back to idle |
| Error | Toast notification with error message |

### 6.2 Bulk Download (Future)

A future enhancement could allow downloading archives for multiple nodes at once (e.g. all nodes in a role or environment). This is **out of scope** for the initial implementation but the API design does not preclude it.

---

## 7. Configuration

New configuration keys under the `kitchen_archive` section:

```yaml
kitchen_archive:
  # Default Test Kitchen driver. Must be a valid kitchen driver name.
  # Default: "dokken"
  default_driver: dokken

  # Map of Chef platform names to Docker image templates.
  # The string "<version>" is replaced with the node's platform_version.
  # Entries here are merged with (and override) the built-in defaults.
  platform_images:
    ubuntu: "dokken/ubuntu-<version>"
    centos: "dokken/centos-<version>"
    rocky: "dokken/rockylinux-<version>"

  # Whether to include test/ and spec/ directories from git-sourced cookbooks.
  # Default: true
  include_test_dirs: true

  # Maximum archive size in megabytes. If exceeded, remaining cookbooks are
  # replaced with placeholder entries.
  # Default: 500
  max_archive_size_mb: 500

  # Number of concurrent cookbook fetch workers during archive generation.
  # Default: 4
  cookbook_fetch_concurrency: 4
```

### 7.1 Config Struct

A `KitchenArchiveConfig` struct is added to `internal/config/config.go` with fields corresponding to each YAML key above (`default_driver`, `platform_images`, `include_test_dirs`, `max_archive_size_mb`, `cookbook_fetch_concurrency`). It is embedded in the top-level `Config` struct under the `kitchen_archive` YAML key. Defaults are applied during `config.Load()` if the section is absent or fields are zero-valued.

### 7.2 Environment Variable Overrides

| Config Key | Environment Variable |
|------------|---------------------|
| `kitchen_archive.default_driver` | `CMM_KITCHEN_ARCHIVE_DEFAULT_DRIVER` |
| `kitchen_archive.include_test_dirs` | `CMM_KITCHEN_ARCHIVE_INCLUDE_TEST_DIRS` |
| `kitchen_archive.max_archive_size_mb` | `CMM_KITCHEN_ARCHIVE_MAX_ARCHIVE_SIZE_MB` |
| `kitchen_archive.cookbook_fetch_concurrency` | `CMM_KITCHEN_ARCHIVE_COOKBOOK_FETCH_CONCURRENCY` |

`platform_images` is not overridable via environment variables due to its map structure. Use the YAML config file for this setting.

---

## 8. Database Impact

This feature requires **no new tables or migrations**. All data is read from existing tables:

| Table | Usage |
|-------|-------|
| `node_snapshots` | Node metadata, cookbooks, run_list, roles, policy_name, policy_group |
| `organisations` | Chef server connection details for live role/cookbook fetching |
| `git_repos` | Determine which cookbooks have git sources |
| `server_cookbooks` | Cookbook dependency metadata (`dependencies` JSONB) for transitive expansion |
| `role_dependencies` | Compute transitive role closure |

Role detail and Chef server cookbook content are fetched **live** from the Chef API at archive generation time — they are not cached in the database.

---

## 9. Logging

All archive generation activity is logged with scope `kitchen_archive`:

| Level | Event | Context Fields |
|-------|-------|----------------|
| `INFO` | Archive generation started | `organisation`, `node_name`, `target_chef_version` |
| `INFO` | Archive generation completed | `organisation`, `node_name`, `size_bytes`, `cookbook_count`, `role_count`, `duration_ms` |
| `WARN` | Cookbook fetch failed (non-fatal) | `organisation`, `node_name`, `cookbook_name`, `cookbook_version`, `source`, `error` |
| `WARN` | Cookbook dependency unresolvable | `organisation`, `node_name`, `parent_cookbook`, `dependency_name`, `version_constraint` |
| `WARN` | Role fetch failed (non-fatal) | `organisation`, `node_name`, `role_name`, `error` |
| `WARN` | Archive size limit exceeded | `organisation`, `node_name`, `current_size_mb`, `limit_mb`, `remaining_cookbooks` |
| `WARN` | Client disconnected mid-stream | `organisation`, `node_name`, `bytes_written` |
| `ERROR` | Archive generation failed (fatal) | `organisation`, `node_name`, `error` |

---

## 10. Testing Strategy

### 10.1 Unit Tests

| Test Area | What to Test |
|-----------|-------------|
| `.kitchen.yml` generation | Classic node, Policyfile node, custom driver, target version override, platform mapping (known + unknown platforms) |
| Product name mapping | `chef` for versions < 19 (e.g. `18.5.0`, `17.10.0`), `chef-ice` for versions ≥ 19 (e.g. `19.0.0`, `19.1.2`, `20.0.0`), fallback to `chef` for empty/malformed version strings, correct derivation from `target_chef_version` when provided (overrides node version) |
| Berksfile generation | Correct cookbook entries, local source path |
| Policyfile generation | Correct policy name, cookbook path entries |
| README generation | All fields populated, missing fields handled, cookbook source table |
| Role serialisation | Valid Chef role JSON, placeholder for missing roles |
| Platform mapping | All default platforms, custom overrides, fallback behaviour |
| Run-list parsing | Reuse existing `ParseRunListEntry` tests — verify integration |
| Archive assembly | Correct tar structure, file permissions, directory entries |
| Size limit enforcement | Archive truncated at limit, warning added |
| Cookbook source priority | Git preferred over server, fallback to placeholder |
| Dependency expansion | Transitive deps resolved from `server_cookbooks.dependencies` JSONB, fixed-point algorithm terminates, unresolvable deps produce warnings, circular deps handled |
| Git cookbook completeness | Full repo including `.git/` directory, `.kitchen.yml` preserved, only `.kitchen.local.yml` excluded |
| Sample data bags | Correct directory structure, valid JSON, both sample items present |

### 10.2 Integration Tests

Tagged with `//go:build functional`:

| Test | Description |
|------|-------------|
| Full archive round-trip | Generate archive from test fixtures, extract, verify structure and file contents |
| Chef API mock | Generate archive with mocked Chef API responses for roles and cookbooks |
| Git repo mock | Generate archive with a temporary git repo on disk |

### 10.3 Manual Testing Checklist

- [ ] Download archive for a classic (role/run-list) node
- [ ] Download archive for a Policyfile node
- [ ] Download archive with `target_chef_version` override
- [ ] Download archive where some cookbooks are unavailable
- [ ] Download archive where some roles are unavailable
- [ ] Verify the downloaded archive extracts cleanly
- [ ] Run `kitchen list` in the extracted directory
- [ ] Run `kitchen converge` successfully (requires Docker)
- [ ] Verify the README contains accurate node metadata
- [ ] Verify git-sourced cookbooks have working `.git/` (can `git log`, `git status`)
- [ ] Verify transitive cookbook dependencies are included
- [ ] Modify a git-sourced cookbook, commit, and run `kitchen test` on it
- [ ] Verify `data_bags/example/` contains both sample items with valid JSON

---

## 11. Security Considerations

1. **Path traversal** — Node names and cookbook names are used as directory names in the tar archive. All names must be sanitised to prevent path traversal attacks (no `..`, no absolute paths, no symlinks). Use `filepath.Clean()` and reject names containing `..` or starting with `/`.
2. **Credential exclusion** — The archive must **never** include Chef server credentials, encryption keys, or data bag secrets. Encrypted data bag items are not included (the `data_bags/` directory is empty).
3. **Access control** — The endpoint requires authentication and the `viewer` role. The node's organisation must match the user's accessible organisations (when organisation-level access control is implemented).
4. **Rate limiting** — Archive generation is CPU and I/O intensive. Consider rate limiting this endpoint (e.g. max 5 concurrent archive generations globally). This can be enforced via a semaphore in the handler.

---

## 12. Future Enhancements (Out of Scope)

The following are explicitly out of scope for the initial implementation but are noted for future consideration:

1. **Data bag inclusion** — Optionally fetching and including data bag items referenced by the node's cookbooks.
2. **Encrypted data bag support** — Including encrypted data bags with a user-supplied decryption key.
3. **Bulk archive download** — Generating archives for multiple nodes (e.g. all nodes matching a filter) as a single zip of tar.gz files.
4. **Vagrant/Hyper-V driver support** — Generating `.kitchen.yml` variants for non-Docker drivers.
5. **InSpec profile inclusion** — Bundling relevant InSpec compliance profiles.
6. **Environment-specific attributes** — Including Chef environment attributes that affect the node's convergence.
7. **Async generation with job tracking** — For very large node cookbook sets, generate the archive asynchronously using the existing `export_jobs` infrastructure.

---

## Related Specifications

- [Data Collection](data-collection.md) — Node and cookbook collection pipeline
- [Analysis](analysis.md) — Test Kitchen invocation model, CookStyle scanning
- [Chef API](chef-api.md) — Chef server API authentication and endpoints
- [Web API](web-api.md) — REST API conventions, authentication, error responses
- [Configuration](configuration.md) — YAML config schema, environment variable overrides
- [Datastore](datastore.md) — Database schema, node_snapshots table
- [Visualisation](visualisation.md) — Node detail page where the download button lives