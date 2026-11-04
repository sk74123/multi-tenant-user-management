package domain

import (
	"time"

	"github.com/google/uuid"
)

// AuditLog records a single meaningful action in the system.
type AuditLog struct {
	ID         uuid.UUID  `json:"id"`
	TenantID   uuid.UUID  `json:"tenant_id"`
	ActorID    *uuid.UUID `json:"actor_id,omitempty"` // nil for system actions
	Action     string     `json:"action"`
	TargetType string     `json:"target_type"`
	TargetID   *uuid.UUID `json:"target_id,omitempty"`
	Changes    any        `json:"changes,omitempty"` // JSONB diff
	IPAddress  string     `json:"ip_address,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// RefreshToken stores a hashed refresh token scoped to user + tenant.
type RefreshToken struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	TenantID  uuid.UUID `json:"tenant_id"`
	TokenHash string    `json:"-"`
	ExpiresAt time.Time `json:"expires_at"`
	Revoked   bool      `json:"revoked"`
	CreatedAt time.Time `json:"created_at"`
}

