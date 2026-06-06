# Ownership — Configuration, Data Collection, Export & Notification Integration

## 6. Configuration

### 6.1 Configuration Schema

```yaml
ownership:
  enabled: true  # Default: false. When false, all ownership features are hidden.

  audit_log:
    retention_days: 365  # Default: 365. Set to 0 to retain indefinitely.

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
```

### 6.2 Configuration Reference

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `ownership.enabled` | boolean | `false` | Enable ownership tracking features. When disabled, ownership tables still exist but are not populated, and UI elements are hidden. |
| `ownership.auto_rules` | list | `[]` | List of auto-derivation rule definitions (see § 2.2) |
| `ownership.audit_log.retention_days` | integer | `365` | Number of days to retain audit log entries. Entries older than this are purged daily. Set to `0` to disable purging (retain indefinitely). |

### 6.3 Environment Variable Overrides

| Variable | Overrides |
|----------|-----------|
| `CMM_OWNERSHIP_ENABLED` | `ownership.enabled` |
| `CMM_OWNERSHIP_AUDIT_LOG_RETENTION_DAYS` | `ownership.audit_log.retention_days` |

Auto-derivation rules are not overridable via environment variables due to their complex structure. They must be defined in the YAML configuration file.

---

## 7. Data Collection Integration

### 7.1 Additional Partial Search Keys

When `ownership.enabled` is `true` and `node_attribute` auto-derivation rules are configured, the data collection component must include the configured `attribute_path` values in the partial search key map sent to the Chef API. The returned values are stored in the `custom_attributes` field on node snapshots, keyed by the dot-separated attribute path.

### 7.2 Git Committer Collection

When `ownership.enabled` is `true`, the data collection component must extract committer information from each git-sourced cookbook repository during the fetch cycle. After fetching/pulling a repository, the collector gathers the distinct committers from the git log of the default branch — recording each committer's name, email, total commit count, earliest commit date, and most recent commit date.

This data is stored in the `git_repo_committers` table and fully refreshed on each collection run (the previous set of committer rows for the repository is replaced). The committer data is read-only from the application's perspective — it reflects the state of the git history, not user input.

### 7.3 Auto-Derivation Trigger

After each collection run completes for an organisation:

- If `ownership.enabled` is `false`, skip.
- Evaluate all auto-derivation rules against the newly collected data.
- Create assignments for new matches and remove stale `auto_rule` assignments from rules that no longer match.
- Log a summary at `INFO` severity with the rule count, assignments created, and stale assignments removed.

---

## 8. Export Integration

### 8.1 Export Columns

When ownership is enabled, all export types (CSV, JSON) include ownership columns:

| Export Type | Additional Columns |
|-------------|-------------------|
| Ready node export | `owners` (comma-separated list of owner names) |
| Blocked node export | `owners`, `blocking_cookbook_owners` (owners of the blocking cookbooks) |
| Cookbook remediation export | `owners`, `git_repo_url` (for git-sourced cookbooks) |

### 8.2 Export Filters

The `owner` and `unowned` filters are available in export requests, allowing operators to export data scoped to a specific team.

---

## 9. Notification Integration

### 9.1 Owner-Scoped Notifications

Notification channels (see [Configuration Specification](../configuration/Specification.md)) gain an optional `owners` filter:

```yaml
notifications:
  channels:
    - name: web-platform-alerts
      type: webhook
      url_env: WEB_PLATFORM_WEBHOOK_URL
      events:
        - cookbook_status_change
        - readiness_milestone
      filters:
        owners:
          - web-platform
```

When an `owners` filter is set on a channel, the channel only fires for events related to entities owned by the specified owners.

### 9.2 Ownership Change Events

Three notification event types for ownership:

| Event | Description |
|-------|-------------|
| `ownership_assigned` | Fired when ownership is assigned (manual, import, or auto-rule). Payload includes the owner name, entity type/key, and source. |
| `ownership_removed` | Fired when ownership is removed. Payload includes the owner name and entity type/key. |
| `ownership_reassigned` | Fired when assignments are bulk-reassigned between owners. Payload includes the source owner, target owner, number of assignments moved, and whether the source owner was deleted. Fired once per reassignment operation (not per individual assignment). |

These events are low-volume (ownership changes are infrequent) and are primarily useful for audit trails and team communication.
