# Docker Compose — PostgreSQL for Local Development

Starts a PostgreSQL database for local development and testing. The application runs on the host via `make run` or `make dev`.

## Prerequisites

- [Docker Engine](https://docs.docker.com/engine/install/) 24+ with the Compose plugin (`docker compose`)

## Quick Start

```bash
cd deploy/docker-compose

# 1. Create your .env file
cp .env.example .env
# Edit .env — at minimum set POSTGRES_PASSWORD to something secure.

# 2. Start the database
docker compose up -d

# 3. Run the application on the host
make run
```

The application is available at [http://localhost:8080](http://localhost:8080) once started with `make run`. Use `make dev` for live-reload during development.

## Services

| Service | Image | Default Port | Purpose |
|---------|-------|-------------|---------|
| `db` | `postgres:16-bookworm` | 5432 | PostgreSQL database with persistent volume |

Database migrations are applied automatically when the application starts.

## Files

| File | Purpose |
|------|---------|
| `docker-compose.yml` | Service definition and volume |
| `.env.example` | Documented environment variables — copy to `.env` |
| `.env` | Your local environment overrides (git-ignored) |

## Environment Variables (`.env`)

| Variable | Default | Description |
|----------|---------|-------------|
| `POSTGRES_DB` | `chef_migration_metrics` | PostgreSQL database name |
| `POSTGRES_USER` | `chef_migration_metrics` | PostgreSQL username |
| `POSTGRES_PASSWORD` | *(required)* | PostgreSQL password — must be set |
| `POSTGRES_PORT` | `5432` | Host port for PostgreSQL (useful if 5432 is in use) |

## Common Operations

### View DB Logs

```bash
docker compose logs -f db
```

### Connect to PostgreSQL

```bash
# Via docker exec
docker compose exec db psql -U chef_migration_metrics -d chef_migration_metrics

# Via local psql (if installed)
psql "postgres://chef_migration_metrics:YOUR_PASSWORD@localhost:5432/chef_migration_metrics"
```

### Stop the Database

```bash
# Stop (data is preserved in the volume)
docker compose down

# Stop and remove all data
docker compose down -v
```

### Reset the Database

```bash
# Remove the database volume and restart — migrations re-run when the app starts
docker compose down -v
docker compose up -d
```

## Volumes

| Volume | Mount Point | Purpose |
|--------|-------------|---------|
| `pgdata` | `/var/lib/postgresql/data` | PostgreSQL data files — survives `docker compose down` |

The volume is removed when you run `docker compose down -v`.

## Port Conflicts

If the default port conflicts with a service already running on your machine, change it in `.env`:

```bash
POSTGRES_PORT=15432
```

## Connecting to the ELK Stack

To run the Elasticsearch + Logstash + Kibana testing stack alongside this Compose stack, see [`../elk/README.md`](../elk/README.md).

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
