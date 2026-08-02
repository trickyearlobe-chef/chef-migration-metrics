# Audit Log — ToDo

Behaviour and intent: `specifications/audit-log.md`. Do not restate it here.

Not started. Parked behind the ownership MVP — raised 2026-08-02 while adding the
`owner_merged` action, which needed a migration before it could be written at all.

## Build the general facility

- [ ] **A single audit table keyed on a subject pair**, not a column per subject kind, so
  owners, cops, configuration keys and job triggers all fit. No CHECK constraint on the
  action vocabulary.
- [ ] **Decide how the write is made reliable** — same transaction as the change it
  describes, or a surfaced failure. Both are open; the current best-effort warning is not
  a third option. `InsertAuditEntryTx` already exists and nothing uses it.
- [ ] **Secret redaction derived from the config store**, not remembered at each call site.

## Cover what is uncovered

- [ ] **Configuration edits.** `config_store` keeps only who last touched a key. The first
  real consumer, and the reason this exists.
- [ ] **Credential changes** — that a key changed and by whom, never a value.
- [ ] **Triggered processes** — rescan-all-cookstyle, the ownership duplicate scan, purge,
  restore. Currently application-log only, which is shipped off-box.

## Fold in what exists

Only after the above is in use. Nothing already recorded is discarded.

- [ ] **`ownership_audit_log`** → general table. The ownership audit view queries
  owner-shaped columns and moves with it.
- [ ] **`cookstyle_audit_log`** → general table.
- [ ] **Per-subject retention**, replacing the single ownership-scoped age.

## Open questions

- [ ] **Does an audit entry need to survive a restore?** Backup covers the datastore, so it
  does today by default — but a restore that rolls state back leaves audit entries
  describing actions on state that no longer exists. Decide whether that is a problem or
  the point.
- [ ] **Non-human actors.** Auto-derivation and startup cleanup already write `system`.
  Whether a scheduled process should be distinguishable from a person is unresolved, and
  it matters if the log is ever used to answer an accountability question.
