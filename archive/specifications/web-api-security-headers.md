# Web API — Security Response Headers

## Security Response Headers

All responses include the following HTTP security headers:

| Header | Value | Purpose |
|--------|-------|---------|
| `X-Content-Type-Options` | `nosniff` | Prevents browsers from MIME-sniffing a response away from the declared content type |
| `X-Frame-Options` | `DENY` | Prevents the app from being embedded in an iframe (clickjacking protection) |
| `Referrer-Policy` | `strict-origin-when-cross-origin` | Limits referrer information sent to cross-origin destinations |

These are applied by a `SecurityHeadersMiddleware` wrapping the entire router.

---



After authentication, the middleware checks the user's role against the endpoint's required permission level:

| Role | Permissions |
|------|-------------|
| `viewer` | Read access to all dashboard, log, ownership, and status endpoints |
| `operator` | All viewer permissions plus create/update owners and assignments, bulk import, bulk reassignment (without delete-source-owner) |
| `admin` | All operator permissions plus user management, owner deletion, bulk reassignment with delete-source-owner, manual rescan triggers, and configuration |

Endpoints that require `admin` or `operator` are annotated below. All other authenticated endpoints require at minimum `viewer`.

Unauthorised requests return `403 Forbidden`:

```json
{
  "error": "forbidden",
  "message": "This action requires the admin role."
}
```

---
