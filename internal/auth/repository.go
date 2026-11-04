package auth

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

// Repository defines data-access operations for refresh tokens.
type Repository interface {
	CreateRefreshToken(ctx context.Context, rt *domain.RefreshToken) (*domain.RefreshToken, error)
	GetRefreshTokenByHash(ctx context.Context, tokenHash string) (*domain.RefreshToken, error)
	RevokeRefreshToken(ctx context.Context, tokenHash string) error
	RevokeAllUserRefreshTokens(ctx context.Context, tenantID, userID uuid.UUID) error
}

type repository struct {
	db *pgxpool.Pool
}

// NewRepository creates a new auth repository backed by pgxpool.
func NewRepository(db *pgxpool.Pool) Repository {
	return &repository{db: db}
}

func (r *repository) CreateRefreshToken(ctx context.Context, rt *domain.RefreshToken) (*domain.RefreshToken, error) {
	const q = `
		INSERT INTO refresh_tokens (user_id, tenant_id, token_hash, expires_at, revoked)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, user_id, tenant_id, token_hash, expires_at, revoked, created_at`

	row := r.db.QueryRow(ctx, q, rt.UserID, rt.TenantID, rt.TokenHash, rt.ExpiresAt, rt.Revoked)
	created, err := scanRefreshToken(row)
	if err != nil {
		return nil, fmt.Errorf("create refresh token: %w", err)
	}
	return created, nil
}

func (r *repository) GetRefreshTokenByHash(ctx context.Context, tokenHash string) (*domain.RefreshToken, error) {
	const q = `
		SELECT id, user_id, tenant_id, token_hash, expires_at, revoked, created_at
		FROM refresh_tokens
		WHERE token_hash = $1`

	row := r.db.QueryRow(ctx, q, tokenHash)
	rt, err := scanRefreshToken(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, common.ErrNotFound
		}
		return nil, fmt.Errorf("get refresh token by hash: %w", err)
	}
	return rt, nil
}

func (r *repository) RevokeRefreshToken(ctx context.Context, tokenHash string) error {
	const q = `
		UPDATE refresh_tokens
		SET revoked = TRUE
		WHERE token_hash = $1 AND revoked = FALSE`

	tag, err := r.db.Exec(ctx, q, tokenHash)
	if err != nil {
		return fmt.Errorf("revoke refresh token: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return common.ErrNotFound
	}
	return nil
}

func (r *repository) RevokeAllUserRefreshTokens(ctx context.Context, tenantID, userID uuid.UUID) error {
	const q = `
		UPDATE refresh_tokens
		SET revoked = TRUE
		WHERE tenant_id = $1 AND user_id = $2 AND revoked = FALSE`

	if _, err := r.db.Exec(ctx, q, tenantID, userID); err != nil {
		return fmt.Errorf("revoke all user refresh tokens: %w", err)
	}
	return nil
}

type scannable interface {
	Scan(dest ...any) error
}

func scanRefreshToken(row scannable) (*domain.RefreshToken, error) {
	var rt domain.RefreshToken
	if err := row.Scan(
		&rt.ID,
		&rt.UserID,
		&rt.TenantID,
		&rt.TokenHash,
		&rt.ExpiresAt,
		&rt.Revoked,
		&rt.CreatedAt,
	); err != nil {
		return nil, err
	}
	return &rt, nil
}
