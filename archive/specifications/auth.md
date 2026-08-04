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

`/saml/acs` is the single Assertion Consumer Service for **both** SP-initiated and
IdP-initiated SSO: the IdP POSTs the SAML Response there in either case (HTTP-POST
binding). IdP-initiated (unsolicited, no `InResponseTo`) is accepted only when
`allow_idp_initiated` is enabled; otherwise the response reaches the ACS but fails
validation.

### SP Metadata Export (UI)

The admin SAML configuration page provides an **Export SP Metadata (XML)** button
that downloads this SP's metadata document so an administrator can hand it to the
IdP during setup. The button fetches the existing `/saml/metadata` endpoint and
saves the response as a file (e.g. `sp-metadata.xml`, `application/samlmetadata+xml`).

The button is available only when a SAML provider is configured and initialised —
the metadata endpoint returns `501 Not Implemented` otherwise (no SP to describe).
The exported document is the live SP metadata (entity ID, ACS/SLO URLs, SP signing
certificate, NameID format); it contains no private key material.

The page also surfaces the absolute **SP Metadata URL** (with a copy action) for
IdPs that fetch metadata by URL and refresh it automatically (e.g. ADFS,
Shibboleth, Keycloak, PingFederate); IdPs without URL support (e.g. Google, Okta)
take the downloaded file. `/saml/metadata` is a public endpoint (no session
required) so the IdP can poll it directly — it exposes no private key material.

Alongside the metadata URL the page surfaces the absolute **ACS (callback) URL**,
**SLO URL**, and **SP entity ID**, each with a copy action. These are computed by
the backend from the same base URL the SP metadata advertises, so they match
exactly what the IdP must be told. They exist for IdPs configured by hand rather
than by metadata import (e.g. Google, Okta), where the administrator pastes the
ACS/reply URL directly — without a surfaced value the correct path (`/saml/acs`,
not the `/saml` prefix) is not discoverable. The values are served by an
admin-only endpoint and are available only once a SAML provider is configured
and initialised.

The advertised base URL (scheme + host + port) comes from the per-provider
**`sp_base_url`** setting. The admin auth page defaults this field to the browser
origin the administrator is currently using — i.e. the externally-reachable
scheme/host (and port, if non-standard) — and persists it on save, so the
exported metadata points at a host users can actually reach. When `sp_base_url`
is unset the backend falls back to the host of an http(s) `sp_entity_id`, and
finally to `scheme://localhost:<effective-https-port>`. The fallback uses the
effective HTTPS port (e.g. 443 under automatic HTTPS), never the
HTTP-redirect/plain port — the previous hardcoded `https://localhost:8080` was
both unreachable and pointed at the redirect listener. `sp_base_url` must be an
absolute http(s) URL with no path.

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

IdP metadata is supplied by exactly one of three mutually exclusive sources,
selected in the admin auth page via a dropdown:

- `idp_metadata_url` — fetched from a configured HTTPS URL (refreshable).
- `idp_metadata_path` — read from a local file (e.g. Google Workspace export).
- `idp_metadata_xml` — pasted directly and stored inline in the provider config
  (for IdPs that expose neither a fetchable URL nor a server-readable file).

Rules:
- Exactly one source must be set; setting more than one is a validation error.
- Only `https://` metadata URLs are accepted (SSRF protection).
- Size limit: 1 MB (applies to all three sources). URL fetch timeout: 30 seconds.
- The URL source is cached locally with a configurable refresh interval
  (default 24h); on refresh failure, last-known-good metadata continues to be
  used. The file and pasted-XML sources are static — there is nothing to refresh.
- IdP certificate rollover is supported for the URL source via metadata refresh
  (new certs in metadata are trusted alongside existing ones until old ones
  expire). For file/pasted sources, rollover requires re-saving the metadata.

### RelayState Security

- RelayState values returned after SSO must be validated before redirect.
- Only relative paths (starting with `/`) are accepted.
- Absolute URLs are rejected to prevent open redirect attacks.

---

## JIT User Provisioning

On successful SAML assertion:

1. Resolve user identity, in order:
   a. By federated key `{idp_entity_id}:{NameID}` stored as `saml_subject` (the normal case — IdP sends a stable NameID).
   b. Fallback: by the stable `username` of an existing SAML user, refreshing that user's `saml_subject`. This tolerates IdPs that emit an unstable (transient) NameID, where the federated key changes every login while the username attribute stays constant.
2. If no existing SAML user matches:
   - Create user with `auth_provider: "saml"`, mapped role, display_name, email.
   - Store `saml_subject` for future login matching.
   - Auto-link to owner via alias if email matches an existing alias (log the link).
3. If an existing SAML user matches (by either key):
   - Update display_name and email if changed; refresh `saml_subject` from the current assertion.
   - Re-evaluate role from group mapping (see Role Mapping below).
4. Create session with `auth_provider: "saml"`.

### Identity Matching

- User identity is matched by `saml_subject` (`{idp_entity_id}:{NameID}`), with a fallback to the stable `username` of an existing SAML user when `saml_subject` does not match (e.g. an unstable/transient NameID). The matched row's `saml_subject` is then refreshed.
- Email is a display/contact field that may change; it does not determine identity.
- The username fallback applies only to SAML users. A SAML login never links to or takes over an existing local-password user: if the username belongs to a local account, provisioning fails rather than hijacking it (admin can resolve manually). The proper fix for an unstable NameID remains IdP-side — configure a persistent NameID.

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
      # ...or idp_metadata_path / idp_metadata_xml (exactly one source).
      sp_entity_id: "https://cmm.example.com"
      sp_base_url: "https://cmm.example.com"
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
      debug_log_assertions: false
      clock_skew_tolerance: "5m"
      metadata_refresh_interval: "24h"
```

`sign_requests`, `allow_idp_initiated`, and `debug_log_assertions` are editable as
checkboxes on the admin auth page (per SAML provider). When `sign_requests` is
enabled, outgoing AuthnRequests are signed with the SP key (RSA-SHA256) and the SP
metadata advertises `AuthnRequestsSigned="true"`; the IdP validates the signature
against the SP signing certificate published in that metadata.

`debug_log_assertions` (default `false`) is a diagnostic toggle: when enabled, the
full decrypted assertion XML is written to the server log at the ACS point on every
login, at `WARN` level with a notice that the output contains PII and a replayable
credential. It exists to troubleshoot attribute/mapping problems on a customer site
reachable only via support bundle or screenshot. Leave it off in normal operation
and turn it back off once finished. Like the other flags it takes effect on the
next login without a restart (the provider is rebuilt live on save).

The entire auth section live-reloads (per `configuration-live-reload.md`); no auth
change requires a restart:

- **SAML provider (subsystem):** on save the auth section's applier reconstructs the
  provider from the reloaded config (re-resolving SP credentials and re-fetching IdP
  metadata) and swaps it into the running SAML handler under a lock. `sign_requests`,
  `allow_idp_initiated`, `debug_log_assertions`, `sp_entity_id`, the IdP metadata
  source, and the attribute/role mappings all take effect immediately. A rebuild failure surfaces as
  an error on the save (the previous provider keeps serving) rather than being
  silently dropped.
- **Session expiry / lockout / min password length (applied):** `session_expiry`,
  `lockout_attempts`, and `min_password_length` are read live at the point of use
  (session creation, login attempt, password validation) rather than captured at
  startup, so a change applies to the next session/login without a restart.

Because every part of the section re-applies live, the section reports
`restart_required: false`.