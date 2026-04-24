# Authentication and Authorisation — ToDo

- [ ] Implement SAML authentication — config validation exists (`config.go` validates idp_metadata_url, sp_entity_id for saml providers) but no SAML client implementation
- [ ] Ensure credentials and secrets are never stored in source control — partially addressed (password hashing, HTTP-only cookies, no plaintext storage) but needs formal audit