# Multi-Tenant User Management System

A production-grade multi-tenant user management system built with **Go** and **PostgreSQL**.

## Quick Start

```bash
# 1. Clone and enter the project
git clone <repo-url>
cd multi-tenant-user-management

# 2. Copy environment file
cp .env.example .env
# Edit .env to set strong secrets before deploying to production

# 3. Start the stack (PostgreSQL + app) — migrations run automatically
docker compose up --build -d

# 4. Seed the database with tenants, users, roles and permissions
make seed

# 5. Verify the server is healthy
curl http://localhost:8080/health
```

## Credentials (seed data)

| Email | Password | Tenant | Role |
|---|---|---|---|
| admin@acme.com | Admin@123 | Acme Corp | admin |
| user@acme.com | User@123 | Acme Corp | viewer |
| admin@globex.com | Admin@123 | Globex Inc | admin |

## Makefile commands

| Command | Description |
|---|---|
| `make run` | Run the app locally (requires local PG) |
| `make build` | Compile binary to `bin/server` |
| `make docker-up` | Start full stack with Docker Compose |
| `make docker-down` | Stop Docker Compose |
| `make docker-logs` | Tail app logs |
| `make docker-db` | Open psql shell |
| `make migrate-up` | Apply pending migrations |
| `make migrate-down` | Roll back all migrations |
| `make seed` | Load seed data |
| `make test` | Run all unit tests |
| `make swagger` | Regenerate Swagger docs |

## Project Structure

See [blueprint.md](./blueprint.md) — the single source of truth for architecture,
phases, schema, API design, and conventions.

## API

Base URL: `http://localhost:8080/api/v1`

Full OpenAPI docs: `docs/swagger.yaml` (generated in Phase 7).

## Architecture

- **Clean architecture**: `handler → service → repository` per domain package
- **Tenant isolation**: shared schema, `tenant_id` column, tenant from JWT only
- **RBAC**: `resource:action` permissions → roles → users (tenant-scoped)
- **Auth**: bcrypt + JWT access (15 min) + hashed refresh tokens (7 days)
- **Audit**: every mutation logged with actor, action, target, and diff
