# Plan — SAML customer-site fixes (EntraID)

Three issues found at a customer using EntraID (Azure AD) SAML. Diagnosed
2026-06-18. No customer data here (placeholders only; real hostnames hit
`.git-deny-patterns`). Customer reachable via VDI/file transfer only — design
fixes for diagnostic capture (support bundle / screenshot).

## Issue 1 — SP base URL: ACS/SLO don't update on save + bogus restart [frontend] ✅ DONE

Implemented on `fix/saml-sp-baseurl-refresh` (TDD, 4 vitest cases). `handleSave`
now uses the API's `restartRequired` and re-fetches `fetchSAMLEndpoints` after a
successful save; stale "restart required" subtitle removed. Backend untouched.


Symptom: after editing SP Base URL and saving, the ACS/SLO copy fields still show
`localhost:8080`; UI shows "Restart required" though config should be dynamic; no
UI restart trigger, forcing a manual CLI restart.

Root cause: **frontend only** — `frontend/src/pages/AdminAuthPage.tsx` `handleSave`:
- line ~508 hardcodes `setRestartRequired(true)`, ignoring the API's
  `restart_required` (backend applies auth/SAML live via the `samlApplier`
  reconciler → returns `false`).
- never re-fetches `fetchSAMLEndpoints()` after save, so ACS/SLO/entity-id copy
  fields keep mount-time values. (Metadata URL only "updates" because it's derived
  client-side from `window.location.origin` in `api/saml.ts`.)
Backend is correct: `PUT /api/v1/admin/config/auth` runs `appliedApplier` +
`samlApplier` (rebuilds provider+endpoints via `buildSAMLProvider`, all four URLs
share one `baseURL`); `restart_required` is derived and stays false.

Fix (TDD, vitest): use returned `restartRequired`; after successful save re-fetch
SAML endpoints + update state; drop the stale "Changes require an application
restart." subtitle (line ~585). Backend untouched.

## Issue 2 — Username derived from wrong key (opaque NameID) [IdP + app robustness]

Symptom: usernames are opaque random chars instead of email.

Evidence (logs): `SAML assertion attributes: []` (DEBUG), `groups=[]`, username ==
NameID suffix. `flattenAttributes` (`internal/auth/samlsp/provider.go`) indexes by
both `attr.Name` and `attr.FriendlyName` and is correct — empty list means the
assertion carries **no AttributeStatement**. So EntraID is releasing no claims.
`extractUserInfo` then falls back username→NameID (Entra's opaque persistent id).

Likely real fix is IdP-side: configure the Entra Enterprise App to release email
(and groups) claims, and/or set NameID = email/UPN. Entra emits claims under long
URIs (e.g. `http://schemas.xmlsoap.org/ws/2005/05/identity/claims/emailaddress`),
so the configured `email_attr` must match that exactly, or set NameID to email.

App work:
- [ ] **Diagnostic first (the artifact for the IdP team) — SECURITY-SENSITIVE.**
  A decrypted assertion is regulated PII (bank/GDPR) AND a replayable bearer
  credential within its validity window (worse here: in-memory replay cache +
  "Allow IdP-initiated SSO" is on → weaker replay protection). Do NOT just dump it
  to DEBUG. Design:
  - Default-OFF explicit opt-in toggle (config flag / env, e.g.
    `saml_debug_log_assertions`), not merely DEBUG level. Emit a WARN while on.
  - One-shot / time-boxed: capture the next assertion (or N), then auto-disable.
  - **Names-only by default**: log attribute names + `NameID` Format (structure),
    which already answers the Entra-config question with minimal exposure. Full
    decrypted XML (values) only under the stricter toggle.
  - Prefer a non-log channel: return the one-shot capture to the authenticated
    admin, or write to an access-controlled short-retention file (`.local/`,
    gitignored), NOT the shared log stream that feeds support bundles. Purge after.
  - Capture point: `HandleACS` (`internal/webapi/handle_saml.go`, around
    `ParseACSResponse`) — `r.FormValue("SAMLResponse")` is base64 (HTTP-POST
    binding, not deflated) → decode to Response XML. If Entra **encrypts**, the
    raw value is opaque (lower risk to log) but attributes need the decrypted
    assertion from inside the provider.
  Zero-code alternatives (prefer these first): Entra Enterprise App → Single
  sign-on → **Test** shows the claims Entra emits with zero exposure on our side;
  or a SAML-tracer browser extension captures it client-side.
- [ ] Consider documenting/defaulting the Entra URI claim names, and surfacing a
  clear warning when username falls back to NameID (silent fallback hid this).

## Issue 3 — Role stuck at viewer; manual admin reverts [app design]

Symptom: SAML users are always `viewer`; setting a user to admin in the UI reverts
to viewer on next login.

Root cause: two layers.
- (a) `groups=[]` (same empty-attributes cause as #2) → no role mapping → viewer.
- (b) **design**: `jit.Provisioner.Provision` → `UpsertSAMLUser`
  (`internal/auth/jit/provisioner.go`) sets `Role` from the assertion on **every**
  login (insert AND update), so a manual role change is always overwritten.

Decision needed (check `specifications/auth.md`): is SAML group→role mapping the
sole source of truth (then manual edits aren't expected to persist — fix via Entra
groups + role_mapping), or should a manual override persist (then JIT must not
downgrade an existing user's role)? Fix accordingly; spec edit needs owner sign-off.

## Sequencing
1. Land Issue 1 (frontend-only, low risk) on its own branch, TDD.
2. Issue 2/3: add the assertion-XML diagnostic, have the customer re-login and
   capture a (scrubbed) support bundle to confirm Entra claims; then fix the Entra
   config and decide the role-precedence model.

## Key files
`frontend/src/pages/AdminAuthPage.tsx`, `frontend/src/api/{saml,config}.ts`,
`internal/auth/samlsp/provider.go` (extractUserInfo/flattenAttributes/resolveRole),
`internal/auth/jit/provisioner.go`, `internal/webapi/handle_saml*.go`,
`cmd/chef-migration-metrics/main.go` (buildSAMLProvider, reconciler),
`specifications/auth.md`.
