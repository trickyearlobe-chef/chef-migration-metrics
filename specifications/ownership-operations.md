# Ownership — Retention, Scalability & Migration Path

## 10. Retention and Cleanup

- **Owner deletion** cascades to all `ownership_assignments` for that owner. The cascade is recorded in the audit log as an `owner_deleted` entry with the count of cascaded assignments in `details.assignments_cascaded`.
- **Organisation deletion** cascades to all `ownership_assignments` scoped to that organisation.
- **Auto-rule assignments** are cleaned up when the rule is removed from configuration. On startup, if a rule name no longer appears in the config, all `auto_rule` assignments with that `auto_rule_name` are deleted. These deletions are logged in the audit log with `actor = 'system'`.
- Ownership assignments are not subject to time-based retention. They persist until explicitly deleted or removed by auto-rule evaluation.
- **Audit log retention** — Audit log entries are purged based on the `ownership.audit_log.retention_days` configuration setting (default: 365 days). A background cleanup job runs daily and deletes entries older than the configured threshold. The audit log itself is append-only — entries are never modified, only purged by the retention job.

---

## 11. Scalability Considerations

- **Ownership resolution at query time** avoids materialisation costs but means ownership lookups add overhead to dashboard queries. Implementations should ensure this remains responsive at scale.
- **Auto-derivation** runs after collection and may involve pattern matching against thousands of entity names. For very large fleets, auto-derivation should be parallelised.
- **Bulk import** is limited to 10,000 assignments per request to keep individual operations bounded.
- **Bulk reassignment** may involve thousands of assignments for a large owner. The operation runs in a single transaction to ensure consistency. For very large reassignments, the transaction size should remain manageable since it involves only UPDATE and INSERT operations on the `ownership_assignments` table plus INSERT operations on the `ownership_audit_log` table.
- **Audit log** volume is proportional to the rate of ownership changes, not to fleet size. Auto-derivation runs may produce bursts of entries after collection, but these are bounded by the number of rule matches that changed. The retention purge job should use batched deletes to avoid long-running transactions.

---

## 12. Migration Path

Ownership tracking is a new, additive feature. The migration path is:

1. **Database migration** creates the `owners`, `ownership_assignments`, `git_repo_committers`, and `ownership_audit_log` tables, and adds the `custom_attributes` column to `node_snapshots`.
2. **Always enabled** — there is no feature flag. Existing deployments gain the tables and endpoints at migration and see no owners until ownership data is imported or auto-derivation rules are configured.
3. **No breaking changes** — all existing API endpoints, filters, and behaviour remain unchanged.
4. **Incremental adoption** — teams can start by importing ownership for their most critical cookbooks and nodes, then gradually expand coverage and add auto-derivation rules.
