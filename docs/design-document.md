# System Design Document — Multi-Tenant User Management System

## 1. Architecture Overview

The **Multi-Tenant User Management System (MTUMS)** is a production-grade HTTP API built with **Go 1.23+** and **PostgreSQL 16** that enables multiple organizations (tenants) to share a single database while maintaining strict logical isolation, role-based access control (RBAC), and comprehensive audit logging.

### Layered Clean Architecture
Every domain package (`tenant`, `user`, `role`, `permission`, `auth`, `audit`) follows a strict three-layer separation of concerns:

```
HTTP Request
   │
   ▼
[Chi Router & Middleware Pipeline]
   ├── RequestID / RealIP / Logger / Recoverer / Timeout
   ├── Audit Middleware (captures mutations & before/after diffs)
   ├── Authenticate Middleware (validates Bearer JWT → sets tenant_id & user_id in context)
   └── PermissionAuthorizer Middleware (checks resource:action via TTL cache / DB)
   │
   ▼
[Handler Layer]      (internal/<domain>/handler.go)
   • Parses URL params, query filters, pagination, and validates JSON request structs
   • Formats standard JSON envelopes ({ success, data, meta, error })
   │
   ▼
[Service Layer]      (internal/<domain>/service.go)
   • Enforces business rules, tenant isolation checks, password hashing, token rotation
   • Triggers permission cache invalidation and populates audit snapshots
   │
   ▼
[Repository Layer]   (internal/<domain>/repository.go)
   • Executes parameterized SQL queries via pgx/v5 connection pool (no ORM)
   • Enforces WHERE tenant_id = $1 and deleted_at IS NULL predicates
```

---

## 2. Multi-Tenant Isolation Strategy

### Shared Database, Shared Schema (`tenant_id` Discriminator)
We use the **shared database, shared schema** model with a mandatory `tenant_id` foreign key on all tenant-scoped tables (`tenant_users`, `roles`, `user_roles`, `refresh_tokens`, `audit_logs`).

### Key Security Invariants
1. **Tenant Context Strictly from JWT**:
   Protected endpoints never trust client-supplied headers (such as `X-Tenant-ID`) or query parameters to establish tenant context. `middleware.Authenticate` extracts `tenant_id` and `user_id` exclusively from the cryptographically verified HS256 Access JWT and stores them in `context.Context`.
2. **Many-to-Many User Membership**:
   Users are stored globally in `users` (allowing one email identity to belong to multiple organizations via `tenant_users`), while all user queries join through `tenant_users WHERE tu.tenant_id = $1 AND u.deleted_at IS NULL AND tu.is_active = TRUE`.
3. **Anti-Enumeration (`404 Not Found` on Cross-Tenant Access)**:
   Whenever a user in Tenant A attempts to read, update, delete, or assign roles to a resource belonging to Tenant B, the repository query (`WHERE tenant_id = $1 AND id = $2`) returns `pgx.ErrNoRows`, which maps to **`404 Not Found`** rather than `403 Forbidden`. This prevents attackers from probing UUIDs belonging to other tenants.

---

## 3. RBAC Model & Permission Caching

- **Permissions (`permissions`)**: Global capabilities expressed as `resource:action` slugs (`user:create`, `role:update`, `audit:read`).
- **Roles (`roles`)**: Tenant-scoped named sets of permissions (`admin`, `manager`, `viewer`).
- **Role-Permissions (`role_permissions`)**: Maps permissions to a tenant role.
- **User-Roles (`user_roles`)**: Assigns one or more tenant roles to a user within a tenant (`UNIQUE(tenant_id, user_id, role_id)`).
- **Effective Permissions**: Computed as the `DISTINCT` union of all permissions across all roles held by `(tenant_id, user_id)`.
- **In-Memory Permission Cache (`middleware.PermissionCache`)**:
  Caches `map[string]bool` keyed by `tenantID:userID` with a **5-minute TTL** protected by `sync.RWMutex`. Role/permission mutations immediately trigger `InvalidateUser(tenantID, userID)` or `InvalidateTenant(tenantID)`.

---

## 4. Authentication & Token Design

- **Password Security**: Hashed using `bcrypt` (cost `12`) and never serialized in JSON (`json:"-"`).
- **Access Token**: Stateless HS256 JWT (TTL `15m`) carrying `user_id`, `tenant_id`, `email`, `sub`, `jti`, `iat`, `exp`.
- **Refresh Token**: Stateful HS256 JWT (TTL `168h` / 7 days). Only its **SHA-256 hex digest** (`token_hash`) is persisted in `refresh_tokens`. Calling `POST /api/v1/auth/refresh` validates the token, marks the old token `revoked = TRUE`, and issues a rotated token pair.

---

## 5. Audit Logging Design

`middleware.Audit` automatically captures all successful (`2xx`) state-changing requests (`POST`, `PATCH`, `PUT`, `DELETE`), recording `tenant_id`, `actor_id`, `action`, `target_type`, `target_id`, `ip_address`, and a `JSONB` `changes` diff (`before`, `after`, and sanitized `input` with sensitive fields redacted).

---

## 6. Scaling, Trade-Offs & Next Steps

| Decision | Trade-Off | Next Step at Scale |
|---|---|---|
| Shared schema with `tenant_id` | High resource density & simple migrations, relies on SQL discipline | Enable PostgreSQL **Row-Level Security (RLS)** policies as defense-in-depth |
| In-memory `PermissionCache` | Zero external dependencies & sub-ms lookups, per-instance state | Replace with **Redis** pub/sub invalidation for horizontal scaling |
| Synchronous audit insert | Guarantees audit consistency, adds ~1ms write latency | Offload to buffered worker pool or transactional outbox |
