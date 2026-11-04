package role

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/yourusername/multi-tenant-user-management/internal/common"
	"github.com/yourusername/multi-tenant-user-management/internal/domain"
	"github.com/yourusername/multi-tenant-user-management/internal/permission"
	"github.com/yourusername/multi-tenant-user-management/internal/user"
)

// Service defines business operations for tenant-scoped roles,
// role-permission assignments, and user-role assignments.
type Service interface {
	// Role CRUD
	Create(ctx context.Context, tenantID uuid.UUID, req CreateRequest) (*domain.Role, error)
	GetByID(ctx context.Context, tenantID, roleID uuid.UUID) (*domain.Role, error)
	Update(ctx context.Context, tenantID, roleID uuid.UUID, req UpdateRequest) (*domain.Role, error)
	Delete(ctx context.Context, tenantID, roleID uuid.UUID) error
	List(ctx context.Context, tenantID uuid.UUID, params ListParams) ([]*domain.Role, int64, error)

	// Role <-> Permission
	AssignPermissions(ctx context.Context, tenantID, roleID uuid.UUID, req AssignPermissionsRequest) ([]*domain.Permission, error)
	RemovePermission(ctx context.Context, tenantID, roleID, permissionID uuid.UUID) error
	ListPermissions(ctx context.Context, tenantID, roleID uuid.UUID) ([]*domain.Permission, error)

	// User <-> Role & Effective Permissions
	AssignRolesToUser(ctx context.Context, tenantID, userID uuid.UUID, req AssignUserRolesRequest) ([]*domain.Role, error)
	RemoveRoleFromUser(ctx context.Context, tenantID, userID, roleID uuid.UUID) error
	ListUserRoles(ctx context.Context, tenantID, userID uuid.UUID) ([]*domain.Role, error)
	GetEffectivePermissions(ctx context.Context, tenantID, userID uuid.UUID) ([]*domain.Permission, error)
}

// CreateRequest is the validated payload for creating a role in a tenant.
type CreateRequest struct {
	Name        string `json:"name"        validate:"required,min=2,max=100"`
	Description string `json:"description" validate:"max=500"`
}

// UpdateRequest is the validated payload for updating a role in a tenant.
type UpdateRequest struct {
	Name        *string `json:"name"        validate:"omitempty,min=2,max=100"`
	Description *string `json:"description" validate:"omitempty,max=500"`
}

// AssignPermissionsRequest supports either a single permission_id or a list of permission_ids.
type AssignPermissionsRequest struct {
	PermissionID  *uuid.UUID  `json:"permission_id"`
	PermissionIDs []uuid.UUID `json:"permission_ids"`
}

// IDs returns the deduplicated list of non-nil permission UUIDs in the request.
func (r AssignPermissionsRequest) IDs() []uuid.UUID {
	seen := make(map[uuid.UUID]struct{})
	out := make([]uuid.UUID, 0, len(r.PermissionIDs)+1)
	if r.PermissionID != nil && *r.PermissionID != uuid.Nil {
		seen[*r.PermissionID] = struct{}{}
		out = append(out, *r.PermissionID)
	}
	for _, id := range r.PermissionIDs {
		if id != uuid.Nil {
			if _, ok := seen[id]; !ok {
				seen[id] = struct{}{}
				out = append(out, id)
			}
		}
	}
	return out
}

// AssignUserRolesRequest supports either a single role_id or a list of role_ids.
type AssignUserRolesRequest struct {
	RoleID  *uuid.UUID  `json:"role_id"`
	RoleIDs []uuid.UUID `json:"role_ids"`
}

// IDs returns the deduplicated list of non-nil role UUIDs in the request.
func (r AssignUserRolesRequest) IDs() []uuid.UUID {
	seen := make(map[uuid.UUID]struct{})
	out := make([]uuid.UUID, 0, len(r.RoleIDs)+1)
	if r.RoleID != nil && *r.RoleID != uuid.Nil {
		seen[*r.RoleID] = struct{}{}
		out = append(out, *r.RoleID)
	}
	for _, id := range r.RoleIDs {
		if id != uuid.Nil {
			if _, ok := seen[id]; !ok {
				seen[id] = struct{}{}
				out = append(out, id)
			}
		}
	}
	return out
}

// PermissionCacheInvalidator invalidates cached permissions when role or user-role mappings change.
type PermissionCacheInvalidator interface {
	InvalidateUser(tenantID, userID uuid.UUID)
	InvalidateTenant(tenantID uuid.UUID)
}

