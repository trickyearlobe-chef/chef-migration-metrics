# Authentication and Authorisation - Component Specification

> Component specification for the Chef Migration Metrics authentication and authorisation system.

---

## TL;DR

Two authentication providers (both can be active simultaneously): **local accounts** (bcrypt-hashed passwords, admin-created) and **SAML 2.0** (SP-initiated SSO, JIT provisioning). RBAC with three hierarchical roles: `admin` (full access), `operator` (operational actions globally), and `viewer` (read-only, with ownership-scoped operational actions on owned resources). Sessions via secure HTTP-only cookies with configurable expiry. Owner identity aliasing links app users to ownership records for permission resolution. All auth config in `configuration.md`.

---

## Overview

The web UI must restrict access to authenticated and authorised users. Two authentication providers must be supported: local user accounts and SAML. Multiple providers may be active simultaneously, allowing organisations to choose the most appropriate method for their environment.

---

## Authentication Providers

### Local User Accounts

- Users are created and managed within the application.
- Passwords must be stored as salted hashes using bcrypt.
- Password complexity and minimum length must be configurable.
- Accounts must be lockable by an administrator.
- SAML-provisioned users cannot use local password authentication unless explicitly granted a local password by an admin.

### SAML 2.0

The application acts as a SAML 2.0 Service Provider (SP). An external Identity Provider (IdP) is configured by the administrator.

### SP Endpoints

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/saml/metadata` | GET | Serve SP metadata XML for IdP configuration |
| `/saml/login` | GET | Initiate SP-initiated SSO (redirect to IdP) |
| `/saml/acs` | POST | Assertion Consumer Service — process IdP response |
| `/saml/slo` | POST | Inbound Single Logout — process IdP LogoutRequest |

### SP-Initiated SSO Flow

1. User navigates to `/saml/login` (or clicks "Sign in with SSO")
2. SP generates AuthnRequest, stores request state server-side keyed by request ID
3. User is redirected to IdP SSO URL with AuthnRequest
4. User authenticates at IdP
5. IdP POSTs SAML Response to `/saml/acs`
6. SP validates response and assertion (see Assertion Validation below)
7. SP performs JIT provisioning (see below)
8. SP creates session and sets session cookie
9. SP redirects user to RelayState URL (must be relative path or allowlisted origin)

### SAML Request State

SAML correlation state (AuthnRequest ID, RelayState, timestamp) must be stored server-side, not in cookies. The ACS handler retrieves state using the `InResponseTo` field from the SAML Response. This avoids SameSite cookie issues with cross-site POST.

### Assertion Validation

The SP must validate all of the following on every ACS response:

- Response signature OR assertion signature using trusted IdP certificate(s)
- Audience restriction matches SP entity ID
- Recipient/Destination matches ACS URL
- `InResponseTo` matches a stored AuthnRequest ID (SP-initiated flows)
- `NotBefore` / `NotOnOrAfter` with configurable clock skew tolerance (default ±5min)
- Assertion ID is not replayed (maintain short-lived cache of seen assertion IDs)
- Issuer matches configured IdP entity ID
- XML signature wrapping protections (delegated to `crewjam/saml` library)

If `allow_idp_initiated` is false (default), responses without `InResponseTo` must be rejected.

### Inbound Single Logout (SLO)

- The `/saml/slo` endpoint accepts `POST` with a signed `<LogoutRequest>` from the IdP.
- The LogoutRequest signature must be validated using the trusted IdP certificate.
- On valid request, all sessions for the identified user where `auth_provider = 'saml'` are invalidated.
- Local password sessions for the same username are NOT invalidated by SAML SLO.
- A `<LogoutResponse>` is returned to the IdP.
- If the NameID cannot be resolved to a user, return success (idempotent).

SP-initiated SLO is not implemented. Local logout destroys only the app session.

### SP Key Storage

- The SP signing certificate and private key must be stored in the encrypted database credential store.
- File-path-based key storage is not supported for SAML SP keys.
- Credential names are referenced in config: `sp_certificate_credential`, `sp_private_key_credential`.

### Attribute Mappings

Configurable attribute names to extract identity fields from SAML assertions:

| Config Field | Purpose | Default |
|--------------|---------|---------|
| `username_attr` | Primary user identifier | (NameID used if empty) |
| `email_attr` | User email address | `email` |
| `display_name_attr` | Human-readable name | `displayName` |
| `groups_attr` | Group membership list | `groups` |

### IdP Metadata

- IdP metadata must be fetchable from a configured HTTPS URL.
- Metadata is cached locally with configurable refresh interval (default 24h).
- Only `https://` metadata URLs are accepted (SSRF protection).
- Fetch timeout: 30 seconds. Response size limit: 1 MB.
- On refresh failure, last-known-good metadata continues to be used.
- IdP certificate rollover is supported via metadata refresh (new certs in metadata are trusted alongside existing ones until old ones expire).

