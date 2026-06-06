# Web API — Authentication and Session Management

## Authentication and Session Management

### Login

#### `POST /api/v1/auth/login`

Authenticates a user via the local provider and returns a session token.

**Request:**

```json
{
  "username": "alice",
  "password": "s3cret"
}
```

**Response (200):**

```json
{
  "token": "<signed-session-token>",
  "expires_at": "2024-07-01T08:00:00Z",
  "user": {
    "username": "alice",
    "display_name": "Alice Smith",
    "role": "admin"
  }
}
```

**Errors:**

| Status | Condition |
|--------|-----------|
| `400` | Missing or malformed request body |
| `401` | Invalid credentials |
| `423` | Account locked due to excessive failed attempts |
| `429` | Rate limited — too many login attempts from this source |

#### `POST /api/v1/auth/saml/acs`

SAML Assertion Consumer Service endpoint. Receives the SAML response from the IdP, validates the assertion, establishes a session, and redirects to the dashboard.

**Public** — no existing session required.

#### `GET /api/v1/auth/saml/metadata`

Returns the SAML Service Provider metadata XML.

**Public** — no existing session required.

#### `GET /api/v1/auth/saml/login`

Initiates the SAML authentication flow by redirecting to the configured IdP.

**Public** — no existing session required.

### Logout

#### `POST /api/v1/auth/logout`

Invalidates the current session.

**Response (204):** No content.

### Session Enforcement

- All endpoints except those marked **Public** require a valid session token.
- The session token must be sent in the `Authorization` header as a Bearer token:
  ```
  Authorization: Bearer <token>
  ```
- Alternatively, the token may be sent in a secure, HTTP-only cookie named `session`.
- If the token is missing, expired, or invalid, the API returns `401 Unauthorized`.
- Session expiry is configured via `auth.session_expiry` (see [Configuration specification](../configuration/Specification.md)).

### Current User

#### `GET /api/v1/auth/me`

Returns the authenticated user's profile and role.

**Response (200):**

```json
{
  "username": "alice",
  "display_name": "Alice Smith",
  "email": "alice@example.com",
  "role": "admin",
  "provider": "local"
}
```

---
