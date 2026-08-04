# Dependency Graph Refactor — Component Specification

## Problem

The fleet-wide force-directed graph (`DependencyGraphPage.tsx`, ~1646 lines) renders ALL roles and cookbooks for an organisation in a single SVG simulation. At scale (thousands of roles/cookbooks) it is slow to render and produces an unreadable hairball.

The graph is useful at smaller scope — a single node's dependency chain or a single role's includes tree — where the number of entities is tens, not thousands.

## Solution

Move the force-directed graph to scoped contexts where it adds value. Keep the fleet-wide table view. Extract the graph renderer into a reusable component.

### Node Dependency Graph

New "Dependencies" tab on the node detail page showing this node's full dependency chain.

**Starting point:** the node's `run_list` entries (from `node_snapshots.run_list`).

**Expansion rules:**
- `recipe[cookbook::recipe]` → cookbook node
- `role[name]` → role node → expand that role's direct includes from `role_dependencies`
- Nested roles expand recursively (cycle-safe)
- Cookbook nodes include transitive cookbook dependencies from `server_cookbooks.dependencies`

**Node types in the graph:**
- Run-list entry (role or recipe) — distinguished as the "root" layer
- Role (intermediate) — pulled in via nesting
- Cookbook (leaf) — colour-coded by compatibility status

**Visual treatment:**
- Edge style distinguishes how a cookbook was reached: direct run-list recipe, via role, via nested role, via transitive cookbook dependency
- Cookbook nodes colour-coded by `compatibility_status`: green (compatible), red (incompatible), amber (CookStyle-only / partial), grey (untested/unknown)
- Complexity label shown as a badge on cookbook nodes

**Typical size:** even complex nodes have dozens of dependencies, not thousands. The graph is readable without level-of-detail tricks.

**API endpoint:** `GET /api/v1/nodes/:organisation/:name/dependency-graph`

### Role Dependency Graph

New "Dependencies" tab on the role detail page (defined in the roles spec).

**Starting point:** the role's direct includes from `role_dependencies` for the selected organisation.

**Expansion rules:** same as node graph — nested roles expand recursively, cookbooks include transitive dependencies.

**Visual treatment:** identical to the node graph. The role itself is the root node.

**API endpoint:** `GET /api/v1/roles/:organisation/:name/dependency-graph`

### Policyfile Nodes

Nodes with `policy_name` set use Policyfile lockfiles instead of run-lists and roles. For now, the node dependency graph for Policyfile nodes shows the cookbook set from `node_snapshots.cookbooks` as a flat star (node → cookbooks). Transitive cookbook dependencies still expand. Role nodes do not appear because Policyfile nodes do not use roles. A future enhancement may parse the Policyfile lock for recipe-level detail.

## Fleet-Wide Page Changes

**Keep:** the table view (`GET /api/v1/dependency-graph/table`). It answers "which roles use cookbook X?" and "which roles have incompatible transitive dependencies?" — queries that need the full dataset.

**Remove or gate:** the force-directed graph toggle on the fleet page. Options (choose during implementation):
- Remove the graph/table toggle entirely; page is table-only
- Keep the toggle but show a warning for datasets exceeding a threshold (e.g. >200 nodes) with a "Render anyway" confirmation

**Update:** page title and description text to reflect the change. Add links/hints directing users to the node or role detail pages for visual dependency exploration.

## Shared Graph Component

Extract the force-directed graph renderer from `DependencyGraphPage.tsx` into a reusable component.

### Contract

**Props (inputs):**
- `nodes` — array of graph nodes (same `{ id, type, name, compatibility_status?, complexity_label? }` shape)
- `edges` — array of graph edges (`{ from, to, type }`)
- `rootNodeId` — optional; the starting entity, rendered with distinct styling
- `selectedNodeId` / `onSelectNode` — controlled selection state
- `hoveredNodeId` / `onHoverNode` — controlled hover state
- `colorByStatus` — boolean; when true, cookbook nodes are coloured by `compatibility_status` instead of by type
- `edgeStyleByType` — boolean; when true, edge rendering varies by edge type (includes_role, includes_cookbook, depends)
- `height` — optional; container height (default: fill parent)

**Preserved behaviour from existing implementation:**
- Custom SVG force-directed simulation (repulsion, link forces, gravity, damping)
- Pan (mouse drag on background), zoom (scroll wheel), node drag
- Selected-node highlight with connected-node emphasis and dimmed non-connected nodes
- Search/filter controls (search term, type filter)
- Zoom controls (in/out/reset)

**Does NOT include:**
- Data fetching — the parent page fetches and passes data
- Summary stat cards — the parent page renders those
- Selected-node detail panel — see below

### Selected Node Panel

The `SelectedNodePanel` component is also extracted as reusable, but its content varies by context:
- Node dependency graph: links to cookbook detail, shows compatibility status, shows how the cookbook was reached
- Role dependency graph: links to cookbook detail and nested role detail, shows compatibility status
- Fleet-wide (if retained): existing behaviour (links to cookbook/role, shows connections)

The panel accepts a render prop or slot for context-specific content below the common header (node name, type, connection count).

