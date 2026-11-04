package tenant

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

// Repository defines the data-access interface for tenants.
type Repository interface {
	Create(ctx context.Context, t *domain.Tenant) (*domain.Tenant, error)
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Tenant, error)
	GetByCode(ctx context.Context, code string) (*domain.Tenant, error)
	Update(ctx context.Context, t *domain.Tenant) (*domain.Tenant, error)
	List(ctx context.Context, params ListParams) ([]*domain.Tenant, int64, error)
}

// ListParams holds filters for listing tenants.
type ListParams struct {
	Search string
	Status string
	common.PaginationParams
}

type repository struct {
	db *pgxpool.Pool
}

// NewRepository creates a new tenant repository backed by a pgxpool.
func NewRepository(db *pgxpool.Pool) Repository {
	return &repository{db: db}
}

func (r *repository) Create(ctx context.Context, t *domain.Tenant) (*domain.Tenant, error) {
	const q = `
		INSERT INTO tenants (name, code, status)
		VALUES ($1, $2, $3)
		RETURNING id, name, code, status, created_at, updated_at`

	row := r.db.QueryRow(ctx, q, t.Name, t.Code, t.Status)
	return scanTenant(row)
}

func (r *repository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Tenant, error) {
	const q = `
		SELECT id, name, code, status, created_at, updated_at
		FROM tenants WHERE id = $1`

	row := r.db.QueryRow(ctx, q, id)
	t, err := scanTenant(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, common.ErrNotFound
		}
		return nil, fmt.Errorf("get tenant by id: %w", err)
	}
	return t, nil
}

func (r *repository) GetByCode(ctx context.Context, code string) (*domain.Tenant, error) {
	const q = `
		SELECT id, name, code, status, created_at, updated_at
		FROM tenants WHERE code = $1`

	row := r.db.QueryRow(ctx, q, code)
	t, err := scanTenant(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, common.ErrNotFound
		}
		return nil, fmt.Errorf("get tenant by code: %w", err)
	}
	return t, nil
}

func (r *repository) Update(ctx context.Context, t *domain.Tenant) (*domain.Tenant, error) {
	const q = `
		UPDATE tenants
		SET name = $1, status = $2, updated_at = NOW()
		WHERE id = $3
		RETURNING id, name, code, status, created_at, updated_at`

	row := r.db.QueryRow(ctx, q, t.Name, t.Status, t.ID)
	updated, err := scanTenant(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, common.ErrNotFound
		}
		return nil, fmt.Errorf("update tenant: %w", err)
	}
	return updated, nil
}

func (r *repository) List(ctx context.Context, params ListParams) ([]*domain.Tenant, int64, error) {
	// Build dynamic WHERE clause
	args := []any{}
	where := "WHERE 1=1"
	i := 1

	if params.Search != "" {
		where += fmt.Sprintf(" AND name ILIKE '%%' || $%d || '%%'", i)
		args = append(args, params.Search)
		i++
	}
	if params.Status != "" {
		where += fmt.Sprintf(" AND status = $%d", i)
		args = append(args, params.Status)
		i++
	}

	// Count query
	var total int64
	countQ := "SELECT COUNT(*) FROM tenants " + where
	if err := r.db.QueryRow(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count tenants: %w", err)
	}

	// Data query
	args = append(args, params.PerPage, params.Offset())
	dataQ := fmt.Sprintf(`
		SELECT id, name, code, status, created_at, updated_at
		FROM tenants %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d`, where, i, i+1)

	rows, err := r.db.Query(ctx, dataQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list tenants: %w", err)
	}
	defer rows.Close()

	tenants := make([]*domain.Tenant, 0)
	for rows.Next() {
		t, err := scanTenant(rows)
		if err != nil {
			return nil, 0, err
		}
		tenants = append(tenants, t)
	}
	return tenants, total, rows.Err()
}

// scanTenant scans a single tenant row from any pgx Row/Rows compatible type.
type scannable interface {
	Scan(dest ...any) error
}

func scanTenant(row scannable) (*domain.Tenant, error) {
	var t domain.Tenant
	err := row.Scan(&t.ID, &t.Name, &t.Code, &t.Status, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

