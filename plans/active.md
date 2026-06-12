# Active — config live-reload + SAML config UX (complete)

Both workstreams below are implemented, tested (`-race`), and merged. No active
chunk is queued. Retained as the decision record; safe to delete once read.

## Config live-reload (H1–H4) — DONE, merged to main

`server.*` changes apply in place via an in-process listener rebind (serverctl
controller), replacing the process re-exec. Spec: `configuration-live-reload.md`.

Live (no restart): `graceful_shutdown_seconds` (H1); `listen_address`/`port` (H2);
`websocket.*` subsystem (H3); off↔static TLS mode (H4a); static TLS fields —
cert source/paths, min_version, mTLS CA (H4b-1); `http_redirect_port` topology
(H4b-2); auto-443 lifeboat re-plan (H4b-3); `acme→off`/`acme→static` exit (H4c-2a).

### H4c-2b — entering/reconfiguring ACME stays restart-required [WON'T DO]
Decided 2026-06-12. Persisted change applies on the next (graceful) restart —
the spec's documented fallback. Live in-place was deliberately not built: ACME
owns three coupled OS resources (HTTPS listener + port-80 challenge + renewer)
vs one listener elsewhere, so it is the branch's most complex code for its rarest
operator action; a process restart gets the clean slate for free; and the two
reasons to avoid restart (unsupervised host / in-flight work) barely apply to a
deliberate ACME mode change with `POST /api/v1/admin/restart` available. If ever
revived: `prepareACMEInstance` validate-then-bind closure, acme fingerprint key,
port-80 challenge pre-bind+retry, auto-443-in-acme. (Full record: commit f84ce5c.)

## SAML config UX — DONE

Spec: `auth.md`. Driver: an operator hit `405` guessing the ACS callback URL
(nothing surfaced it), and an `sp_entity_id` change needed a restart.

- **SAML-1 — surface SP endpoint URLs.** Provider accessors
  `ACSURL()/SLOURL()/MetadataURL()/EntityID()`; admin-only
  `GET /api/v1/admin/saml/endpoints`; `AdminAuthPage` renders ACS/SLO/metadata/entity
  read-only with copy actions. Hand-configured IdPs (Google/Okta) no longer guess → 405.
- **SAML-2 — live-reload the SAML provider (auth subsystem applier).**
  `session_expiry`/`lockout_attempts` read live at point of use; `SAMLHandler` guards
  provider+endpoints under one mutex (`SetProvider`/`prov()`, 501 on nil);
  `WithSAMLReconciler` + `samlApplier` (subsystem); auth PUT registers
  `appliedApplier` + the SAML applier; `buildSAMLProvider` rebuilds from the holder
  and swaps in place (incl. enable/disable). Auth section reports applied/false (no
  SAML) or subsystem/false (SAML wired) — `sp_entity_id` change now applies live.

## Notes
- `auth.*` reconciled: `configuration-live-reload.md` §auth (subsystem — session/
  lockout live reads + SAML provider rebuilt in place) now matches the implementation
  merged here. The earlier "not yet implemented on this branch" caveat is resolved.
