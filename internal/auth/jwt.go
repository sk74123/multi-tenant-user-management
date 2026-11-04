package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/yourusername/multi-tenant-user-management/internal/config"
)

// AccessClaims represents the JWT claims embedded in an access token.
// Per blueprint: contains user_id, tenant_id, email, and standard registered claims.
type AccessClaims struct {
	UserID   uuid.UUID `json:"user_id"`
	TenantID uuid.UUID `json:"tenant_id"`
	Email    string    `json:"email"`
	jwt.RegisteredClaims
}

// RefreshClaims represents the JWT claims embedded in a refresh token.
type RefreshClaims struct {
	UserID   uuid.UUID `json:"user_id"`
	TenantID uuid.UUID `json:"tenant_id"`
	jwt.RegisteredClaims
}

// JWTManager issues and validates access and refresh JWTs.
type JWTManager struct {
	accessSecret  []byte
	refreshSecret []byte
	accessExpiry  time.Duration
	refreshExpiry time.Duration
}

// NewJWTManager constructs a JWTManager from configuration.
func NewJWTManager(cfg config.JWTConfig) *JWTManager {
	accessExpiry := cfg.AccessExpiry
	if accessExpiry <= 0 {
		accessExpiry = 15 * time.Minute
	}
	refreshExpiry := cfg.RefreshExpiry
	if refreshExpiry <= 0 {
		refreshExpiry = 7 * 24 * time.Hour
	}

	return &JWTManager{
		accessSecret:  []byte(cfg.AccessSecret),
		refreshSecret: []byte(cfg.RefreshSecret),
		accessExpiry:  accessExpiry,
		refreshExpiry: refreshExpiry,
	}
}

// AccessExpirySeconds returns the access token TTL in seconds.
func (m *JWTManager) AccessExpirySeconds() int64 {
	return int64(m.accessExpiry.Seconds())
}

// GenerateAccessToken creates a signed JWT access token scoped to user + tenant.
func (m *JWTManager) GenerateAccessToken(userID, tenantID uuid.UUID, email string) (string, time.Time, error) {
	now := time.Now().UTC()
	expiresAt := now.Add(m.accessExpiry)

	claims := AccessClaims{
		UserID:   userID,
		TenantID: tenantID,
		Email:    email,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.NewString(),
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(m.accessSecret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign access token: %w", err)
	}
	return signed, expiresAt, nil
}

// ValidateAccessToken parses and validates an access token string.
func (m *JWTManager) ValidateAccessToken(tokenStr string) (*AccessClaims, error) {
	var claims AccessClaims
	token, err := jwt.ParseWithClaims(tokenStr, &claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return m.accessSecret, nil
	})
	if err != nil {
		return nil, err
	}
	if !token.Valid {
		return nil, errors.New("invalid access token")
	}
	if claims.UserID == uuid.Nil || claims.TenantID == uuid.Nil {
		return nil, errors.New("access token missing required claims")
	}
	return &claims, nil
}

// GenerateRefreshToken creates a signed JWT refresh token scoped to user + tenant.
func (m *JWTManager) GenerateRefreshToken(userID, tenantID uuid.UUID) (string, time.Time, error) {
	now := time.Now().UTC()
	expiresAt := now.Add(m.refreshExpiry)

	claims := RefreshClaims{
		UserID:   userID,
		TenantID: tenantID,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.NewString(),
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(m.refreshSecret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign refresh token: %w", err)
	}
	return signed, expiresAt, nil
}

// ValidateRefreshToken parses and validates a refresh token string.
func (m *JWTManager) ValidateRefreshToken(tokenStr string) (*RefreshClaims, error) {
	var claims RefreshClaims
	token, err := jwt.ParseWithClaims(tokenStr, &claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return m.refreshSecret, nil
	})
	if err != nil {
		return nil, err
	}
	if !token.Valid {
		return nil, errors.New("invalid refresh token")
	}
	if claims.UserID == uuid.Nil || claims.TenantID == uuid.Nil {
		return nil, errors.New("refresh token missing required claims")
	}
	return &claims, nil
}

// HashToken returns the hex-encoded SHA-256 hash of a raw token string.
func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
