package user

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/yourusername/multi-tenant-user-management/internal/common"
	"github.com/yourusername/multi-tenant-user-management/internal/domain"
)

type mockUserRepo struct {
	users       map[uuid.UUID]*domain.User
	byEmail     map[string]*domain.User
	memberships map[string]bool
}

func newMockUserRepo() *mockUserRepo {
	return &mockUserRepo{
		users:       make(map[uuid.UUID]*domain.User),
		byEmail:     make(map[string]*domain.User),
		memberships: make(map[string]bool),
	}
}

func (m *mockUserRepo) GetByEmail(_ context.Context, email string) (*domain.User, error) {
	if u, ok := m.byEmail[email]; ok && u.DeletedAt == nil {
		cp := *u
		return &cp, nil
	}
	return nil, common.ErrNotFound
}

func (m *mockUserRepo) Create(_ context.Context, u *domain.User) (*domain.User, error) {
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
	m.users[created.ID] = created
	m.byEmail[created.Email] = created
	cp := *created
	return &cp, nil
}

func (m *mockUserRepo) GetByIDInTenant(_ context.Context, tenantID, userID uuid.UUID) (*domain.User, error) {
	if !m.memberships[tenantID.String()+":"+userID.String()] {
		return nil, common.ErrNotFound
	}
	u, ok := m.users[userID]
	if !ok || u.DeletedAt != nil {
		return nil, common.ErrNotFound
	}
	cp := *u
	return &cp, nil
}

func (m *mockUserRepo) Update(_ context.Context, u *domain.User) (*domain.User, error) {
	existing, ok := m.users[u.ID]
	if !ok || existing.DeletedAt != nil {
		return nil, common.ErrNotFound
	}
	existing.FirstName = u.FirstName
	existing.LastName = u.LastName
	existing.IsActive = u.IsActive
	existing.UpdatedAt = time.Now().UTC()
	cp := *existing
	return &cp, nil
}

func (m *mockUserRepo) SoftDelete(_ context.Context, tenantID, userID uuid.UUID) error {
	if !m.memberships[tenantID.String()+":"+userID.String()] {
		return common.ErrNotFound
	}
	u, ok := m.users[userID]
	if !ok || u.DeletedAt != nil {
		return common.ErrNotFound
	}
	now := time.Now().UTC()
	u.DeletedAt = &now
	return nil
}

func (m *mockUserRepo) List(_ context.Context, tenantID uuid.UUID, params ListParams) ([]*domain.User, int64, error) {
	out := make([]*domain.User, 0)
	for _, u := range m.users {
		if u.DeletedAt != nil || !m.memberships[tenantID.String()+":"+u.ID.String()] {
			continue
		}
		if params.Search != "" && !strings.Contains(strings.ToLower(u.Email), strings.ToLower(params.Search)) {
			continue
		}
		if params.IsActive != nil && u.IsActive != *params.IsActive {
			continue
		}
		cp := *u
		out = append(out, &cp)
	}
	return out, int64(len(out)), nil
}

func (m *mockUserRepo) AddToTenant(_ context.Context, tenantID, userID uuid.UUID) error {
	m.memberships[tenantID.String()+":"+userID.String()] = true
	return nil
}

func (m *mockUserRepo) IsInTenant(_ context.Context, tenantID, userID uuid.UUID) (bool, error) {
	return m.memberships[tenantID.String()+":"+userID.String()], nil
}

func TestUserServiceAndHandler_CRUDSoftDeleteAndIsolation(t *testing.T) {
	repo := newMockUserRepo()
	svc := NewService(repo, 4)
	h := NewHandler(svc)

	tenantA, tenantB := uuid.New(), uuid.New()
	r := chi.NewRouter()
	h.RegisterRoutes(r)

	withTenant := func(req *http.Request, tid uuid.UUID) *http.Request {
		return req.WithContext(common.SetTenantID(req.Context(), tid))
	}

	// 1. Create user in Tenant A
	body := []byte(`{"email":"john@acme.com","password":"Password123!","first_name":"John","last_name":"Doe"}`)
	wCreate := httptest.NewRecorder()
	r.ServeHTTP(wCreate, withTenant(httptest.NewRequest(http.MethodPost, "/users", bytes.NewReader(body)), tenantA))
	if wCreate.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", wCreate.Code)
	}
	var createEnv struct{ Data domain.User `json:"data"` }
	_ = json.Unmarshal(wCreate.Body.Bytes(), &createEnv)
	userID := createEnv.Data.ID

	// 2. Cross-tenant access from Tenant B returns 404
	wCross := httptest.NewRecorder()
	r.ServeHTTP(wCross, withTenant(httptest.NewRequest(http.MethodGet, "/users/"+userID.String(), nil), tenantB))
	if wCross.Code != http.StatusNotFound {
		t.Fatalf("expected 404 on cross-tenant GetByID, got %d", wCross.Code)
	}

	// 3. Soft-delete user in Tenant A -> 204, excluded from list, retained in store
	wDel := httptest.NewRecorder()
	r.ServeHTTP(wDel, withTenant(httptest.NewRequest(http.MethodDelete, "/users/"+userID.String(), nil), tenantA))
	if wDel.Code != http.StatusNoContent {
		t.Fatalf("expected 204 on soft-delete, got %d", wDel.Code)
	}

	wGetDel := httptest.NewRecorder()
	r.ServeHTTP(wGetDel, withTenant(httptest.NewRequest(http.MethodGet, "/users/"+userID.String(), nil), tenantA))
	if wGetDel.Code != http.StatusNotFound || repo.users[userID].DeletedAt == nil {
		t.Fatalf("expected 404 after soft-delete with retained deleted_at")
	}
}
