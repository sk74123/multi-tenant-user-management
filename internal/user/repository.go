package user

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

// Repository defines data-access methods for users.
type Repository interface {
	// Global lookups (not tenant-scoped)
	GetByEmail(ctx context.Context, email string) (*domain.User, error)
	Create(ctx context.Context, u *domain.User) (*domain.User, error)

	// Tenant-scoped operations
	GetByIDInTenant(ctx context.Context, tenantID, userID uuid.UUID) (*domain.User, error)
	Update(ctx context.Context, u *domain.User) (*domain.User, error)
	SoftDelete(ctx context.Context, tenantID, userID uuid.UUID) error
	List(ctx context.Context, tenantID uuid.UUID, params ListParams) ([]*domain.User, int64, error)

	// Membership
	AddToTenant(ctx context.Context, tenantID, userID uuid.UUID) error
	IsInTenant(ctx context.Context, tenantID, userID uuid.UUID) (bool, error)
}

// ListParams holds filters for listing users within a tenant.
type ListParams struct {
	Search   string
	IsActive *bool
	common.PaginationParams
}

type repository struct {
	db *pgxpool.Pool
}

// NewRepository creates a new user repository.
func NewRepository(db *pgxpool.Pool) Repository {
	return &repository{db: db}
}

func (r *repository) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	const q = `
		SELECT id, email, password_hash, first_name, last_name, is_active, deleted_at, created_at, updated_at
		FROM users WHERE email = $1 AND deleted_at IS NULL`

	row := r.db.QueryRow(ctx, q, email)
	u, err := scanUser(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, common.ErrNotFound
		}
		return nil, fmt.Errorf("get user by email: %w", err)
	}
	return u, nil
}

func (r *repository) Create(ctx context.Context, u *domain.User) (*domain.User, error) {
	const q = `
		INSERT INTO users (email, password_hash, first_name, last_name, is_active)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, email, password_hash, first_name, last_name, is_active, deleted_at, created_at, updated_at`

	row := r.db.QueryRow(ctx, q, u.Email, u.PasswordHash, u.FirstName, u.LastName, u.IsActive)
	created, err := scanUser(row)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	return created, nil
}

func (r *repository) GetByIDInTenant(ctx context.Context, tenantID, userID uuid.UUID) (*domain.User, error) {
	// Join through tenant_users to enforce tenant isolation
	const q = `
		SELECT u.id, u.email, u.password_hash, u.first_name, u.last_name,
		       u.is_active, u.deleted_at, u.created_at, u.updated_at
		FROM users u
		INNER JOIN tenant_users tu ON tu.user_id = u.id
		WHERE tu.tenant_id = $1
		  AND u.id = $2
		  AND u.deleted_at IS NULL
		  AND tu.is_active = TRUE`

	row := r.db.QueryRow(ctx, q, tenantID, userID)
	u, err := scanUser(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, common.ErrNotFound // 404 — also covers cross-tenant access
		}
		return nil, fmt.Errorf("get user by id in tenant: %w", err)
	}
	return u, nil
}

func (r *repository) Update(ctx context.Context, u *domain.User) (*domain.User, error) {
	const q = `
		UPDATE users
		SET first_name = $1, last_name = $2, is_active = $3, updated_at = NOW()
		WHERE id = $4 AND deleted_at IS NULL
		RETURNING id, email, password_hash, first_name, last_name, is_active, deleted_at, created_at, updated_at`

	row := r.db.QueryRow(ctx, q, u.FirstName, u.LastName, u.IsActive, u.ID)
	updated, err := scanUser(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, common.ErrNotFound
		}
		return nil, fmt.Errorf("update user: %w", err)
	}
	return updated, nil
}

func (r *repository) SoftDelete(ctx context.Context, tenantID, userID uuid.UUID) error {
	// Verify user is in this tenant before deleting
	inTenant, err := r.IsInTenant(ctx, tenantID, userID)
	if err != nil {
		return err
	}
	if !inTenant {
		return common.ErrNotFound
	}

	const q = `UPDATE users SET deleted_at = NOW(), updated_at = NOW() WHERE id = $1 AND deleted_at IS NULL`
	tag, err := r.db.Exec(ctx, q, userID)
	if err != nil {
		return fmt.Errorf("soft delete user: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return common.ErrNotFound
	}
	return nil
}

func (r *repository) List(ctx context.Context, tenantID uuid.UUID, params ListParams) ([]*domain.User, int64, error) {
	args := []any{tenantID}
	where := `
		FROM users u
		INNER JOIN tenant_users tu ON tu.user_id = u.id
		WHERE tu.tenant_id = $1
		  AND u.deleted_at IS NULL
		  AND tu.is_active = TRUE`
	i := 2

	if params.Search != "" {
		where += fmt.Sprintf(` AND (u.email ILIKE '%%' || $%d || '%%'
			OR u.first_name ILIKE '%%' || $%d || '%%'
			OR u.last_name  ILIKE '%%' || $%d || '%%')`, i, i, i)
		args = append(args, params.Search)
		i++
	}
	if params.IsActive != nil {
		where += fmt.Sprintf(" AND u.is_active = $%d", i)
		args = append(args, *params.IsActive)
		i++
	}

	// Count
	var total int64
	if err := r.db.QueryRow(ctx, "SELECT COUNT(*) "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count users: %w", err)
	}

	// Data
	args = append(args, params.PerPage, params.Offset())
	dataQ := fmt.Sprintf(`
		SELECT u.id, u.email, u.password_hash, u.first_name, u.last_name,
		       u.is_active, u.deleted_at, u.created_at, u.updated_at
		%s
		ORDER BY u.created_at DESC
		LIMIT $%d OFFSET $%d`, where, i, i+1)

	rows, err := r.db.Query(ctx, dataQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	users := make([]*domain.User, 0)
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, 0, err
		}
		users = append(users, u)
	}
	return users, total, rows.Err()
}

func (r *repository) AddToTenant(ctx context.Context, tenantID, userID uuid.UUID) error {
	const q = `
		INSERT INTO tenant_users (tenant_id, user_id)
		VALUES ($1, $2)
		ON CONFLICT (tenant_id, user_id) DO UPDATE SET is_active = TRUE`

	_, err := r.db.Exec(ctx, q, tenantID, userID)
	if err != nil {
		return fmt.Errorf("add user to tenant: %w", err)
	}
	return nil
}

func (r *repository) IsInTenant(ctx context.Context, tenantID, userID uuid.UUID) (bool, error) {
	const q = `
		SELECT EXISTS(
			SELECT 1 FROM tenant_users
			WHERE tenant_id = $1 AND user_id = $2 AND is_active = TRUE
		)`

	var exists bool
	if err := r.db.QueryRow(ctx, q, tenantID, userID).Scan(&exists); err != nil {
		return false, fmt.Errorf("check tenant membership: %w", err)
	}
	return exists, nil
}

type scannable interface {
	Scan(dest ...any) error
}

func scanUser(row scannable) (*domain.User, error) {
	var u domain.User
	err := row.Scan(
		&u.ID, &u.Email, &u.PasswordHash,
		&u.FirstName, &u.LastName, &u.IsActive,
		&u.DeletedAt, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

