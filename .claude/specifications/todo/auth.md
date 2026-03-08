# Authentication and Authorisation — ToDo

Status key: [ ] Not started | [~] In progress | [x] Done

---

## Local Authentication

- [x] Implement local user account authentication — `internal/auth/local.go` with `LocalAuthenticator` supporting username/password login, failed attempt tracking, account lockout (configurable threshold, default 5), login success recording; 129 tests across local, session, middleware, and password packages
- [x] Implement password hashing and validation — `internal/auth/password.go` with bcrypt hashing (`HashPassword`, `CheckPassword`), `ValidatePassword` enforcing minimum length, uppercase, lowercase, digit, and special character requirements
- [x] Implement session management — `internal/auth/session.go` with `SessionManager` supporting create/validate/invalidate sessions, per-user session invalidation, expired session cleanup; `SessionStore` interface backed by `internal/datastore/sessions.go` with full CRUD (insert, get, get-valid, delete, delete-by-user, delete-expired, list-by-user, count-active)
- [x] Implement authentication middleware — `internal/auth/middleware.go` with `RequireAuth` (session token from Authorization header or cookie), `RequireAdmin` (admin role check), `RequireRole` (configurable allowed roles), `Authenticated` and `AdminOnly` convenience wrappers; structured JSON 401/403 error responses
- [x] Implement session token extraction — `ExtractToken` supports `Authorization: Bearer <token>` header and `cmm_session` HTTP-only cookie; `SetSessionCookie` and `ClearSessionCookie` helpers
- [x] Implement default admin user creation on first startup — `datastore.EnsureDefaultAdmin` creates admin user if no users exist

## Web API Integration

- [x] Implement login endpoint — `POST /api/v1/auth/login` in `internal/webapi/handle_auth.go` accepting `{username, password}`, returns session token + user info, sets HTTP-only session cookie
- [x] Implement logout endpoint — `POST /api/v1/auth/logout` invalidates session, clears cookie
- [x] Implement current user endpoint — `GET /api/v1/auth/me` returns authenticated user profile (username, display name, email, role, auth provider)
- [x] Implement admin user list endpoint — `GET /api/v1/admin/users` in `internal/webapi/handle_admin_users.go` returns paginated user list (admin only)
- [x] Implement admin user creation endpoint — `POST /api/v1/admin/users` creates new user with username, password, role, display name, email (admin only)
- [x] Implement admin user update endpoint — `PUT /api/v1/admin/users/:username` updates display name, email, role, lock status (admin only)
- [x] Implement admin password reset endpoint — `PUT /api/v1/admin/users/:username/password` resets user password (admin only)
- [x] Implement admin user deletion endpoint — `DELETE /api/v1/admin/users/:username` deletes user and all sessions (admin only)
- [x] Wire auth middleware into router — `internal/webapi/router.go` applies `RequireAuth` to all API routes except login/health/version; `AdminOnly` wrapper for `/api/v1/admin/*` routes; graceful fallback when auth is not configured (no-auth dev mode)

## Frontend Integration

- [x] Implement login page — `frontend/src/pages/LoginPage.tsx` with username/password form, error display, redirect to dashboard on success
- [x] Implement auth context — `frontend/src/context/AuthContext.tsx` with `AuthProvider`, `useAuth` hook, session restoration via `GET /api/v1/auth/me` on mount, login/logout actions, `isAuthenticated`/`isAdmin` state
- [x] Implement route guards — `App.tsx` `RequireAuth` redirects to `/login` when unauthenticated, `RequireAdmin` redirects non-admin users, `LoginRoute` redirects authenticated users to dashboard
- [x] Implement user menu and logout — `AppLayout.tsx` displays user avatar, display name, role badge, and logout button in sidebar footer
- [x] Implement admin user management page — `frontend/src/pages/AdminUsersPage.tsx` with user list, create/edit/delete user dialogs (admin only); route `/admin/users` guarded by `RequireAdmin`

## Datastore

- [x] Implement users table operations — `internal/datastore/users.go` with `InsertUser`, `GetUserByUsername`, `GetUserByID`, `ListUsers`, `CountUsers`, `UpdateUser`, `UpdateUserPassword`, `DeleteUser`, `IncrementFailedLoginAttempts`, `LockUser`, `RecordLoginSuccess`, `EnsureDefaultAdmin`
- [x] Implement sessions table operations — `internal/datastore/sessions.go` with `InsertSession`, `GetSession`, `GetValidSession`, `DeleteSession`, `DeleteSessionsByUserID`, `DeleteSessionsByUsername`, `ListSessionsByUserID`, `CountActiveSessions`, `DeleteExpiredSessions`

## Role-Based Authorisation

- [x] Implement role-based access control — `admin` and `viewer` roles; middleware enforces role checks; admin-only routes for user management; `SessionInfo.IsAdmin()` and `IsViewer()` helpers

## Not Yet Implemented

- [ ] Implement LDAP authentication — config validation exists (`config.go` validates host, base_dn for ldap providers) but no LDAP client implementation
- [ ] Implement SAML authentication — config validation exists (`config.go` validates idp_metadata_url, sp_entity_id for saml providers) but no SAML client implementation
- [ ] Ensure credentials and secrets are never stored in source control — partially addressed (password hashing, HTTP-only cookies, no plaintext storage) but needs formal audit