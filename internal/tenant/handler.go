package tenant

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/yourusername/multi-tenant-user-management/internal/common"
)

// PermissionMiddleware creates a route middleware enforcing one of the given permissions.
type PermissionMiddleware func(perms ...string) func(http.Handler) http.Handler

// Handler wires HTTP routes to the tenant service.
type Handler struct {
	svc       Service
	permCheck PermissionMiddleware
}

// NewHandler creates a tenant HTTP handler.
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

// RegisterRoutes mounts tenant routes onto the given router.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Route("/tenants", func(r chi.Router) {
		r.With(h.guard("tenant:create", "tenant:update")).Post("/", h.create)
		r.With(h.guard("tenant:read")).Get("/", h.list)
		r.With(h.guard("tenant:read")).Get("/{id}", h.getByID)
		r.With(h.guard("tenant:update")).Patch("/{id}", h.update)
	})
}

// create handles POST /api/v1/tenants
func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var req CreateRequest
	if !common.Decode(w, r, &req) {
		return
	}

	t, err := h.svc.Create(r.Context(), req)
	if err != nil {
		common.Error(w, common.AsAppError(err))
		return
	}

	common.Created(w, t)
}

// getByID handles GET /api/v1/tenants/{id}
func (h *Handler) getByID(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		common.Error(w, common.New(http.StatusBadRequest, "INVALID_ID", "tenant id must be a valid UUID"))
		return
	}

	t, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		common.Error(w, common.AsAppError(err))
		return
	}

	common.OK(w, t)
}

// update handles PATCH /api/v1/tenants/{id}
func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		common.Error(w, common.New(http.StatusBadRequest, "INVALID_ID", "tenant id must be a valid UUID"))
		return
	}

	var req UpdateRequest
	if !common.Decode(w, r, &req) {
		return
	}

	t, err := h.svc.Update(r.Context(), id, req)
	if err != nil {
		common.Error(w, common.AsAppError(err))
		return
	}

	common.OK(w, t)
}

// list handles GET /api/v1/tenants
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	pagination := common.ParsePagination(r)
	params := ListParams{
		Search:           r.URL.Query().Get("search"),
		Status:           r.URL.Query().Get("status"),
		PaginationParams: pagination,
	}

	tenants, total, err := h.svc.List(r.Context(), params)
	if err != nil {
		common.Error(w, common.AsAppError(err))
		return
	}

	common.OKWithMeta(w, tenants, common.Meta{
		Page:    pagination.Page,
		PerPage: pagination.PerPage,
		Total:   total,
	})
}

