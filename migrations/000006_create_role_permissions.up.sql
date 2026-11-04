CREATE TABLE role_permissions (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    role_id       UUID        NOT NULL REFERENCES roles(id)       ON DELETE CASCADE,
    permission_id UUID        NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT role_permissions_unique UNIQUE (role_id, permission_id)
);

CREATE INDEX idx_role_permissions_role_id       ON role_permissions (role_id);
CREATE INDEX idx_role_permissions_permission_id ON role_permissions (permission_id);

