# Web API — Notification & Ownership Endpoints

## Notification Endpoints

### List Notification History

#### `GET /api/v1/notifications`

Returns a paginated list of sent notifications.

**Query parameters:** `event_type`, `channel_name`, `status`, `from`, `to`, pagination.

**Response (200):**

```json
{
  "data": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440099",
      "channel_name": "slack-ops",
      "channel_type": "webhook",
      "event_type": "cookbook_status_change",
      "summary": "Cookbook 'nginx' is now compatible with Chef Client 19.0.0",
      "status": "sent",
      "sent_at": "2024-06-15T15:00:00Z"
    }
  ],
  "pagination": { ... }
}
```

### Get Notification Detail

#### `GET /api/v1/notifications/:id`

Returns the full detail of a sent notification, including the payload.

**Response (200):**

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440099",
  "channel_name": "slack-ops",
  "channel_type": "webhook",
  "event_type": "cookbook_status_change",
  "summary": "Cookbook 'nginx' is now compatible with Chef Client 19.0.0",
  "payload": {
    "cookbook_name": "nginx",
    "previous_status": "incompatible",
    "new_status": "compatible",
    "target_chef_version": "19.0.0",
    "commit_sha": "f1e2d3c4b5a6"
  },
  "status": "sent",
  "error_message": null,
  "retry_count": 0,
  "sent_at": "2024-06-15T15:00:00Z"
}
```

---

## Ownership Endpoints

Ownership tracking endpoints allow managing owners, ownership assignments, bulk reassignment, bulk import, audit log, and committer-to-owner workflows. These endpoints are fully specified in the [Ownership Specification](../ownership/Specification.md) § 4 and are summarised here for cross-reference.

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

→ Full endpoint specifications: [Ownership Specification § 4](../ownership/Specification.md)

---
