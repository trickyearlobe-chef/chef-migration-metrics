# Secrets Storage - Component Specification

> **TL;DR** — Three credential storage methods in precedence order: **database** (AES-256-GCM encrypted, managed via Web UI/API), **environment variable** (Kubernetes Secrets, ECS, CI/CD), **file path** (traditional on-prem PEM files). Database credentials are encrypted with a master key (`CMM_CREDENTIAL_ENCRYPTION_KEY`) that must live outside the database. The `credentials` table stores Chef API keys, SMTP passwords, and webhook URLs — all encrypted at rest with per-row nonces and AAD binding. Plaintext is only held in memory for the duration of each operation. Key rotation (both credential values and the master encryption key) is supported without downtime. Admin-only Web API endpoints manage credentials (CRUD + test) and never return plaintext. Kubernetes deployments use `existingSecret` references or chart-managed Secrets; RPM/DEB installs use file paths and env files. See `todo/secrets-storage.md` for implementation status.

---

## Overview

This specification consolidates all secrets and credential management for the Chef Migration Metrics application. Secrets include Chef API private keys, SMTP credentials, webhook URLs, the database connection string, TLS private keys, and the credential encryption master key itself.

The design follows a defence-in-depth model: secrets are protected at the application layer (encryption, memory-only plaintext), the transport layer (TLS for database and API connections), the storage layer (database access controls, file permissions), and the operational layer (key separation, rotation procedures, audit logging).

This specification is the authoritative reference for secrets management. The following specs contain related sections that must remain consistent with this document:

| Specification | Related section |
|---------------|----------------|
| [`configuration/Specification.md`](../configuration/Specification.md) | § Secrets and Credentials, § Environment Variable Overrides |
| [`datastore/Specification.md`](../datastore/Specification.md) | § `credentials` table, § Credential Storage Security |
| [`web-api/Specification.md`](../web-api/Specification.md) | § Credential Management (Admin Endpoints) |
| [`chef-api/Specification.md`](../chef-api/Specification.md) | § Credentials Security |
| [`packaging/Specification.md`](../packaging/Specification.md) | § Docker Compose Configuration, § Environment File |
| [`tls/Specification.md`](../tls/Specification.md) | § Static certificate key files, § ACME storage |
| [`auth/Specification.md`](../auth/Specification.md) | § Local account password hashing |

---

## Credential Types

Moved to [secrets-storage-credential-model.md](secrets-storage-credential-model.md).

## Credential Resolution Precedence

Moved to [secrets-storage-credential-model.md](secrets-storage-credential-model.md).

## Database Credential Storage

Moved to [secrets-storage-encryption-keys.md](secrets-storage-encryption-keys.md).

## Master Encryption Key Management

Moved to [secrets-storage-encryption-keys.md](secrets-storage-encryption-keys.md).

## Key Rotation

Moved to [secrets-storage-encryption-keys.md](secrets-storage-encryption-keys.md).

## Plaintext Handling Rules

Moved to [secrets-storage-encryption-keys.md](secrets-storage-encryption-keys.md).

## File-Based Credential Security

Moved to [secrets-storage-packaging.md](secrets-storage-packaging.md).

## Docker Compose Secrets

Moved to [secrets-storage-packaging.md](secrets-storage-packaging.md).

## RPM / DEB Package Secrets

Moved to [secrets-storage-packaging.md](secrets-storage-packaging.md).

## Audit and Observability

Moved to [secrets-storage-audit-deletion.md](secrets-storage-audit-deletion.md).

## Defence in Depth Summary

Moved to [secrets-storage-audit-deletion.md](secrets-storage-audit-deletion.md).

## Credential Deletion

Moved to [secrets-storage-audit-deletion.md](secrets-storage-audit-deletion.md).

## Validation

Moved to [secrets-storage-validation-api.md](secrets-storage-validation-api.md).

## Web API Endpoints

Moved to [secrets-storage-validation-api.md](secrets-storage-validation-api.md).

## Configuration Reference

Moved to [secrets-storage-validation-api.md](secrets-storage-validation-api.md).

## Implementation Notes

Moved to [secrets-storage-validation-api.md](secrets-storage-validation-api.md).

---

## Related Specifications

| Specification | Relevance |
|---------------|-----------|
| [`configuration/Specification.md`](../configuration/Specification.md) | YAML schema for credential references and env var overrides |
| [`datastore/Specification.md`](../datastore/Specification.md) | `credentials` table schema, encryption model, retention, deletion |
| [`web-api/Specification.md`](../web-api/Specification.md) | Admin credential CRUD + test endpoints |
| [`chef-api/Specification.md`](../chef-api/Specification.md) | Chef API signing using resolved credentials |
| [`packaging/Specification.md`](../packaging/Specification.md) | RPM/DEB env files, Docker Compose configuration |
| [`tls/Specification.md`](../tls/Specification.md) | TLS key file handling, ACME storage |
| [`auth/Specification.md`](../auth/Specification.md) | Local account password hashing |
| [`logging/Specification.md`](../logging/Specification.md) | `secrets` log scope definition |
