package role

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/yourusername/multi-tenant-user-management/internal/common"
	"github.com/yourusername/multi-tenant-user-management/internal/domain"
)

// Repository defines tenant-scoped data-access operations for roles,
// role-permission mappings, and user-role mappings.
type Repository interface {
	// Role CRUD (tenant-scoped)
	Create(ctx context.Context, role *domain.Role) (*domain.Role, error)
	GetByID(ctx context.Context, tenantID, roleID uuid.UUID) (*domain.Role, error)
	GetByName(ctx context.Context, tenantID uuid.UUID, name string) (*domain.Role, error)
	Update(ctx context.Context, role *domain.Role) (*domain.Role, error)
	Delete(ctx context.Context, tenantID, roleID uuid.UUID) error
	List(ctx context.Context, tenantID uuid.UUID, params ListParams) ([]*domain.Role, int64, error)

	// Role <-> Permission mappings
	AssignPermission(ctx context.Context, roleID, permissionID uuid.UUID) error
	RemovePermission(ctx context.Context, roleID, permissionID uuid.UUID) error
	ListPermissions(ctx context.Context, roleID uuid.UUID) ([]*domain.Permission, error)

	// User <-> Role mappings (tenant-scoped)
	AssignRoleToUser(ctx context.Context, tenantID, userID, roleID uuid.UUID) error
	RemoveRoleFromUser(ctx context.Context, tenantID, userID, roleID uuid.UUID) error
	ListUserRoles(ctx context.Context, tenantID, userID uuid.UUID) ([]*domain.Role, error)
	ListEffectivePermissions(ctx context.Context, tenantID, userID uuid.UUID) ([]*domain.Permission, error)
}

// ListParams holds filter and pagination parameters for listing roles in a tenant.
type ListParams struct {
	Search string
	common.PaginationParams
}

type repository struct {
	db *pgxpool.Pool
}

// NewRepository creates a new role repository backed by pgxpool.
func NewRepository(db *pgxpool.Pool) Repository {
	return &repository{db: db}
}

func (r *repository) Create(ctx context.Context, role *domain.Role) (*domain.Role, error) {
	const q = `
		INSERT INTO roles (tenant_id, name, description)
		VALUES ($1, $2, $3)
		RETURNING id, tenant_id, name, description, created_at, updated_at`

	row := r.db.QueryRow(ctx, q, role.TenantID, role.Name, role.Description)
	created, err := scanRole(row)
	if err != nil {
		return nil, fmt.Errorf("create role: %w", err)
	}
	return created, nil
}

func (r *repository) GetByID(ctx context.Context, tenantID, roleID uuid.UUID) (*domain.Role, error) {
	const q = `
		SELECT id, tenant_id, name, description, created_at, updated_at
		FROM roles
		WHERE tenant_id = $1 AND id = $2`

	row := r.db.QueryRow(ctx, q, tenantID, roleID)
	role, err := scanRole(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, common.ErrNotFound
		}
		return nil, fmt.Errorf("get role by id: %w", err)
	}
	return role, nil
}

func (r *repository) GetByName(ctx context.Context, tenantID uuid.UUID, name string) (*domain.Role, error) {
	const q = `
		SELECT id, tenant_id, name, description, created_at, updated_at
		FROM roles
		WHERE tenant_id = $1 AND name = $2`

	row := r.db.QueryRow(ctx, q, tenantID, name)
	role, err := scanRole(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, common.ErrNotFound
		}
		return nil, fmt.Errorf("get role by name: %w", err)
	}
	return role, nil
}

func (r *repository) Update(ctx context.Context, role *domain.Role) (*domain.Role, error) {
	const q = `
		UPDATE roles
		SET name = $1, description = $2, updated_at = NOW()
		WHERE tenant_id = $3 AND id = $4
		RETURNING id, tenant_id, name, description, created_at, updated_at`

	row := r.db.QueryRow(ctx, q, role.Name, role.Description, role.TenantID, role.ID)
	updated, err := scanRole(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, common.ErrNotFound
		}
		return nil, fmt.Errorf("update role: %w", err)
	}
	return updated, nil
}

