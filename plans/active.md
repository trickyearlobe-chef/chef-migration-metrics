# Active Plan — SAML config improvements

Branch: `feature/saml-config-improvements` (off `main`). Do not merge without
sign-off. Full plan: `plans/saml-config-improvements.md`.

## Chunk A — IdP metadata via paste (3rd source) [TDD]
Add inline `idp_metadata_xml` config field + provider support; UI swaps the two
URL/path fields for a 3-way source dropdown (URL / file path / paste XML).
Backend: `config.go` (field + one-of validation), `samlsp/provider.go` (Config
field, New() load branch, refresh guard), `main.go buildSAMLProvider`.
Frontend: `types/config.ts`, `AdminAuthPage.tsx`.
Accept: `go test ./...`, golangci-lint, `npm test`+tsc+lint green.

## Chunk B — SP base URL for export (hostname + port) [TDD]
Per-provider `sp_base_url` field as canonical base for ACS/SLO/metadata/entity
(fixes `localhost:8080`). UI field defaults to admin browser origin; fallback
never emits the http-redirect port.
Backend: `config.go` (field + validation), `main.go buildSAMLProvider`.
Frontend: `types/config.ts`, `AdminAuthPage.tsx`.
Accept: same as Chunk A.

## Spec edits (sign-off: amend as part of work, review in final summary)
`auth.md` §IdP Metadata (paste source), §SP Metadata Export (sp_base_url),
config example; check `web-api-auth.md`.

## Parked
- Notifications retirement: Chunk 1 committed on
  `chore/retire-notifications-cleanup` (31c887c); Chunks 2-3 queued there.
