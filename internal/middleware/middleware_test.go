package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/yourusername/multi-tenant-user-management/internal/auth"
	"github.com/yourusername/multi-tenant-user-management/internal/common"
	"github.com/yourusername/multi-tenant-user-management/internal/config"
	"github.com/yourusername/multi-tenant-user-management/internal/domain"
)

type stubPermResolver struct {
	calls int
	perms map[string][]*domain.Permission
}

func (s *stubPermResolver) ListEffectivePermissions(_ context.Context, tenantID, userID uuid.UUID) ([]*domain.Permission, error) {
	s.calls++
	return s.perms[tenantID.String()+":"+userID.String()], nil
}

func TestAuthenticateAndPermissionMiddleware(t *testing.T) {
	jwtMgr := auth.NewJWTManager(config.JWTConfig{
		AccessSecret:  "test-access-secret-at-least-32-chars",
		RefreshSecret: "test-refresh-secret-at-least-32-chars",
		AccessExpiry:  15 * time.Minute,
		RefreshExpiry: 24 * time.Hour,
	})

	tenantA := uuid.New()
	tenantB := uuid.New()
	userA := uuid.New()

	keyA := tenantA.String() + ":" + userA.String()
	resolver := &stubPermResolver{
		perms: map[string][]*domain.Permission{
			keyA: {
				{ID: uuid.New(), Resource: "user", Action: "read"},
			},
		},
	}

	cache := NewPermissionCache(5 * time.Minute)
	authorizer := NewPermissionAuthorizer(resolver, cache)

	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(Authenticate(jwtMgr))
		r.With(authorizer.Require("user:read")).Get("/protected-read", func(w http.ResponseWriter, r *http.Request) {
			common.OK(w, map[string]string{
				"tenant_id": common.GetTenantID(r.Context()).String(),
				"user_id":   common.GetUserID(r.Context()).String(),
			})
		})
		r.With(authorizer.Require("user:create")).Post("/protected-create", func(w http.ResponseWriter, r *http.Request) {
			common.Created(w, map[string]string{"status": "created"})
		})
	})

	// 1. Missing token -> 401 Unauthorized
	reqNoAuth := httptest.NewRequest(http.MethodGet, "/protected-read", nil)
	wNoAuth := httptest.NewRecorder()
	r.ServeHTTP(wNoAuth, reqNoAuth)
	if wNoAuth.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", wNoAuth.Code)
	}

	// 2. Valid token with user:read -> 200 OK, and X-Tenant-ID header is ignored in favor of JWT tenant
	tokenA, _, err := jwtMgr.GenerateAccessToken(userA, tenantA, "a@acme.com")
	if err != nil {
		t.Fatalf("generate access token: %v", err)
	}

	reqRead := httptest.NewRequest(http.MethodGet, "/protected-read", nil)
	reqRead.Header.Set("Authorization", "Bearer "+tokenA)
	reqRead.Header.Set("X-Tenant-ID", tenantB.String()) // Attempt to spoof tenant via header
	wRead := httptest.NewRecorder()
	r.ServeHTTP(wRead, reqRead)
	if wRead.Code != http.StatusOK {
		t.Fatalf("expected 200 with user:read, got %d: %s", wRead.Code, wRead.Body.String())
	}

	// 3. Second request hits cache without calling resolver again
	reqRead2 := httptest.NewRequest(http.MethodGet, "/protected-read", nil)
	reqRead2.Header.Set("Authorization", "Bearer "+tokenA)
	wRead2 := httptest.NewRecorder()
	r.ServeHTTP(wRead2, reqRead2)
	if wRead2.Code != http.StatusOK || resolver.calls != 1 {
		t.Fatalf("expected cached permission lookup (calls=1), got status=%d calls=%d", wRead2.Code, resolver.calls)
	}

	// 4. Missing permission (user:create) -> 403 Forbidden
	reqCreate := httptest.NewRequest(http.MethodPost, "/protected-create", nil)
	reqCreate.Header.Set("Authorization", "Bearer "+tokenA)
	wCreate := httptest.NewRecorder()
	r.ServeHTTP(wCreate, reqCreate)
	if wCreate.Code != http.StatusForbidden {
		t.Fatalf("expected 403 without user:create permission, got %d", wCreate.Code)
	}

	// 5. Grant user:create and invalidate cache -> now returns 201 Created
	resolver.perms[keyA] = append(resolver.perms[keyA], &domain.Permission{
		ID:       uuid.New(),
		Resource: "user",
		Action:   "create",
	})
	cache.InvalidateUser(tenantA, userA)

	reqCreate2 := httptest.NewRequest(http.MethodPost, "/protected-create", nil)
	reqCreate2.Header.Set("Authorization", "Bearer "+tokenA)
	wCreate2 := httptest.NewRecorder()
	r.ServeHTTP(wCreate2, reqCreate2)
	if wCreate2.Code != http.StatusCreated {
		t.Fatalf("expected 201 after cache invalidation and permission grant, got %d", wCreate2.Code)
	}
}
