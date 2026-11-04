package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yourusername/multi-tenant-user-management/internal/common"
	"github.com/yourusername/multi-tenant-user-management/internal/domain"
	"github.com/yourusername/multi-tenant-user-management/internal/tenant"
	"github.com/yourusername/multi-tenant-user-management/internal/user"
	"golang.org/x/crypto/bcrypt"
)

// Service defines authentication and token lifecycle operations.
type Service interface {
	Register(ctx context.Context, req RegisterRequest) (*AuthResponse, error)
	Login(ctx context.Context, req LoginRequest) (*AuthResponse, error)
	Refresh(ctx context.Context, req RefreshRequest) (*AuthResponse, error)
	Logout(ctx context.Context, req LogoutRequest) error
}

// RegisterRequest is the validated payload for registering a user into a tenant.
type RegisterRequest struct {
	Email      string     `json:"email"       validate:"required,email"`
	Password   string     `json:"password"    validate:"required,min=8,max=72"`
	FirstName  string     `json:"first_name"  validate:"required,min=1,max=100"`
	LastName   string     `json:"last_name"   validate:"required,min=1,max=100"`
	TenantID   *uuid.UUID `json:"tenant_id"`
	TenantCode string     `json:"tenant_code"`
}

// LoginRequest is the validated payload for authenticating a user into a tenant.
type LoginRequest struct {
	Email      string     `json:"email"       validate:"required,email"`
	Password   string     `json:"password"    validate:"required"`
	TenantID   *uuid.UUID `json:"tenant_id"`
	TenantCode string     `json:"tenant_code"`
}

// RefreshRequest is the validated payload for rotating a refresh token.
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

// LogoutRequest is the payload for revoking a refresh token.
type LogoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// AuthResponse is returned upon successful registration, login, or token refresh.
type AuthResponse struct {
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	TokenType    string       `json:"token_type"`
	ExpiresIn    int64        `json:"expires_in"`
	TenantID     uuid.UUID    `json:"tenant_id"`
	User         *domain.User `json:"user"`
}

type service struct {
	repo       Repository
	userRepo   user.Repository
	tenantRepo tenant.Repository
	jwtManager *JWTManager
	bcryptCost int
}

// NewService creates a new authentication service.
func NewService(
	repo Repository,
	userRepo user.Repository,
	tenantRepo tenant.Repository,
	jwtManager *JWTManager,
	bcryptCost int,
) Service {
	if bcryptCost <= 0 {
		bcryptCost = bcrypt.DefaultCost
	}
	return &service{
		repo:       repo,
		userRepo:   userRepo,
		tenantRepo: tenantRepo,
		jwtManager: jwtManager,
		bcryptCost: bcryptCost,
	}
}

func (s *service) Register(ctx context.Context, req RegisterRequest) (*AuthResponse, error) {
	t, err := s.resolveTenant(ctx, req.TenantID, req.TenantCode)
	if err != nil {
		return nil, err
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))

	existingUser, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil && !errors.Is(err, common.ErrNotFound) {
		return nil, err
	}

	var u *domain.User
	if existingUser != nil {
		inTenant, err := s.userRepo.IsInTenant(ctx, t.ID, existingUser.ID)
		if err != nil {
			return nil, err
		}
		if inTenant {
			return nil, common.New(http.StatusConflict, "EMAIL_EXISTS", fmt.Sprintf("user with email '%s' already exists in tenant", email))
		}

		if err := bcrypt.CompareHashAndPassword([]byte(existingUser.PasswordHash), []byte(req.Password)); err != nil {
			return nil, common.New(http.StatusConflict, "EMAIL_EXISTS", fmt.Sprintf("user with email '%s' already exists", email))
		}

		if err := s.userRepo.AddToTenant(ctx, t.ID, existingUser.ID); err != nil {
			return nil, err
		}
		u = existingUser
	} else {
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), s.bcryptCost)
		if err != nil {
			return nil, fmt.Errorf("hash password: %w", err)
		}

		newUser := &domain.User{
			Email:        email,
			PasswordHash: string(hash),
			FirstName:    strings.TrimSpace(req.FirstName),
			LastName:     strings.TrimSpace(req.LastName),
			IsActive:     true,
		}

		created, err := s.userRepo.Create(ctx, newUser)
		if err != nil {
			return nil, err
		}

		if err := s.userRepo.AddToTenant(ctx, t.ID, created.ID); err != nil {
			return nil, err
		}
		u = created
	}

	if meta := common.GetAuditMeta(ctx); meta != nil {
		meta.TenantID = t.ID
		meta.ActorID = &u.ID
		meta.TargetID = &u.ID
		meta.TargetType = "user"
	}

	return s.issueTokens(ctx, u, t.ID)
}

