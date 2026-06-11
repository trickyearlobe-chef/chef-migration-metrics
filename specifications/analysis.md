# Analysis Component - Specification

> **Implementation language:** Go. See `../CLAUDE.md` for language and concurrency rules.

## TL;DR

This spec covers five areas: **(1) Cookbook usage analysis** — which cookbooks/versions are in use, by which nodes, roles, and policies. **(2) Cookbook compatibility testing** — Test Kitchen (git-sourced cookbooks) and CookStyle linting (server-sourced cookbooks) against target Chef Client versions, with version-specific cop profiles. **(3) Remediation guidance** — auto-correct previews (diff generation), migration doc links per deprecation cop, and cookbook complexity scoring (weighted score + blast radius). **(4) Node upgrade readiness** — per-node pass/fail per target version based on cookbook compatibility and disk space, with stale-node handling. **(5) External tool resolution** — CookStyle/Test Kitchen are provided by Chef Workstation on the host and resolved from `PATH`; they are not bundled. All work is parallelised via bounded worker pools (see configuration spec).

---

## Overview

The analysis component processes data collected from Chef Infra Servers and git repositories to produce the metrics that drive the dashboard and upgrade readiness assessments. In addition to detecting compatibility issues, it provides **remediation guidance** — actionable information that helps practitioners fix problems, not just find them.

---

## Responsibilities

Moved to [analysis-responsibilities.md](analysis-responsibilities.md).

## Sub-Components

- [Cookbook Usage Analysis](analysis-cookbook-usage.md)
- [Cookbook Compatibility Testing](analysis-compatibility-testing.md)
- [CookStyle Invocation](analysis-cookstyle.md)
- [Remediation Guidance](analysis-remediation.md)
- [Node Upgrade Readiness](analysis-node-readiness.md)

## Startup Validation

Moved to [analysis-startup-validation.md](analysis-startup-validation.md).

## Scheduling and Trigger

Moved to [analysis-scheduling.md](analysis-scheduling.md).

## Data Inputs

Moved to [analysis-data-io.md](analysis-data-io.md).

## Data Outputs

Moved to [analysis-data-io.md](analysis-data-io.md).

## Related Specifications

- [`overview.md`](overview.md) — top-level project specification
- [`data-collection.md`](data-collection.md) — data collection component
- [`visualisation.md`](visualisation.md) — dashboard and log viewer
- [`chef-api.md`](chef-api.md) — Chef Infra Server API reference
- [`datastore.md`](datastore.md) — database schema
- [`configuration.md`](configuration.md) — configuration schema
- [`logging.md`](logging.md) — logging subsystem
- [`packaging.md`](packaging.md) — packaging and host prerequisites (cookstyle/kitchen from Chef Workstation on `PATH`)
- [`test-kitchen-drivers.md`](test-kitchen-drivers.md) — Test Kitchen driver abstraction (multi-driver overlay, credential injection, platform mapping, coverage analysis)
