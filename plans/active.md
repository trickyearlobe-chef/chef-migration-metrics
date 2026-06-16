# Active Plan — SAML config improvements

Branch: `feature/saml-config-improvements` (off `main`). Do not merge without
sign-off. Full plan: `plans/saml-config-improvements.md`.

## Chunk A — IdP metadata via paste (3rd source) [TDD]  ✅ DONE (1a32fef)
Inline `idp_metadata_xml` config field + provider support; UI 3-way source
dropdown (URL / file path / paste XML). spec auth.md §IdP Metadata updated.

## Chunk B — SP base URL for export (hostname + port) [TDD]  ✅ DONE (764c1da)
Per-provider `sp_base_url` (canonical base for ACS/SLO/metadata/entity, fixes
`localhost:8080`); UI defaults to admin browser origin, persisted on save;
fallback uses effective HTTPS port, never the redirect port. spec auth.md
§SP Metadata Export + config example updated.

## Status: ready for sign-off + merge (NOT merged). web-api-auth.md needed no change.

## Done (merged to main)
- Notifications retirement: ✅ complete (Chunks 1-3) — code, UI, specs, and
  backlog all removed; ownership kept. Merged from
  `chore/retire-notifications-cleanup`.
