# Active Plan — Retire SMTP/webhook + notifications scaffolding; K8s/TLS-perm backlog cleanup

Branch: `chore/retire-notifications-cleanup`. Do not merge without sign-off.

## Context
SMTP/webhook were spec'd but never built — no `internal/notify`, no senders.
Only scaffolding exists (credential types + validation + UI). User decision:
drop fully (code, UI, specs, docs) AND retire the notifications-via-ownership
spec scaffolding, **keeping ownership** (a real feature). K8s was dropped; the
app is not containerised (already true + correctly documented). TLS key-file
perm check is already done.

## Done this branch (backlog hygiene)
- [x] todo-secrets-storage: removed stale "TLS key file perm" item (already
      implemented); kept keys-dir/env-file items; added clarifying note.
- [x] todo-secrets-storage: removed "Document K8s External Secrets Operator" item.

## Chunk 1 — Remove SMTP/webhook credential types (code + tests) [TDD]
Files: `internal/secrets/validation.go` (type consts CredentialTypeSMTPPassword/
WebhookURL, valid-types map entries, validateWebhookURL, switch cases,
ErrInvalidWebhookURL) + fixtures in `internal/secrets/validation_test.go`,
`credential_store_test.go`, `encryption_test.go`,
`internal/webapi/handle_credentials_test.go`,
`internal/configstore/credential_adapter_test.go`.
- Update tests first (remove smtp/webhook cases; ensure remaining types pass).
- Confirm no other consumer references the removed constants.
- Accept: `go test ./...` + golangci-lint green.

## Chunk 2 — Remove SMTP/webhook from credentials UI
Files: `frontend/src/pages/credentials/constants.ts` (type option, badge, label),
`ValueField.tsx` (webhook_url branch), `AdminCredentialsPage.tsx` (description text).
- Update/adjust any credentials component test referencing those types.
- Accept: `npm test` + tsc + lint green.

## Chunk 3 — Specs/docs prune (needs owner sign-off per CLAUDE.md)
- Delete the **notification half** of `web-api-notifications-ownership.md`,
  keeping ownership endpoints; rename if appropriate.
- Remove SMTP/webhook from `secrets-storage*.md` (notably credential-model,
  validation-api, secrets-storage.md TL;DR "Chef API keys, SMTP passwords, and
  webhook URLs").
- Correct `secrets-storage.md:3` TL;DR line implying a Helm chart /
  chart-managed Secrets (no k8s packaging exists).
- Remove smtp/webhook live-test items from `todo-secrets-storage.md` (Credential
  Testing section) — now out of scope.
- Check `web-api-admin.md`, `logging.md`, `ownership*.md` for dangling notif refs.

## Out of scope (leave as-is)
- `deploy/docker-compose/` (DB) and `deploy/elk/` (ES) — kept for local testing.
- Legit K8s/orchestrator deployment guidance (env-var secret injection,
  cert-manager mounts, HTTPS probes) — true, not "packaging we built".
- Credential keys-dir (>0700) / env-file (>0640) perm warnings — still open work.
