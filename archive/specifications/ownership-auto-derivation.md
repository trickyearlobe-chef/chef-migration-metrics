# Ownership — Auto-Derivation Rules

## 2. Auto-Derivation Rules

Auto-derivation rules allow ownership to be assigned automatically based on patterns in Chef data. Rules are evaluated after each collection run and produce `auto_rule` / `inferred` assignments.

### 2.1 Rule Types

| Rule Type | Description | Example |
|-----------|-------------|---------|
| `node_attribute` | Match nodes by a value at a configurable attribute path in the node's collected data | Assign all nodes with `automatic.cloud.provider = "aws"` to owner `cloud-team` |
| `node_name_pattern` | Match nodes by a regex pattern on the node name | Assign all nodes matching `^web-prod-.*` to owner `web-platform` |
| `policy_match` | Match nodes by policy name (exact or pattern) | Assign all nodes with `policy_name = "payment-app"` to owner `payments-team` |
| `cookbook_name_pattern` | Match cookbooks by a regex pattern on the cookbook name | Assign all cookbooks matching `^acme-.*` to owner `acme-platform` |
| `git_repo_url_pattern` | Match git repositories by a regex pattern on the repository URL | Assign all repos matching `gitlab.example.com/team-web/.*` to owner `web-platform` |
| `role_match` | Match roles by name (exact or pattern) | Assign role `web-server` to owner `web-platform` |

### 2.2 Rule Configuration

Auto-derivation rules are defined in the YAML configuration file:

```yaml
ownership:
  enabled: true
  auto_rules:
    - name: aws-nodes-to-cloud-team
      owner: cloud-team
      type: node_attribute
      attribute_path: automatic.cloud.provider
      match_value: "aws"

    - name: web-prod-nodes
      owner: web-platform
      type: node_name_pattern
      pattern: "^web-prod-.*"

    - name: payment-policy
      owner: payments-team
      type: policy_match
      policy_name: "payment-app"

    - name: acme-cookbooks
      owner: acme-platform
      type: cookbook_name_pattern
      pattern: "^acme-.*"

    - name: web-team-repos
      owner: web-platform
      type: git_repo_url_pattern
      pattern: "gitlab\\.example\\.com/team-web/.*"

    - name: web-server-role
      owner: web-platform
      type: role_match
      pattern: "web-server"
```

**Rule fields:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Unique name for the rule (used in `ownership_assignments.auto_rule_name`) |
| `owner` | string | Yes | Name of the owner to assign. Must match an existing owner. |
| `type` | string | Yes | One of the rule types from § 2.1 |
| `attribute_path` | string | Conditional | Dot-separated path into the node's collected attributes (required for `node_attribute` type) |
| `match_value` | string | Conditional | Value to match at the attribute path (required for `node_attribute` type). Supports exact match only. |
| `pattern` | string | Conditional | Regex pattern (required for `node_name_pattern`, `cookbook_name_pattern`, `git_repo_url_pattern`, `role_match` types; optional for `policy_match`) |
| `policy_name` | string | Conditional | Exact policy name to match (required for `policy_match` type) |
| `organisation` | string | No | Limit this rule to a specific organisation name. If omitted, the rule applies across all organisations. |

### 2.3 Rule Evaluation

- Auto-derivation rules are evaluated **after each collection run completes** for the affected organisation(s).
- For each rule, the system queries the latest node snapshots (or cookbooks, git repo URLs, or roles depending on rule type) and generates `ownership_assignments` for matching entities.
- For `git_repo_url_pattern` rules, the system queries the distinct `git_repo_url` values from the `cookbooks` table where `source = 'git'` and matches the regex against each URL.
- Existing `auto_rule` assignments from the same rule that no longer match are **deleted** (the entity no longer matches the pattern). This ensures auto-derived ownership stays in sync with the current state.
- Existing `manual` or `import` assignments are **never modified or deleted** by auto-derivation. Manual assignments always take precedence.
- Rule evaluation must be logged at `DEBUG` severity with the `ownership` scope, including the rule name, match count, and any errors.
- If a rule references an owner name that does not exist, the rule is skipped and a `WARN` log is emitted.

### 2.4 Node Attribute Access for Auto-Derivation

For `node_attribute` rules, the system needs access to node attributes beyond the standard collected set (§ 1.4 of the Data Collection specification). The `attribute_path` field specifies a dot-separated path into the node's data.

**Supported attribute paths:**

- Paths starting with `automatic.` are resolved against the node's automatic attributes (already collected via partial search).
- The standard collected attributes (`automatic.platform`, `automatic.chef_packages.chef.version`, etc.) are always available.
- For attributes not in the standard set, the collector must extend the partial search query to include the requested attribute paths. The ownership configuration is read at collector startup, and any `attribute_path` values from `node_attribute` rules are merged into the partial search keys.

**Implementation:**

- At startup, scan all `ownership.auto_rules` entries of type `node_attribute`.
- Extract the `attribute_path` values and add them to the partial search key map sent to the Chef API.
- Store the additional attributes in a new `custom_attributes` JSONB column on the `node_snapshots` table (see § 3).
- At rule evaluation time, resolve the `attribute_path` against the combined standard and custom attributes.
