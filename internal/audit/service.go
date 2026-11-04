package audit

import (
	"context"

	"github.com/google/uuid"
	"github.com/yourusername/multi-tenant-user-management/internal/domain"
)

// Service defines business operations for recording and querying audit logs.
type Service interface {
	Log(ctx context.Context, entry *domain.AuditLog) (*domain.AuditLog, error)
	List(ctx context.Context, tenantID uuid.UUID, params ListParams) ([]*domain.AuditLog, int64, error)
}

type service struct {
	repo Repository
}

// NewService creates a new audit service.
func NewService(repo Repository) Service {
	return &service{repo: repo}
}

func (s *service) Log(ctx context.Context, entry *domain.AuditLog) (*domain.AuditLog, error) {
	if entry == nil || entry.TenantID == uuid.Nil || entry.Action == "" {
		return nil, nil
	}
	return s.repo.Create(ctx, entry)
}

func (s *service) List(ctx context.Context, tenantID uuid.UUID, params ListParams) ([]*domain.AuditLog, int64, error) {
	return s.repo.List(ctx, tenantID, params)
}
