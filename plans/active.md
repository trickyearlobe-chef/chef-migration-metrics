# Active Plan — vCenter SSL-verify key reconciliation + typed UI widgets

## Problem
Orphan sweep fails with x509 even though SSL verify is "disabled" in the UI.
Root cause: three consumers of `driver_settings` disagree on TLS.
- Test Kitchen (VM launch) reads `vcenter_disable_ssl_verify` (lenient, works).
- CMM test-connection (`ListTemplates`, SOAP) hardcodes `insecure=true` (vcenter.go:136) — always green, hides the problem.
- CMM orphan sweep (REST) reads a *different* key `vcenter_insecure` and needs a real bool; the UI's freeform text box stores a string. Key never matches + type never matches → verify stays on → x509.

## Decisions (confirmed with user)
- Standardise on `vcenter_disable_ssl_verify` as the canonical key; CMM reads it, falls back to legacy `vcenter_insecure`.
- UI: checkbox for `vcenter_disable_ssl_verify` (emits JSON bool); dropdown for `clone_type` (full/linked).
- `ListTemplates` must honour the flag so test-connection stops lying.

## Chunk A — Backend (Go)  [internal/hypervisor/factory.go, vcenter.go + tests]  ✅ DONE
- [x] `settingBool` accepts bool AND string "true"/"false".
- [x] `newVCenterFromConfig` reads `vcenter_disable_ssl_verify`, falls back to `vcenter_insecure`.
- [x] `VCenterClient` stores `insecureSkipTLSVerify`; `ListTemplates` passes it to `govmomi.NewClient`.
- Accept: factory tests (bool/string/legacy/default) pass; `go test ./...` + golangci-lint green.

## Chunk B — Frontend (typed widgets)  [AdminTestKitchenPage.tsx + test]  ✅ DONE
- [x] Known-key widgets: `vcenter_disable_ssl_verify`→checkbox, `clone_type`→select[full,linked].
- [x] Save converts known boolean keys to real JSON booleans (`kvToRecordTyped`).
- [x] Load tolerates string "true"/"false".
- Accept: 402 frontend tests pass; tsc + lint green.

## Chunk C — Docs/spec/tech-debt  ✅ DONE
- [x] `specifications/test-kitchen-config-ui.md`: typed widgets + canonical `vcenter_disable_ssl_verify` read by both TK and CMM.
- [x] `plans/todo-tech-debt.md`: reconciled item + legacy-key deprecation + proxmox-unify follow-ups.

## Status: ready for commit + sign-off (NOT yet committed; NOT merged).

## Notes
- Proxmox left as-is (`proxmox_insecure`); benefits from the `settingBool` string fix.
- Branch: `fix/vcenter-ssl-verify-key`. Do not merge without sign-off.
