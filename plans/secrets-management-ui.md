# Plan: Secrets Management UI

## Goal

Implement the Web API credential CRUD endpoints and the frontend admin UI for managing vSphere Test Kitchen credentials (and all other credential types). This covers the "Web API Integration" section of `todo-secrets-storage.md`.

## Specs to Read

- `.claude/specifications/secrets-storage.md` §Web API Endpoints, §Audit and Observability
- `.claude/specifications/todo-secrets-storage.md` §Web API Integration
- `.claude/specifications/web-api.md` (if it exists — for response envelope conventions)

## Existing Code

- `internal/secrets/` — complete: store interface, DBCredentialStore, encryption, validation, resolver, rotation, zeroing
- `internal/webapi/router.go` L372–373 — credential routes registered as `handleNotImplemented`
- `internal/webapi/handle_admin_users.go` — reference pattern for admin CRUD handlers
- `internal/webapi/store_mock_test.go` — mock store needs no changes (credentials use `secrets.CredentialStore`, not `DataStore`)
- `internal/secrets/db_store_test.go` — `InMemoryCredentialStore` already exists for test use
- `frontend/src/pages/AdminUsersPage.tsx` — reference pattern for admin CRUD UI
- `frontend/src/api.ts` — API client functions to extend

## Ordered Steps

### 1. Add `CredentialStore` to Router

- Add `credentialStore secrets.CredentialStore` field to `Router` struct
- Add `WithCredentialStore(store secrets.CredentialStore) RouterOption` constructor option
- Update `registerRoutes` to wire credential routes when store is non-nil (replace `handleNotImplemented`)

### 2. Write handler tests (TDD — tests first)

- Create `internal/webapi/handle_credentials_test.go`
- Import `InMemoryCredentialStore` from `internal/secrets` package
- Test cases per the todo:
  - GET /api/v1/admin/credentials — list (empty, populated, metadata-only)
  - POST /api/v1/admin/credentials — create (success, duplicate, validation errors, missing fields)
  - PUT /api/v1/admin/credentials/:name — rotate (success, not found, validation error)
  - DELETE /api/v1/admin/credentials/:name — delete (success, not found, in-use, missing confirm)
  - POST /api/v1/admin/credentials/:name/test — test (success, not found)
  - 503 when encryption key not configured (credentialStore is nil)
  - Auth enforcement: non-admin rejected (already covered by `adminOnly` middleware)
  - Verify no response body contains `encrypted_value` or plaintext

### 3. Implement credential handlers

- Create `internal/webapi/handle_credentials.go`
- `handleCredentials` — dispatch GET/POST on collection, PUT/DELETE/POST on item
- `handleListCredentials` — GET, returns paginated metadata
- `handleCreateCredential` — POST, validates, encrypts, stores
- `handleUpdateCredential` — PUT :name, rotates value
- `handleDeleteCredential` — DELETE :name, requires `?confirm=true`, checks references
- `handleTestCredential` — POST :name/test, returns validation result
- All handlers: log at INFO with `scope: secrets`, never return encrypted_value/plaintext
- Run tests until green

### 4. Add frontend API functions

- Add to `frontend/src/api.ts`:
  - `fetchCredentials()` — GET list
  - `createCredential(body)` — POST create
  - `updateCredential(name, body)` — PUT rotate
  - `deleteCredential(name)` — DELETE
  - `testCredential(name)` — POST test

### 5. Build frontend credentials page

- Create `frontend/src/pages/AdminCredentialsPage.tsx`
- Table listing credentials (name, type, created_by, last_rotated_at, actions)
- Create modal (name, type dropdown, value textarea)
- Rotate modal (new value textarea)
- Delete confirmation modal (shows references if in-use)
- Test button with result display
- Follow patterns from `AdminUsersPage.tsx`

### 6. Wire frontend route

- Add import and Route in `frontend/src/App.tsx` for `/admin/credentials`
- Add nav link in `AppLayout` sidebar (admin section)

### 7. Update todos

- Mark completed items in `todo-secrets-storage.md` §Web API Integration
- Update `todo-tech-debt.md` P3 if notify references are resolved

## Acceptance Criteria

- All handler tests pass (`go test ./internal/webapi/ -run TestCredential`)
- All existing tests still pass (`go test ./...`)
- Frontend builds without errors (`cd frontend && npm run build`)
- No response from any credential endpoint contains `encrypted_value` or plaintext
- 503 returned when `CredentialStore` is nil
- Admin role required on all credential endpoints
- All credential operations logged at INFO with scope: secrets