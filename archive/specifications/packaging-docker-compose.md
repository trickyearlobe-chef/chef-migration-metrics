# Packaging — Docker Compose

## 4. Docker Compose

### 4.1 Purpose

The Docker Compose file starts a PostgreSQL database for local development. The application runs on the host via `make run` or `make dev`.

### 4.2 File Location

```
deploy/
└── docker-compose/
    ├── docker-compose.yml          # Compose file
    ├── config.yml                  # Example application configuration for local use
    ├── .env.example                # Example environment variables
    └── README.md                   # Quick-start instructions
```

### 4.3 Services

#### `db` — PostgreSQL

| Property | Value |
|----------|-------|
| Image | `postgres:16-bookworm` |
| Ports | `5432:5432` (exposed for local debugging; not required in production) |
| Volumes | Named volume `pgdata` for data persistence across restarts |
| Environment | `POSTGRES_DB=chef_migration_metrics`, `POSTGRES_USER`, `POSTGRES_PASSWORD` from `.env` |
| Health check | `pg_isready -U $POSTGRES_USER -d $POSTGRES_DB` |

### 4.4 docker-compose.yml

```yaml
services:
  db:
    image: postgres:16-bookworm
    restart: unless-stopped
    command: ["-c", "shared_preload_libraries=pg_stat_statements"]
    environment:
      POSTGRES_DB: ${POSTGRES_DB:-chef_migration_metrics}
      POSTGRES_USER: ${POSTGRES_USER:-chef_migration_metrics}
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD:?Set POSTGRES_PASSWORD in .env}
    volumes:
      - pgdata:/var/lib/postgresql/data
    ports:
      - "${POSTGRES_PORT:-5432}:5432"
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ${POSTGRES_USER:-chef_migration_metrics} -d ${POSTGRES_DB:-chef_migration_metrics}"]
      interval: 5s
      timeout: 3s
      retries: 10

volumes:
  pgdata:
    driver: local
```

### 4.5 Environment File

The `.env.example` file documents all configurable environment variables:

```
# PostgreSQL
POSTGRES_DB=chef_migration_metrics
POSTGRES_USER=chef_migration_metrics
POSTGRES_PASSWORD=changeme
POSTGRES_PORT=5432
```

### 4.6 Usage

```bash
cd deploy/docker-compose
cp .env.example .env
# Edit .env — at minimum set POSTGRES_PASSWORD

docker compose up -d    # start PostgreSQL
make run                # start the app on the host

# View DB logs
docker compose logs -f db

# Stop
docker compose down

# Stop and remove data
docker compose down -v
```
