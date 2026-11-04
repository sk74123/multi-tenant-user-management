-- =============================================================================
-- Seed Data for Multi-Tenant User Management System
-- Run via: make seed
-- Passwords below are bcrypt hash of:
--   Admin@123  → $2a$12$tlTWFeW0ByPJKsa67B22BuWiJpts8gvHy0JHTbzug7/TRuRiSPKvW
--   User@123   → $2a$12$IqpIwRaqQtOIl1OFENVRAOz5fldSuEfmIS43sTVDIV66jRtFPZH2u
-- =============================================================================

BEGIN;

-- ── Tenants ──────────────────────────────────────────────────────────────────
INSERT INTO tenants (id, name, code, status) VALUES
  ('a0000000-0000-0000-0000-000000000001', 'Acme Corp',   'acme',   'active'),
  ('a0000000-0000-0000-0000-000000000002', 'Globex Inc',  'globex', 'active')
ON CONFLICT (code) DO NOTHING;

-- ── Users ─────────────────────────────────────────────────────────────────────
INSERT INTO users (id, email, password_hash, first_name, last_name, is_active) VALUES
  ('b0000000-0000-0000-0000-000000000001', 'admin@acme.com',   '$2a$12$tlTWFeW0ByPJKsa67B22BuWiJpts8gvHy0JHTbzug7/TRuRiSPKvW', 'Acme',   'Admin',  TRUE),
  ('b0000000-0000-0000-0000-000000000002', 'user@acme.com',    '$2a$12$IqpIwRaqQtOIl1OFENVRAOz5fldSuEfmIS43sTVDIV66jRtFPZH2u', 'Acme',   'User',   TRUE),
  ('b0000000-0000-0000-0000-000000000003', 'admin@globex.com', '$2a$12$tlTWFeW0ByPJKsa67B22BuWiJpts8gvHy0JHTbzug7/TRuRiSPKvW', 'Globex', 'Admin',  TRUE)
ON CONFLICT (email) DO UPDATE SET password_hash = EXCLUDED.password_hash;

-- ── Tenant membership ─────────────────────────────────────────────────────────
INSERT INTO tenant_users (tenant_id, user_id) VALUES
  ('a0000000-0000-0000-0000-000000000001', 'b0000000-0000-0000-0000-000000000001'), -- admin@acme  → Acme
  ('a0000000-0000-0000-0000-000000000001', 'b0000000-0000-0000-0000-000000000002'), -- user@acme   → Acme
  ('a0000000-0000-0000-0000-000000000002', 'b0000000-0000-0000-0000-000000000003')  -- admin@globex→ Globex
ON CONFLICT (tenant_id, user_id) DO NOTHING;

-- ── Permissions ───────────────────────────────────────────────────────────────
INSERT INTO permissions (id, resource, action, description) VALUES
  ('c0000000-0000-0000-0000-000000000001', 'tenant',     'read',        'Read tenant details'),
  ('c0000000-0000-0000-0000-000000000002', 'tenant',     'update',      'Update tenant details'),
  ('c0000000-0000-0000-0000-000000000003', 'user',       'create',      'Create a user in the tenant'),
  ('c0000000-0000-0000-0000-000000000004', 'user',       'read',        'Read user details'),
  ('c0000000-0000-0000-0000-000000000005', 'user',       'update',      'Update user details'),
  ('c0000000-0000-0000-0000-000000000006', 'user',       'delete',      'Soft-delete a user'),
  ('c0000000-0000-0000-0000-000000000007', 'user',       'list',        'List users in the tenant'),
  ('c0000000-0000-0000-0000-000000000008', 'user',       'assign_role', 'Assign or remove roles from a user'),
  ('c0000000-0000-0000-0000-000000000009', 'role',       'create',      'Create a tenant role'),
  ('c0000000-0000-0000-0000-000000000010', 'role',       'read',        'Read role details'),
  ('c0000000-0000-0000-0000-000000000011', 'role',       'list',        'List tenant roles'),
  ('c0000000-0000-0000-0000-000000000012', 'role',       'update',      'Update or reassign role permissions'),
  ('c0000000-0000-0000-0000-000000000013', 'role',       'delete',      'Delete a role'),
  ('c0000000-0000-0000-0000-000000000014', 'permission', 'create',      'Create a permission'),
  ('c0000000-0000-0000-0000-000000000015', 'permission', 'list',        'List permissions'),
  ('c0000000-0000-0000-0000-000000000016', 'audit',      'read',        'Read audit logs')
ON CONFLICT (resource, action) DO NOTHING;

-- ── Roles (per tenant) ────────────────────────────────────────────────────────
-- Acme roles
INSERT INTO roles (id, tenant_id, name, description) VALUES
  ('d0000000-0000-0000-0000-000000000001', 'a0000000-0000-0000-0000-000000000001', 'admin',   'Full access within the tenant'),
  ('d0000000-0000-0000-0000-000000000002', 'a0000000-0000-0000-0000-000000000001', 'manager', 'Manage users and roles'),
  ('d0000000-0000-0000-0000-000000000003', 'a0000000-0000-0000-0000-000000000001', 'viewer',  'Read-only access')
ON CONFLICT (tenant_id, name) DO NOTHING;

-- Globex roles
INSERT INTO roles (id, tenant_id, name, description) VALUES
  ('d0000000-0000-0000-0000-000000000004', 'a0000000-0000-0000-0000-000000000002', 'admin',   'Full access within the tenant'),
  ('d0000000-0000-0000-0000-000000000005', 'a0000000-0000-0000-0000-000000000002', 'viewer',  'Read-only access')
ON CONFLICT (tenant_id, name) DO NOTHING;

-- ── Role permissions ──────────────────────────────────────────────────────────
-- Acme admin: all permissions
INSERT INTO role_permissions (role_id, permission_id)
  SELECT 'd0000000-0000-0000-0000-000000000001', id FROM permissions
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- Acme manager: user + role management (not audit, not permission create)
INSERT INTO role_permissions (role_id, permission_id)
  SELECT 'd0000000-0000-0000-0000-000000000002', id
  FROM permissions
  WHERE (resource = 'user') OR (resource = 'role') OR (resource = 'tenant' AND action = 'read')
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- Acme viewer: read + list only
INSERT INTO role_permissions (role_id, permission_id)
  SELECT 'd0000000-0000-0000-0000-000000000003', id
  FROM permissions
  WHERE action IN ('read', 'list')
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- Globex admin: all permissions
INSERT INTO role_permissions (role_id, permission_id)
  SELECT 'd0000000-0000-0000-0000-000000000004', id FROM permissions
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- Globex viewer: read + list only
INSERT INTO role_permissions (role_id, permission_id)
  SELECT 'd0000000-0000-0000-0000-000000000005', id
  FROM permissions
  WHERE action IN ('read', 'list')
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- ── User role assignments ─────────────────────────────────────────────────────
INSERT INTO user_roles (tenant_id, user_id, role_id) VALUES
  ('a0000000-0000-0000-0000-000000000001', 'b0000000-0000-0000-0000-000000000001', 'd0000000-0000-0000-0000-000000000001'), -- admin@acme  → admin
  ('a0000000-0000-0000-0000-000000000001', 'b0000000-0000-0000-0000-000000000002', 'd0000000-0000-0000-0000-000000000003'), -- user@acme   → viewer
  ('a0000000-0000-0000-0000-000000000002', 'b0000000-0000-0000-0000-000000000003', 'd0000000-0000-0000-0000-000000000004')  -- admin@globex→ admin
ON CONFLICT (tenant_id, user_id, role_id) DO NOTHING;

COMMIT;

