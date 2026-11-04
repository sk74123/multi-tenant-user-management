# Multi-Tenant User Management System — Blueprint

> This is the single source of truth for the project. Each phase references this
> document so we never repeat context.

---

## 1. What We Are Building

A **multi-tenant User Management System** in **Go + PostgreSQL** that lets
multiple organizations (tenants) share a single database while keeping their
data strictly isolated.

### Core capabilities

| Capability | Summary |
|---|---|
| **Tenants** | Create/manage organizations — name, unique code, active/suspended status |
| **Users** | CRUD + soft-delete; a user can belong to **multiple** tenants |
| **Roles** | Tenant-scoped roles; a user can hold multiple roles per tenant |
| **Permissions** | `resource:action` strings (e.g. `user:create`) mapped to roles (RBAC) |
| **Authentication** | Email/password login, bcrypt hashing, JWT access + refresh tokens scoped to one tenant |
| **Authorization** | Middleware validates JWT, checks permissions — returns **401** (no token) or **403** (no permission) |
| **Tenant isolation** | Shared DB, shared schema, `tenant_id` column; tenant derived from JWT only; cross-tenant → **404** |
| **Audit logs** | Tracks login, user/role/assignment/permission changes with actor, action, target, timestamp |

---

## 2. Progress Tracker

| Phase | Description | Status | Commits / Verifications |
|---|---|---|---|
| **Phase 1** | Foundation (Docker, Postgres, Migrations, Seed, Domain structs, Utils) | **COMPLETED** ✅ | `a304c0e` — All 9 DB tables, seed data, /health endpoint |
| **Phase 2** | Tenant & User CRUD (Tenant isolation, Soft delete, Pagination, Validation) | **COMPLETED** ✅ | `d15f330`..`109205f` — Tested 10/10 E2E scenarios, 404 cross-tenant isolation verified |
| **Phase 3** | Roles & Permissions (RBAC, Role-permission & User-role mappings, Effective permissions) | **COMPLETED** ✅ | `d21b8d3`..`967bd30` — Role/Permission CRUD, RBAC mappings, effective permissions & cross-tenant 404 tests verified |
| **Phase 4** | Authentication & Authorization (Bcrypt, JWT access+refresh tokens, Auth middleware, Cache) | **COMPLETED** ✅ | `2817ba1`..`1c46f77` — JWT access+refresh tokens, auth endpoints, permission middleware + TTL cache & invalidation verified |
| **Phase 5** | Audit Logging (Mutation audit logs, actor, action, diffs, query endpoint) | **COMPLETED** ✅ | `7aacefc`..`de61f10` — Audit repository/service/handler, automatic mutation audit middleware, paginated/filterable `/audit-logs` verified |
| **Phase 6** | Automated Tests (Unit, Integration, Cross-tenant isolation test suite) | **COMPLETED** ✅ | Unit tests for all services + E2E integration suite (`tests/`) covering auth, CRUD, 404 isolation, 403 RBAC, soft-delete & audit logs |
| **Phase 7** | Documentation & Polish (Swagger OpenAPI, Design Doc, ER diagram, Final README) | **COMPLETED** ✅ | `docs/swagger.yaml`, `docs/design-document.md`, `docs/er-diagram.md`, `README.md`, and `Makefile` targets completed |

---

## 3. Tech Stack

| Component | Choice | Why |
|---|---|---|
| Language | Go 1.26 | Already installed |
| Database | PostgreSQL 16 | Robust, JSON support for audit `changes` |
| Router | `chi/v5` | Lightweight, idiomatic, excellent middleware |
| DB driver | `pgx/v5` + `pgxpool` | Native PG driver with built-in connection pooling |
| Migrations | `golang-migrate/v4` | SQL-file based, no ORM lock-in |
| JWT | `golang-jwt/v5` | Industry-standard JWT library |
| Hashing | `golang.org/x/crypto/bcrypt` | bcrypt cost 12 |
| Validation | `go-playground/validator/v10` | Struct tag validation |
| UUIDs | `google/uuid` | UUID v4 for all primary keys |
| Testing | `stretchr/testify` | Assertions + mocks |
| Docs | `swaggo/swag` | Auto-generate Swagger from comments |
| Env | `joho/godotenv` | Load `.env` files |

---

## 3. Prerequisites

### 3.1 Tools required

| Tool | Version (minimum) | Check |
|---|---|---|
| Go | 1.21+ | `go version` |
| Docker | 24+ | `docker --version` |
| Docker Compose | v2+ | `docker compose version` |
| Make | any | `make --version` |
| curl / httpie | any | For manual API testing |

