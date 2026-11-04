CREATE TABLE user_roles (
    id         UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id  UUID        NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id    UUID        NOT NULL REFERENCES users(id)   ON DELETE CASCADE,
    role_id    UUID        NOT NULL REFERENCES roles(id)   ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT user_roles_unique UNIQUE (tenant_id, user_id, role_id)
);

CREATE INDEX idx_user_roles_tenant_id         ON user_roles (tenant_id);
CREATE INDEX idx_user_roles_user_id           ON user_roles (user_id);
CREATE INDEX idx_user_roles_tenant_id_user_id ON user_roles (tenant_id, user_id);

