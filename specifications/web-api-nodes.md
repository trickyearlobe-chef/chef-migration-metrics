# Web API — Node Endpoints

## Node Endpoints

### List Nodes

#### `GET /api/v1/nodes`

Returns a paginated, filterable list of nodes.

**Query parameters:** standard filters (including `policy_name`, `policy_group`, `stale_status`), pagination, sorting.

The `readiness` filter (scoped to the active `target_chef_version`) accepts the three node rollup states — `ready`, `needs_review`, `blocked` — mirroring the CookStyle rollup vocabulary (see [cop-classification.md](cop-classification.md)). `needs_review` only ever returns rows when the operator config `readiness.review_blocks_readiness` is on; with it off (default) no node is `needs_review` and the filter behaves as `ready`/`blocked` only — identical to today.

**Sortable fields:** `name`, `chef_version`, `platform`, `platform_version`, `chef_environment`, `policy_name`, `policy_group`, `last_collected_at`, `ohai_time`.

**Response (200):**

```json
{
  "data": [
    {
      "name": "web-node-01",
      "organisation": "myorg-production",
      "chef_environment": "production",
      "chef_version": "17.10.0",
      "platform": "ubuntu",
      "platform_version": "22.04",
      "platform_family": "debian",
      "roles": ["base", "webserver"],
      "policy_name": null,
      "policy_group": null,
      "cookbook_count": 12,
      "is_stale": false,
      "last_checkin_age_hours": 2.5,
      "last_collected_at": "2024-06-15T12:00:00Z"
    }
  ],
  "pagination": { ... }
}
```

### Get Node Detail

#### `GET /api/v1/nodes/:organisation/:name`

Returns full detail for a single node, including readiness status per target version, blocking reasons with complexity scores, Policyfile metadata, and stale data status.

Each per-target readiness row carries a three-state node rollup `status` — `ready` / `needs_review` / `blocked` (🟢/🟠/🔴) — mirroring the CookStyle rollup (see [cop-classification.md](cop-classification.md) and [analysis-node-readiness.md](analysis-node-readiness.md)). The boolean `ready` is retained for back-compat = `status == "ready"`. `needs_review` only occurs when `readiness.review_blocks_readiness` is on; with it off (default) no row is `needs_review` and the `ready` set is identical to today. CookStyle (CS) and Test Kitchen (TK) remain separate signals and are never merged — the per-cookbook `verdicts` array exposes each source independently.

**Response (200):**

```json
{
  "name": "web-node-01",
  "organisation": "myorg-production",
  "chef_environment": "production",
  "chef_version": "17.10.0",
  "platform": "ubuntu",
  "platform_version": "22.04",
  "platform_family": "debian",
  "roles": ["base", "webserver"],
  "policy_name": null,
  "policy_group": null,
  "run_list": ["role[base]", "recipe[nginx::default]", "recipe[nginx::config]"],
  "cookbooks": {
    "nginx": "5.1.0",
    "base": "1.3.2",
    "apt": "7.4.0"
  },
  "disk_space": {
    "root_partition_free_mb": 4096,
    "threshold_mb": 2048,
    "sufficient": true
  },
  "is_stale": false,
  "last_checkin_age_hours": 2.5,
  "ohai_time": "2024-06-15T09:30:00Z",
  "readiness": [
    {
      "target_chef_version": "18.5.0",
      "status": "ready",
      "ready": true,
      "stale_data": false,
      "blocking_reasons": [],
      "review_reasons": []
    },
    {
      "target_chef_version": "19.0.0",
      "status": "blocked",
      "ready": false,
      "stale_data": false,
      "review_reasons": [],
      "blocking_reasons": [
        {
          "type": "incompatible_cookbook",
          "cookbook_name": "nginx",
          "cookbook_version": "5.1.0",
          "detail": "Server version incompatible; git version compatible — upload git version to Chef Server",
          "complexity_score": 30,
          "complexity_label": "medium",
          "verdicts": [
            {
              "source": "git_test_kitchen",
              "status": "compatible",
              "version": "HEAD",
              "commit_sha": "a1b2c3d4e5f6"
            },
            {
              "source": "git_cookstyle",
              "status": "compatible",
              "version": "HEAD",
              "commit_sha": "a1b2c3d4e5f6",
              "complexity_score": 0,
              "complexity_label": "low"
            },
            {
              "source": "server_cookstyle",
              "status": "incompatible",
              "version": "5.1.0",
              "complexity_score": 30,
              "complexity_label": "medium"
            }
          ]
        }
      ]
    }
  ],
  "last_collected_at": "2024-06-15T12:00:00Z"
}
```