### 3.2 Docker images

| Image | Tag | Purpose |
|---|---|---|
| `postgres` | `16-alpine` | Database |
| `golang` | `1.23-alpine` | App build (multi-stage Dockerfile) |
| `alpine` | `3.19` | Final runtime image |

### 3.3 Ports

| Service | Port |
|---|---|
| App (HTTP) | `8080` |
| PostgreSQL | `5432` |

### 3.4 Environment variables

```env
# Server
APP_PORT=8080
APP_ENV=development

# Database
DB_HOST=postgres
DB_PORT=5432
DB_USER=mtums
DB_PASSWORD=mtums_secret
DB_NAME=mtums_db
DB_SSL_MODE=disable
DB_MAX_CONNS=25
DB_MIN_CONNS=5

# JWT
JWT_ACCESS_SECRET=change-me-access-secret-min-32-chars
JWT_REFRESH_SECRET=change-me-refresh-secret-min-32-chars
JWT_ACCESS_EXPIRY=15m
JWT_REFRESH_EXPIRY=168h

# Bcrypt
BCRYPT_COST=12
```

---

## 4. Database Schema (Quick Reference)

Nine tables. All use UUID PKs. Tenant-scoped tables carry a `tenant_id` FK.

```
tenants              (id, name, code, status, created_at, updated_at)
users                (id, email, password_hash, first_name, last_name, is_active, deleted_at, created_at, updated_at)
tenant_users         (id, tenant_id, user_id, is_active, created_at)              UNIQUE(tenant_id, user_id)
roles                (id, tenant_id, name, description, created_at, updated_at)   UNIQUE(tenant_id, name)
permissions          (id, resource, action, description, created_at)              UNIQUE(resource, action)
role_permissions     (id, role_id, permission_id, created_at)                     UNIQUE(role_id, permission_id)
user_roles           (id, tenant_id, user_id, role_id, created_at)                UNIQUE(tenant_id, user_id, role_id)
refresh_tokens       (id, user_id, tenant_id, token_hash, expires_at, revoked, created_at)
audit_logs           (id, tenant_id, actor_id, action, target_type, target_id, changes, ip_address, created_at)
```

Key indexes (created in migrations):
- `idx_tenant_users_tenant_id` / `idx_tenant_users_user_id`
- `idx_roles_tenant_id`
- `idx_user_roles_tenant_id_user_id`
- `idx_audit_logs_tenant_id_created_at`
- `idx_refresh_tokens_token_hash`
- `idx_users_email`
- `idx_users_deleted_at`

---

## 5. Project Structure

```
multi-tenant-user-management/
├── cmd/server/main.go           # Entry point — wires everything
├── internal/
│   ├── config/config.go         # Loads env vars into a struct
│   ├── database/postgres.go     # pgxpool connection
│   ├── middleware/               # auth.go, permission.go, audit.go
│   ├── domain/                   # Pure structs: tenant.go, user.go, role.go, permission.go, audit.go
│   ├── tenant/                   # handler.go, service.go, repository.go
│   ├── user/                     # handler.go, service.go, repository.go
│   ├── role/                     # handler.go, service.go, repository.go
│   ├── permission/               # handler.go, service.go, repository.go
│   ├── auth/                     # handler.go, service.go, jwt.go, repository.go
│   ├── audit/                    # service.go, repository.go
│   └── common/                   # errors.go, pagination.go, response.go, validator.go
├── migrations/                   # .up.sql / .down.sql files + seed.sql
├── tests/                        # Integration tests
├── docs/                         # design-document.md, swagger.yaml, er-diagram.md
├── docker-compose.yml
├── Dockerfile
├── Makefile
├── .env.example
├── go.mod
└── README.md
```

Each domain module follows the same pattern:
```
handler.go      — HTTP handlers (parse request, call service, write response)
service.go      — Business logic (validation, orchestration)
repository.go   — SQL queries (accepts pgxpool, returns domain structs)
```

---

## 6. API Overview

All routes are prefixed with `/api/v1`.

### Public (no auth)
```
POST   /auth/register          Register a user + join a tenant
POST   /auth/login             Returns access_token + refresh_token
POST   /auth/refresh           Rotate tokens
```