### RelayState Security

- RelayState values returned after SSO must be validated before redirect.
- Only relative paths (starting with `/`) are accepted.
- Absolute URLs are rejected to prevent open redirect attacks.

---

## JIT User Provisioning

On successful SAML assertion:

1. Resolve user identity using stable federated key: `{idp_entity_id}:{NameID}` stored as `saml_subject` on the user record.
2. If no user exists with that `saml_subject`:
   - Create user with `auth_provider: "saml"`, mapped role, display_name, email.
   - Store `saml_subject` for future login matching.
   - Auto-link to owner via alias if email matches an existing alias (log the link).
3. If user exists:
   - Update display_name and email if changed.
   - Re-evaluate role from group mapping (see Role Mapping below).
4. Create session with `auth_provider: "saml"`.

### Identity Matching

- User identity is matched by `saml_subject` (`{idp_entity_id}:{NameID}`), NOT by email or username alone.
- Email and username are display/contact fields that may change; they do not determine identity.
- A SAML login never automatically links to an existing local-password user. If a local user with the same username exists, the SAML user is created as a separate record (admin can merge manually).

---

## Authorisation

### Role Model

Three hierarchical roles (higher role inherits all permissions of lower roles):

| Role | Level | Permissions |
|------|-------|-------------|
| `viewer` | 1 | Read dashboards, lists, logs, export downloads. Default for all authenticated users. |
| `operator` | 2 | All viewer permissions PLUS: trigger rescans, manage kitchen batches/runs, manage ownership assignments, trigger exports — on any resource globally. |
| `admin` | 3 | All operator permissions PLUS: user management, config changes, credential management, hypervisor control. |

### Ownership-Scoped Permissions

A `viewer` who is a resolved owner of a specific resource may perform operational actions on that resource:
- Re-fetch cookbook from Chef Server
- Re-run CookStyle analysis
- Trigger Test Kitchen run
- Re-clone git repository

Ownership-scoped permissions do NOT include:
- Modifying ownership assignments (requires `operator`)
- Managing owner aliases (requires `operator`)
- Any admin-level actions

### Permission Resolution

For each protected action, the middleware checks (in order):
1. Does the user's role grant the action globally? → allow
2. Is the action ownership-scopable AND is the user a resolved owner of the target resource? → allow
3. Otherwise → deny (403)

Owner resolution: user's `saml_subject` or username → `owner_aliases` lookup → owner → `ownership_assignments` → resource.

### Role Assignment

- Local users: role assigned directly by admin.
- SAML users: role derived from group membership via `role_mapping` config on each login.
- If multiple SAML groups match, the highest-privilege role wins.
- If no groups match, the default role is `viewer`.
- Role changes take effect on next login (existing sessions retain the role from login time).

### Capability-Based Checks

