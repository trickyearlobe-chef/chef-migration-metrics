# Test Kitchen Driver Abstraction — Specification

## TL;DR

Generalises Test Kitchen compatibility testing from a Docker-only (`kitchen-dokken`) model to a pluggable driver architecture. Test Kitchen runs on the CMM host and provisions the test targets — containers for dokken, real VMs for vCenter/EC2/vRA/etc. The application controls what Test Kitchen does by generating a `.kitchen.local.yml` overlay that overrides the driver, remaps platform names to available images (VM templates, AMIs, etc.) via a configurable lookup table, and injects credentials from the encrypted secret store. Cookbook repos keep their existing `.kitchen.yml` untouched. Adding a new driver (vCenter today, vRA tomorrow, EC2 next quarter) is a configuration change, not a code change.

## Overview

Moved to [test-kitchen-drivers-overview.md](test-kitchen-drivers-overview.md).

## Driver Override Mechanism

Moved to [test-kitchen-drivers-driver-override.md](test-kitchen-drivers-driver-override.md).

## Credential Model

Moved to [test-kitchen-drivers-credentials.md](test-kitchen-drivers-credentials.md).

## Platform Image Mapping

Moved to [test-kitchen-drivers-platform-mapping.md](test-kitchen-drivers-platform-mapping.md).

## Platform Coverage Analysis

Moved to [test-kitchen-drivers-platform-mapping.md](test-kitchen-drivers-platform-mapping.md).

## Configuration Schema

Moved to [test-kitchen-drivers-configuration.md](test-kitchen-drivers-configuration.md).

## Overlay Generation

Moved to [test-kitchen-drivers-overlay-generation.md](test-kitchen-drivers-overlay-generation.md).

## Startup Validation

Moved to [test-kitchen-drivers-overlay-generation.md](test-kitchen-drivers-overlay-generation.md).

## Database Changes

Moved to [test-kitchen-drivers-database.md](test-kitchen-drivers-database.md).

## Deployment Reference: VMware vCenter

Moved to [test-kitchen-drivers-vcenter.md](test-kitchen-drivers-vcenter.md).

## Related Specifications

| Specification | Relevance |
|---------------|-----------|
| analysis.md | Parent spec for cookbook compatibility testing (§2). Overlay generation steps 3–8 are extended by this spec. |
| configuration.md | Config schema (§ Analysis Tools). The `test_kitchen` sub-section is extended. |
| secrets-storage.md | Credential encryption, resolution precedence, `generic` credential type. |
| datastore.md | `credentials` table, `git_repo_test_kitchen_results` table, new `cookbook_platform_coverage` table. |
| packaging.md | Embedded kitchen drivers shipped in all packaging formats (§4.5). |
| data-collection.md | Node attribute collection: `platform`, `platform_version`, `platform_family` (§1.4). |
