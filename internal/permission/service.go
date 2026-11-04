package permission

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/yourusername/multi-tenant-user-management/internal/common"
	"github.com/yourusername/multi-tenant-user-management/internal/domain"
)

// Service defines business operations for global permissions.
type Service interface {
	Create(ctx context.Context, req CreateRequest) (*domain.Permission, error)
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Permission, error)
	List(ctx context.Context, params ListParams) ([]*domain.Permission, int64, error)
}

// CreateRequest is the validated payload for creating a permission.
type CreateRequest struct {
	Resource    string `json:"resource"    validate:"required,min=2,max=100"`
	Action      string `json:"action"      validate:"required,min=2,max=100"`
	Description string `json:"description" validate:"max=500"`
}

type service struct {
	repo Repository
}

// NewService creates a new permission service.
func NewService(repo Repository) Service {
	return &service{repo: repo}
}

func (s *service) Create(ctx context.Context, req CreateRequest) (*domain.Permission, error) {
	resource := strings.ToLower(strings.TrimSpace(req.Resource))
	action := strings.ToLower(strings.TrimSpace(req.Action))

	if _, err := s.repo.GetByResourceAction(ctx, resource, action); err == nil {
		return nil, common.New(
			http.StatusConflict,
			"PERMISSION_EXISTS",
			fmt.Sprintf("permission '%s:%s' already exists", resource, action),
		)
	}

	p := &domain.Permission{
		Resource:    resource,
		Action:      action,
		Description: strings.TrimSpace(req.Description),
	}
	return s.repo.Create(ctx, p)
}

func (s *service) GetByID(ctx context.Context, id uuid.UUID) (*domain.Permission, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *service) List(ctx context.Context, params ListParams) ([]*domain.Permission, int64, error) {
	return s.repo.List(ctx, params)
}
