# Web API — Static Assets, Rate Limiting & CORS

## Static Assets and Frontend

The web dashboard frontend is a single-page application (SPA) served by the Go backend. All routes not matching `/api/` are served from the embedded static assets directory, with a fallback to `index.html` for client-side routing.

```
GET /              → serves index.html
GET /dashboard     → serves index.html (client-side route)
GET /assets/*      → serves static files (JS, CSS, images)
GET /api/v1/*      → API endpoints (documented above)
GET /api/v1/ws     → WebSocket endpoint (documented above)
```

---

## Rate Limiting

- The login endpoint (`POST /api/v1/auth/login`) must be rate-limited per source IP to prevent brute-force attacks.
- API endpoints may optionally be rate-limited per authenticated user to prevent abuse, but this is not required for the initial implementation.

---

## CORS

If the frontend is served from the same origin as the API (recommended), no CORS configuration is needed. If a separate frontend origin is used, the API must support configurable CORS headers:

- `Access-Control-Allow-Origin`
- `Access-Control-Allow-Methods`
- `Access-Control-Allow-Headers`
- `Access-Control-Allow-Credentials`

---
