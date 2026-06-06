# Configuration — Configuration File, Secrets and Credentials

## Configuration File

Configuration is stored in a YAML file. The path to the configuration file may be specified via a command-line flag or the `CHEF_MIGRATION_METRICS_CONFIG` environment variable. If neither is specified, the application looks for `config.yml` in the current working directory.

## Secrets and Credentials

Sensitive values (private key paths, passwords, tokens) must never be stored in source control. All sensitive configuration values must support being overridden by environment variables. The configuration file should reference key files by path, not inline their contents.

### Credential Storage Options

The application supports three ways to supply credentials, listed in order of resolution precedence:

| Method | When to use | Example |
|--------|-------------|---------|
| **Database** | Multi-org deployments, containerised environments, management via Web UI | `client_key_credential: myorg-production-key` references a row in the `credentials` table |
| **Environment variable** | Container orchestrators (Kubernetes Secrets, ECS task definitions), CI/CD | `CMM_CREDENTIAL_ENCRYPTION_KEY` |
| **File path** | Traditional on-premises installs, simple single-org setups | `client_key_path: /etc/chef-migration-metrics/keys/myorg.pem` |

When multiple sources are configured for the same credential, database takes precedence over environment variable, which takes precedence over file path. This allows operators to migrate incrementally from file-based to database-stored credentials without changing the config file.

### Credential Encryption Key

Credentials stored in the database are encrypted at the application layer using AES-256-GCM. The master encryption key must be provided externally — it is never stored in the database alongside the encrypted values. See the [Datastore Specification](../datastore/Specification.md) for the full encryption model.

```yaml
credential_encryption_key_env: CMM_CREDENTIAL_ENCRYPTION_KEY
```

| Setting | Required | Default | Description |
|---------|----------|---------|-------------|
| `credential_encryption_key_env` | When DB credentials are used | `CMM_CREDENTIAL_ENCRYPTION_KEY` | Name of the environment variable containing the master encryption key. The key must be at least 32 bytes (256 bits), Base64-encoded. |

The key itself must **never** appear in the YAML config file. Only the name of the environment variable that holds it is configured.

| Environment Variable | Description |
|----------------------|-------------|
| `CMM_CREDENTIAL_ENCRYPTION_KEY` | Base64-encoded AES-256 master key for encrypting/decrypting database-stored credentials. Required if any `*_credential` references exist in the config or if credentials have been created via the Web API. |
| `CMM_CREDENTIAL_ENCRYPTION_KEY_PREVIOUS` | Base64-encoded previous master key, required during key rotation. Set this alongside the new key when rotating. After successful restart and re-encryption, remove it. |
