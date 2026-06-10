# Active — Onboarding: dynamic org sync + setup flow

Branch: `fix/onboarding-dynamic-orgs`. Pre-existing bugs on `main`, surfaced while
testing the TLS blank-DB bootstrap. Not TLS regressions.

## Root cause

Org config has two representations: the **write model** `config_store.organisations`
(declarative connection config; what the wizard PUTs) and the **read model**
`organisations` table (relational anchor — 6 FK-cascade dependents; the collector
reads it via `ListOrganisations`; holds `source: config|api`, a deliberate superset
per `web-api-organisations.md`). `SyncOrganisationsFromConfig` is already a correct
transactional reconciler (upsert + delete-stale `source='config'`, preserves
`source='api'`) — but `syncOrganisations` (`main.go`) only invokes it **at boot**.
Nothing re-reconciles on a config-org change. Violates `configuration.md`
(live-reloadable without restart) and `encrypted-config-store.md` §156 (collector
runs once ≥1 org configured — intended dynamic, not restart-gated).

Evidence (local DB after wizard): `config_store` has `organisations` +
`credentials/pivotal` (writes OK); `organisations` table has 0 rows.

## Chunks

### Chunk A — reconcile org table + trigger collection on config change (#3) — DONE
- `webapi.WithOrganisationsChanged` callback + a post-reload hook on
  `storeAdminConfigSection` (org PUT only). Main wires it to `syncOrganisations`
  (now reads the reloaded `ConfigHolder`, not the boot snapshot) + a non-blocking
  `sched.TriggerNow`. Reconciler already preserves `source='api'` rows.
- TDD: hook invoked on org PUT, 500 on hook error, not invoked for non-org PUTs.
  Full Go suite green. Commit `15128dc`.
- Note: this fixes #3 (collector picks up the org without restart). Whether it
  also clears #1 (setup prompt) is Chunk B's repro — the GET already reads live
  config, so #1 may be a reload-failure or cache path still to pin.

### Chunk B — setup mode clears without restart (#1) — DONE
- Repro pinned the cause: **not** a backend reload failure. Added Go round-trip
  test (`PUT_then_GET_ReflectsSavedOrg`, holder backed by same store) — proves
  the in-request `configHolder.Reload` succeeds and the GET reflects the org. So
  the backend already clears setup mode (Chunk A's reload-on-PUT did it).
- Real cause was frontend: `SetupModeGuard`'s `useSetupRequired` runs once on
  mount and stays mounted across SPA navigation, so its `setupRequired` stays
  stale (`true`). The wizard's "Go to Dashboard" did a soft `navigate("/")` →
  the stale guard bounced the user straight back to `/admin/setup`. Only a full
  browser refresh remounted the guard and cleared it.
- Fix: "Go to Dashboard" now does a full load (`window.location.assign("/")`),
  re-initialising the guard + `OrgProvider` + filters against the now-populated
  config — no restart. Removed unused `useNavigate`.
- TDD: vitest drives the wizard to completion → asserts full load to "/".
  Full frontend suite (384) + Go webapi suite green.
- Spec: `encrypted-config-store.md` §156 already states setup mode clears
  immediately without restart (landed in Chunk A); now true end-to-end.

### Chunk C — wizard credential inline flow (#2)
- Scope: `frontend` `AdminSetupWizardPage.tsx` (frontend only).
- Replace `window.open("/admin/credentials","_blank")` with inline credential
  creation in the wizard's credentials step (reuse the credentials create form),
  then auto-advance to the org step with the new credential preselected.
- TDD: vitest — create cred inline → advances → org step shows the credential.
- Acceptance: single flow, no new tab, no manual return.

## Spec deltas (CONFIRM before editing — do not modify specs without asking)
- `encrypted-config-store.md` §156: state that configuring an org takes effect
  immediately (collector starts, setup mode clears) — no restart.
- `web-api-organisations.md` / `configuration.md`: state that config-org changes
  reconcile the `organisations` table synchronously and trigger collection.

### Also landed (org-form dedup) — DONE
Removed the redundant Org Name input. `chef_server_url` stays the authoritative
**full** org URL (stored verbatim — existing configs/data untouched); the backend
derives `org_name` from its `/organizations/<org>` segment (explicit honoured;
rejects only if omitted AND underivable). Friendly `name` (data-owning PK)
unchanged. UI validates the URL shape pre-save (`lib/chefOrgUrl.ts`) with a
full-org-URL placeholder + inline error. Spec updated. (An earlier base-URL
reconstruction approach was reverted as unsafe for existing configs.)

## Notes
- TLS refinement backlog (after this): 443-lifeboat port move
  ([todo-tls-antilockout.md](todo-tls-antilockout.md) §2) and cert-chain
  display + leaf/intermediate/CA reorder-on-save ([todo-tls.md](todo-tls.md)).