### Protected (JWT required, tenant from token)
```
POST   /auth/logout                         Revoke refresh token

GET    /tenants/:id                          tenant:read
PATCH  /tenants/:id                          tenant:update
POST   /tenants                              tenant:create

GET    /users                                user:list       (paginated, filterable)
GET    /users/:id                            user:read
POST   /users                                user:create
PATCH  /users/:id                            user:update
DELETE /users/:id                            user:delete      (soft)

GET    /roles                                role:list
POST   /roles                                role:create
PATCH  /roles/:id                            role:update
DELETE /roles/:id                            role:delete
POST   /roles/:id/permissions                role:update
DELETE /roles/:id/permissions/:pid           role:update

GET    /users/:id/roles                      user:read
POST   /users/:id/roles                      user:assign_role
DELETE /users/:id/roles/:rid                 user:assign_role

GET    /permissions                          permission:list
POST   /permissions                          permission:create

GET    /audit-logs                           audit:read       (paginated, filterable)
```

Standard JSON response envelope:
```json
{
  "success": true,
  "data": {},
  "meta": { "page": 1, "per_page": 20, "total": 100 },
  "error": { "code": "VALIDATION_ERROR", "message": "...", "details": [] }
}
```

---

## 7. Phases

### Phase 1 — Foundation `[COMPLETED ✅]`
**Goal:** Runnable project skeleton with Docker, DB connection, migrations, and helpers.

**Delivers:**
- [x] `go.mod` with all dependencies
- [x] `docker-compose.yml` (postgres + app)
- [x] `Dockerfile` (multi-stage)
- [x] `Makefile` (common commands)
- [x] `.env.example`
- [x] `internal/config/` — config loader
- [x] `internal/database/` — pgxpool setup
- [x] `internal/common/` — errors, pagination, response, validator
- [x] `internal/domain/` — all domain structs
- [x] `migrations/` — all 9 tables (up + down)
- [x] `migrations/seed.sql` — default tenant, admin user, roles, permissions
- [x] `cmd/server/main.go` — boots server, runs migrations, health-check endpoint

**Status:** Completed in commit `a304c0e`. Verified `GET /health` returns `200` with DB status ok.

---

### Phase 2 — Tenant & User CRUD `[COMPLETED ✅]`
**Goal:** Full CRUD for tenants and users with tenant isolation.

**Delivers:**
- [x] `internal/tenant/` — handler, service, repository
- [x] `internal/user/` — handler, service, repository (with soft-delete)
- [x] Routes wired in `main.go`
- [x] Input validation on all create/update endpoints
- [x] Pagination + filtering on list endpoints
- [x] Tenant-scoped queries (`WHERE tenant_id = $1`)
- [x] Header-based tenant extraction middleware (`internal/middleware/tenant.go`)
- [x] Generated `go.sum` and optimized Docker layer caching

**Status:** Completed in commits `d15f330`..`109205f`. All 10 E2E integration test scenarios verified including cross-tenant 404 isolation.

---

### Phase 3 — Roles & Permissions (RBAC) `[COMPLETED ✅]`
**Goal:** Tenant-scoped roles, global permissions, and role ↔ permission + user ↔ role mappings.

**Delivers:**
- [x] `internal/role/` — handler, service, repository
- [x] `internal/permission/` — handler, service, repository
- [x] Assign/remove permissions to roles
- [x] Assign/remove roles to users (within a tenant)
- [x] Get a user's effective permissions (union of all role permissions)
- [x] Routes wired in `cmd/server/main.go` and verified via unit/HTTP tests

**Done when:** Can create roles in a tenant, attach permissions, assign roles to
users, and query a user's effective permission set.

**Status:** Completed in commits `d21b8d3`..`967bd30`. Verified role/permission CRUD, RBAC mappings, effective permission resolution, and cross-tenant 404 isolation.

---

### Phase 4 — Authentication & Authorization `[COMPLETED ✅]`
**Goal:** Secure the API with JWT auth and permission checks.

**Delivers:**
- [x] `internal/auth/` — handler, service, jwt.go, repository
- [x] Register, login, refresh, logout endpoints
- [x] bcrypt password hashing
- [x] JWT access token (15 min, contains `user_id`, `tenant_id`, `email`)
- [x] Refresh token (7 days, hashed in DB, rotated on use)
- [x] `internal/middleware/auth.go` — extracts + validates JWT, sets tenant context
- [x] `internal/middleware/permission.go` — checks `resource:action` from user's roles, returns 401/403
- [x] In-memory permission cache with TTL + invalidation
- [x] Remove temporary `X-Tenant-ID` header; tenant comes from token only

**Done when:** All protected routes require valid JWT; accessing another tenant's
data returns 404; missing permission returns 403.

