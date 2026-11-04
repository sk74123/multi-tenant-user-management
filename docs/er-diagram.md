# Entity-Relationship Diagram (ER Diagram)

This document describes the database schema for the **Multi-Tenant User Management System**.
All 9 tables reside in a shared PostgreSQL 16 schema and use `UUID` v4 primary keys (`uuid_generate_v4()`).

---

## 1. Mermaid ER Diagram

```mermaid
erDiagram
    tenants {
        UUID id PK
        VARCHAR_255 name
        VARCHAR_50 code UK
        VARCHAR_20 status "active | suspended"
        TIMESTAMPTZ created_at
        TIMESTAMPTZ updated_at
    }

    users {
        UUID id PK
        VARCHAR_255 email UK
        VARCHAR_255 password_hash
        VARCHAR_100 first_name
        VARCHAR_100 last_name
        BOOLEAN is_active
        TIMESTAMPTZ deleted_at "nullable (soft delete)"
        TIMESTAMPTZ created_at
        TIMESTAMPTZ updated_at
    }

    tenant_users {
        UUID id PK
        UUID tenant_id FK
        UUID user_id FK
        BOOLEAN is_active
        TIMESTAMPTZ created_at
    }

    roles {
        UUID id PK
        UUID tenant_id FK
        VARCHAR_100 name
        TEXT description
        TIMESTAMPTZ created_at
        TIMESTAMPTZ updated_at
    }

    permissions {
        UUID id PK
        VARCHAR_100 resource
        VARCHAR_100 action
        TEXT description
        TIMESTAMPTZ created_at
    }

    role_permissions {
        UUID id PK
        UUID role_id FK
        UUID permission_id FK
        TIMESTAMPTZ created_at
    }

    user_roles {
        UUID id PK
        UUID tenant_id FK
        UUID user_id FK
        UUID role_id FK
        TIMESTAMPTZ created_at
    }

    refresh_tokens {
        UUID id PK
        UUID user_id FK
        UUID tenant_id FK
        VARCHAR_255 token_hash UK
        TIMESTAMPTZ expires_at
        BOOLEAN revoked
        TIMESTAMPTZ created_at
    }

    audit_logs {
        UUID id PK
        UUID tenant_id FK
        UUID actor_id FK "nullable"
        VARCHAR_100 action
        VARCHAR_100 target_type
        UUID target_id "nullable"
        JSONB changes
        VARCHAR_45 ip_address
        TIMESTAMPTZ created_at
    }

    tenants ||--o{ tenant_users : "has members"
    users ||--o{ tenant_users : "belongs to"
    tenants ||--o{ roles : "defines"
    roles ||--o{ role_permissions : "grants"
    permissions ||--o{ role_permissions : "assigned to"
    tenants ||--o{ user_roles : "scopes"
    users ||--o{ user_roles : "holds"
    roles ||--o{ user_roles : "assigned via"
    users ||--o{ refresh_tokens : "owns"
    tenants ||--o{ refresh_tokens : "scopes"
    tenants ||--o{ audit_logs : "records"
    users ||--o{ audit_logs : "acts as actor"
```

---

## 2. Table Relationships & Constraints Summary

| Table | Scope | Unique Constraints | Foreign Keys |
|---|---|---|---|
| `tenants` | Global | `UNIQUE (code)` | — |
| `users` | Global | `UNIQUE (email)` | — |
| `tenant_users` | Tenant ↔ User | `UNIQUE (tenant_id, user_id)` | `tenant_id → tenants(id)`, `user_id → users(id)` |
| `roles` | Tenant-scoped | `UNIQUE (tenant_id, name)` | `tenant_id → tenants(id)` |
| `permissions` | Global | `UNIQUE (resource, action)` | — |
| `role_permissions` | Role ↔ Permission | `UNIQUE (role_id, permission_id)` | `role_id → roles(id)`, `permission_id → permissions(id)` |
| `user_roles` | Tenant-scoped | `UNIQUE (tenant_id, user_id, role_id)` | `tenant_id → tenants(id)`, `user_id → users(id)`, `role_id → roles(id)` |
| `refresh_tokens` | Tenant-scoped | `UNIQUE (token_hash)` | `user_id → users(id)`, `tenant_id → tenants(id)` |
| `audit_logs` | Tenant-scoped | — | `tenant_id → tenants(id)`, `actor_id → users(id) ON DELETE SET NULL` |

---

## 3. Key Indexes

- `idx_tenants_code`, `idx_tenants_status`
- `idx_users_email`, `idx_users_deleted_at` (partial index `WHERE deleted_at IS NULL`), `idx_users_is_active`
- `idx_tenant_users_tenant_id`, `idx_tenant_users_user_id`
- `idx_roles_tenant_id`
- `idx_role_permissions_role_id`, `idx_role_permissions_permission_id`
- `idx_user_roles_tenant_id_user_id`, `idx_user_roles_role_id`
- `idx_refresh_tokens_user_id`, `idx_refresh_tokens_token_hash`, `idx_refresh_tokens_expires_at`
- `idx_audit_logs_tenant_id`, `idx_audit_logs_actor_id`, `idx_audit_logs_tenant_id_created_at`, `idx_audit_logs_target_type`, `idx_audit_logs_action`
