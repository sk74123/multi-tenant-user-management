package audit_test

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
	"github.com/yourusername/multi-tenant-user-management/internal/audit"
	"github.com/yourusername/multi-tenant-user-management/internal/common"
	"github.com/yourusername/multi-tenant-user-management/internal/domain"
	"github.com/yourusername/multi-tenant-user-management/internal/middleware"
)

type memAuditRepo struct {
	logs []*domain.AuditLog
}

func (m *memAuditRepo) Create(_ context.Context, entry *domain.AuditLog) (*domain.AuditLog, error) {
	created := &domain.AuditLog{
		ID:         uuid.New(),
		TenantID:   entry.TenantID,
		ActorID:    entry.ActorID,
		Action:     entry.Action,
		TargetType: entry.TargetType,
		TargetID:   entry.TargetID,
		Changes:    entry.Changes,
		IPAddress:  entry.IPAddress,
		CreatedAt:  time.Now().UTC(),
	}
	m.logs = append(m.logs, created)
	return created, nil
}

func (m *memAuditRepo) List(_ context.Context, tenantID uuid.UUID, params audit.ListParams) ([]*domain.AuditLog, int64, error) {
	out := make([]*domain.AuditLog, 0)
	for _, item := range m.logs {
		if item.TenantID != tenantID {
			continue
		}
		if params.Action != "" && !strings.Contains(strings.ToLower(item.Action), strings.ToLower(params.Action)) {
			continue
		}
		if params.TargetType != "" && !strings.EqualFold(item.TargetType, params.TargetType) {
			continue
		}
		if params.StartDate != nil && item.CreatedAt.Before(*params.StartDate) {
			continue
		}
		if params.EndDate != nil && item.CreatedAt.After(*params.EndDate) {
			continue
		}
		out = append(out, item)
	}
	return out, int64(len(out)), nil
}

func TestAuditMiddlewareAndQueryEndpoint(t *testing.T) {
	repo := &memAuditRepo{}
	svc := audit.NewService(repo)
	h := audit.NewHandler(svc)

	tenantID := uuid.New()
	actorID := uuid.New()
	createdUserID := uuid.New()

	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ctx := common.SetTenantID(r.Context(), tenantID)
				ctx = common.SetUserID(ctx, actorID)
				next.ServeHTTP(w, r.WithContext(ctx))
			})
		})
		r.Use(middleware.Audit(svc))

		r.Post("/users", func(w http.ResponseWriter, r *http.Request) {
			common.Created(w, map[string]any{
				"id":         createdUserID.String(),
				"email":      "new@acme.com",
				"first_name": "New",
				"last_name":  "User",
			})
		})

		r.Patch("/users/{id}", func(w http.ResponseWriter, r *http.Request) {
			common.SetAuditBefore(r.Context(), map[string]any{"first_name": "New"})
			common.SetAuditAfter(r.Context(), map[string]any{"first_name": "Updated"})
			common.OK(w, map[string]any{
				"id":         chi.URLParam(r, "id"),
				"first_name": "Updated",
			})
		})

		h.RegisterRoutes(r)
	})

	// 1. Trigger user.create mutation
	reqCreate := httptest.NewRequest(http.MethodPost, "/api/v1/users",
		bytes.NewReader([]byte(`{"email":"new@acme.com","password":"Secret123!","first_name":"New","last_name":"User"}`)))
	reqCreate.Header.Set("X-Forwarded-For", "203.0.113.10")
	wCreate := httptest.NewRecorder()
	r.ServeHTTP(wCreate, reqCreate)
	if wCreate.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", wCreate.Code)
	}

	// 2. Trigger user.update mutation
	reqUpdate := httptest.NewRequest(http.MethodPatch, "/api/v1/users/"+createdUserID.String(),
		bytes.NewReader([]byte(`{"first_name":"Updated"}`)))
	wUpdate := httptest.NewRecorder()
	r.ServeHTTP(wUpdate, reqUpdate)
	if wUpdate.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", wUpdate.Code)
	}

	if len(repo.logs) != 2 {
		t.Fatalf("expected 2 audit logs, got %d", len(repo.logs))
	}

	// Verify password was stripped from input diff
	changesMap, ok := repo.logs[0].Changes.(map[string]any)
	if !ok {
		t.Fatalf("expected changes map, got %T", repo.logs[0].Changes)
	}
	if inputMap, ok := changesMap["input"].(map[string]any); ok {
		if _, hasPwd := inputMap["password"]; hasPwd {
			t.Fatalf("expected password to be redacted from audit log changes")
		}
	}

	// 3. Query GET /api/v1/audit-logs with filter ?action=user.update&target_type=user
	reqList := httptest.NewRequest(http.MethodGet, "/api/v1/audit-logs?action=user.update&target_type=user&from=2020-01-01&to=2030-12-31", nil)
	wList := httptest.NewRecorder()
	r.ServeHTTP(wList, reqList)
	if wList.Code != http.StatusOK {
		t.Fatalf("expected 200 on list audit logs, got %d: %s", wList.Code, wList.Body.String())
	}

	var listEnv struct {
		Success bool              `json:"success"`
		Data    []domain.AuditLog `json:"data"`
		Meta    common.Meta       `json:"meta"`
	}
	if err := json.Unmarshal(wList.Body.Bytes(), &listEnv); err != nil {
		t.Fatalf("unmarshal audit logs: %v", err)
	}
	if !listEnv.Success || listEnv.Meta.Total != 1 || len(listEnv.Data) != 1 {
		t.Fatalf("expected 1 filtered audit log, got %+v", listEnv)
	}
	if listEnv.Data[0].Action != "user.update" {
		t.Fatalf("expected action user.update, got %s", listEnv.Data[0].Action)
	}
}
