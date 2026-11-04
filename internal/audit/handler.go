package audit

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/yourusername/multi-tenant-user-management/internal/common"
)

// PermissionMiddleware creates a route middleware enforcing one of the given permissions.
type PermissionMiddleware func(perms ...string) func(http.Handler) http.Handler

// Handler wires HTTP routes for querying tenant audit logs.
type Handler struct {
	svc       Service
	permCheck PermissionMiddleware
}

// NewHandler creates a new audit log HTTP handler.
func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc}
}

// WithPermissionCheck attaches a permission-checking middleware factory to the handler.
func (h *Handler) WithPermissionCheck(pm PermissionMiddleware) *Handler {
	h.permCheck = pm
	return h
}

func (h *Handler) guard(perms ...string) func(http.Handler) http.Handler {
	if h.permCheck != nil {
		return h.permCheck(perms...)
	}
	return func(next http.Handler) http.Handler { return next }
}

// RegisterRoutes mounts audit log routes onto the given router.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.With(h.guard("audit:read")).Get("/audit-logs", h.list)
}

// list handles GET /api/v1/audit-logs
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	tenantID := common.GetTenantID(r.Context())
	if tenantID == uuid.Nil {
		common.Error(w, common.ErrUnauthorized)
		return
	}

	q := r.URL.Query()
	pagination := common.ParsePagination(r)

	params := ListParams{
		Action:           strings.TrimSpace(q.Get("action")),
		TargetType:       strings.TrimSpace(q.Get("target_type")),
		PaginationParams: pagination,
	}

	if rawActor := strings.TrimSpace(q.Get("actor_id")); rawActor != "" {
		id, err := uuid.Parse(rawActor)
		if err != nil {
			common.Error(w, common.New(http.StatusBadRequest, "INVALID_FILTER", "actor_id must be a valid UUID"))
			return
		}
		params.ActorID = &id
	}

	if rawTarget := strings.TrimSpace(q.Get("target_id")); rawTarget != "" {
		id, err := uuid.Parse(rawTarget)
		if err != nil {
			common.Error(w, common.New(http.StatusBadRequest, "INVALID_FILTER", "target_id must be a valid UUID"))
			return
		}
		params.TargetID = &id
	}

	startRaw := firstNonEmpty(q.Get("from"), q.Get("start_date"), q.Get("created_after"))
	if startRaw != "" {
		t, err := parseDateFilter(startRaw, false)
		if err != nil {
			common.Error(w, common.New(http.StatusBadRequest, "INVALID_FILTER", "invalid start date format; use RFC3339 or YYYY-MM-DD"))
			return
		}
		params.StartDate = &t
	}

	endRaw := firstNonEmpty(q.Get("to"), q.Get("end_date"), q.Get("created_before"))
	if endRaw != "" {
		t, err := parseDateFilter(endRaw, true)
		if err != nil {
			common.Error(w, common.New(http.StatusBadRequest, "INVALID_FILTER", "invalid end date format; use RFC3339 or YYYY-MM-DD"))
			return
		}
		params.EndDate = &t
	}

	logs, total, err := h.svc.List(r.Context(), tenantID, params)
	if err != nil {
		common.Error(w, common.AsAppError(err))
		return
	}

	common.OKWithMeta(w, logs, common.Meta{
		Page:    pagination.Page,
		PerPage: pagination.PerPage,
		Total:   total,
	})
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func parseDateFilter(raw string, endOfDay bool) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return t.UTC(), nil
	}
	t, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return time.Time{}, err
	}
	if endOfDay {
		return t.UTC().Add(24*time.Hour - time.Nanosecond), nil
	}
	return t.UTC(), nil
}
