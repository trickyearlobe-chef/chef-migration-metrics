# Secrets Storage — File-Based Security, Docker Compose & RPM/DEB Packaging

## File-Based Credential Security

When credentials are stored as files on disk (Chef API PEM keys, TLS private keys):

| Requirement | Detail |
|-------------|--------|
| **File permissions** | `0600` or `0400` (owner read/write or read-only). Log `WARN` on startup if more permissive. |
| **Ownership** | Must be owned by the application's service account (`chef-migration-metrics` user created by package install scripts). |
| **Directory permissions** | The `keys/` directory at `/etc/chef-migration-metrics/keys/` must be `0700`. |
| **Ignore files** | `*.pem`, `*.key`, and `keys/` patterns must appear in `.gitignore` and `.dockerignore`. |
| **Config file references** | The YAML config references keys by path, never by inline content. |

### Standard File Locations

| File | Path | Description |
|------|------|-------------|
| Chef API key (RPM/DEB) | `/etc/chef-migration-metrics/keys/<org-name>.pem` | One PEM file per organisation |
| Chef API key (container) | `/etc/chef-migration-metrics/keys/<org-name>.pem` | Mounted from Kubernetes Secret or Docker secret |
| TLS private key | `/etc/chef-migration-metrics/tls/tls.key` | For `server.tls.mode: static` |
| TLS certificate | `/etc/chef-migration-metrics/tls/tls.crt` | For `server.tls.mode: static` |
| ACME storage | `/var/lib/chef-migration-metrics/acme/` | Auto-managed ACME account keys and certificates |
| Environment file (RPM) | `/etc/sysconfig/chef-migration-metrics` | `0640`, contains env var overrides including secrets |
| Environment file (DEB) | `/etc/default/chef-migration-metrics` | `0640`, contains env var overrides including secrets |

---

## Docker Compose Secrets

For local development and evaluation using Docker Compose:

- Sensitive environment variables are set in a `.env` file (listed in `.env.example` as a template).
- The `.env` file is listed in `.gitignore` and `.dockerignore`.
- Chef API keys are bind-mounted from a local directory into the container.
- The credential encryption master key is set via `CMM_CREDENTIAL_ENCRYPTION_KEY` in the `.env` file.

**This model is for development only.** `.env` files provide no encryption, access control, or audit trail.

---

## RPM / DEB Package Secrets

For traditional Linux installations:

- Chef API keys are placed in `/etc/chef-migration-metrics/keys/` with `0600` permissions.
- The environment file (`/etc/sysconfig/chef-migration-metrics` or `/etc/default/chef-migration-metrics`) contains sensitive env vars (`DATABASE_URL`, `CMM_CREDENTIAL_ENCRYPTION_KEY`). File permissions are `0640`, owned by `root:chef-migration-metrics`.
- The systemd unit file references the environment file via `EnvironmentFile=`.
- The `postinstall.sh` script sets correct ownership and permissions on the keys directory and environment file.
- The `preremove.sh` script does **not** delete credential files — this is left to the operator to avoid accidental data loss.
