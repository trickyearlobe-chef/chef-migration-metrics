# Active Plan — SAML customer-site fixes

Customer EntraID/Google SAML issues. Full diagnosis: `plans/saml-customer-fixes.md`.
Constraint: customer is VDI/file-transfer only — design for support-bundle/screenshot
diagnostics. No customer data in the repo (deny-patterns enforce this).

## Next chunk — assertion diagnostic toggle (start a fresh thread)

Add `saml_debug_log_assertions` (per-SAML-provider bool, dynamic, default OFF).
When ON: log the full decrypted assertion XML at the ACS point, with a WARN while
enabled (logs PII + a replayable credential — accepted, owner-approved). Off by default.

- Backend: `samlsp.Config.DebugLogAssertions`; in `ParseACSResponse`
  (`internal/auth/samlsp/provider.go`) marshal the decrypted `*saml.Assertion` and
  log when on. Config field in `internal/config/config.go` (SAML AuthProvider);
  wire in `buildSAMLProvider` (`cmd/.../main.go`). Provider is rebuilt live on
  config change, so the toggle takes effect without restart.
- Frontend: `debug_log_assertions?: boolean` in `frontend/src/types/config.ts`;
  a checkbox in `AdminAuthPage.tsx` (alongside Sign AuthnRequests / Allow IdP-init)
  with a sensitivity warning.
- TDD: Go (toggle on → assertion logged; off → not) + vitest checkbox.
- Spec: document the flag in `auth.md` / `configuration.md`.

## Then — optional follow-ups

- Config warning when a SAML provider has empty `username_attr` (username then
  falls back to NameID; with a transient NameID this breaks login anchoring AND
  ownership matching — see `plans/todo-ownership.md`).
- Turn the remaining local-user username collision (`ErrAlreadyExists` →
  opaque 500 "User provisioning failed") into a clear, actionable message.

## Done (merged to main)
- **Transient-NameID identity fix**: `UpsertSAMLUser` anchors on stable `username`
  (subject-match first, then username fallback refreshing `saml_subject`); local
  accounts never hijacked. Fixed repeated-login "User provisioning failed". 3
  functional tests; `auth.md` JIT/Identity sections updated. Also recorded the
  user↔owner/git alias-matching design in `plans/todo-ownership.md`.
- **Issue 1** (frontend): `AdminAuthPage.handleSave` honours the API's
  `restartRequired` and re-fetches `fetchSAMLEndpoints` after save; dropped the
  stale restart subtitle. 4 vitest cases.
- **Issue 2/3 role precedence**: decided — SAML stays authoritative (role
  re-evaluated from groups every login; manual edits not expected to persist). No
  code change. Customer's role issue resolved IdP-side (release group claims).
- Frontend deps pinned to Harness-registry-permitted versions.
