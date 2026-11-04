package tenant

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/yourusername/multi-tenant-user-management/internal/common"
	"github.com/yourusername/multi-tenant-user-management/internal/domain"
)

// Service defines business operations on tenants.
type Service interface {
	Create(ctx context.Context, req CreateRequest) (*domain.Tenant, error)
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Tenant, error)
	Update(ctx context.Context, id uuid.UUID, req UpdateRequest) (*domain.Tenant, error)
	List(ctx context.Context, params ListParams) ([]*domain.Tenant, int64, error)
}

// CreateRequest is the validated payload for creating a tenant.
type CreateRequest struct {
	Name string `json:"name" validate:"required,min=2,max=255"`
	Code string `json:"code" validate:"required,min=2,max=50"`
}

// UpdateRequest is the validated payload for updating a tenant.
type UpdateRequest struct {
	Name   *string `json:"name"   validate:"omitempty,min=2,max=255"`
	Status *string `json:"status" validate:"omitempty,oneof=active suspended"`
}

type service struct {
	repo Repository
}

// NewService creates a new tenant service.
func NewService(repo Repository) Service {
	return &service{repo: repo}
}

func (s *service) Create(ctx context.Context, req CreateRequest) (*domain.Tenant, error) {
	// Normalize code: lowercase, no spaces
	code := strings.ToLower(strings.ReplaceAll(req.Code, " ", "-"))

	// Check uniqueness
	if _, err := s.repo.GetByCode(ctx, code); err == nil {
		return nil, common.New(409, "TENANT_CODE_EXISTS", fmt.Sprintf("tenant with code '%s' already exists", code))
	}

	t := &domain.Tenant{
		Name:   req.Name,
		Code:   code,
		Status: domain.TenantStatusActive,
	}
	return s.repo.Create(ctx, t)
}

func (s *service) GetByID(ctx context.Context, id uuid.UUID) (*domain.Tenant, error) {
	if callerTenantID := common.GetTenantID(ctx); callerTenantID != uuid.Nil && callerTenantID != id {
		return nil, common.ErrNotFound
	}
	return s.repo.GetByID(ctx, id)
}

func (s *service) Update(ctx context.Context, id uuid.UUID, req UpdateRequest) (*domain.Tenant, error) {
	if callerTenantID := common.GetTenantID(ctx); callerTenantID != uuid.Nil && callerTenantID != id {
		return nil, common.ErrNotFound
	}

	t, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	beforeSnapshot := *t
	common.SetAuditBefore(ctx, beforeSnapshot)

	if req.Name != nil {
		t.Name = *req.Name
	}
	if req.Status != nil {
		t.Status = domain.TenantStatus(*req.Status)
	}

	updated, err := s.repo.Update(ctx, t)
	if err != nil {
		return nil, err
	}

	common.SetAuditAfter(ctx, updated)
	return updated, nil
}


func (s *service) List(ctx context.Context, params ListParams) ([]*domain.Tenant, int64, error) {
	return s.repo.List(ctx, params)
}

