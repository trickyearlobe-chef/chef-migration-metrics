# Analysis — Node Upgrade Readiness

### 5. Node Upgrade Readiness

Computes a readiness status per node per target Chef Client version.

A node is considered **ready** when ALL of the following are true:

1. All cookbooks in the node's expanded run-list are compatible with the target Chef Client version (passing Test Kitchen results)
2. Sufficient disk space is available on the node to install the Habitat-packaged Chef Client bundle (including bundled InSpec)

Blocking reasons must be recorded per node (e.g. specific incompatible cookbooks, insufficient disk space) for display in the dashboard.

Readiness status is computed and persisted after each collection and testing cycle.

#### Concurrency

- Readiness computation is independent per node per target Chef Client version. Nodes must be evaluated in parallel using goroutines, bounded by the `concurrency.readiness_evaluation` worker pool setting (see [Configuration Specification](configuration.md)).

---

#### Design: Disk Space Evaluation

The `automatic.filesystem` attribute collected from each node contains a map of mounted filesystems with size and availability information. The structure varies by platform:

**Linux nodes:**

```json
{
  "filesystem": {
    "/dev/sda1": {
      "kb_size": "20511356",
      "kb_used": "5123456",
      "kb_available": "14340800",
      "percent_used": "26%",
      "mount": "/"
    },
    "/dev/sdb1": {
      "kb_size": "102400000",
      "kb_used": "50000000",
      "kb_available": "47360000",
      "percent_used": "51%",
      "mount": "/opt"
    }
  }
}
```

**Windows nodes:**

```json
{
  "filesystem": {
    "C:": {
      "kb_size": "104857600",
      "kb_used": "52428800",
      "kb_available": "52428800",
      "percent_used": "50%"
    }
  }
}
```

**Evaluation algorithm:**

1. **Determine the installation target path and size.** Use the configured values for the node's platform:
   - Linux: path = `readiness.install_path_linux` (default: `/hab`), size = `readiness.install_size_mb_linux` (default: 3072 MB)
   - Windows: path = `readiness.install_path_windows` (default: `C:\hab`), size = `readiness.install_size_mb_windows` (default: 6144 MB)

2. **Find the matching filesystem entry.** Iterate through the `filesystem` map and find the entry whose `mount` value is the longest prefix match for the installation path. For example:
   - If `/apps/hab` is a mount point, use that entry
   - If `/apps/hab` is not mounted separately but `/apps` is, use `/apps`
   - If neither is a separate mount, fall back to `/`

   For Windows nodes, match on the drive letter of the configured path.

3. **Extract space values.** From the matched filesystem entry:
   - `kb_available` — current free space
   - `kb_size` — total filesystem capacity

4. **Apply dual threshold.** Both conditions must pass:
   - **Absolute:** `kb_available / 1024 >= install_size_mb` (platform-specific) — enough space for the install
   - **Percentage:** `(kb_available - install_size_kb) / kb_size >= readiness.min_remaining_free_percent / 100` — at least the configured % of total capacity remains free after the install is allocated

   If either condition fails, the node is blocked with a reason indicating which check failed.

**Edge cases:**

- If the `filesystem` attribute is missing or empty (e.g. the node has not completed a recent Chef run), the disk space check is recorded as **unknown** rather than pass or fail. The dashboard must display this as a distinct state.
- If `kb_available` is missing from a filesystem entry, treat that filesystem as having 0 KB available.
- Values in the `filesystem` map are strings in some Chef Client versions and integers in others. The implementation must handle both.

**Version invariance and cross-view consistency:**

- The disk verdict depends only on the node's platform (install path/size) and its filesystem free space — it does **not** depend on the target Chef Client version. The verdict is therefore identical across every per-target readiness row for a node.
- Consequently, all views (node list, node detail, filters, exports) MUST surface the same disk status for a given node, resolved **independently of the selected target version**. A view must not report disk status as unknown merely because the selected target version has no readiness row — it must fall back to the node's disk verdict from any available readiness row.
- "Unknown" (evaluated, but disk indeterminate — missing/stale filesystem data) is a distinct state from "not evaluated for the selected target". Filters and badges must not conflate the two (e.g. a `LEFT JOIN ... IS NULL` that maps an absent row to the same value as an indeterminate verdict).

---

#### Design: Stale Node Handling

Nodes flagged as **stale** by the data collection component (see [Data Collection Specification](data-collection.md)) require special handling during readiness evaluation:

- Stale nodes are still evaluated for readiness, but their disk space data is treated as **unknown** (same as missing filesystem data) since the data may be outdated.
- The readiness result for stale nodes includes an additional `stale_data` flag set to `true`.
- The dashboard must surface stale nodes distinctly so that operators can prioritise getting these nodes to check in before attempting an upgrade.

