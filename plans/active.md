# Active Plan

Current: **Spec/Plan Drift Control** — see `plans/spec-drift-control.md`.
Chunk A (lint impl out of specs) next; B/D rules done; then E (drift sweep),
then C (criteria↔test linkage). Branch: `chore/spec-drift-control`.

Pending merge (separate, complete): `docs/ui-revamp-followup-reconcile`.

## Parked — SAML config follow-ups (lower priority)

- Warn when a SAML provider has empty `username_attr` (transient-NameID footgun;
  breaks login anchoring + ownership matching — `plans/todo-ownership.md`).
- Turn the local-user username collision (`ErrAlreadyExists` → opaque 500) into a
  clear, actionable message.
