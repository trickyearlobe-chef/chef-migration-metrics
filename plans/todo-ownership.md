# Ownership — ToDo

## Matching app users to owners (and thereby to git repos / cookbooks / nodes)

Context: a logged-in user's session carries only `username` (+ provider, role) —
NOT `saml_subject`. SAML identity is anchored on the stable `username`
(`username_attr`); `saml_subject` is a volatile login-match token (refreshed each
login, drifts with a transient NameID) and is used nowhere downstream. The hub
for ownership is the `owners` record; `owner_aliases` map identifiers
(`username`, `email`, `git_email`, `git_name`, `custom`) to one owner.

Join key between an app identity and git/commit data is **email**, not username
and not `saml_subject`.

- [ ] **Auto-create `username` alias on SAML JIT login.** Without it the session's
  `username` resolves to no owner. Tag `source='saml'`. (Spec mentions JIT
  auto-link; not yet implemented — see `auth.md` JIT Provisioning.)
- [ ] **Auto-create/ensure `email` alias from the email claim.** Email is the
  reliable bridge to git/commit identities. Auto-link to an existing owner when
  the email matches an existing alias (log the link); else seed it.
- [ ] **Do NOT fabricate `git_email = corp_email`.** Commit email ≠ corporate
  email in many shops (noreply, personal SCM, contractors). Instead:
  - Discover `git_email`/`git_name` from the repo/commit side.
  - Match to owners by **exact email equality first** (commit email == SAML email
    claim → safe auto-link), then fall back to the existing **pg_trgm fuzzy
    similarity** (`owner_aliases.go`) surfaced for **admin confirmation** — not
    silent creation.
- [ ] **Guardrails:** stamp `source` on every auto-created alias (auditable,
  reversible); never overwrite a `manual` alias.

Dependency on stable username: the whole graph assumes `username` stays stable
(driven by `username_attr` → a stable claim). If `username_attr` is unset,
username falls back to the (possibly transient) NameID and both login anchoring
and ownership matching break. Surface a config warning if `username_attr` is
empty for a SAML provider.
