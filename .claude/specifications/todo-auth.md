# Authentication and Authorisation — ToDo

- [ ] Implement SAML authentication — SP-initiated SSO, ACS, metadata, inbound SLO endpoints using `crewjam/saml` v0.5.1
- [ ] Extend config schema with SAML-specific fields (attribute mappings, role_mapping, SP credentials, clock skew, metadata refresh)
- [ ] JIT user provisioning — auto-create/update users on SAML login, identity matching via `saml_subject`
- [ ] Three-tier role model — add `operator` role, update middleware to viewer/operator/admin hierarchy
- [ ] Ownership-scoped permissions — viewers with ownership can perform operational actions on owned resources
- [ ] Owner aliases table — `owner_aliases` with fuzzy matching (`pg_trgm`) for identity suggestions
- [ ] Extend CSV/JSON import to support aliases column
- [ ] Frontend — SSO login button, admin SAML config, alias management, role-aware UI
- [ ] Ensure credentials and secrets are never stored in source control — partially addressed (password hashing, HTTP-only cookies, no plaintext storage) but needs formal audit