---

#### Design: Cookbook Compatibility Evaluation

For each node, the readiness evaluator must determine whether **all** cookbooks in the node's resolved cookbook list are compatible with the target Chef Client version.

**Algorithm per node per target Chef Client version:**

1. **Get the node's cookbook list.** Read the `automatic.cookbooks` attribute, which is a map of `cookbook_name → { version, ... }`.

2. **For each cookbook + version in the map, check ALL available sources:**

   a. **Check for a git repo Test Kitchen result.** Query the datastore for the most recent test result where:
      - `cookbook_name` matches (via `git_repos.name`)
      - `target_chef_version` matches
      - Result exists (pass or fail)

      Record the verdict: `compatible` if `converge_passed = true` AND `verify_passed = true`, otherwise `incompatible`.

   b. **Check for a git repo CookStyle result.** Query the datastore for a CookStyle result where:
      - `cookbook_name` matches (via `git_repos.name`)
      - `target_chef_version` matches

      Record the verdict: `compatible` if `passed = true`, otherwise `incompatible`. If no result exists, record `untested`.

   c. **Check for a server cookbook CookStyle result.** Query the datastore for a CookStyle result where:
      - `organisation` matches the node's organisation
      - `cookbook_name` and `cookbook_version` match the version on the node
      - `target_chef_version` matches (also try with no target version as fallback)

      Record the verdict: `compatible` if `passed = true`, otherwise `incompatible`. If no result exists, record `untested`.

   d. **Compute the overall status from all verdicts:**

      | Scenario | Overall Status | Blocks Readiness? |
      |----------|---------------|-------------------|
      | Any source says `compatible` | `compatible` | No |
      | All tested sources say `incompatible` (none compatible) | `incompatible` | **Yes** |
      | No sources have results at all | `untested` | **Yes** |

      > **Policy note:** If the server version is incompatible but the git version is compatible, the node is still considered **ready** because a compatible version exists and can be uploaded. The per-source verdicts make it clear what action is needed.

   e. **Record per-source verdicts.** Each cookbook in the blocking list (or non-blocking list) carries a `verdicts` array with one entry per source checked:

      | Field | Description |
      |-------|-------------|
      | `source` | `server_cookstyle`, `git_cookstyle`, or `git_test_kitchen` |
      | `status` | `compatible`, `incompatible`, or `untested` |
      | `version` | The version that was tested (server version for server CookStyle, `HEAD` for git sources) |
      | `commit_sha` | Git HEAD SHA (git sources only, empty for server) |
      | `complexity_score` | Complexity score from the matching source (0 if not available) |
      | `complexity_label` | Complexity label from the matching source (empty if not available) |

3. **Classify the cookbook with confidence level:**

   | Overall Status | Confidence | Meaning |
   |---------------|------------|---------|
   | `compatible` (Test Kitchen) | High | Full integration test passed |
   | `compatible` (CookStyle only — git or server) | Medium | Static analysis only — no integration test |
   | `incompatible` | N/A | All tested sources report incompatibility |
   | `untested` | N/A | No test or scan results from any source |

4. **Aggregate blocking reasons.** Collect the list of cookbooks that are `incompatible` or `untested`. Each entry in the blocking list records:
   - Cookbook name and version (the version on the node)
   - Overall reason (`incompatible` or `untested`)
   - Primary source (the source that determined the overall status)
   - Complexity score and label (from the highest-confidence source, if available)
   - Per-source verdicts array (all sources checked with their individual results)

5. **Combine with disk space result.** The node is **ready** only if:
   - The cookbook blocking list is empty, AND
   - Disk space is sufficient

   > **Note on unknown disk space:** Nodes with unknown disk space status are classified as **blocked (unknown disk space)** to err on the side of caution.

**Persistence:**

Write one readiness record per node per target Chef Client version:

| Field | Value |
|-------|-------|
| `organisation` | Chef server organisation name |
| `node_name` | Node name |
| `target_chef_version` | Target Chef Client version |
| `ready` | Boolean |
| `disk_space_available_mb` | Available MB on the installation mount (null if unknown) |
| `disk_space_sufficient` | Boolean or null (unknown) |
| `blocking_cookbooks` | JSON array of `{ name, version, reason, source, complexity_score, complexity_label, verdicts: [{ source, status, version, commit_sha, complexity_score, complexity_label }] }` |
| `stale_data` | Boolean — true if the node's last check-in exceeds the stale threshold |
| `evaluated_at` | UTC timestamp |