**Readiness row notes:**

- `status` is the canonical three-state verdict (`ready` / `needs_review` / `blocked`); `ready` (boolean) is a derived convenience = `status == "ready"`.
- `review_reasons` mirrors the shape of `blocking_reasons` and lists cookbooks resolved to `needs_review`. It is populated only when `readiness.review_blocks_readiness` is on; otherwise it is an empty array and no row has `status == "needs_review"`.
- The authoritative persisted shape lives in the node-readiness record (see [analysis-node-readiness.md](analysis-node-readiness.md)); this response is its read projection.

### Get Node Disk Detail

#### `GET /api/v1/nodes/disks/:organisation/:name`

Returns parsed filesystem/disk data from the node's Ohai filesystem attribute. Data is extracted from the `by_mountpoint` key of the Ohai 14+ filesystem format.

**Query Parameters:**

| Parameter | Type    | Default | Description |
|-----------|---------|---------|-------------|
| `show_all`| boolean | `false` | When `true`, includes virtual/pseudo filesystems (proc, sysfs, tmpfs, squashfs, cgroup, etc.) |

**Response (200):**

```json
{
  "node_name": "pandora.home.arpa",
  "organisation_name": "prod",
  "platform": "ubuntu",
  "disks": [
    {
      "mount": "/",
      "device": "/dev/nvme0n1p1",
      "fs_type": "ext4",
      "kb_size": 120984300,
      "kb_used": 22104852,
      "kb_available": 92687576,
      "percent_used": 20,
      "uuid": "11e08d25-aae5-49af-9b08-a302795df03f",
      "mount_options": ["rw", "relatime"],
      "inodes_used": 234976,
      "total_inodes": 7725056,
      "inodes_available": 7490080,
      "inodes_percent_used": 4
    },
    {
      "mount": "/boot/efi",
      "device": "/dev/nvme0n1p10",
      "fs_type": "vfat",
      "kb_size": 64511,
      "kb_used": 110,
      "kb_available": 64402,
      "percent_used": 0
    }
  ]
}
```

Windows nodes include additional fields per disk entry:

```json
{
  "node_name": "win11-001.home.arpa",
  "organisation_name": "prod",
  "platform": "windows",
  "disks": [
    {
      "mount": "C:",
      "device": "",
      "fs_type": "ntfs",
      "kb_size": 41949327,
      "kb_used": 36166840,
      "kb_available": 5782487,
      "percent_used": 86,
      "drive_type": "Local Fixed Disk",
      "volume_name": "",
      "encryption_status": "FullyEncrypted"
    }
  ]
}
```

**Behaviour notes:**

- `percent_used` is parsed from the raw Ohai value (which may include a `%` suffix on Linux); if parsing fails, it is computed from `kb_used / kb_size`.
- `inodes_percent_used` prefers the raw Ohai `inodes_percent_used` value; falls back to computing from `inodes_used / total_inodes`.
- Virtual/pseudo filesystems (proc, sysfs, tmpfs, squashfs, cgroup, bpf, devtmpfs, etc.) and virtual mount paths (`/sys/`, `/proc/`, `/dev/`, `/run/`) are filtered out by default.
- Disk entries are sorted by mount path.
- If the node has no filesystem data, `disks` is an empty array.

### List Nodes by Chef Version

#### `GET /api/v1/nodes/by-version/:chef_version`

Returns nodes running a specific Chef Client version. This supports drill-down from the version distribution view.

**Query parameters:** standard filters, pagination.

**Response:** Same structure as `GET /api/v1/nodes`.

### List Nodes by Cookbook

#### `GET /api/v1/nodes/by-cookbook/:cookbook_name`

Returns nodes running a specific cookbook (any version).

**Additional query parameter:** `cookbook_version` (optional, filters to a specific version).

**Query parameters:** standard filters, pagination.

**Response:** Same structure as `GET /api/v1/nodes`.

---
