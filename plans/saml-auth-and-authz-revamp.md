# SAML Auth & Authorisation Revamp

## Goal

Implement SAML 2.0 SP-initiated SSO, revamp authorisation to three roles (viewer/operator/admin) with ownership-scoped permissions, and add identity aliasing to the ownership model with fuzzy matching.

## Specs

- `.claude/specifications/auth.md` (update)
- `.claude/specifications/configuration.md` (reference)
- `.claude/specifications/ownership.md` (reference)

## Phases

1. Update auth.md specification (SAML endpoints, 3 roles, ownership permissions)
2. Extend config schema (SAML fields, SP key via credential store)
3. SAML SP implementation (`crewjam/saml` v0.5.1)
4. JIT user provisioning + owner alias auto-linking
5. HTTP routes (`/saml/metadata`, `/saml/login`, `/saml/acs`, `/saml/slo`)
6. Authorisation revamp (viewer/operator/admin + ownership-scoped middleware)
7. Ownership aliases (`owner_aliases` table + `pg_trgm` fuzzy matching)
8. Import enhancements (CSV aliases, CMDB adapter)
9. Frontend (SSO button, alias management, role-aware UI)

## Decisions

- Library: `crewjam/saml` v0.5.1
- SLO: inbound only
- SP key: encrypted DB credential store only
- Roles: viewer < operator < admin (schema supports future custom roles)
- Ownership identity: alias table with `pg_trgm` fuzzy suggestions
- Testing: Google Workspace as E2E IdP, mock fixtures for unit tests
