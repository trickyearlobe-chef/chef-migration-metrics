# Configuration - Component Specification

> **TL;DR** — Single YAML config file with environment variable overrides for secrets. Key sections: `server` (bind address, port, TLS mode — off/static/acme), `database` (PostgreSQL URL), `collection` (Chef server orgs, schedule, stale thresholds), `target_versions` (Chef Client versions to test against), `git` (cookbook repo URLs), `concurrency` (worker pool sizes per task type), `analysis_tools` (CookStyle/Test Kitchen timeouts and TK driver config; `cookstyle`/`kitchen` resolved from `PATH` via Chef Workstation), `auth` (local/SAML), `exports` (output dir, retention, async threshold), `elasticsearch` (NDJSON export toggle, output dir), `logging` (level, retention). All sensitive values must be set via env vars, never inlined. See `todo/configuration.md` for implementation status.

## Overview

This document specifies the configuration surface area for the Chef Migration Metrics application. Configuration controls all aspects of the application including Chef server connectivity, collection scheduling, analysis targets, datastore connectivity, logging behaviour, and authentication providers.

## Live Reload Requirement

Moved to [configuration-live-reload.md](configuration-live-reload.md).

## Configuration File

Moved to [configuration-file-and-secrets.md](configuration-file-and-secrets.md).

## Secrets and Credentials

Moved to [configuration-file-and-secrets.md](configuration-file-and-secrets.md).

---

## Configuration Schema

Moved to [configuration-schema-collection.md](configuration-schema-collection.md) and [configuration-schema-server.md](configuration-schema-server.md).

## Environment Variable Overrides

Moved to [configuration-env-overrides.md](configuration-env-overrides.md).

## Validation

Moved to [configuration-validation.md](configuration-validation.md).

## Full Example

Moved to [configuration-full-example.md](configuration-full-example.md).