Authorization checks should be expressed as named capabilities internally (e.g. `CanManageUsers`, `CanTriggerRescan`, `CanManageOwnership`) that derive from role level. This allows future fine-grained permission models without rewriting handler logic.

### SAML User Restrictions

- SAML-provisioned users cannot change their own password (they have no local password).
- Admins can lock/unlock SAML users (prevents login even with valid IdP assertion).
- If a SAML user is removed from all IdP groups, they retain `viewer` on next login.

---

## Owner Identity Aliasing

### `owner_aliases` Table

| Field | Type | Constraints |
|-------|------|-------------|
| `owner_name` | TEXT | FK → `owners.name`, NOT NULL |
| `alias_type` | TEXT | One of: `email`, `name`, `username`, `saml_subject` |
| `alias_value` | TEXT | Case-insensitive, NOT NULL |
| `created_at` | TIMESTAMPTZ | Row creation time |

**Unique constraint:** `(alias_type, alias_value)` — one identity maps to exactly one owner.

For `saml_subject` aliases, the value includes the IdP entity ID prefix to avoid cross-IdP collisions: `{idp_entity_id}:{subject}`.

### Fuzzy Identity Matching

To help administrators link identities, the system suggests potential matches:

**Deterministic rules (high confidence):**
- Same email address (case-insensitive)
- Same full name string
- Email local-part matches existing username alias

**Trigram similarity (`pg_trgm` extension, threshold 0.3):**
- Near-matches on name/email strings
- Catches typos, domain changes, name abbreviations

**Token overlap (normalisation-based):**
- Lowercase, strip dots from email local-parts
- Extract tokens, compare overlap

Suggestions are surfaced via API and displayed in the UI as dismissible prompts. Auto-linking from fuzzy matches requires explicit user confirmation. Only deterministic email matches from trusted IdPs may be auto-linked without confirmation (logged).

---

## Session Management

- Sessions are server-side records in PostgreSQL, identified by UUID token.
- Session token is delivered via HTTP-only cookie (`SameSite=Lax`, `Secure` when TLS).
- Session expiry is configurable (default 8h).
- Users may log out explicitly, invalidating their session.
- Session cookie uses `SameSite=Lax` (not Strict) to allow the post-ACS redirect to carry the cookie on subsequent navigation.

---

## Security Requirements

- All authentication traffic must be over HTTPS. Plain HTTP must not be accepted for login flows.
- SAML SP private keys must be stored in the encrypted credential store. They must never appear in config files, source control, or logs.
- Failed login attempts must be logged with timestamp and source IP.
- Brute-force protection (account lockout after configurable failed attempts) is implemented for local accounts.
- SAML login success/failure, JIT user creation, role changes, owner alias linking, and SLO events must all be logged.
- Permission denials must be logged with username, role, requested action, and resource.

---

## Configuration

Authentication provider configuration is part of the application configuration file. See the [Configuration specification](configuration.md) for the overall configuration structure.

### Global Settings

| Setting | Description | Default |
|---------|-------------|---------|
| `auth.session_expiry` | Session lifetime | `8h` |
| `auth.min_password_length` | Minimum password length for local accounts | `8` |
| `auth.lockout_attempts` | Failed attempts before account lockout | `5` |

### Provider Configuration

```yaml
auth:
  session_expiry: "8h"
  min_password_length: 8
  lockout_attempts: 5
  providers:
    - type: local
    - type: saml
      idp_metadata_url: "https://idp.example.com/metadata"
      sp_entity_id: "https://cmm.example.com"
      sp_certificate_credential: "saml-sp-cert"
      sp_private_key_credential: "saml-sp-key"
      username_attr: ""
      email_attr: "email"
      display_name_attr: "displayName"
      groups_attr: "groups"
      role_mapping:
        "cmm-admins": "admin"
        "cmm-operators": "operator"
      allow_idp_initiated: false
      sign_requests: true
      clock_skew_tolerance: "5m"
      metadata_refresh_interval: "24h"
```