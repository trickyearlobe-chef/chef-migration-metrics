# Secrets Storage — Credential Types & Resolution Precedence

## Credential Types

The application manages the following categories of secrets:

| Credential Type | Storage Methods | Notes |
|-----------------|-----------------|-------|
| Chef API private key (RSA PEM) | Database, env var, file path | One per Chef server organisation. Database storage recommended for multi-org and containerised deployments. |
| SMTP password | Database, env var | Referenced via `password_credential` (database) or `password_env` (env var) in the SMTP config. |
| Webhook URL | Database, env var | May contain authentication tokens in the URL. Referenced via `url_credential` or `url_env`. |
| Database connection string | Env var only | `DATABASE_URL`. Never stored in the database (circular dependency). |
| Credential encryption master key | Env var only | `CMM_CREDENTIAL_ENCRYPTION_KEY`. Must not reside in the same storage system as the encrypted credentials. |
| TLS private key (for `static` mode) | File path, Kubernetes Secret | Mounted at `/etc/chef-migration-metrics/tls/tls.key`. Not stored in the credentials table. |
| ACME account key | File path (auto-managed) | Stored in `acme.storage_path`. Managed by the ACME client library, not by the application's credential system. |
| Local user passwords | Database (bcrypt hash) | Stored in the `users` table as bcrypt hashes, not in the `credentials` table. Not reversible. |
| Generic secrets | Database | Catch-all type for operator-defined secrets that don't fit other categories. |

---

## Credential Resolution Precedence

When the application needs a credential, it resolves the value using this precedence order:

```
1. Database  →  2. Environment variable  →  3. File path
```

If multiple sources are configured for the same credential, the highest-precedence source wins. This allows operators to migrate incrementally from file-based to database-stored credentials without changing the config file.

### Resolution Flow

```
┌─────────────────────────────────────────────────────┐
│  Credential needed (e.g. Chef API key for org X)    │
└──────────────────────┬──────────────────────────────┘
                       ▼
        ┌──────────────────────────────┐
        │ client_key_credential_id set │
        │ in organisations table?      │
        └──────┬───────────────┬───────┘
           Yes │               │ No
               ▼               ▼
    ┌──────────────────┐  ┌────────────────────────────┐
    │ Decrypt from      │  │ client_key_env configured  │
    │ credentials table │  │ for this org in YAML?      │
    └──────────────────┘  └──────┬───────────────┬──────┘
                             Yes │               │ No
                                 ▼               ▼
                      ┌──────────────────┐  ┌──────────────────────────┐
                      │ Read from env var│  │ client_key_path set      │
                      └──────────────────┘  │ in YAML config?          │
                                            └──────┬───────────────┬───┘
                                               Yes │               │ No
                                                   ▼               ▼
                                        ┌──────────────────┐  ┌────────────────┐
                                        │ Read PEM from    │  │ ERROR:         │
                                        │ file on disk     │  │ no credential  │
                                        └──────────────────┘  │ configured     │
                                                              └────────────────┘
```

### Resolution by Credential Type

| Credential | Database reference | Env var | File path |
|------------|-------------------|---------|-----------|
| Chef API key | `client_key_credential: <name>` in org config → FK in `organisations.client_key_credential_id` | `client_key_env: VAR_NAME` in org config | `client_key_path: /path/to/key.pem` in org config |
| SMTP password | `password_credential: <name>` in SMTP config | `password_env: SMTP_PASSWORD` in SMTP config | — |
| Webhook URL | `url_credential: <name>` in notification channel config | `url_env: NOTIFICATION_WEBHOOK_URL` in notification channel config | — |
| Database URL | — | `DATABASE_URL` | — |
| Master encryption key | — | `CMM_CREDENTIAL_ENCRYPTION_KEY` | — |
