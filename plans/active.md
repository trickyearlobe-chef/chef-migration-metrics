# Active Plan — SAML customer-site fixes

Three EntraID SAML issues from the customer. Full diagnosis + fixes:
`plans/saml-customer-fixes.md`. Diagnosed 2026-06-18; not yet implemented.

- **Issue 2/3** (IdP + app): assertion arrives with no attributes
  (`attributes: []`) → username falls back to opaque NameID and role to viewer;
  JIT also overwrites role from SAML every login. Add an assertion-XML diagnostic,
  confirm EntraID claim release, then fix Entra config + decide role precedence.

Constraint: customer is VDI/file-transfer only — design for support-bundle/screenshot
diagnostics. No customer data in the repo (deny-patterns enforce this).

## Done
- **Issue 1** (frontend-only): `AdminAuthPage.handleSave` now honours the API's
  `restartRequired` (no more hardcoded `true`) and re-fetches `fetchSAMLEndpoints`
  after save so ACS/SLO/entity copy fields reflect the new base URL; dropped the
  stale "Changes require an application restart." subtitle. TDD, 4 new vitest
  cases. Branch `fix/saml-sp-baseurl-refresh` (awaiting merge).
- Frontend deps pinned to Harness-registry-permitted versions; `frontend/.npmrc`
  points at the Harness Artifact Registry. Merged from `fix/pin-harness-blocked-deps`.
