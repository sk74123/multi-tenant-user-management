package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/yourusername/multi-tenant-user-management/internal/common"
	"github.com/yourusername/multi-tenant-user-management/internal/config"
	"github.com/yourusername/multi-tenant-user-management/internal/domain"
	"github.com/yourusername/multi-tenant-user-management/internal/tenant"
	"github.com/yourusername/multi-tenant-user-management/internal/user"
)

type memAuthRepo struct {
	byHash map[string]*domain.RefreshToken
}

func newMemAuthRepo() *memAuthRepo {
	return &memAuthRepo{byHash: make(map[string]*domain.RefreshToken)}
}

func (m *memAuthRepo) CreateRefreshToken(_ context.Context, rt *domain.RefreshToken) (*domain.RefreshToken, error) {
	created := &domain.RefreshToken{
		ID:        uuid.New(),
		UserID:    rt.UserID,
		TenantID:  rt.TenantID,
		TokenHash: rt.TokenHash,
		ExpiresAt: rt.ExpiresAt,
		Revoked:   rt.Revoked,
		CreatedAt: time.Now().UTC(),
	}
	m.byHash[created.TokenHash] = created
	return created, nil
}

func (m *memAuthRepo) GetRefreshTokenByHash(_ context.Context, tokenHash string) (*domain.RefreshToken, error) {
	if rt, ok := m.byHash[tokenHash]; ok {
		return rt, nil
	}
	return nil, common.ErrNotFound
}

func (m *memAuthRepo) RevokeRefreshToken(_ context.Context, tokenHash string) error {
	rt, ok := m.byHash[tokenHash]
	if !ok || rt.Revoked {
		return common.ErrNotFound
	}
	rt.Revoked = true
	return nil
}

func (m *memAuthRepo) RevokeAllUserRefreshTokens(_ context.Context, tenantID, userID uuid.UUID) error {
	for _, rt := range m.byHash {
		if rt.TenantID == tenantID && rt.UserID == userID {
			rt.Revoked = true
		}
	}
	return nil
}

type memTenantRepo struct {
	byID   map[uuid.UUID]*domain.Tenant
	byCode map[string]*domain.Tenant
}

func newMemTenantRepo() *memTenantRepo {
	return &memTenantRepo{
		byID:   make(map[uuid.UUID]*domain.Tenant),
		byCode: make(map[string]*domain.Tenant),
	}
}

