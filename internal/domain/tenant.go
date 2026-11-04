package domain

import (
	"time"

	"github.com/google/uuid"
)

// TenantStatus represents the lifecycle state of a tenant.
type TenantStatus string

const (
	TenantStatusActive    TenantStatus = "active"
	TenantStatusSuspended TenantStatus = "suspended"
)

// Tenant represents an organization in the system.
type Tenant struct {
	ID        uuid.UUID    `json:"id"`
	Name      string       `json:"name"`
	Code      string       `json:"code"`
	Status    TenantStatus `json:"status"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
}