type service struct {
	repo     Repository
	permRepo permission.Repository
	userRepo user.Repository
	cacheInv PermissionCacheInvalidator
}

// NewService creates a new role service.
func NewService(repo Repository, permRepo permission.Repository, userRepo user.Repository, cacheInv ...PermissionCacheInvalidator) Service {
	var inv PermissionCacheInvalidator
	if len(cacheInv) > 0 {
		inv = cacheInv[0]
	}
	return &service{
		repo:     repo,
		permRepo: permRepo,
		userRepo: userRepo,
		cacheInv: inv,
	}
}


func (s *service) Create(ctx context.Context, tenantID uuid.UUID, req CreateRequest) (*domain.Role, error) {
	name := strings.ToLower(strings.TrimSpace(req.Name))

	if _, err := s.repo.GetByName(ctx, tenantID, name); err == nil {
		return nil, common.New(
			http.StatusConflict,
			"ROLE_EXISTS",
			fmt.Sprintf("role '%s' already exists in this tenant", name),
		)
	}

	role := &domain.Role{
		TenantID:    tenantID,
		Name:        name,
		Description: strings.TrimSpace(req.Description),
	}
	return s.repo.Create(ctx, role)
}

func (s *service) GetByID(ctx context.Context, tenantID, roleID uuid.UUID) (*domain.Role, error) {
	return s.repo.GetByID(ctx, tenantID, roleID)
}

func (s *service) Update(ctx context.Context, tenantID, roleID uuid.UUID, req UpdateRequest) (*domain.Role, error) {
	role, err := s.repo.GetByID(ctx, tenantID, roleID)
	if err != nil {
		return nil, err
	}

	beforeSnapshot := *role
	common.SetAuditBefore(ctx, beforeSnapshot)

	if req.Name != nil {
		newName := strings.ToLower(strings.TrimSpace(*req.Name))
		if newName != role.Name {
			if existing, err := s.repo.GetByName(ctx, tenantID, newName); err == nil && existing.ID != role.ID {
				return nil, common.New(
					http.StatusConflict,
					"ROLE_EXISTS",
					fmt.Sprintf("role '%s' already exists in this tenant", newName),
				)
			}
			role.Name = newName
		}
	}

	if req.Description != nil {
		role.Description = strings.TrimSpace(*req.Description)
	}

	updated, err := s.repo.Update(ctx, role)
	if err != nil {
		return nil, err
	}

	common.SetAuditAfter(ctx, updated)
	return updated, nil
}

func (s *service) Delete(ctx context.Context, tenantID, roleID uuid.UUID) error {
	if existing, err := s.repo.GetByID(ctx, tenantID, roleID); err == nil {
		common.SetAuditBefore(ctx, existing)
	}
	if err := s.repo.Delete(ctx, tenantID, roleID); err != nil {
		return err
	}
	if s.cacheInv != nil {
		s.cacheInv.InvalidateTenant(tenantID)
	}
	return nil
}

func (s *service) List(ctx context.Context, tenantID uuid.UUID, params ListParams) ([]*domain.Role, int64, error) {
	return s.repo.List(ctx, tenantID, params)
}

func (s *service) AssignPermissions(ctx context.Context, tenantID, roleID uuid.UUID, req AssignPermissionsRequest) ([]*domain.Permission, error) {
	permIDs := req.IDs()
	if len(permIDs) == 0 {
		return nil, common.New(http.StatusUnprocessableEntity, "VALIDATION_ERROR", "permission_id or permission_ids is required")
	}

	// Verify role belongs to tenant (enforces 404 on cross-tenant access)
	if _, err := s.repo.GetByID(ctx, tenantID, roleID); err != nil {
		return nil, err
	}

	if beforePerms, err := s.repo.ListPermissions(ctx, roleID); err == nil {
		common.SetAuditBefore(ctx, beforePerms)
	}

	// Verify all permissions exist before assigning
	for _, pid := range permIDs {
		if _, err := s.permRepo.GetByID(ctx, pid); err != nil {
			return nil, err
		}
	}

	for _, pid := range permIDs {
		if err := s.repo.AssignPermission(ctx, roleID, pid); err != nil {
			return nil, err
		}
	}

	if s.cacheInv != nil {
		s.cacheInv.InvalidateTenant(tenantID)
	}

	afterPerms, err := s.repo.ListPermissions(ctx, roleID)
	if err != nil {
		return nil, err
	}
	common.SetAuditAfter(ctx, afterPerms)
	return afterPerms, nil
}

