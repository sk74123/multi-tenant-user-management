package common

import (
	"context"

	"github.com/google/uuid"
)

type contextKey string

const (
	tenantIDKey  contextKey = "tenant_id"
	userIDKey    contextKey = "user_id"
	userEmailKey contextKey = "user_email"
	clientIPKey  contextKey = "client_ip"
	auditMetaKey contextKey = "audit_meta"
)

// AuditMeta holds optional enrichment data populated by handlers or services
// during request execution for the audit middleware.
type AuditMeta struct {
	TenantID   uuid.UUID
	ActorID    *uuid.UUID
	Action     string
	TargetType string
	TargetID   *uuid.UUID
	Before     any
	After      any
	Changes     map[string]any
	Skip       bool
}

// SetTenantID stores the tenant UUID into the context.
func SetTenantID(ctx context.Context, id uuid.UUID) context.Context {
	return context.WithValue(ctx, tenantIDKey, id)
}

// GetTenantID retrieves the tenant UUID from the context.
// Returns uuid.Nil if not set.
func GetTenantID(ctx context.Context) uuid.UUID {
	if v, ok := ctx.Value(tenantIDKey).(uuid.UUID); ok {
		return v
	}
	return uuid.Nil
}

// SetUserID stores the actor user UUID into the context.
func SetUserID(ctx context.Context, id uuid.UUID) context.Context {
	return context.WithValue(ctx, userIDKey, id)
}

// GetUserID retrieves the actor user UUID from the context.
// Returns uuid.Nil if not set.
func GetUserID(ctx context.Context) uuid.UUID {
	if v, ok := ctx.Value(userIDKey).(uuid.UUID); ok {
		return v
	}
	return uuid.Nil
}

// SetUserEmail stores the authenticated user's email into the context.
func SetUserEmail(ctx context.Context, email string) context.Context {
	return context.WithValue(ctx, userEmailKey, email)
}

// GetUserEmail retrieves the authenticated user's email from the context.
func GetUserEmail(ctx context.Context) string {
	if v, ok := ctx.Value(userEmailKey).(string); ok {
		return v
	}
	return ""
}

// SetClientIP stores the request client IP into the context.
func SetClientIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, clientIPKey, ip)
}

// GetClientIP retrieves the request client IP from the context.
func GetClientIP(ctx context.Context) string {
	if v, ok := ctx.Value(clientIPKey).(string); ok {
		return v
	}
	return ""
}

// WithAuditMeta attaches a mutable AuditMeta pointer to the context.
func WithAuditMeta(ctx context.Context, meta *AuditMeta) context.Context {
	return context.WithValue(ctx, auditMetaKey, meta)
}

// GetAuditMeta returns the mutable AuditMeta pointer from the context, or nil.
func GetAuditMeta(ctx context.Context) *AuditMeta {
	if v, ok := ctx.Value(auditMetaKey).(*AuditMeta); ok {
		return v
	}
	return nil
}

// SetAuditBefore records the pre-mutation state in context if AuditMeta is present.
func SetAuditBefore(ctx context.Context, before any) {
	if m := GetAuditMeta(ctx); m != nil {
		m.Before = before
	}
}

// SetAuditAfter records the post-mutation state in context if AuditMeta is present.
func SetAuditAfter(ctx context.Context, after any) {
	if m := GetAuditMeta(ctx); m != nil {
		m.After = after
	}
}


