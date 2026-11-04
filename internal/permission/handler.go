package permission

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/yourusername/multi-tenant-user-management/internal/common"
)

// PermissionMiddleware creates a route middleware enforcing one of the given permissions.
type PermissionMiddleware func(perms ...string) func(http.Handler) http.Handler

// Handler wires HTTP routes to the permission service.
type Handler struct {
	svc       Service
	permCheck PermissionMiddleware
}

// NewHandler creates a permission HTTP handler.
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

// RegisterRoutes mounts permission routes onto the given router.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Route("/permissions", func(r chi.Router) {
		r.With(h.guard("permission:create")).Post("/", h.create)
		r.With(h.guard("permission:list")).Get("/", h.list)
	})
}

// create handles POST /api/v1/permissions
func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var req CreateRequest
	if !common.Decode(w, r, &req) {
		return
	}

	p, err := h.svc.Create(r.Context(), req)
	if err != nil {
		common.Error(w, common.AsAppError(err))
		return
	}

	common.Created(w, p)
}

// list handles GET /api/v1/permissions
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	pagination := common.ParsePagination(r)
	params := ListParams{
		Resource:         r.URL.Query().Get("resource"),
		Search:           r.URL.Query().Get("search"),
		PaginationParams: pagination,
	}

	perms, total, err := h.svc.List(r.Context(), params)
	if err != nil {
		common.Error(w, common.AsAppError(err))
		return
	}

	common.OKWithMeta(w, perms, common.Meta{
		Page:    pagination.Page,
		PerPage: pagination.PerPage,
		Total:   total,
	})
}
