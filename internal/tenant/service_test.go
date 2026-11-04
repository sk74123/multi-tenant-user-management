package tenant

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

type mockTenantRepo struct {
	byID   map[uuid.UUID]*domain.Tenant
	byCode map[string]*domain.Tenant
}

func newMockTenantRepo() *mockTenantRepo {
	return &mockTenantRepo{
		byID:   make(map[uuid.UUID]*domain.Tenant),
		byCode: make(map[string]*domain.Tenant),
	}
}

func (m *mockTenantRepo) Create(_ context.Context, t *domain.Tenant) (*domain.Tenant, error) {
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

func (m *mockTenantRepo) GetByID(_ context.Context, id uuid.UUID) (*domain.Tenant, error) {
	if t, ok := m.byID[id]; ok {
		cp := *t
		return &cp, nil
	}
	return nil, common.ErrNotFound
}

func (m *mockTenantRepo) GetByCode(_ context.Context, code string) (*domain.Tenant, error) {
	if t, ok := m.byCode[code]; ok {
		cp := *t
		return &cp, nil
	}
	return nil, common.ErrNotFound
}

func (m *mockTenantRepo) Update(_ context.Context, t *domain.Tenant) (*domain.Tenant, error) {
	existing, ok := m.byID[t.ID]
	if !ok {
		return nil, common.ErrNotFound
	}
	existing.Name = t.Name
	existing.Status = t.Status
	existing.UpdatedAt = time.Now().UTC()
	cp := *existing
	return &cp, nil
}

func (m *mockTenantRepo) List(_ context.Context, params ListParams) ([]*domain.Tenant, int64, error) {
	out := make([]*domain.Tenant, 0)
	for _, t := range m.byID {
		if params.Search != "" && !strings.Contains(strings.ToLower(t.Name), strings.ToLower(params.Search)) {
			continue
		}
		if params.Status != "" && string(t.Status) != params.Status {
			continue
		}
		out = append(out, t)
	}
	return out, int64(len(out)), nil
}

func TestTenantServiceAndHandler_CRUDAndIsolation(t *testing.T) {
	repo := newMockTenantRepo()
	svc := NewService(repo)
	h := NewHandler(svc)

	r := chi.NewRouter()
	h.RegisterRoutes(r)

	// 1. Create Tenant A
	bodyA := []byte(`{"name":"Acme Corp","code":"Acme Corp"}`)
	reqA := httptest.NewRequest(http.MethodPost, "/tenants", bytes.NewReader(bodyA))
	wA := httptest.NewRecorder()
	r.ServeHTTP(wA, reqA)
	if wA.Code != http.StatusCreated {
		t.Fatalf("expected 201 on create tenant A, got %d: %s", wA.Code, wA.Body.String())
	}

	var envA struct {
		Success bool          `json:"success"`
		Data    domain.Tenant `json:"data"`
	}
	if err := json.Unmarshal(wA.Body.Bytes(), &envA); err != nil {
		t.Fatalf("unmarshal tenant A: %v", err)
	}
	if envA.Data.Code != "acme-corp" || envA.Data.Status != domain.TenantStatusActive {
		t.Fatalf("expected normalized code acme-corp and status active, got %+v", envA.Data)
	}

	// 2. Duplicate tenant code returns 409 Conflict
	reqDup := httptest.NewRequest(http.MethodPost, "/tenants", bytes.NewReader(bodyA))
	wDup := httptest.NewRecorder()
	r.ServeHTTP(wDup, reqDup)
	if wDup.Code != http.StatusConflict {
		t.Fatalf("expected 409 on duplicate tenant code, got %d", wDup.Code)
	}

	// 3. Create Tenant B
	bodyB := []byte(`{"name":"Globex Inc","code":"globex"}`)
	reqB := httptest.NewRequest(http.MethodPost, "/tenants", bytes.NewReader(bodyB))
	wB := httptest.NewRecorder()
	r.ServeHTTP(wB, reqB)
	if wB.Code != http.StatusCreated {
		t.Fatalf("expected 201 on create tenant B, got %d", wB.Code)
	}
	var envB struct {
		Data domain.Tenant `json:"data"`
	}
	_ = json.Unmarshal(wB.Body.Bytes(), &envB)

	// 4. Cross-tenant isolation: caller in Tenant A accessing Tenant B gets 404 Not Found
	reqCross := httptest.NewRequest(http.MethodGet, "/tenants/"+envB.Data.ID.String(), nil)
	reqCross = reqCross.WithContext(common.SetTenantID(reqCross.Context(), envA.Data.ID))
	wCross := httptest.NewRecorder()
	r.ServeHTTP(wCross, reqCross)
	if wCross.Code != http.StatusNotFound {
		t.Fatalf("expected 404 on cross-tenant GetByID, got %d", wCross.Code)
	}

	// 5. Same-tenant update succeeds
	reqUpdate := httptest.NewRequest(http.MethodPatch, "/tenants/"+envA.Data.ID.String(),
		bytes.NewReader([]byte(`{"name":"Acme Global","status":"suspended"}`)))
	reqUpdate = reqUpdate.WithContext(common.SetTenantID(reqUpdate.Context(), envA.Data.ID))
	wUpdate := httptest.NewRecorder()
	r.ServeHTTP(wUpdate, reqUpdate)
	if wUpdate.Code != http.StatusOK {
		t.Fatalf("expected 200 on update tenant A, got %d: %s", wUpdate.Code, wUpdate.Body.String())
	}

	// 6. List tenants with search filter
	reqList := httptest.NewRequest(http.MethodGet, "/tenants?search=Globex", nil)
	wList := httptest.NewRecorder()
	r.ServeHTTP(wList, reqList)
	if wList.Code != http.StatusOK {
		t.Fatalf("expected 200 on list tenants, got %d", wList.Code)
	}
}
