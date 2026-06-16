# Ownership Tracking - Component Specification

> **Implementation language:** Go. See `../CLAUDE.md` for language and concurrency rules.

> Component specification for Ownership Tracking in Chef Migration Metrics.
> See the [top-level specification](overview.md) for project overview and scope.

---

## TL;DR

Ownership tracking lets organisations assign **owners** (teams, individuals, or cost centres) to nodes, roles, policyfiles, cookbooks, and git repositories so that migration progress, remediation work, and upgrade readiness can be viewed, filtered, and exported per owner. Ownership can be assigned manually via the Web API/UI, imported from CSV/JSON, or auto-derived from Chef node attributes, policy names, git repo URL patterns, or configurable attribute paths. For git-sourced cookbooks, committer history is collected from the repository and surfaced in the cookbook detail view so that operators can identify and select contributors as owners. Bulk reassignment moves all (or a filtered subset of) assignments from one owner to another in a single operation, supporting team reorganisations and staff departures. All ownership mutations are recorded in an append-only audit log with the acting user, timestamp, and change details. Both `definitive` and `inferred` owners are displayed for every entity — the UI visually distinguishes them but does not suppress lower-priority owners. Owners are stored in a dedicated `owners` table with many-to-many `ownership_assignments` linking owners to the entities they are responsible for. The feature is optional — when no owners are configured, all existing behaviour is unchanged. Related specs: `datastore/`, `web-api/`, `visualisation/`, `configuration/`, `data-collection/`.

---

## Overview

Large Chef estates are managed by multiple teams. When planning a Chef Client upgrade project, a common question is: **"Which team owns this cookbook / node / policy, and who needs to do the remediation work?"** Without ownership data, the migration dashboard shows a flat view of all entities, making it difficult to delegate work, track progress per team, or report to management by organisational unit.

The Ownership Tracking feature adds:

1. **Owner entities** — named owners representing teams, individuals, business units, or cost centres.
2. **Ownership assignments** — many-to-many links between owners and nodes, cookbooks, git repositories, roles, and policyfiles.
3. **Auto-derivation rules** — configurable rules that automatically assign ownership based on Chef attributes, naming conventions, policy metadata, or git repository URL patterns.
4. **Bulk import** — CSV and JSON import of ownership mappings for bootstrapping from external CMDBs or spreadsheets.
5. **Bulk reassignment** — move all (or a filtered subset of) assignments from one owner to another, supporting team reorganisations and staff departures.
6. **Audit log** — a record of all ownership changes (assignments created, removed, reassigned, owners created/updated/deleted) with the acting user and timestamp.
7. **Owner-scoped views** — filtering, grouping, and exporting all dashboard data by owner.

---

## 1. Owner Model

Moved to [ownership-owner-model.md](ownership-owner-model.md).

## 2. Auto-Derivation Rules

Moved to [ownership-auto-derivation.md](ownership-auto-derivation.md).

## 3. Datastore Changes

Moved to [ownership-datastore.md](ownership-datastore.md).

## 4. Web API Endpoints

Moved to [ownership-api.md](ownership-api.md) and [ownership-api-2.md](ownership-api-2.md).

## 5. Dashboard / Visualisation Changes

Moved to [ownership-visualisation.md](ownership-visualisation.md).

## 6. Configuration

Moved to [ownership-integration.md](ownership-integration.md).

## 7. Data Collection Integration

Moved to [ownership-integration.md](ownership-integration.md).

## 8. Export Integration

Moved to [ownership-integration.md](ownership-integration.md).

## 9. Retention and Cleanup

Moved to [ownership-operations.md](ownership-operations.md).

## 10. Scalability Considerations

Moved to [ownership-operations.md](ownership-operations.md).

## 11. Migration Path

Moved to [ownership-operations.md](ownership-operations.md).

---

## Related Specifications

- [Top-level Specification](overview.md) — project overview and scope
- [Data Collection Specification](data-collection.md) — node collection, cookbook fetching, partial search
- [Datastore Specification](datastore.md) — database schema and tables
- [Web API Specification](web-api.md) — HTTP API endpoints
- [Visualisation Specification](visualisation.md) — dashboard views, filters, drill-downs
- [Configuration Specification](configuration.md) — YAML configuration schema
- [Analysis Specification](analysis.md) — complexity scoring, blast radius (related to ownership-scoped remediation)
- [Logging Specification](logging.md) — structured logging with ownership scope
