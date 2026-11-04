package permission

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

// Repository defines data-access operations for global permissions.
type Repository interface {
	Create(ctx context.Context, p *domain.Permission) (*domain.Permission, error)
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Permission, error)
	GetByResourceAction(ctx context.Context, resource, action string) (*domain.Permission, error)
	List(ctx context.Context, params ListParams) ([]*domain.Permission, int64, error)
}

// ListParams holds filter and pagination parameters for listing permissions.
type ListParams struct {
	Resource string
	Search   string
	common.PaginationParams
}

type repository struct {
	db *pgxpool.Pool
}

// NewRepository creates a new permission repository backed by pgxpool.
func NewRepository(db *pgxpool.Pool) Repository {
	return &repository{db: db}
}

func (r *repository) Create(ctx context.Context, p *domain.Permission) (*domain.Permission, error) {
	const q = `
		INSERT INTO permissions (resource, action, description)
		VALUES ($1, $2, $3)
		RETURNING id, resource, action, description, created_at`

	row := r.db.QueryRow(ctx, q, p.Resource, p.Action, p.Description)
	created, err := scanPermission(row)
	if err != nil {
		return nil, fmt.Errorf("create permission: %w", err)
	}
	return created, nil
}

func (r *repository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Permission, error) {
	const q = `
		SELECT id, resource, action, description, created_at
		FROM permissions
		WHERE id = $1`

	row := r.db.QueryRow(ctx, q, id)
	p, err := scanPermission(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, common.ErrNotFound
		}
		return nil, fmt.Errorf("get permission by id: %w", err)
	}
	return p, nil
}

func (r *repository) GetByResourceAction(ctx context.Context, resource, action string) (*domain.Permission, error) {
	const q = `
		SELECT id, resource, action, description, created_at
		FROM permissions
		WHERE resource = $1 AND action = $2`

	row := r.db.QueryRow(ctx, q, resource, action)
	p, err := scanPermission(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, common.ErrNotFound
		}
		return nil, fmt.Errorf("get permission by resource and action: %w", err)
	}
	return p, nil
}

func (r *repository) List(ctx context.Context, params ListParams) ([]*domain.Permission, int64, error) {
	args := []any{}
	where := "WHERE 1=1"
	i := 1

	if params.Resource != "" {
		where += fmt.Sprintf(" AND resource = $%d", i)
		args = append(args, params.Resource)
		i++
	}
	if params.Search != "" {
		where += fmt.Sprintf(" AND (resource ILIKE '%%' || $%d || '%%' OR action ILIKE '%%' || $%d || '%%' OR description ILIKE '%%' || $%d || '%%')", i, i, i)
		args = append(args, params.Search)
		i++
	}

	var total int64
	countQ := "SELECT COUNT(*) FROM permissions " + where
	if err := r.db.QueryRow(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count permissions: %w", err)
	}

	args = append(args, params.PerPage, params.Offset())
	dataQ := fmt.Sprintf(`
		SELECT id, resource, action, description, created_at
		FROM permissions %s
		ORDER BY resource ASC, action ASC
		LIMIT $%d OFFSET $%d`, where, i, i+1)

	rows, err := r.db.Query(ctx, dataQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list permissions: %w", err)
	}
	defer rows.Close()

	perms := make([]*domain.Permission, 0)
	for rows.Next() {
		p, err := scanPermission(rows)
		if err != nil {
			return nil, 0, err
		}
		perms = append(perms, p)
	}
	return perms, total, rows.Err()
}

type scannable interface {
	Scan(dest ...any) error
}

func scanPermission(row scannable) (*domain.Permission, error) {
	var p domain.Permission
	if err := row.Scan(&p.ID, &p.Resource, &p.Action, &p.Description, &p.CreatedAt); err != nil {
		return nil, err
	}
	return &p, nil
}
