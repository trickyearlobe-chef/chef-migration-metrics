# Analysis Component - Specification

> **Implementation language:** Go. See `../../Claude.md` for language and concurrency rules.

## TL;DR

This spec covers five areas: **(1) Cookbook usage analysis** — which cookbooks/versions are in use, by which nodes, roles, and policies. **(2) Cookbook compatibility testing** — Test Kitchen (git-sourced cookbooks) and CookStyle linting (server-sourced cookbooks) against target Chef Client versions, with version-specific cop profiles. **(3) Remediation guidance** — auto-correct previews (diff generation), migration doc links per deprecation cop, and cookbook complexity scoring (weighted score + blast radius). **(4) Node upgrade readiness** — per-node pass/fail per target version based on cookbook compatibility and disk space, with stale-node handling. **(5) Embedded tool resolution** — CookStyle/Test Kitchen/Ruby looked up in `analysis_tools.embedded_bin_dir` first, then `PATH`. All work is parallelised via bounded worker pools (see configuration spec).

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

- [`../Specification.md`](../Specification.md) — top-level project specification
- [`../data-collection/Specification.md`](../data-collection/Specification.md) — data collection component
- [`../visualisation/Specification.md`](../visualisation/Specification.md) — dashboard and log viewer
- [`../chef-api/Specification.md`](../chef-api/Specification.md) — Chef Infra Server API reference
- [`../datastore/Specification.md`](../datastore/Specification.md) — database schema
- [`../configuration/Specification.md`](../configuration/Specification.md) — configuration schema (includes `embedded_bin_dir` setting)
- [`../logging/Specification.md`](../logging/Specification.md) — logging subsystem
- [`../packaging/Specification.md`](../packaging/Specification.md) — embedded Ruby environment build and layout
- [`test-kitchen-drivers.md`](test-kitchen-drivers.md) — Test Kitchen driver abstraction (multi-driver overlay, credential injection, platform mapping, coverage analysis)
