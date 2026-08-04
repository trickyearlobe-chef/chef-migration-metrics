# Datastore - Component Specification

> **Implementation language:** Go (PostgreSQL). See `../CLAUDE.md` for language and concurrency rules.

> **Status: stub.** This spec is referenced by several other specs but has not yet been written in full. The authoritative schema currently lives in the SQL migrations. Tracked in `plans/todo-tech-debt.md` for completion.

## TL;DR

The datastore is the PostgreSQL persistence layer for Chef Migration Metrics. It holds collected node/cookbook/git-repo data, analysis results (CookStyle, Test Kitchen, readiness, complexity), ownership, configuration, logs, and metric snapshots. The data-access layer is Go. The canonical schema is defined by the migration files under [`migrations/`](../migrations/) — read those for exact tables, columns, and constraints.

## Schema Source of Truth

Until this spec is completed, treat the ordered migration files in `migrations/*.up.sql` as the authoritative schema definition. They are applied in filename order and cover table creation, indexes, natural keys, and later structural changes.

## Scope

- Table definitions and relationships (see migrations for current DDL).
- Data-access patterns used by the collector, analysis, web API, and ownership components.
- Retention and snapshotting for historical trending (see [enriched-metric-snapshots](enriched-metric-snapshots.md)).

## Related Specifications

- [overview](overview.md) — index of all specifications
- [data-collection](data-collection.md) — writes collected data into the datastore
- [analysis](analysis.md) — writes analysis results into the datastore
- [web-api](web-api.md) — reads the datastore to serve the dashboard
- [configuration](configuration.md) — datastore connection configuration
- [enriched-metric-snapshots](enriched-metric-snapshots.md) — historical snapshots
