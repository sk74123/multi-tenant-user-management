CREATE TABLE audit_logs (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id   UUID         NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    actor_id    UUID         REFERENCES users(id) ON DELETE SET NULL,
    action      VARCHAR(100) NOT NULL,
    target_type VARCHAR(100) NOT NULL DEFAULT '',
    target_id   UUID,
    changes     JSONB,
    ip_address  VARCHAR(45)  NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_audit_logs_tenant_id            ON audit_logs (tenant_id);
CREATE INDEX idx_audit_logs_actor_id             ON audit_logs (actor_id);
CREATE INDEX idx_audit_logs_tenant_id_created_at ON audit_logs (tenant_id, created_at DESC);
CREATE INDEX idx_audit_logs_target_type          ON audit_logs (target_type);
CREATE INDEX idx_audit_logs_action               ON audit_logs (action);