func (s *service) Login(ctx context.Context, req LoginRequest) (*AuthResponse, error) {
	t, err := s.resolveTenant(ctx, req.TenantID, req.TenantCode)
	if err != nil {
		return nil, err
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	invalidCredsErr := common.New(http.StatusUnauthorized, "INVALID_CREDENTIALS", "invalid email or password")

	u, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			return nil, invalidCredsErr
		}
		return nil, err
	}

	if !u.IsActive || u.DeletedAt != nil {
		return nil, invalidCredsErr
	}

	inTenant, err := s.userRepo.IsInTenant(ctx, t.ID, u.ID)
	if err != nil {
		return nil, err
	}
	if !inTenant {
		return nil, invalidCredsErr
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.Password)); err != nil {
		return nil, invalidCredsErr
	}

	if meta := common.GetAuditMeta(ctx); meta != nil {
		meta.TenantID = t.ID
		meta.ActorID = &u.ID
		meta.TargetID = &u.ID
		meta.TargetType = "user"
	}

	return s.issueTokens(ctx, u, t.ID)
}

func (s *service) Refresh(ctx context.Context, req RefreshRequest) (*AuthResponse, error) {
	rawToken := strings.TrimSpace(req.RefreshToken)
	if rawToken == "" {
		return nil, common.New(http.StatusUnauthorized, "INVALID_REFRESH_TOKEN", "refresh token is required")
	}

	invalidTokenErr := common.New(http.StatusUnauthorized, "INVALID_REFRESH_TOKEN", "invalid or expired refresh token")

	claims, err := s.jwtManager.ValidateRefreshToken(rawToken)
	if err != nil {
		return nil, invalidTokenErr
	}

	tokenHash := HashToken(rawToken)
	stored, err := s.repo.GetRefreshTokenByHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			return nil, invalidTokenErr
		}
		return nil, err
	}

	if stored.Revoked || time.Now().UTC().After(stored.ExpiresAt) {
		return nil, invalidTokenErr
	}

	if stored.UserID != claims.UserID || stored.TenantID != claims.TenantID {
		return nil, invalidTokenErr
	}

	t, err := s.tenantRepo.GetByID(ctx, stored.TenantID)
	if err != nil {
		return nil, invalidTokenErr
	}
	if t.Status != domain.TenantStatusActive {
		return nil, common.New(http.StatusForbidden, "TENANT_SUSPENDED", "tenant is suspended")
	}

	u, err := s.userRepo.GetByIDInTenant(ctx, stored.TenantID, stored.UserID)
	if err != nil {
		return nil, invalidTokenErr
	}
	if !u.IsActive {
		return nil, invalidTokenErr
	}

	if err := s.repo.RevokeRefreshToken(ctx, tokenHash); err != nil {
		return nil, invalidTokenErr
	}

	return s.issueTokens(ctx, u, stored.TenantID)
}

func (s *service) Logout(ctx context.Context, req LogoutRequest) error {
	rawToken := strings.TrimSpace(req.RefreshToken)
	if rawToken != "" {
		tokenHash := HashToken(rawToken)
		err := s.repo.RevokeRefreshToken(ctx, tokenHash)
		if err != nil && !errors.Is(err, common.ErrNotFound) {
			return err
		}
		return nil
	}

	tenantID := common.GetTenantID(ctx)
	userID := common.GetUserID(ctx)
	if tenantID == uuid.Nil || userID == uuid.Nil {
		return common.ErrUnauthorized
	}

	return s.repo.RevokeAllUserRefreshTokens(ctx, tenantID, userID)
}

func (s *service) resolveTenant(ctx context.Context, tenantID *uuid.UUID, tenantCode string) (*domain.Tenant, error) {
	var t *domain.Tenant
	var err error

	switch {
	case tenantID != nil && *tenantID != uuid.Nil:
		t, err = s.tenantRepo.GetByID(ctx, *tenantID)
	case strings.TrimSpace(tenantCode) != "":
		code := strings.ToLower(strings.TrimSpace(tenantCode))
		t, err = s.tenantRepo.GetByCode(ctx, code)
	default:
		return nil, common.New(http.StatusUnprocessableEntity, "VALIDATION_ERROR", "tenant_id or tenant_code is required")
	}

	if err != nil {
		return nil, err
	}

	if t.Status != domain.TenantStatusActive {
		return nil, common.New(http.StatusForbidden, "TENANT_SUSPENDED", "tenant is suspended")
	}

	return t, nil
}

func (s *service) issueTokens(ctx context.Context, u *domain.User, tenantID uuid.UUID) (*AuthResponse, error) {
	accessToken, _, err := s.jwtManager.GenerateAccessToken(u.ID, tenantID, u.Email)
	if err != nil {
		return nil, err
	}

	refreshToken, refreshExpiresAt, err := s.jwtManager.GenerateRefreshToken(u.ID, tenantID)
	if err != nil {
		return nil, err
	}

	rt := &domain.RefreshToken{
		UserID:    u.ID,
		TenantID:  tenantID,
		TokenHash: HashToken(refreshToken),
		ExpiresAt: refreshExpiresAt,
		Revoked:   false,
	}

	if _, err := s.repo.CreateRefreshToken(ctx, rt); err != nil {
		return nil, err
	}

	return &AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    s.jwtManager.AccessExpirySeconds(),
		TenantID:     tenantID,
		User:         u,
	}, nil
}
