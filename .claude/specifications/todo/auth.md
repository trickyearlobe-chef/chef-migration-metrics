# Authentication and Authorisation — ToDo

Status key: [ ] Not started | [~] In progress | [x] Done

---

## Not Yet Implemented

- [ ] Implement LDAP authentication — config validation exists (`config.go` validates host, base_dn for ldap providers) but no LDAP client implementation
- [ ] Implement SAML authentication — config validation exists (`config.go` validates idp_metadata_url, sp_entity_id for saml providers) but no SAML client implementation
- [ ] Ensure credentials and secrets are never stored in source control — partially addressed (password hashing, HTTP-only cookies, no plaintext storage) but needs formal audit