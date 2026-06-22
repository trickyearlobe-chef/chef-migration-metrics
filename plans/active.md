# Active Plan

No active chunk. Both queued chunks are complete — next candidates are the parked
SAML follow-ups below (or the credential-source status bug in
`plans/todo-secrets-storage.md` § Bugs).

Done recently:
- **Orphan-sweep ticker wiring** — merged to `main` (2026-06-19, commit 6bfe197):
  `StartSweepTicker` made dynamic (live params + hypervisor factory each tick),
  wired in main.go, synchronous `stop()` in `awaitShutdown`. Folder-scoping
  deferred per owner decision (prefix+age scoped, logged caveat) — tracked in
  `plans/todo-tech-debt.md` § "Scheduled Orphan Sweep Has No Folder Scoping".
- **UI revamp follow-up cleanup** — reconciled 2026-06-22 (docs only). The
  2026-06-19 audit was stale: System Health tabs accepted as-is
  (`Overview|Performance|Status`; the 4-tab split was a planning note, not in the
  spec), and the "orphaned" Kitchen routes already had redirects since 2026-06-02.
  Decisions recorded in `plans/todo-ui-polish.md` § "Follow-up cleanup".

## Parked — SAML config follow-ups (lower priority)

- Warn when a SAML provider has empty `username_attr` (transient-NameID footgun;
  breaks login anchoring + ownership matching — `plans/todo-ownership.md`).
- Turn the local-user username collision (`ErrAlreadyExists` → opaque 500) into a
  clear, actionable message.
