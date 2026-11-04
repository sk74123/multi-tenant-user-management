package common

import (
	"context"

	"github.com/google/uuid"
)

type contextKey string

const (
	tenantIDKey contextKey = "tenant_id"
	userIDKey   contextKey = "user_id"
)

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