func (s *service) RemovePermission(ctx context.Context, tenantID, roleID, permissionID uuid.UUID) error {
	// Verify role belongs to tenant (enforces 404 on cross-tenant access)
	if _, err := s.repo.GetByID(ctx, tenantID, roleID); err != nil {
		return err
	}

	if beforePerms, err := s.repo.ListPermissions(ctx, roleID); err == nil {
		common.SetAuditBefore(ctx, beforePerms)
	}

	if err := s.repo.RemovePermission(ctx, roleID, permissionID); err != nil {
		return err
	}

	if s.cacheInv != nil {
		s.cacheInv.InvalidateTenant(tenantID)
	}
	if afterPerms, err := s.repo.ListPermissions(ctx, roleID); err == nil {
		common.SetAuditAfter(ctx, afterPerms)
	}
	return nil
}

func (s *service) ListPermissions(ctx context.Context, tenantID, roleID uuid.UUID) ([]*domain.Permission, error) {
	// Verify role belongs to tenant (enforces 404 on cross-tenant access)
	if _, err := s.repo.GetByID(ctx, tenantID, roleID); err != nil {
		return nil, err
	}

	return s.repo.ListPermissions(ctx, roleID)
}

func (s *service) AssignRolesToUser(ctx context.Context, tenantID, userID uuid.UUID, req AssignUserRolesRequest) ([]*domain.Role, error) {
	roleIDs := req.IDs()
	if len(roleIDs) == 0 {
		return nil, common.New(http.StatusUnprocessableEntity, "VALIDATION_ERROR", "role_id or role_ids is required")
	}

	// Verify user belongs to tenant and is active/non-deleted (enforces 404 on cross-tenant access)
	if _, err := s.userRepo.GetByIDInTenant(ctx, tenantID, userID); err != nil {
		return nil, err
	}

	if beforeRoles, err := s.repo.ListUserRoles(ctx, tenantID, userID); err == nil {
		common.SetAuditBefore(ctx, beforeRoles)
	}

	// Verify each role belongs to the same tenant (enforces 404 on cross-tenant role assignment)
	for _, rid := range roleIDs {
		if _, err := s.repo.GetByID(ctx, tenantID, rid); err != nil {
			return nil, err
		}
	}

	for _, rid := range roleIDs {
		if err := s.repo.AssignRoleToUser(ctx, tenantID, userID, rid); err != nil {
			return nil, err
		}
	}

	if s.cacheInv != nil {
		s.cacheInv.InvalidateUser(tenantID, userID)
	}

	afterRoles, err := s.repo.ListUserRoles(ctx, tenantID, userID)
	if err != nil {
		return nil, err
	}
	common.SetAuditAfter(ctx, afterRoles)
	return afterRoles, nil
}

func (s *service) RemoveRoleFromUser(ctx context.Context, tenantID, userID, roleID uuid.UUID) error {
	// Verify user belongs to tenant (enforces 404 on cross-tenant access)
	if _, err := s.userRepo.GetByIDInTenant(ctx, tenantID, userID); err != nil {
		return err
	}

	if beforeRoles, err := s.repo.ListUserRoles(ctx, tenantID, userID); err == nil {
		common.SetAuditBefore(ctx, beforeRoles)
	}

	if err := s.repo.RemoveRoleFromUser(ctx, tenantID, userID, roleID); err != nil {
		return err
	}

	if s.cacheInv != nil {
		s.cacheInv.InvalidateUser(tenantID, userID)
	}
	if afterRoles, err := s.repo.ListUserRoles(ctx, tenantID, userID); err == nil {
		common.SetAuditAfter(ctx, afterRoles)
	}
	return nil
}

func (s *service) ListUserRoles(ctx context.Context, tenantID, userID uuid.UUID) ([]*domain.Role, error) {
	// Verify user belongs to tenant (enforces 404 on cross-tenant access)
	if _, err := s.userRepo.GetByIDInTenant(ctx, tenantID, userID); err != nil {
		return nil, err
	}

	return s.repo.ListUserRoles(ctx, tenantID, userID)
}

func (s *service) GetEffectivePermissions(ctx context.Context, tenantID, userID uuid.UUID) ([]*domain.Permission, error) {
	// Verify user belongs to tenant (enforces 404 on cross-tenant access)
	if _, err := s.userRepo.GetByIDInTenant(ctx, tenantID, userID); err != nil {
		return nil, err
	}

	return s.repo.ListEffectivePermissions(ctx, tenantID, userID)
}

