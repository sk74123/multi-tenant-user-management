package permission

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/yourusername/multi-tenant-user-management/internal/common"
)

// Handler wires HTTP routes to the permission service.
type Handler struct {
	svc Service
}

// NewHandler creates a permission HTTP handler.
func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes mounts permission routes onto the given router.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Route("/permissions", func(r chi.Router) {
		r.Post("/", h.create)
		r.Get("/", h.list)
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
