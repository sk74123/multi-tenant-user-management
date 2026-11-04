package domain

import (
	"time"

	"github.com/google/uuid"
)

// User represents a system user. A user may belong to multiple tenants.
type User struct {
	ID           uuid.UUID  `json:"id"`
	Email        string     `json:"email"`
	PasswordHash string     `json:"-"` // never serialised
	FirstName    string     `json:"first_name"`
	LastName     string     `json:"last_name"`
	IsActive     bool       `json:"is_active"`
	DeletedAt    *time.Time `json:"deleted_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// TenantUser is the join record linking a user to a tenant.
type TenantUser struct {
	ID        uuid.UUID `json:"id"`
	TenantID  uuid.UUID `json:"tenant_id"`
	UserID    uuid.UUID `json:"user_id"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
}

