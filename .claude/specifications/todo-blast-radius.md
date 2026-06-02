# Blast Radius — ToDo

Spec: `blast-radius.md`

All underlying data already exists (`affected_node_count`, `affected_role_count`, `affected_policy_count`, dependency edges). This is a visualisation/UX design problem.

## Design

- [ ] Decide on visualisation approach: force-directed cascade graph vs. structured drill-down panel (cookbook → roles → nodes) vs. side-by-side impact cards
- [ ] Decide entry points: from cookbook detail page only, or also from remediation priority list and role detail page?
- [ ] Confirm scope with customer: do they want transitive cookbook impact (A depends on broken B → A is also at risk) or direct impact only?

## Implementation (after design decision)

- [ ] Backend: `GET /api/v1/cookbooks/:name/blast-radius` — return affected roles (with node counts), affected nodes (paginated), transitive at-risk cookbooks, policy groups affected
- [ ] Frontend: impact explorer panel/page — entry from cookbook detail, cascades through roles → nodes
- [ ] Link affected roles and nodes to their existing list pages (filtered)
