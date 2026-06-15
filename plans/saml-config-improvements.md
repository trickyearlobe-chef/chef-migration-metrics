# Plan — SAML config improvements (IdP metadata paste + SP base URL export)

Branch: `feature/saml-config-improvements` (NOT the notifications branch). Two
logical commits, one per feature. Do not merge without sign-off.

## Decisions (from owner)
- IdP metadata paste stored INLINE in provider config (`idp_metadata_xml`).
- SP external base URL = editable per-provider field `sp_base_url`; UI defaults
  it to the admin browser's `window.location.origin` (scheme + host + non-default
  port). Field is the canonical source baked into metadata/ACS/SLO/entity URLs.
- Export port bug: never emit the http-redirect port; primary fix is the field.
- Spec edits to `auth.md` require explicit sign-off before writing (CLAUDE.md).

## Context (current state)
- Metadata source: URL or file only (`samlsp.Config.IDPMetadata{URL,Path}`,
  `provider.go` New() lines 139/171-175; loaders `fetchIDPMetadata`,
  `loadIDPMetadataFromFile`). Spec `auth.md` §IdP Metadata (137-142).
- SP base URL built in `cmd/.../main.go buildSAMLProvider` (623-633) as
  `scheme://localhost:<Server.Port>` → baked into metadata XML ACS/SLO Locations
  and `/admin/saml/endpoints` JSON. `Server.Port` (8080) is the redirect/non-TLS
  side when TLS+auto-443 active; `localhost` not externally reachable.
- Frontend metadata-URL *link* already uses `window.location.origin`
  (`saml.ts samlMetadataUrl`); the XML contents + endpoints JSON do not.
- UI: `AdminAuthPage.tsx` (URL field ~187, Path field ~197, sp_entity ~210;
  export/endpoints display ~650-720). Types `frontend/src/types/config.ts`
  AuthProvider (157-181).

## Chunk A — IdP metadata via paste (3rd source) [TDD]
Backend:
- `config.go` AuthProvider: add `IDPMetadataXML string yaml:"idp_metadata_xml,omitempty"`.
- `config.go` validation (~1836): require EXACTLY ONE of url/path/xml; xml
  non-empty + ≤1MB; https-only rule stays for url. (Light parse only in config to
  avoid importing samlsp; full parse in provider.)
- `samlsp/provider.go`: add `Config.IDPMetadataXML []byte`; New() required-check
  accepts any of 3 sources; load branch adds inline parse via
  `samlsp.ParseMetadata`; new helper `loadIDPMetadataFromXML`. Guard
  `RefreshMetadata` to no-op when source ≠ URL (verify periodic refresher only
  schedules for URL source).
- `main.go buildSAMLProvider`: pass `IDPMetadataXML`.
Frontend:
- `config.ts`: add `idp_metadata_xml?: string`.
- `AdminAuthPage.tsx`: replace two fields with a SOURCE DROPDOWN (URL / File path /
  Paste XML) → show matching input (text / text / textarea). Selecting a source
  clears the other two so only one is sent.
Tests: config one-of validation; provider New() inline-XML load + 3-source
required error; AdminAuthPage source switching.
Accept: `go test ./...`, golangci-lint, `npm test`+tsc+lint green.

## Chunk B — SP base URL for export (hostname + port) [TDD]
Backend:
- `config.go` AuthProvider: add `SPBaseURL string yaml:"sp_base_url,omitempty"`;
  validate (if set) absolute http(s) URL, no path/query.
- `main.go buildSAMLProvider` (623-633): baseURL precedence:
  1) `samlCfg.SPBaseURL` (trim trailing /), else
  2) best-effort: correct scheme + host + effective HTTPS port (never
     HTTPRedirectPort; omit standard port), else existing SPEntityID-URL fallback.
  Document that operators should set sp_base_url for correct export.
Frontend:
- `config.ts`: add `sp_base_url?: string`.
- `AdminAuthPage.tsx`: editable "SP Base URL" field in SAML section; when empty,
  initialise to `window.location.origin` (so it persists on save); hint that this
  is what the IdP is told. Existing endpoints/export display reflects it after save.
Tests: config sp_base_url validation; buildSAMLProvider uses SPBaseURL when set +
fallback omits redirect port (check testability in cmd/, else extract helper);
AdminAuthPage default-origin behaviour.
Accept: same as Chunk A.

## Spec edits (BLOCKED on sign-off)
- `auth.md` §IdP Metadata: add paste source (inline `idp_metadata_xml`, one-of,
  ≤1MB, https only for url, no refresh for file/xml).
- `auth.md` §SP Metadata Export: ACS/SLO/metadata/entity derive from per-provider
  `sp_base_url` (UI-defaulted to browser origin); correct the "(not guessed from
  browser origin)" wording; note redirect-port pitfall.
- `auth.md` config example: add `sp_base_url`, mention `idp_metadata_xml`.
- Check `web-api-auth.md` /saml/metadata + /admin/saml/endpoints notes.
