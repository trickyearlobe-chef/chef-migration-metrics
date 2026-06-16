# Web API — Ownership Endpoints

## Ownership Endpoints

Ownership tracking endpoints allow managing owners, ownership assignments, bulk reassignment, bulk import, audit log, and committer-to-owner workflows. These endpoints are fully specified in the [Ownership Specification](ownership.md) § 4 and are summarised here for cross-reference.

| Endpoint | Method | Description | Auth |
|----------|--------|-------------|------|
| `/api/v1/owners` | GET | List all owners | viewer+ |
| `/api/v1/owners` | POST | Create an owner | operator+ |
| `/api/v1/owners/:name` | GET | Get owner detail with migration progress | viewer+ |
| `/api/v1/owners/:name` | PUT | Update an owner | operator+ |
| `/api/v1/owners/:name` | DELETE | Delete an owner (cascades) | admin |
| `/api/v1/owners/:name/assignments` | GET | List assignments for an owner | viewer+ |
| `/api/v1/owners/:name/assignments` | POST | Create assignments | operator+ |
| `/api/v1/owners/:name/assignments/:id` | DELETE | Delete an assignment | operator+ |
| `/api/v1/ownership/reassign` | POST | Bulk reassign between owners | operator+ |
| `/api/v1/ownership/import` | POST | Bulk import from CSV/JSON | operator+ |
| `/api/v1/ownership/lookup` | GET | Look up ownership for an entity | viewer+ |
| `/api/v1/ownership/audit-log` | GET | Query ownership audit log | viewer+ |
| `/api/v1/cookbooks/:name/committers` | GET | List git committers for a cookbook | viewer+ |
| `/api/v1/cookbooks/:name/committers/assign` | POST | Assign committers as owners | operator+ |

→ Full endpoint specifications: [Ownership Specification § 4](ownership.md)
