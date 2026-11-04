# Multi-Tenant User Management System

A production-grade **Multi-Tenant User Management System** built with **Go 1.23+** and **PostgreSQL 16**. It enables multiple organizations (tenants) to share a single database while enforcing strict tenant isolation, role-based access control (RBAC) with in-memory TTL caching, JWT access/refresh token rotation, and automatic mutation audit logging.

---

## Quick Start

```bash
# 1. Clone and enter the project
git clone <repo-url>
cd multi-tenant-user-management

# 2. Copy environment file
cp .env.example .env

# 3. Start the stack (PostgreSQL 16 + Go server) — migrations run automatically on startup
docker compose up --build -d

# 4. Seed the database with default tenants, users, roles, and permissions
make seed

# 5. Verify the server is healthy
curl http://localhost:8080/health

# 6. Run the automated test suite
make test
```

---

## Seeded Credentials

| Email | Password | Tenant Name | Tenant Code | Role | Permissions |
|---|---|---|---|---|---|
| `admin@acme.com` | `Admin@123` | Acme Corp | `acme` | `admin` | All 16 permissions |
| `user@acme.com` | `User@123` | Acme Corp | `acme` | `viewer` | All `*:read` and `*:list` permissions |
| `admin@globex.com` | `Admin@123` | Globex Inc | `globex` | `admin` | All 16 permissions |

---

## API Overview (`/api/v1`)

### Public Endpoints (No Auth Required)
| Method | Endpoint | Description |
|---|---|---|
| `POST` | `/api/v1/auth/register` | Register a user and join a tenant (`tenant_code` or `tenant_id`) |
| `POST` | `/api/v1/auth/login` | Authenticate with email/password + tenant; returns `access_token` & `refresh_token` |
| `POST` | `/api/v1/auth/refresh` | Rotate `refresh_token` and issue a new token pair |

### Protected Endpoints (Bearer JWT Required — Tenant Derived Strictly from Token)
| Method | Endpoint | Required Permission | Description |
|---|---|---|---|
| `POST` | `/api/v1/auth/logout` | *(Authenticated)* | Revoke refresh token(s) |
| `POST` | `/api/v1/tenants` | `tenant:create` / `tenant:update` | Create a new tenant |
| `GET` | `/api/v1/tenants` | `tenant:read` | List tenants (paginated, filterable) |
| `GET` | `/api/v1/tenants/{id}` | `tenant:read` | Get tenant details (404 on cross-tenant) |
| `PATCH` | `/api/v1/tenants/{id}` | `tenant:update` | Update tenant name or status (404 on cross-tenant) |
| `GET` | `/api/v1/users` | `user:list` | List users in active tenant (`?page=1&per_page=20&search=&is_active=`) |
| `POST` | `/api/v1/users` | `user:create` | Create a user in active tenant |
| `GET` | `/api/v1/users/{id}` | `user:read` | Get user in active tenant (404 on cross-tenant) |
| `PATCH` | `/api/v1/users/{id}` | `user:update` | Update user in active tenant |
| `DELETE` | `/api/v1/users/{id}` | `user:delete` | Soft-delete user (`deleted_at` set) |
| `GET` | `/api/v1/roles` | `role:list` | List roles in active tenant |
| `POST` | `/api/v1/roles` | `role:create` | Create a role in active tenant |
| `GET` | `/api/v1/roles/{id}` | `role:read` | Get role by ID |
| `PATCH` | `/api/v1/roles/{id}` | `role:update` | Update role name/description |
| `DELETE` | `/api/v1/roles/{id}` | `role:delete` | Delete role |
| `GET` | `/api/v1/roles/{id}/permissions` | `role:read` | List permissions assigned to a role |
| `POST` | `/api/v1/roles/{id}/permissions` | `role:update` | Assign permission(s) to a role |
| `DELETE` | `/api/v1/roles/{id}/permissions/{pid}` | `role:update` | Remove a permission from a role |
| `GET` | `/api/v1/users/{id}/roles` | `user:read` | List roles assigned to a user in active tenant |
| `POST` | `/api/v1/users/{id}/roles` | `user:assign_role` | Assign role(s) to a user in active tenant |
| `DELETE` | `/api/v1/users/{id}/roles/{rid}` | `user:assign_role` | Remove a role from a user |
| `GET` | `/api/v1/users/{id}/permissions` | `user:read` | Get user's effective permissions (union across roles) |
| `GET` | `/api/v1/permissions` | `permission:list` | List global permissions |
| `POST` | `/api/v1/permissions` | `permission:create` | Create a global `resource:action` permission |
| `GET` | `/api/v1/audit-logs` | `audit:read` | Query tenant audit logs (`?action=&target_type=&from=&to=`) |

---

## Makefile Targets

| Target | Description |
|---|---|
| `make run` | Run the server locally (`go run ./cmd/server`) |
| `make build` | Build binary to `bin/server` |
| `make migrate` / `make migrate-up` | Apply all up SQL migrations |
| `make migrate-down` | Roll back SQL migrations |
| `make seed` | Seed default tenants, users, roles, and permissions |
| `make test` | Run all unit and integration tests (`go test -v -race ./...`) |
| `make swagger` | Generate/verify OpenAPI documentation (`docs/swagger.yaml`) |
| `make docker-up` | Start PostgreSQL + App containers |
| `make docker-down` | Stop containers |

---

## Documentation

- **[Blueprint](./blueprint.md)** — Single source of truth for phases, schema, and conventions
- **[Design Document](./docs/design-document.md)** — Architecture, tenant isolation, RBAC caching, token rotation, and scaling trade-offs
- **[ER Diagram](./docs/er-diagram.md)** — Database schema diagram and relationship reference
- **[OpenAPI 3.0 Spec](./docs/swagger.yaml)** — Full Swagger/OpenAPI specification

