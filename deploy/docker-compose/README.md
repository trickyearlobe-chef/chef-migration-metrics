# Docker Compose — Local Development Stack

Starts Chef Migration Metrics with a PostgreSQL database in a single command. Designed for local development, evaluation, and testing.

## Prerequisites

- [Docker Engine](https://docs.docker.com/engine/install/) 24+ with the Compose plugin (`docker compose`)
- A Chef Infra Server organisation with an API client and private key
- (Optional) A pre-built container image — if not available, Compose builds from the root `Dockerfile`

## Quick Start

```bash
cd deploy/docker-compose

# 1. Create your .env file
cp .env.example .env
# Edit .env — at minimum set POSTGRES_PASSWORD to something secure.

# 2. Edit config.yml for your Chef server organisations
#    Update chef_server_url, org_name, client_name, and client_key_path.

# 3. Place your Chef API private key(s) in ./keys/
mkdir -p keys
cp /path/to/my-org.pem keys/

# 4. Start the stack
docker compose up -d
```

The application is available at [http://localhost:8080](http://localhost:8080) (or the port set by `APP_PORT` in `.env`).

## Services

| Service | Image | Default Port | Purpose |
|---------|-------|-------------|---------|
| `db` | `postgres:16-bookworm` | 5432 | PostgreSQL database with persistent volume |
| `app` | `chef-migration-metrics:latest` | 8080 | Application server (API + embedded frontend) |

The `app` service waits for `db` to pass its health check before starting. Database migrations are applied automatically on first startup.

## Files

| File | Purpose |
|------|---------|
| `docker-compose.yml` | Service definitions, volumes, networking |
| `.env.example` | Documented environment variables — copy to `.env` |
| `.env` | Your local environment overrides (git-ignored) |
| `config.yml` | Application configuration mounted into the container |
| `keys/` | Chef API private keys mounted into the container (git-ignored) |

## Configuration

### Environment Variables (`.env`)

| Variable | Default | Description |
|----------|---------|-------------|
| `POSTGRES_DB` | `chef_migration_metrics` | PostgreSQL database name |
| `POSTGRES_USER` | `chef_migration_metrics` | PostgreSQL username |
| `POSTGRES_PASSWORD` | *(required)* | PostgreSQL password — must be set |
| `POSTGRES_PORT` | `5432` | Host port for PostgreSQL (useful if 5432 is in use) |
| `APP_IMAGE` | `chef-migration-metrics:latest` | Container image for the app service |
| `APP_PORT` | `8080` | Host port for the web UI and API |
| `LDAP_BIND_PASSWORD` | *(empty)* | LDAP bind password (if using LDAP auth) |
| `CMM_CREDENTIAL_ENCRYPTION_KEY` | *(empty)* | Encryption key for database-stored credentials |

### Application Configuration (`config.yml`)

The example `config.yml` is pre-configured for Docker Compose:

- **No `datastore.url` needed** — the `DATABASE_URL` environment variable is injected by Compose and takes precedence.
- **`analysis_tools.embedded_bin_dir`** points to `/opt/chef-migration-metrics/embedded/bin` where the container image installs CookStyle and Test Kitchen.
- **`server.listen_address`** is `0.0.0.0` so the container accepts connections from the Docker network.

Edit the `organisations` section to point at your Chef Infra Server(s) and update the `target_chef_versions` list.

### Chef API Keys

Place `.pem` files in the `keys/` directory. The directory is mounted read-only at `/etc/chef-migration-metrics/keys/` inside the container. Reference them in `config.yml` as:

```yaml
client_key_path: /etc/chef-migration-metrics/keys/my-org.pem
```

> **Security note:** The `keys/` directory is listed in `.gitignore` and `.dockerignore`. Never commit private keys to version control.

## Common Operations

### View Logs

```bash
# All services
docker compose logs -f

# Application only
docker compose logs -f app

# PostgreSQL only
docker compose logs -f db
```

### Check Health

```bash
# Service status
docker compose ps

# Application health endpoint
curl http://localhost:8080/api/v1/health
```

### Rebuild After Code Changes

```bash
# Rebuild and restart the app service
docker compose up -d --build app
```

### Connect to PostgreSQL

```bash
# Via docker exec
docker compose exec db psql -U chef_migration_metrics -d chef_migration_metrics

# Via local psql (if installed)
psql "postgres://chef_migration_metrics:${POSTGRES_PASSWORD}@localhost:5432/chef_migration_metrics"
```

### Stop the Stack

```bash
# Stop services (data is preserved in volumes)
docker compose down

# Stop and remove all data (database, cookbook cache)
docker compose down -v
```

### Reset the Database

```bash
# Remove the database volume and restart — migrations re-run on startup
docker compose down -v
docker compose up -d
```

## Volumes

| Volume | Mount Point | Purpose |
|--------|-------------|---------|
| `pgdata` | `/var/lib/postgresql/data` | PostgreSQL data files — survives `docker compose down` |
| `cookbook_data` | `/var/lib/chef-migration-metrics` | Git clones, cookbook cache, exports — survives restarts |

Both volumes are removed when you run `docker compose down -v`.

## Port Conflicts

If the default ports conflict with services already running on your machine, change them in `.env`:

```bash
# Use port 15432 for PostgreSQL and 9090 for the app
POSTGRES_PORT=15432
APP_PORT=9090
```

## Connecting to the ELK Stack

To also run the Elasticsearch + Logstash + Kibana testing stack alongside this Compose stack, see [`../elk/README.md`](../elk/README.md). The two stacks share data via the NDJSON export directory — enable Elasticsearch export in `config.yml`:

```yaml
elasticsearch:
  enabled: true
  output_directory: /var/lib/chef-migration-metrics/elasticsearch
  retention_hours: 48
```

Then mount the `cookbook_data` volume (or a shared bind mount) into the ELK stack's Logstash service so it can read the NDJSON files.

## Troubleshooting

### `POSTGRES_PASSWORD` error on `docker compose up`

```
POSTGRES_PASSWORD is not set. Set it in .env
```

Copy `.env.example` to `.env` and set a password:

```bash
cp .env.example .env
# Edit .env and set POSTGRES_PASSWORD
```

### App exits immediately

Check the logs for configuration or migration errors:

```bash
docker compose logs app
```

Common causes:
- Invalid `config.yml` syntax (YAML parse error)
- Missing Chef API key file referenced in `config.yml`
- Database connection failure (usually means `db` hasn't finished starting — the health check should prevent this, but check `docker compose ps`)

### Cannot connect to Chef server from inside the container

The container resolves DNS using Docker's embedded DNS. If your Chef server is on a private network:

- Ensure the Docker host can reach the Chef server URL.
- For `https://` URLs with private CAs, you may need to mount the CA certificate and set `SSL_CERT_FILE` in the app environment.

### Permission denied on `keys/` directory

Ensure the key files are readable:

```bash
chmod 644 keys/*.pem
```

The container runs as a non-root user that needs read access to the mounted key files.