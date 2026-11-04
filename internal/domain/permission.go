package domain

import (
	"time"

	"github.com/google/uuid"
)

// Permission represents a single resource:action capability.
type Permission struct {
	ID          uuid.UUID `json:"id"`
	Resource    string    `json:"resource"`
	Action      string    `json:"action"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

// Slug returns the canonical "resource:action" string for this permission.
func (p Permission) Slug() string {
	return p.Resource + ":" + p.Action
}

// RolePermission is the join record between a role and a permission.
type RolePermission struct {
	ID           uuid.UUID `json:"id"`
	RoleID       uuid.UUID `json:"role_id"`
	PermissionID uuid.UUID `json:"permission_id"`
	CreatedAt    time.Time `json:"created_at"`
}

