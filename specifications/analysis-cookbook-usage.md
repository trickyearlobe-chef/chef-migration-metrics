# Analysis — Cookbook Usage Analysis

### 1. Cookbook Usage Analysis

Derives cookbook usage statistics from collected node data.

From each node's `automatic.cookbooks` attribute (the resolved, deduplicated cookbook map), determine:

- Which cookbooks are in active use across the fleet, and which exist on the Chef server but are applied to no nodes
- Which specific versions of those cookbooks are in use
- Which roles reference those cookbooks
- Which Policyfile policy names and policy groups reference those cookbooks
- Which nodes are running each cookbook and version
- How many nodes are running each cookbook and version
- How many platform versions and platform families are running each cookbook and version
- Whether each cookbook version is **actively used** or **unused** — stored as a flag per cookbook version to support dashboard filtering

#### Concurrency

- Cookbook usage statistics must be computed by fanning out over the collected node records using goroutines, bounded by the `concurrency.readiness_evaluation` worker pool setting (see [Configuration Specification](../configuration/Specification.md)).
- Aggregation of results (node counts, platform counts, active/unused flags) must be performed safely using channels or mutex-protected accumulators.

#### Design: Computation Steps

The usage analysis runs after each node collection cycle completes and proceeds in three phases:

**Phase 1 — Per-node extraction (parallel)**

For each collected node, extract the following tuples from the `automatic.cookbooks` map:

- `(organisation, cookbook_name, cookbook_version, node_name, platform, platform_version, platform_family, chef_environment, roles[], policy_name, policy_group)`

For Policyfile nodes (where `policy_name` and `policy_group` are non-null), `roles` may be empty — the policy name and policy group serve as the primary grouping dimensions instead.

Fan out across nodes using a worker pool bounded by `concurrency.readiness_evaluation`. Each goroutine sends its extracted tuples to a shared results channel.

**Phase 2 — Aggregation (single goroutine)**

A dedicated aggregation goroutine reads from the results channel and builds the following in-memory maps:

| Map | Key | Value |
|-----|-----|-------|
| Node count per cookbook version | `(org, cookbook, version)` | count of distinct nodes |
| Nodes per cookbook version | `(org, cookbook, version)` | set of node names |
| Roles per cookbook | `(org, cookbook)` | set of role names (from each node's `roles` attribute where that cookbook appears) |
| Policy names per cookbook | `(org, cookbook)` | set of policy names (from Policyfile nodes where that cookbook appears) |
| Policy groups per cookbook | `(org, cookbook)` | set of policy groups (from Policyfile nodes where that cookbook appears) |
| Platform count per cookbook version | `(org, cookbook, version, platform, platform_version)` | count of nodes |
| Platform family count per cookbook version | `(org, cookbook, version, platform_family)` | count of nodes |

**Phase 3 — Active/unused flagging**

After aggregation, compare the set of cookbook versions observed across all nodes against the full cookbook version inventory fetched from the Chef server (via `GET /organizations/<ORG>/cookbooks?num_versions=all`). Any cookbook version present on the server but absent from the aggregated node data is flagged as **unused**.

**Persistence**

All aggregated results and active/unused flags are written to the datastore in a single transaction at the end of the analysis run. The previous analysis snapshot is retained (not overwritten) to support historical trending.
