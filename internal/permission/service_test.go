package permission

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
	"github.com/yourusername/multi-tenant-user-management/internal/domain"
)

type mockPermRepo struct {
	byID  map[uuid.UUID]*domain.Permission
	byKey map[string]*domain.Permission
}

func newMockPermRepo() *mockPermRepo {
	return &mockPermRepo{
		byID:  make(map[uuid.UUID]*domain.Permission),
		byKey: make(map[string]*domain.Permission),
	}
}

func (m *mockPermRepo) Create(_ context.Context, p *domain.Permission) (*domain.Permission, error) {
	created := &domain.Permission{
		ID:          uuid.New(),
		Resource:    p.Resource,
		Action:      p.Action,
		Description: p.Description,
		CreatedAt:   time.Now().UTC(),
	}
	m.byID[created.ID] = created
	m.byKey[created.Slug()] = created
	return created, nil
}

func (m *mockPermRepo) GetByID(_ context.Context, id uuid.UUID) (*domain.Permission, error) {
	if p, ok := m.byID[id]; ok {
		return p, nil
	}
	return nil, common.ErrNotFound
}

func (m *mockPermRepo) GetByResourceAction(_ context.Context, resource, action string) (*domain.Permission, error) {
	if p, ok := m.byKey[resource+":"+action]; ok {
		return p, nil
	}
	return nil, common.ErrNotFound
}

func (m *mockPermRepo) List(_ context.Context, _ ListParams) ([]*domain.Permission, int64, error) {
	out := make([]*domain.Permission, 0, len(m.byID))
	for _, p := range m.byID {
		out = append(out, p)
	}
	return out, int64(len(out)), nil
}

func TestPermissionHandler_CreateAndList(t *testing.T) {
	repo := newMockPermRepo()
	svc := NewService(repo)
	h := NewHandler(svc)

	r := chi.NewRouter()
	h.RegisterRoutes(r)

	// 1. Create a valid permission
	body := []byte(`{"resource":"Report","action":"Export","description":"Export reports"}`)
	req := httptest.NewRequest(http.MethodPost, "/permissions", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", w.Code, w.Body.String())
	}
	var createResp common.Response
	if err := json.Unmarshal(w.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if !createResp.Success {
		t.Fatalf("expected success=true")
	}

	// 2. Duplicate permission returns 409 Conflict
	reqDup := httptest.NewRequest(http.MethodPost, "/permissions", bytes.NewReader(body))
	wDup := httptest.NewRecorder()
	r.ServeHTTP(wDup, reqDup)
	if wDup.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d", wDup.Code)
	}

	// 3. List permissions returns 200 with meta
	reqList := httptest.NewRequest(http.MethodGet, "/permissions?page=1&per_page=10", nil)
	wList := httptest.NewRecorder()
	r.ServeHTTP(wList, reqList)
	if wList.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", wList.Code)
	}
	var listResp common.Response
	if err := json.Unmarshal(wList.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("unmarshal list response: %v", err)
	}
	if !listResp.Success || listResp.Meta == nil || listResp.Meta.Total != 1 {
		t.Fatalf("unexpected list response: %+v", listResp)
	}
}
