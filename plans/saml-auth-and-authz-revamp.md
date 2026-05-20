# SAML Auth & Authorisation Revamp

## Goal

Implement SAML 2.0 SP-initiated SSO, revamp authorisation to three roles (viewer/operator/admin) with ownership-scoped permissions, and add identity aliasing to the ownership model with fuzzy matching.

## Specs

- `.claude/specifications/auth.md` (update)
- `.claude/specifications/configuration.md` (reference)
- `.claude/specifications/ownership.md` (reference)

## Phases

1. ~~Update auth.md specification~~ — DONE
2. ~~Extend config schema~~ — DONE
3. ~~SAML SP implementation~~ — DONE
4. ~~JIT user provisioning~~ — DONE
5. ~~HTTP routes~~ — DONE
6. ~~Authorisation revamp~~ — DONE
7. ~~Ownership aliases~~ — DONE
8. ~~Import enhancements~~ — DONE
9. ~~Frontend (SSO button, alias management, role-aware UI)~~ — DONE

## Decisions

- Library: `crewjam/saml` v0.5.1
- SLO: inbound only
- SP key: encrypted DB credential store only
- Roles: viewer < operator < admin (schema supports future custom roles)
- Ownership identity: alias table with `pg_trgm` fuzzy suggestions
- Testing: Google Workspace as E2E IdP, mock fixtures for unit tests


