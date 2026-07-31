# Configuration - Component Specification

> **TL;DR** — Configuration lives in the **database** and is edited through the **UI**. The only values sourced from the environment or a file are the ones needed to reach the database at all: the database URL and the credential encryption key. Everything else — server, collection, target version, git, concurrency, analysis tools, auth, exports, logging — is stored and changed through the config store. The schema documents below describe the *shape* of those settings, not a supported file-based way to set them.

## Overview

This document specifies the configuration surface area for the Chef Migration Metrics application. Configuration controls all aspects of the application including Chef server connectivity, collection scheduling, analysis targets, datastore connectivity, logging behaviour, and authentication providers.

### Where configuration lives

Two rules, and they are not negotiable design detail — they are why the product works the way it does:

1. **The database is the source of truth, edited through the UI.** The only inherent exceptions are the values that unlock the database itself — the database URL and the credential encryption key — since they cannot live in the database they unlock.

   Server and TLS settings also have environment overrides in the code, but they are **not** a justified exception: TLS lockout is already prevented by the startup fallback ladder, which degrades to a self-signed certificate and then to plain HTTP rather than failing to start. Those overrides are legacy and are candidates for removal, not a pattern to extend.
2. **Nothing else is configured by environment variable or YAML file.** A setting that cannot be reached from the config store is not configurable — the options are to wire it into the store or remove it. Do not describe a file or env path as a supported way to set something.

*Why it matters:* configuration must take effect immediately without a restart (see Live Reload), which a file-based value cannot do; and the deployment is VDI-only, so "edit a file on the host and restart" is neither usable nor safe while a collection or rescan is running.

*Reading the schema documents:* a `yaml:` struct tag in the code is **not** evidence a setting is usable. Several exist for fields that are absent from the config store and therefore cannot be set at all.

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
