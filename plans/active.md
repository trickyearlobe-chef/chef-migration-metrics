# Active Plan — Phase 1: UI Revamp — COMPLETE

Phase 1 (navigation restructure + minor polish) was already implemented and is
in `main`. Audit on 2026-06-07 confirmed all of it: Kitchen hub with
Hypervisor|Analysis|Batches|Queue|Settings tabs; Analysis Tools + Concurrency
under Settings; SETTINGS regroup (Credentials by Organisations, Auth by Users,
Credentials out of ADMIN); System Stats + Performance merged; ACME conditional
on Server & TLS renders. Typecheck clean, 300 frontend tests pass. All
`todo-ui-polish.md` items ticked.

Accepted divergence: System Health uses Overview|Performance tabs (not
Overview|API|Database|Actions) and Actions stays a standalone nav item — all
pages reachable, no dead routes.

The next chunk of work is queued separately; this file is rewritten when it
opens.
