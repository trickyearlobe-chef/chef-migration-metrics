# Active Plan — SAML customer-site fixes

Customer EntraID/Google SAML issues. Full diagnosis: `plans/saml-customer-fixes.md`.
Constraint: customer is VDI/file-transfer only — design for support-bundle/screenshot
diagnostics. No customer data in the repo (deny-patterns enforce this).

## Next chunk — optional follow-ups

- Config warning when a SAML provider has empty `username_attr` (username then
  falls back to NameID; with a transient NameID this breaks login anchoring AND
  ownership matching — see `plans/todo-ownership.md`).
- Turn the remaining local-user username collision (`ErrAlreadyExists` →
  opaque 500 "User provisioning failed") into a clear, actionable message.

## Done (on branch feature/saml-debug-log-assertions — pending merge)
- **Assertion diagnostic toggle**: `debug_log_assertions` per-SAML-provider bool
  (default OFF). When ON, `ParseACSResponse` marshals the decrypted assertion and
  logs the full XML at WARN (PII + replayable credential notice). `samlsp.Config`
  field + `logAssertionIfEnabled`; `config.AuthProvider.DebugLogAssertions`
  (`yaml:"debug_log_assertions,omitempty"`); wired in `buildSAMLProvider`. Frontend
  type + amber-warning checkbox in `AdminAuthPage.tsx`. Live-reload (no restart).
  Go tests (on/off) + 2 vitest cases; `auth.md` updated.

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