## API Endpoints

### Node Dependency Graph

`GET /api/v1/nodes/:organisation/:name/dependency-graph`

**Query parameters:** `target_chef_version` (optional — filters compatibility status to a specific target version).

**Response (200):**

Same envelope as existing fleet-wide endpoint:

```
{
  "nodes": [
    { "id": "runlist:role[base]", "type": "run_list_entry", "name": "role[base]", "entry_type": "role" },
    { "id": "role:base", "type": "role", "name": "base" },
    { "id": "cookbook:apt", "type": "cookbook", "name": "apt", "compatibility_status": "compatible", "complexity_label": "none" },
    { "id": "cookbook:nginx", "type": "cookbook", "name": "nginx", "compatibility_status": "incompatible", "complexity_label": "medium" }
  ],
  "edges": [
    { "from": "runlist:role[base]", "to": "role:base", "type": "run_list_role" },
    { "from": "role:base", "to": "cookbook:apt", "type": "includes_cookbook" },
    { "from": "runlist:recipe[nginx::default]", "to": "cookbook:nginx", "type": "run_list_recipe" },
    { "from": "cookbook:nginx", "to": "cookbook:apt", "type": "depends" }
  ],
  "metadata": {
    "node_name": "web-01",
    "organisation": "production",
    "total_roles": 1,
    "total_cookbooks": 2,
    "incompatible_cookbooks": 1,
    "target_chef_version": "18.5.0"
  }
}
```

**Node fields:**
- `id` — unique within the graph, prefixed by type (`role:`, `cookbook:`, `runlist:`)
- `type` — `run_list_entry`, `role`, or `cookbook`
- `name` — display name
- `entry_type` — only on `run_list_entry` nodes: `role` or `recipe`
- `compatibility_status` — only on `cookbook` nodes: `compatible`, `incompatible`, `partial`, `untested`
- `complexity_label` — only on `cookbook` nodes: `none`, `low`, `medium`, `high`, `critical`

**Edge types:**
- `run_list_role` — run-list entry → role
- `run_list_recipe` — run-list entry → cookbook
- `includes_role` — role → nested role
- `includes_cookbook` — role → cookbook
- `depends` — cookbook → transitive cookbook dependency

**Error responses:**
- 404 if node not found
- 400 if organisation missing

### Role Dependency Graph

`GET /api/v1/roles/:organisation/:name/dependency-graph`

**Query parameters:** `target_chef_version` (optional).

**Response (200):** same `{ nodes, edges, metadata }` envelope. The root node is the role itself. `metadata` includes `role_name` and `organisation` instead of `node_name`.

**Error responses:**
- 404 if role not found in `role_dependencies` for this organisation
- 400 if organisation missing

### Backend Data Assembly

Both scoped endpoints build the graph server-side by:
1. Starting from the root entity's direct dependencies (run_list for nodes, role_dependencies for roles)
2. Recursively expanding role includes via `role_dependencies` (cycle detection via visited set)
3. Expanding cookbook transitive dependencies via `server_cookbooks.dependencies`
4. Joining cookbook nodes with compatibility results for the selected target version
5. Returning the assembled node/edge lists

No new tables required. Existing `role_dependencies`, `node_snapshots`, `server_cookbooks`, and compatibility result tables provide all data.

## Navigation

- **Node detail page:** new "Dependencies" tab alongside existing content. Tab loads the node dependency graph component with data from the scoped endpoint.
- **Role detail page:** new "Dependencies" tab (coordinated with roles spec). Same graph component, different data source.
- **Fleet-wide dependency page:** table view only (or gated graph). Sidebar nav label unchanged. Page subtitle updated to mention scoped graphs are available on detail pages. Table rows for roles link to the role detail page's dependency tab.
- **Cookbook detail page:** no graph, but the "used by roles" and "used by nodes" sections link to the relevant detail pages where the graph is available.

## Relationship to Other Specs

- **`visualisation.md` § Dependency Graph** — this spec supersedes the fleet-wide graph description. The table view description and filtering requirements still apply.
- **`plans/todo-visualisation.md` § Dependency Graph View** — the colour-coding todo is addressed by `colorByStatus` on the shared component. Lazy loading todo is obviated by scoped graphs. Role-node linking todo moves to the scoped graph context.
- **Roles spec (in progress)** — the role detail page's "Dependencies" tab is defined here; the roles spec defines the rest of the role detail page.
- **`web-api.md` § Dependency Graph Endpoints** — two new endpoints added. Existing fleet-wide endpoints unchanged.

## Acceptance Criteria

- Node detail page shows a "Dependencies" tab with a force-directed graph of that node's dependency chain
- Role detail page shows a "Dependencies" tab with a force-directed graph of that role's includes
- Cookbook nodes in scoped graphs are colour-coded by compatibility status
- Edge styles distinguish how each dependency was reached
- The force-directed graph component is a standalone reusable module, not embedded in page code
- Fleet-wide page no longer renders the force-directed graph by default
- Fleet-wide table view continues to work unchanged
- Scoped graph endpoints return correct transitive expansions with cycle safety
- Policyfile nodes show cookbook-only star graph without roles