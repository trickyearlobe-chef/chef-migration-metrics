# Ownership — Dashboard / Visualisation Changes

## 5. Dashboard / Visualisation Changes

### 5.1 Owner Filter

An **Owner** filter is added to the interactive filter bar alongside the existing filters (organisation, environment, role, policy, platform, etc.). The owner filter:

- Supports multi-select (choose one or more owners).
- Includes an **Unowned** option to show only entities without ownership.
- Applies to all dashboard views consistently (version distribution, readiness, cookbook compatibility, dependency graph, remediation).

### 5.2 Ownership Summary View

A new **Ownership** dashboard view is added as a top-level navigation item. This view provides:

#### Migration Progress by Owner

A table showing each owner's migration progress:

| Column | Description |
|--------|-------------|
| Owner | Name and display name |
| Total Nodes | Number of nodes owned |
| Ready | Nodes ready for upgrade |
| Blocked | Nodes blocked by incompatible cookbooks or insufficient disk space |
| Stale | Nodes with stale data |
| % Ready | Percentage of nodes ready |
| Owned Cookbooks | Total cookbooks owned |
| Compatible Cookbooks | Cookbooks compatible with target version |
| Blocking Cookbooks | Cookbooks blocking one or more nodes |
| Complexity | Aggregate remediation complexity (sum of scores for owned incompatible cookbooks) |

The table supports sorting by any column and filtering by organisation and target Chef Client version.

#### Ownership Coverage

A summary panel showing:
- Total entities with ownership assigned vs. total entities
- Percentage of nodes with an owner
- Percentage of cookbooks with an owner
- Percentage of git repositories with an owner
- Number of unowned nodes / cookbooks / git repos / roles (with a link to the unowned view)

#### Owner Detail Drill-Down

Clicking an owner in the summary table drills down to a filtered view of the standard dashboard showing only that owner's entities — version distribution, readiness, cookbook compatibility, and remediation priority scoped to the owner.

### 5.3 Ownership Indicators on Existing Views

- **Node list** — An `Owner` column showing the resolved owner(s) for each node. Definitive owners are displayed with a solid badge; inferred owners with a dashed-outline badge (see § 1.4).
- **Node detail** — An ownership section showing all resolved owners, their confidence level (`definitive` / `inferred`), assignment source, and the resolution path (direct, inherited). Definitive owners are listed first.
- **Cookbook list** — An `Owner` column showing the resolved owner(s) for each cookbook (including git-repo-inherited ownership), with badge styling per § 1.4.
- **Cookbook detail** — An ownership section. For git-sourced cookbooks, this section includes a link to the associated git repository and a link to the **Committers** sub-page.
- **Cookbook detail → Committers sub-page** — For git-sourced cookbooks, a sub-page listing all committers extracted from the git history. The table shows each committer's name, email, commit count, first commit date, and most recent commit date. The **commit count** and **most recent commit** columns are sortable (click to toggle ascending/descending) so that operators can quickly identify who is currently most active in the repository. A date filter allows narrowing to recent contributors (e.g. last 6 months, last year). Each committer row has a checkbox; the operator can select one or more committers and click an **Assign as Owners** action to create ownership assignments for the repository. If a selected committer does not yet exist as an owner, the system creates an `individual` owner record using the committer's name and email. Committers who are already assigned as owners of the repository are visually indicated and excluded from the selection.
- **Remediation priority list** — An `Owner` column to help identify which team needs to act on each cookbook, with badge styling per § 1.4.

### 5.4 Ownership Management UI

An **Ownership** management page under the admin section provides:

- **Owner list** — CRUD for owners (name, display name, contact info, type, metadata).
- **Assignment list** — View and manage assignments per owner, with filters for entity type and source.
- **Bulk import** — A file upload form for CSV/JSON import with a preview of the parsed data before confirming.
- **Bulk reassignment** — A form to move assignments between owners. The operator selects a source owner and a target owner, optionally filters by entity type and/or organisation, and previews the list of assignments that will be moved before confirming. A checkbox option to delete the source owner after reassignment is available (disabled unless the user has `admin` role). After confirmation, a summary shows the number of assignments moved, skipped, and whether the source owner was deleted.
- **Auto-rule status** — A read-only view showing configured auto-derivation rules, their last evaluation time, and the number of assignments they produced.
- **Audit log** — A filterable, paginated table showing the ownership audit log. Columns: timestamp, action, actor, owner, entity type, entity key, organisation. Each row can be expanded to show the full `details` JSON. Filters for action type, actor, owner name, entity type, and date range are available above the table. The audit log is read-only for all roles.

**Authorisation:**
- Viewers can see ownership data and the audit log but cannot modify anything.
- Operators can create/update owners and assignments, and perform bulk reassignment.
- Admins can delete owners, perform bulk reassignment with the delete-source-owner option, and manage all aspects of ownership.
