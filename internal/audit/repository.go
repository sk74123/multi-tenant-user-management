package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/yourusername/multi-tenant-user-management/internal/common"
	"github.com/yourusername/multi-tenant-user-management/internal/domain"
)

// Repository defines tenant-scoped data-access operations for audit logs.
type Repository interface {
	Create(ctx context.Context, entry *domain.AuditLog) (*domain.AuditLog, error)
	List(ctx context.Context, tenantID uuid.UUID, params ListParams) ([]*domain.AuditLog, int64, error)
}

// ListParams holds filter and pagination parameters for querying audit logs.
type ListParams struct {
	Action     string
	TargetType string
	ActorID    *uuid.UUID
	TargetID   *uuid.UUID
	StartDate  *time.Time
	EndDate    *time.Time
	common.PaginationParams
}

type repository struct {
	db *pgxpool.Pool
}

// NewRepository creates a new audit log repository backed by pgxpool.
func NewRepository(db *pgxpool.Pool) Repository {
	return &repository{db: db}
}

func (r *repository) Create(ctx context.Context, entry *domain.AuditLog) (*domain.AuditLog, error) {
	var changesBytes []byte
	if entry.Changes != nil {
		b, err := json.Marshal(entry.Changes)
		if err != nil {
			return nil, fmt.Errorf("marshal audit changes: %w", err)
		}
		changesBytes = b
	}

	const q = `
		INSERT INTO audit_logs (tenant_id, actor_id, action, target_type, target_id, changes, ip_address)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, tenant_id, actor_id, action, target_type, target_id, changes, ip_address, created_at`

	row := r.db.QueryRow(
		ctx,
		q,
		entry.TenantID,
		entry.ActorID,
		entry.Action,
		entry.TargetType,
		entry.TargetID,
		changesBytes,
		entry.IPAddress,
	)

	created, err := scanAuditLog(row)
	if err != nil {
		return nil, fmt.Errorf("create audit log: %w", err)
	}
	return created, nil
}

func (r *repository) List(ctx context.Context, tenantID uuid.UUID, params ListParams) ([]*domain.AuditLog, int64, error) {
	args := []any{tenantID}
	where := "WHERE tenant_id = $1"
	i := 2

	if params.Action != "" {
		where += fmt.Sprintf(
			" AND (LOWER(action) = LOWER($%d) OR LOWER(REPLACE(action, '.', ':')) = LOWER(REPLACE($%d, '.', ':')) OR action ILIKE '%%' || $%d || '%%')",
			i, i, i,
		)
		args = append(args, params.Action)
		i++
	}
	if params.TargetType != "" {
		where += fmt.Sprintf(" AND LOWER(target_type) = LOWER($%d)", i)
		args = append(args, params.TargetType)
		i++
	}
	if params.ActorID != nil && *params.ActorID != uuid.Nil {
		where += fmt.Sprintf(" AND actor_id = $%d", i)
		args = append(args, *params.ActorID)
		i++
	}
	if params.TargetID != nil && *params.TargetID != uuid.Nil {
		where += fmt.Sprintf(" AND target_id = $%d", i)
		args = append(args, *params.TargetID)
		i++
	}
	if params.StartDate != nil {
		where += fmt.Sprintf(" AND created_at >= $%d", i)
		args = append(args, *params.StartDate)
		i++
	}
	if params.EndDate != nil {
		where += fmt.Sprintf(" AND created_at <= $%d", i)
		args = append(args, *params.EndDate)
		i++
	}

	var total int64
	countQ := "SELECT COUNT(*) FROM audit_logs " + where
	if err := r.db.QueryRow(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count audit logs: %w", err)
	}

	args = append(args, params.PerPage, params.Offset())
	dataQ := fmt.Sprintf(`
		SELECT id, tenant_id, actor_id, action, target_type, target_id, changes, ip_address, created_at
		FROM audit_logs %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d`, where, i, i+1)

	rows, err := r.db.Query(ctx, dataQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list audit logs: %w", err)
	}
	defer rows.Close()

	logs := make([]*domain.AuditLog, 0)
	for rows.Next() {
		item, err := scanAuditLog(rows)
		if err != nil {
			return nil, 0, err
		}
		logs = append(logs, item)
	}
	return logs, total, rows.Err()
}

type scannable interface {
	Scan(dest ...any) error
}

func scanAuditLog(row scannable) (*domain.AuditLog, error) {
	var a domain.AuditLog
	var rawChanges []byte

	if err := row.Scan(
		&a.ID,
		&a.TenantID,
		&a.ActorID,
		&a.Action,
		&a.TargetType,
		&a.TargetID,
		&rawChanges,
		&a.IPAddress,
		&a.CreatedAt,
	); err != nil {
		return nil, err
	}

	if len(rawChanges) > 0 {
		var parsed any
		if err := json.Unmarshal(rawChanges, &parsed); err == nil {
			a.Changes = parsed
		}
	}

	return &a, nil
}
