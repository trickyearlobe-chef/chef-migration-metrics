# Web API - Component Specification

> **Implementation language:** Go. See `../CLAUDE.md` for language and concurrency rules.

> Component specification for the HTTP API layer of Chef Migration Metrics.
> See the [top-level specification](overview.md) for project overview and scope.

---

## TL;DR

RESTful JSON API (Go) between backend and React frontend. Mostly read-only over the datastore; write operations limited to admin actions (user management, manual rescan, auth provider config) and operator actions (ownership management, bulk import/reassignment). Key endpoint groups: nodes, server cookbooks, git repos, compatibility results, readiness, remediation, ownership, dependency graph, exports, logs, and admin. Cookbooks are split into **server cookbooks** (sourced from Chef Infra Server) and **git repos** (cloned from Git), each with their own endpoints. Dashboard and remediation priority endpoints aggregate across both sources. All list endpoints support pagination (`page`/`per_page`), filtering (org, environment, role, policy, platform, stale status, complexity label, owner), and sorting. Auth via session cookie with RBAC middleware (viewer / operator / admin). CORS configurable. Export endpoints support sync (small) and async (large, returns job ID). Ownership endpoints manage owners, assignments, bulk reassignment, audit log, and committer-to-owner workflows (see [Ownership Specification](ownership.md)). See `auth.md` for auth details, `datastore.md` for schema.

---

## Overview

The Web API is the HTTP layer between the Go backend and the web dashboard frontend. It exposes a RESTful JSON API that the frontend consumes to render all dashboard views, filters, drill-downs, log viewer, and administrative functions.

This component is purely a read/query layer over the datastore for dashboard data. The only write operations are administrative (user management, manual rescan triggers, configuration of authentication providers).

All endpoints require authentication unless explicitly marked as public. See the [Authentication specification](auth.md) for provider details and the [Session Management](#session-management) section below for how sessions are enforced.

---

## Base URL

Moved to [web-api-base-url.md](web-api-base-url.md).

## Content Type

Moved to [web-api-base-url.md](web-api-base-url.md).

## Authentication and Session Management

Moved to [web-api-auth.md](web-api-auth.md).

## Security Response Headers

Moved to [web-api-security-headers.md](web-api-security-headers.md).

## Common Patterns

Moved to [web-api-common-patterns.md](web-api-common-patterns.md).

## Dashboard Endpoints

Moved to [web-api-dashboard.md](web-api-dashboard.md).

## Node Endpoints

Moved to [web-api-nodes.md](web-api-nodes.md).

## Server Cookbook Endpoints

Moved to [web-api-server-cookbooks.md](web-api-server-cookbooks.md).

## Git Repo Endpoints

Moved to [web-api-git-repos.md](web-api-git-repos.md).

## Remediation Endpoints

Moved to [web-api-remediation.md](web-api-remediation.md).

## Dependency Graph Endpoints

Moved to [web-api-dependency-graph.md](web-api-dependency-graph.md).

## Export Endpoints

Moved to [web-api-exports.md](web-api-exports.md).

## Ownership Endpoints

Moved to [web-api-ownership.md](web-api-ownership.md).

## Organisation Endpoints

Moved to [web-api-organisations.md](web-api-organisations.md).

## Filter Option Endpoints

Moved to [web-api-filters.md](web-api-filters.md).

## Log Endpoints

Moved to [web-api-logs.md](web-api-logs.md).

## Admin Endpoints

Moved to [web-api-admin.md](web-api-admin.md).

## WebSocket Real-Time Events

Moved to [web-api-websocket.md](web-api-websocket.md).

## Static Assets and Frontend

Moved to [web-api-static-rate-cors.md](web-api-static-rate-cors.md).

## Rate Limiting

Moved to [web-api-static-rate-cors.md](web-api-static-rate-cors.md).

## CORS

Moved to [web-api-static-rate-cors.md](web-api-static-rate-cors.md).

## Related Specifications

- [Top-level Specification](overview.md)
- [Authentication and Authorisation](auth.md)
- [Ownership](ownership.md) — owner management, assignments, bulk reassignment, audit log, committer workflows
- [Visualisation](visualisation.md)
- [Logging](logging.md)
- [Configuration](configuration.md) — credential encryption key, `client_key_credential` and `bind_password_credential` settings
- [Datastore](datastore.md) — `credentials` table encryption model, `organisations` table credential FK
- [Chef API](chef-api.md) — credentials security requirements for API signing
- [Analysis](analysis.md) — for remediation guidance, complexity scoring, and auto-correct preview details
- [Data Collection](data-collection.md) — for Policyfile support and dependency graph collection
