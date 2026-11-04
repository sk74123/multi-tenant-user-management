package user

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/yourusername/multi-tenant-user-management/internal/common"
)

// PermissionMiddleware creates a route middleware enforcing one of the given permissions.
type PermissionMiddleware func(perms ...string) func(http.Handler) http.Handler

// Handler wires HTTP routes to the user service.
type Handler struct {
	svc       Service
	permCheck PermissionMiddleware
}

// NewHandler creates a user HTTP handler.
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

// RegisterRoutes mounts user routes onto the given router.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.With(h.guard("user:create")).Post("/users", h.create)
	r.With(h.guard("user:list")).Get("/users", h.list)
	r.With(h.guard("user:read")).Get("/users/{id}", h.getByID)
	r.With(h.guard("user:update")).Patch("/users/{id}", h.update)
	r.With(h.guard("user:delete")).Delete("/users/{id}", h.softDelete)
}

// create handles POST /api/v1/users
func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	tenantID := common.GetTenantID(r.Context())
	if tenantID == uuid.Nil {
		common.Error(w, common.ErrUnauthorized)
		return
	}

	var req CreateRequest
	if !common.Decode(w, r, &req) {
		return
	}

	u, err := h.svc.Create(r.Context(), tenantID, req)
	if err != nil {
		common.Error(w, common.AsAppError(err))
		return
	}

	common.Created(w, u)
}

// getByID handles GET /api/v1/users/{id}
func (h *Handler) getByID(w http.ResponseWriter, r *http.Request) {
	tenantID, userID, ok := h.parseIDs(w, r)
	if !ok {
		return
	}

	u, err := h.svc.GetByID(r.Context(), tenantID, userID)
	if err != nil {
		common.Error(w, common.AsAppError(err))
		return
	}

	common.OK(w, u)
}

// update handles PATCH /api/v1/users/{id}
func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	tenantID, userID, ok := h.parseIDs(w, r)
	if !ok {
		return
	}

	var req UpdateRequest
	if !common.Decode(w, r, &req) {
		return
	}

	u, err := h.svc.Update(r.Context(), tenantID, userID, req)
	if err != nil {
		common.Error(w, common.AsAppError(err))
		return
	}

	common.OK(w, u)
}

// softDelete handles DELETE /api/v1/users/{id}
func (h *Handler) softDelete(w http.ResponseWriter, r *http.Request) {
	tenantID, userID, ok := h.parseIDs(w, r)
	if !ok {
		return
	}

	if err := h.svc.SoftDelete(r.Context(), tenantID, userID); err != nil {
		common.Error(w, common.AsAppError(err))
		return
	}

	common.NoContent(w)
}

// list handles GET /api/v1/users
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	tenantID := common.GetTenantID(r.Context())
	if tenantID == uuid.Nil {
		common.Error(w, common.ErrUnauthorized)
		return
	}

	pagination := common.ParsePagination(r)
	params := ListParams{
		Search:           r.URL.Query().Get("search"),
		PaginationParams: pagination,
	}

	// Optional ?is_active=true|false filter
	if raw := r.URL.Query().Get("is_active"); raw != "" {
		v, err := strconv.ParseBool(raw)
		if err == nil {
			params.IsActive = &v
		}
	}

	users, total, err := h.svc.List(r.Context(), tenantID, params)
	if err != nil {
		common.Error(w, common.AsAppError(err))
		return
	}

	common.OKWithMeta(w, users, common.Meta{
		Page:    pagination.Page,
		PerPage: pagination.PerPage,
		Total:   total,
	})
}

// parseIDs extracts tenantID from context and userID from URL.
func (h *Handler) parseIDs(w http.ResponseWriter, r *http.Request) (tenantID uuid.UUID, userID uuid.UUID, ok bool) {
	tenantID = common.GetTenantID(r.Context())
	if tenantID == uuid.Nil {
		common.Error(w, common.ErrUnauthorized)
		return uuid.Nil, uuid.Nil, false
	}

	userID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		common.Error(w, common.New(http.StatusBadRequest, "INVALID_ID", "user id must be a valid UUID"))
		return uuid.Nil, uuid.Nil, false
	}

	return tenantID, userID, true
}