func (r *repository) Delete(ctx context.Context, tenantID, roleID uuid.UUID) error {
	const q = `DELETE FROM roles WHERE tenant_id = $1 AND id = $2`

	tag, err := r.db.Exec(ctx, q, tenantID, roleID)
	if err != nil {
		return fmt.Errorf("delete role: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return common.ErrNotFound
	}
	return nil
}

func (r *repository) List(ctx context.Context, tenantID uuid.UUID, params ListParams) ([]*domain.Role, int64, error) {
	args := []any{tenantID}
	where := "WHERE tenant_id = $1"
	i := 2

	if params.Search != "" {
		where += fmt.Sprintf(" AND (name ILIKE '%%' || $%d || '%%' OR description ILIKE '%%' || $%d || '%%')", i, i)
		args = append(args, params.Search)
		i++
	}

	var total int64
	countQ := "SELECT COUNT(*) FROM roles " + where
	if err := r.db.QueryRow(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count roles: %w", err)
	}

	args = append(args, params.PerPage, params.Offset())
	dataQ := fmt.Sprintf(`
		SELECT id, tenant_id, name, description, created_at, updated_at
		FROM roles %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d`, where, i, i+1)

	rows, err := r.db.Query(ctx, dataQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list roles: %w", err)
	}
	defer rows.Close()

	roles := make([]*domain.Role, 0)
	for rows.Next() {
		role, err := scanRole(rows)
		if err != nil {
			return nil, 0, err
		}
		roles = append(roles, role)
	}
	return roles, total, rows.Err()
}

func (r *repository) AssignPermission(ctx context.Context, roleID, permissionID uuid.UUID) error {
	const q = `
		INSERT INTO role_permissions (role_id, permission_id)
		VALUES ($1, $2)
		ON CONFLICT (role_id, permission_id) DO NOTHING`

	if _, err := r.db.Exec(ctx, q, roleID, permissionID); err != nil {
		return fmt.Errorf("assign permission to role: %w", err)
	}
	return nil
}

func (r *repository) RemovePermission(ctx context.Context, roleID, permissionID uuid.UUID) error {
	const q = `DELETE FROM role_permissions WHERE role_id = $1 AND permission_id = $2`

	tag, err := r.db.Exec(ctx, q, roleID, permissionID)
	if err != nil {
		return fmt.Errorf("remove permission from role: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return common.ErrNotFound
	}
	return nil
}

func (r *repository) ListPermissions(ctx context.Context, roleID uuid.UUID) ([]*domain.Permission, error) {
	const q = `
		SELECT p.id, p.resource, p.action, p.description, p.created_at
		FROM permissions p
		INNER JOIN role_permissions rp ON rp.permission_id = p.id
		WHERE rp.role_id = $1
		ORDER BY p.resource ASC, p.action ASC`

	rows, err := r.db.Query(ctx, q, roleID)
	if err != nil {
		return nil, fmt.Errorf("list role permissions: %w", err)
	}
	defer rows.Close()

	perms := make([]*domain.Permission, 0)
	for rows.Next() {
		p, err := scanPermission(rows)
		if err != nil {
			return nil, err
		}
		perms = append(perms, p)
	}
	return perms, rows.Err()
}

func (r *repository) AssignRoleToUser(ctx context.Context, tenantID, userID, roleID uuid.UUID) error {
	const q = `
		INSERT INTO user_roles (tenant_id, user_id, role_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (tenant_id, user_id, role_id) DO NOTHING`

	if _, err := r.db.Exec(ctx, q, tenantID, userID, roleID); err != nil {
		return fmt.Errorf("assign role to user: %w", err)
	}
	return nil
}

func (r *repository) RemoveRoleFromUser(ctx context.Context, tenantID, userID, roleID uuid.UUID) error {
	const q = `
		DELETE FROM user_roles
		WHERE tenant_id = $1 AND user_id = $2 AND role_id = $3`

	tag, err := r.db.Exec(ctx, q, tenantID, userID, roleID)
	if err != nil {
		return fmt.Errorf("remove role from user: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return common.ErrNotFound
	}
	return nil
}

func (r *repository) ListUserRoles(ctx context.Context, tenantID, userID uuid.UUID) ([]*domain.Role, error) {
	const q = `
		SELECT r.id, r.tenant_id, r.name, r.description, r.created_at, r.updated_at
		FROM roles r
		INNER JOIN user_roles ur ON ur.role_id = r.id
		WHERE ur.tenant_id = $1 AND ur.user_id = $2 AND r.tenant_id = $1
		ORDER BY r.name ASC`

	rows, err := r.db.Query(ctx, q, tenantID, userID)
	if err != nil {
		return nil, fmt.Errorf("list user roles: %w", err)
	}
	defer rows.Close()

	roles := make([]*domain.Role, 0)
	for rows.Next() {
		role, err := scanRole(rows)
		if err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}
	return roles, rows.Err()
}

func (r *repository) ListEffectivePermissions(ctx context.Context, tenantID, userID uuid.UUID) ([]*domain.Permission, error) {
	const q = `
		SELECT DISTINCT p.id, p.resource, p.action, p.description, p.created_at
		FROM permissions p
		INNER JOIN role_permissions rp ON rp.permission_id = p.id
		INNER JOIN roles r ON r.id = rp.role_id
		INNER JOIN user_roles ur ON ur.role_id = r.id
		WHERE ur.tenant_id = $1
		  AND ur.user_id = $2
		  AND r.tenant_id = $1
		ORDER BY p.resource ASC, p.action ASC`

	rows, err := r.db.Query(ctx, q, tenantID, userID)
	if err != nil {
		return nil, fmt.Errorf("list effective permissions: %w", err)
	}
	defer rows.Close()

	perms := make([]*domain.Permission, 0)
	for rows.Next() {
		p, err := scanPermission(rows)
		if err != nil {
			return nil, err
		}
		perms = append(perms, p)
	}
	return perms, rows.Err()
}

type scannable interface {
	Scan(dest ...any) error
}

func scanRole(row scannable) (*domain.Role, error) {
	var role domain.Role
	if err := row.Scan(&role.ID, &role.TenantID, &role.Name, &role.Description, &role.CreatedAt, &role.UpdatedAt); err != nil {
		return nil, err
	}
	return &role, nil
}

func scanPermission(row scannable) (*domain.Permission, error) {
	var p domain.Permission
	if err := row.Scan(&p.ID, &p.Resource, &p.Action, &p.Description, &p.CreatedAt); err != nil {
		return nil, err
	}
	return &p, nil
}