func (m *memTenantRepo) Create(_ context.Context, t *domain.Tenant) (*domain.Tenant, error) {
	created := &domain.Tenant{
		ID:        uuid.New(),
		Name:      t.Name,
		Code:      t.Code,
		Status:    t.Status,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	m.byID[created.ID] = created
	m.byCode[created.Code] = created
	return created, nil
}

func (m *memTenantRepo) GetByID(_ context.Context, id uuid.UUID) (*domain.Tenant, error) {
	if t, ok := m.byID[id]; ok {
		return t, nil
	}
	return nil, common.ErrNotFound
}

func (m *memTenantRepo) GetByCode(_ context.Context, code string) (*domain.Tenant, error) {
	if t, ok := m.byCode[code]; ok {
		return t, nil
	}
	return nil, common.ErrNotFound
}

func (m *memTenantRepo) Update(_ context.Context, t *domain.Tenant) (*domain.Tenant, error) {
	m.byID[t.ID] = t
	m.byCode[t.Code] = t
	return t, nil
}

func (m *memTenantRepo) List(_ context.Context, _ tenant.ListParams) ([]*domain.Tenant, int64, error) {
	return nil, 0, nil
}

type memUserRepo struct {
	byID       map[uuid.UUID]*domain.User
	byEmail    map[string]*domain.User
	memberships map[string]bool
}

func newMemUserRepo() *memUserRepo {
	return &memUserRepo{
		byID:        make(map[uuid.UUID]*domain.User),
		byEmail:     make(map[string]*domain.User),
		memberships: make(map[string]bool),
	}
}

func (m *memUserRepo) GetByEmail(_ context.Context, email string) (*domain.User, error) {
	if u, ok := m.byEmail[email]; ok && u.DeletedAt == nil {
		return u, nil
	}
	return nil, common.ErrNotFound
}

func (m *memUserRepo) Create(_ context.Context, u *domain.User) (*domain.User, error) {
	created := &domain.User{
		ID:           uuid.New(),
		Email:        u.Email,
		PasswordHash: u.PasswordHash,
		FirstName:    u.FirstName,
		LastName:     u.LastName,
		IsActive:     u.IsActive,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	m.byID[created.ID] = created
	m.byEmail[created.Email] = created
	return created, nil
}

func (m *memUserRepo) GetByIDInTenant(_ context.Context, tenantID, userID uuid.UUID) (*domain.User, error) {
	if !m.memberships[tenantID.String()+":"+userID.String()] {
		return nil, common.ErrNotFound
	}
	if u, ok := m.byID[userID]; ok && u.DeletedAt == nil {
		return u, nil
	}
	return nil, common.ErrNotFound
}

func (m *memUserRepo) Update(_ context.Context, u *domain.User) (*domain.User, error) {
	m.byID[u.ID] = u
	m.byEmail[u.Email] = u
	return u, nil
}

func (m *memUserRepo) SoftDelete(_ context.Context, _, _ uuid.UUID) error { return nil }
func (m *memUserRepo) List(_ context.Context, _ uuid.UUID, _ user.ListParams) ([]*domain.User, int64, error) {
	return nil, 0, nil
}

func (m *memUserRepo) AddToTenant(_ context.Context, tenantID, userID uuid.UUID) error {
	m.memberships[tenantID.String()+":"+userID.String()] = true
	return nil
}

func (m *memUserRepo) IsInTenant(_ context.Context, tenantID, userID uuid.UUID) (bool, error) {
	return m.memberships[tenantID.String()+":"+userID.String()], nil
}

func TestAuthFlow_RegisterLoginRefreshLogout(t *testing.T) {
	ctx := context.Background()
	authRepo := newMemAuthRepo()
	userRepo := newMemUserRepo()
	tenantRepo := newMemTenantRepo()

	acme, _ := tenantRepo.Create(ctx, &domain.Tenant{
		Name:   "Acme Corp",
		Code:   "acme",
		Status: domain.TenantStatusActive,
	})

	jwtMgr := NewJWTManager(config.JWTConfig{
		AccessSecret:  "test-access-secret-min-32-characters",
		RefreshSecret: "test-refresh-secret-min-32-characters",
		AccessExpiry:  15 * time.Minute,
		RefreshExpiry: 7 * 24 * time.Hour,
	})

	svc := NewService(authRepo, userRepo, tenantRepo, jwtMgr, 4) // low bcrypt cost for fast tests
	h := NewHandler(svc)

	r := chi.NewRouter()
	h.RegisterPublicRoutes(r)
	h.RegisterProtectedRoutes(r)

	// 1. Register user in Acme tenant
	regPayload := `{"email":"alice@acme.com","password":"Password123!","first_name":"Alice","last_name":"Smith","tenant_code":"acme"}`
	reqReg := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader([]byte(regPayload)))
	wReg := httptest.NewRecorder()
	r.ServeHTTP(wReg, reqReg)
	if wReg.Code != http.StatusCreated {
		t.Fatalf("expected 201 on register, got %d: %s", wReg.Code, wReg.Body.String())
	}

	var regEnv struct {
		Success bool         `json:"success"`
		Data    AuthResponse `json:"data"`
	}
	if err := json.Unmarshal(wReg.Body.Bytes(), &regEnv); err != nil {
		t.Fatalf("unmarshal register response: %v", err)
	}
	if !regEnv.Success || regEnv.Data.AccessToken == "" || regEnv.Data.RefreshToken == "" || regEnv.Data.TenantID != acme.ID {
		t.Fatalf("unexpected register response: %+v", regEnv)
	}

	// Verify JWT access token claims
	claims, err := jwtMgr.ValidateAccessToken(regEnv.Data.AccessToken)
	if err != nil {
		t.Fatalf("validate access token: %v", err)
	}
	if claims.TenantID != acme.ID || claims.Email != "alice@acme.com" {
		t.Fatalf("unexpected claims: %+v", claims)
	}

	// 2. Duplicate registration in same tenant returns 409 Conflict
	reqDup := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader([]byte(regPayload)))
	wDup := httptest.NewRecorder()
	r.ServeHTTP(wDup, reqDup)
	if wDup.Code != http.StatusConflict {
		t.Fatalf("expected 409 on duplicate register, got %d", wDup.Code)
	}

	// 3. Login with valid credentials
	loginPayload := `{"email":"alice@acme.com","password":"Password123!","tenant_code":"acme"}`
	reqLogin := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader([]byte(loginPayload)))
	wLogin := httptest.NewRecorder()
	r.ServeHTTP(wLogin, reqLogin)
	if wLogin.Code != http.StatusOK {
		t.Fatalf("expected 200 on login, got %d: %s", wLogin.Code, wLogin.Body.String())
	}

	var loginEnv struct {
		Success bool         `json:"success"`
		Data    AuthResponse `json:"data"`
	}
	if err := json.Unmarshal(wLogin.Body.Bytes(), &loginEnv); err != nil {
		t.Fatalf("unmarshal login response: %v", err)
	}

	// 4. Login with wrong password returns 401
	badLoginPayload := `{"email":"alice@acme.com","password":"WrongPassword","tenant_code":"acme"}`
	reqBadLogin := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader([]byte(badLoginPayload)))
	wBadLogin := httptest.NewRecorder()
	r.ServeHTTP(wBadLogin, reqBadLogin)
	if wBadLogin.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 on wrong password, got %d", wBadLogin.Code)
	}

	// 5. Refresh rotates token and revokes old refresh token
	oldRefresh := loginEnv.Data.RefreshToken
	reqRefresh := httptest.NewRequest(http.MethodPost, "/auth/refresh",
		bytes.NewReader([]byte(`{"refresh_token":"`+oldRefresh+`"}`)))
	wRefresh := httptest.NewRecorder()
	r.ServeHTTP(wRefresh, reqRefresh)
	if wRefresh.Code != http.StatusOK {
		t.Fatalf("expected 200 on refresh, got %d: %s", wRefresh.Code, wRefresh.Body.String())
	}

	var refreshEnv struct {
		Success bool         `json:"success"`
		Data    AuthResponse `json:"data"`
	}
	if err := json.Unmarshal(wRefresh.Body.Bytes(), &refreshEnv); err != nil {
		t.Fatalf("unmarshal refresh response: %v", err)
	}

	// Reusing old refresh token must fail with 401
	reqReuse := httptest.NewRequest(http.MethodPost, "/auth/refresh",
		bytes.NewReader([]byte(`{"refresh_token":"`+oldRefresh+`"}`)))
	wReuse := httptest.NewRecorder()
	r.ServeHTTP(wReuse, reqReuse)
	if wReuse.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 on reusing rotated refresh token, got %d", wReuse.Code)
	}

	// 6. Logout revokes new refresh token
	newRefresh := refreshEnv.Data.RefreshToken
	reqLogout := httptest.NewRequest(http.MethodPost, "/auth/logout",
		bytes.NewReader([]byte(`{"refresh_token":"`+newRefresh+`"}`)))
	wLogout := httptest.NewRecorder()
	r.ServeHTTP(wLogout, reqLogout)
	if wLogout.Code != http.StatusOK {
		t.Fatalf("expected 200 on logout, got %d: %s", wLogout.Code, wLogout.Body.String())
	}

	// Refreshing after logout must fail with 401
	reqAfterLogout := httptest.NewRequest(http.MethodPost, "/auth/refresh",
		bytes.NewReader([]byte(`{"refresh_token":"`+newRefresh+`"}`)))
	wAfterLogout := httptest.NewRecorder()
	r.ServeHTTP(wAfterLogout, reqAfterLogout)
	if wAfterLogout.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 after logout, got %d", wAfterLogout.Code)
	}
}
