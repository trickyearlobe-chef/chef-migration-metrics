# Authentication and Authorisation - Component Specification

> Component specification for the Chef Migration Metrics authentication and authorisation system.
> See the [top-level specification](../Specification.md) for project-wide context.

---

## TL;DR

Three authentication providers (all can be active simultaneously): **local accounts** (bcrypt-hashed passwords, admin-created), **LDAP** (bind-then-search, group mapping), and **SAML 2.0** (SP-initiated SSO, auto-provisioning). RBAC with two roles: `admin` (full access, user management) and `viewer` (read-only dashboards and exports). Sessions via secure HTTP-only cookies with configurable expiry. All auth config in `configuration/Specification.md`.

---

## Overview

The web UI must restrict access to authenticated and authorised users. Three authentication providers must be supported: local user accounts, LDAP, and SAML. Multiple providers may be active simultaneously, allowing organisations to choose the most appropriate method for their environment.

---

## Authentication Providers

### Local User Accounts

- Users are created and managed within the application.
- Passwords must be stored as salted hashes using a modern algorithm (e.g. bcrypt).
- Password complexity and minimum length must be configurable.
- Accounts must be lockable by an administrator.

### LDAP

- The application must bind to a configured LDAP server to authenticate users.
- User lookup must be configurable via base DN and search filter.
- LDAP group membership may optionally be used to determine application roles (see Authorisation below).
- Both plain LDAP and LDAPS (TLS) must be supported.
- Configuration must support:
  - LDAP server host and port
  - Bind DN and password for the service account
  - User search base DN and filter
  - Optional group search base DN and filter for role mapping
  - TLS/LDAPS toggle and CA certificate path

### SAML

- The application must act as a SAML 2.0 Service Provider (SP).
- An external Identity Provider (IdP) is configured by the administrator.
- SAML assertions must be used to establish the user's identity.
- SAML attribute mappings must be configurable to extract username, display name, email, and group membership from the assertion.
- Group membership from the SAML assertion may optionally be used to determine application roles (see Authorisation below).
- Configuration must support:
  - IdP metadata URL or inline metadata XML
  - SP entity ID
  - SP assertion consumer service (ACS) URL
  - Attribute mappings for username, email, display name, and groups

---

## Authorisation

- The application must implement role-based access control (RBAC).
- Initial roles to be defined:
  - **Admin** — full access including user management and configuration
  - **Viewer** — read-only access to all dashboard views and logs
- Roles must be assignable to local users directly.
- Roles may be mapped from LDAP group membership or SAML group assertions via configuration.
- Unauthenticated requests to any protected route must be redirected to the login page.

---

## Session Management

- Sessions must be issued as signed tokens (e.g. JWT or signed server-side session).
- Session expiry must be configurable.
- Users must be able to log out explicitly, invalidating their session.

---

## Security Requirements

- All authentication traffic must be over HTTPS. Plain HTTP must not be accepted for login flows.
- SAML and LDAP credentials (bind passwords, private keys) must never be stored in source control and must be configurable via environment variables or key files.
- Failed login attempts must be logged with timestamp and source IP.
- Brute-force protection (e.g. account lockout or rate limiting on login) must be implemented for local accounts.

---

## Configuration

Authentication provider configuration is part of the application configuration file. See the [Configuration specification](../configuration/Specification.md) for the overall configuration structure.

Provider-specific settings are documented in the sections above. The following top-level settings apply globally:

| Setting | Description |
|---------|-------------|
| `auth.providers` | List of enabled providers: `local`, `ldap`, `saml` |
| `auth.session_expiry` | Session lifetime (e.g. `8h`, `24h`) |
| `auth.local.min_password_length` | Minimum password length for local accounts |
| `auth.local.lockout_attempts` | Number of failed attempts before account lockout |