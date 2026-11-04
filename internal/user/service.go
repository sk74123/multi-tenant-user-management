package user

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/yourusername/multi-tenant-user-management/internal/common"
	"github.com/yourusername/multi-tenant-user-management/internal/domain"
	"golang.org/x/crypto/bcrypt"
)

// Service defines business operations on users.
type Service interface {
	Create(ctx context.Context, tenantID uuid.UUID, req CreateRequest) (*domain.User, error)
	GetByID(ctx context.Context, tenantID, userID uuid.UUID) (*domain.User, error)
	Update(ctx context.Context, tenantID, userID uuid.UUID, req UpdateRequest) (*domain.User, error)
	SoftDelete(ctx context.Context, tenantID, userID uuid.UUID) error
	List(ctx context.Context, tenantID uuid.UUID, params ListParams) ([]*domain.User, int64, error)
}

// CreateRequest is the validated payload for creating a user.
type CreateRequest struct {
	Email     string `json:"email"      validate:"required,email"`
	Password  string `json:"password"   validate:"required,min=8,max=72"`
	FirstName string `json:"first_name" validate:"required,min=1,max=100"`
	LastName  string `json:"last_name"  validate:"required,min=1,max=100"`
}

// UpdateRequest is the validated payload for updating a user.
type UpdateRequest struct {
	FirstName *string `json:"first_name" validate:"omitempty,min=1,max=100"`
	LastName  *string `json:"last_name"  validate:"omitempty,min=1,max=100"`
	IsActive  *bool   `json:"is_active"`
}

type service struct {
	repo       Repository
	bcryptCost int
}

// NewService creates a new user service.
func NewService(repo Repository, bcryptCost int) Service {
	return &service{repo: repo, bcryptCost: bcryptCost}
}

func (s *service) Create(ctx context.Context, tenantID uuid.UUID, req CreateRequest) (*domain.User, error) {
	// Check email uniqueness globally
	if _, err := s.repo.GetByEmail(ctx, req.Email); err == nil {
		return nil, common.New(http.StatusConflict, "EMAIL_EXISTS", fmt.Sprintf("user with email '%s' already exists", req.Email))
	}

	// Hash the password
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), s.bcryptCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	u := &domain.User{
		Email:        req.Email,
		PasswordHash: string(hash),
		FirstName:    req.FirstName,
		LastName:     req.LastName,
		IsActive:     true,
	}

	created, err := s.repo.Create(ctx, u)
	if err != nil {
		return nil, err
	}

	// Add user to the current tenant
	if err := s.repo.AddToTenant(ctx, tenantID, created.ID); err != nil {
		return nil, err
	}

	return created, nil
}

func (s *service) GetByID(ctx context.Context, tenantID, userID uuid.UUID) (*domain.User, error) {
	// Repository enforces tenant isolation — returns ErrNotFound on cross-tenant access
	return s.repo.GetByIDInTenant(ctx, tenantID, userID)
}

func (s *service) Update(ctx context.Context, tenantID, userID uuid.UUID, req UpdateRequest) (*domain.User, error) {
	// Fetch with tenant isolation check
	u, err := s.repo.GetByIDInTenant(ctx, tenantID, userID)
	if err != nil {
		return nil, err
	}

	beforeSnapshot := *u
	common.SetAuditBefore(ctx, beforeSnapshot)

	if req.FirstName != nil {
		u.FirstName = *req.FirstName
	}
	if req.LastName != nil {
		u.LastName = *req.LastName
	}
	if req.IsActive != nil {
		u.IsActive = *req.IsActive
	}

	updated, err := s.repo.Update(ctx, u)
	if err != nil {
		return nil, err
	}

	common.SetAuditAfter(ctx, updated)
	return updated, nil
}

func (s *service) SoftDelete(ctx context.Context, tenantID, userID uuid.UUID) error {
	if u, err := s.repo.GetByIDInTenant(ctx, tenantID, userID); err == nil {
		common.SetAuditBefore(ctx, u)
	}
	return s.repo.SoftDelete(ctx, tenantID, userID)
}

func (s *service) List(ctx context.Context, tenantID uuid.UUID, params ListParams) ([]*domain.User, int64, error) {
	return s.repo.List(ctx, tenantID, params)
}
