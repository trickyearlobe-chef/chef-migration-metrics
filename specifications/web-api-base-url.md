# Web API — Base URL & Content Type

## Base URL

All API endpoints are served under the `/api/v1` prefix:

```
https://<HOST>:<PORT>/api/v1
```

The application listens on a configurable port (default: `8080`). HTTPS termination may be handled by a reverse proxy or natively by the application if TLS certificate paths are configured.

---

## Content Type

- All request bodies must be `application/json`.
- All response bodies are `application/json`.
- Responses include `Content-Type: application/json; charset=utf-8`.

---