**Status:** Completed in commits `2817ba1`..`1c46f77`. Verified JWT access/refresh lifecycle, rotation, revocation, permission cache with invalidation, and strict JWT-only tenant isolation.

---

### Phase 5 — Audit Logging `[COMPLETED ✅]`
**Goal:** Track all meaningful actions.

**Delivers:**
- [x] `internal/audit/` — service, repository, handler
- [x] `internal/middleware/audit.go` — captures mutations automatically
- [x] Audit log entries for: login, user create/update/delete, role create/update/delete, role-permission changes, user-role changes
- [x] Each entry: `actor_id`, `tenant_id`, `action`, `target_type`, `target_id`, `changes` (JSONB diff), `ip_address`, `created_at`
- [x] `GET /audit-logs` with pagination + filters (action, target_type, date range)

**Done when:** Every mutation creates an audit row; audit-logs endpoint returns
paginated, filterable results.

**Status:** Completed in commits `7aacefc`..`de61f10`. Verified automatic mutation capture with sanitized JSONB diffs and paginated/filterable `GET /api/v1/audit-logs`.

---

### Phase 6 — Testing `[COMPLETED ✅]`
**Goal:** Comprehensive test coverage.

**Delivers:**
- [x] Unit tests for every service (mocked repository via interfaces)
- [x] Integration tests (`tests/integration_test.go`)
  - [x] Auth flow: register → login → access protected route → refresh → logout
  - [x] CRUD flows for tenants, users, roles, permissions
  - [x] Cross-tenant isolation: user in tenant A cannot see tenant B data (expects 404)
  - [x] Permission enforcement: user without `user:create` gets 403
  - [x] Soft-delete: deleted user excluded from lists but recoverable
- [x] Test helpers in `tests/setup_test.go`

**Done when:** `go test ./...` passes; cross-tenant and permission tests are green.

**Status:** Completed. All unit tests (`internal/tenant`, `internal/user`, `internal/role`, `internal/permission`, `internal/auth`, `internal/audit`, `internal/middleware`) and full-stack E2E tests (`tests/`) pass with `-race`.

---

### Phase 7 — Documentation & Polish `[COMPLETED ✅]`
**Goal:** Production-ready docs and final touches.

**Delivers:**
- [x] `docs/swagger.yaml` — full OpenAPI 3.0 spec
- [x] `docs/design-document.md` — 1–2 page design doc (architecture, isolation strategy, RBAC model, token design, scaling, trade-offs, next steps)
- [x] `docs/er-diagram.md` — rendered ER diagram
- [x] `README.md` — setup guide (clone, env, docker compose up, seed, test)
- [x] Makefile targets: `make run`, `make migrate`, `make seed`, `make test`, `make swagger`

**Done when:** A new developer can clone the repo, run `docker compose up`, and
have a fully working system with docs.

**Status:** Completed. Full OpenAPI 3.0 spec, system design document, Mermaid ER diagram, updated README, and Makefile targets delivered.

---

## 8. Conventions

| Area | Convention |
|---|---|
| Error responses | `{ "success": false, "error": { "code": "...", "message": "..." } }` |
| IDs | UUID v4 everywhere |
| Timestamps | UTC, ISO 8601 |
| Soft delete | `deleted_at IS NULL` filter on all user queries |
| Pagination | `?page=1&per_page=20`, max 100 |
| Filtering | Query params: `?search=john&status=active` |
| Naming | snake_case in JSON, DB; camelCase in Go structs with json tags |
| Tenant context | Stored in `context.Context` after auth middleware |
| SQL | Raw SQL via pgx — no ORM |

---

## 9. Seed Data

Created in `migrations/seed.sql`:

- **Tenant:** "Acme Corp" (code: `acme`, active)
- **Tenant:** "Globex Inc" (code: `globex`, active)
- **Admin user:** `admin@acme.com` / `Admin@123` → member of Acme
- **Regular user:** `user@acme.com` / `User@123` → member of Acme
- **Globex user:** `admin@globex.com` / `Admin@123` → member of Globex
- **Roles:** `admin`, `manager`, `viewer` in each tenant
- **Permissions:** `tenant:read`, `tenant:update`, `user:create`, `user:read`, `user:update`, `user:delete`, `user:list`, `user:assign_role`, `role:create`, `role:read`, `role:list`, `role:update`, `role:delete`, `permission:create`, `permission:list`, `audit:read`
- **admin role** gets all permissions; **viewer** gets read/list only

