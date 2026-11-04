package role

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/yourusername/multi-tenant-user-management/internal/common"
)

// PermissionMiddleware creates a route middleware enforcing one of the given permissions.
type PermissionMiddleware func(perms ...string) func(http.Handler) http.Handler

// Handler wires HTTP routes to the role service.
type Handler struct {
	svc       Service
	permCheck PermissionMiddleware
}

// NewHandler creates a role HTTP handler.
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

// RegisterRoutes mounts role and user-role routes onto the given router.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Route("/roles", func(r chi.Router) {
		r.With(h.guard("role:create")).Post("/", h.create)
		r.With(h.guard("role:list")).Get("/", h.list)
		r.With(h.guard("role:read")).Get("/{id}", h.getByID)
		r.With(h.guard("role:update")).Patch("/{id}", h.update)
		r.With(h.guard("role:delete")).Delete("/{id}", h.delete)

		// Role <-> Permission endpoints
		r.With(h.guard("role:read")).Get("/{id}/permissions", h.listPermissions)
		r.With(h.guard("role:update")).Post("/{id}/permissions", h.assignPermissions)
		r.With(h.guard("role:update")).Delete("/{id}/permissions/{pid}", h.removePermission)
	})

	// User <-> Role & Effective Permission endpoints
	r.With(h.guard("user:read")).Get("/users/{id}/roles", h.listUserRoles)
	r.With(h.guard("user:assign_role")).Post("/users/{id}/roles", h.assignUserRoles)
	r.With(h.guard("user:assign_role")).Delete("/users/{id}/roles/{rid}", h.removeUserRole)
	r.With(h.guard("user:read")).Get("/users/{id}/permissions", h.listEffectivePermissions)
}

// create handles POST /api/v1/roles
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

	role, err := h.svc.Create(r.Context(), tenantID, req)
	if err != nil {
		common.Error(w, common.AsAppError(err))
		return
	}

	common.Created(w, role)
}

// list handles GET /api/v1/roles
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

	roles, total, err := h.svc.List(r.Context(), tenantID, params)
	if err != nil {
		common.Error(w, common.AsAppError(err))
		return
	}

	common.OKWithMeta(w, roles, common.Meta{
		Page:    pagination.Page,
		PerPage: pagination.PerPage,
		Total:   total,
	})
}

// getByID handles GET /api/v1/roles/{id}
func (h *Handler) getByID(w http.ResponseWriter, r *http.Request) {
	tenantID, roleID, ok := h.parseTenantAndID(w, r, "id", "role id must be a valid UUID")
	if !ok {
		return
	}

	role, err := h.svc.GetByID(r.Context(), tenantID, roleID)
	if err != nil {
		common.Error(w, common.AsAppError(err))
		return
	}

	common.OK(w, role)
}

// update handles PATCH /api/v1/roles/{id}
func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	tenantID, roleID, ok := h.parseTenantAndID(w, r, "id", "role id must be a valid UUID")
	if !ok {
		return
	}

	var req UpdateRequest
	if !common.Decode(w, r, &req) {
		return
	}

	role, err := h.svc.Update(r.Context(), tenantID, roleID, req)
	if err != nil {
		common.Error(w, common.AsAppError(err))
		return
	}

	common.OK(w, role)
}

// delete handles DELETE /api/v1/roles/{id}
func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	tenantID, roleID, ok := h.parseTenantAndID(w, r, "id", "role id must be a valid UUID")
	if !ok {
		return
	}

	if err := h.svc.Delete(r.Context(), tenantID, roleID); err != nil {
		common.Error(w, common.AsAppError(err))
		return
	}

	common.NoContent(w)
}

// listPermissions handles GET /api/v1/roles/{id}/permissions
func (h *Handler) listPermissions(w http.ResponseWriter, r *http.Request) {
	tenantID, roleID, ok := h.parseTenantAndID(w, r, "id", "role id must be a valid UUID")
	if !ok {
		return
	}

	perms, err := h.svc.ListPermissions(r.Context(), tenantID, roleID)
	if err != nil {
		common.Error(w, common.AsAppError(err))
		return
	}

	common.OK(w, perms)
}

// assignPermissions handles POST /api/v1/roles/{id}/permissions
func (h *Handler) assignPermissions(w http.ResponseWriter, r *http.Request) {
	tenantID, roleID, ok := h.parseTenantAndID(w, r, "id", "role id must be a valid UUID")
	if !ok {
		return
	}

	var req AssignPermissionsRequest
	if !common.Decode(w, r, &req) {
		return
	}

	perms, err := h.svc.AssignPermissions(r.Context(), tenantID, roleID, req)
	if err != nil {
		common.Error(w, common.AsAppError(err))
		return
	}

	common.OK(w, perms)
}

// removePermission handles DELETE /api/v1/roles/{id}/permissions/{pid}
func (h *Handler) removePermission(w http.ResponseWriter, r *http.Request) {
	tenantID, roleID, ok := h.parseTenantAndID(w, r, "id", "role id must be a valid UUID")
	if !ok {
		return
	}

	permID, err := uuid.Parse(chi.URLParam(r, "pid"))
	if err != nil {
		common.Error(w, common.New(http.StatusBadRequest, "INVALID_ID", "permission id must be a valid UUID"))
		return
	}

	if err := h.svc.RemovePermission(r.Context(), tenantID, roleID, permID); err != nil {
		common.Error(w, common.AsAppError(err))
		return
	}

	common.NoContent(w)
}

// listUserRoles handles GET /api/v1/users/{id}/roles
func (h *Handler) listUserRoles(w http.ResponseWriter, r *http.Request) {
	tenantID, userID, ok := h.parseTenantAndID(w, r, "id", "user id must be a valid UUID")
	if !ok {
		return
	}

	roles, err := h.svc.ListUserRoles(r.Context(), tenantID, userID)
	if err != nil {
		common.Error(w, common.AsAppError(err))
		return
	}

	common.OK(w, roles)
}

// assignUserRoles handles POST /api/v1/users/{id}/roles
func (h *Handler) assignUserRoles(w http.ResponseWriter, r *http.Request) {
	tenantID, userID, ok := h.parseTenantAndID(w, r, "id", "user id must be a valid UUID")
	if !ok {
		return
	}

	var req AssignUserRolesRequest
	if !common.Decode(w, r, &req) {
		return
	}

	roles, err := h.svc.AssignRolesToUser(r.Context(), tenantID, userID, req)
	if err != nil {
		common.Error(w, common.AsAppError(err))
		return
	}

	common.OK(w, roles)
}

// removeUserRole handles DELETE /api/v1/users/{id}/roles/{rid}
func (h *Handler) removeUserRole(w http.ResponseWriter, r *http.Request) {
	tenantID, userID, ok := h.parseTenantAndID(w, r, "id", "user id must be a valid UUID")
	if !ok {
		return
	}

	roleID, err := uuid.Parse(chi.URLParam(r, "rid"))
	if err != nil {
		common.Error(w, common.New(http.StatusBadRequest, "INVALID_ID", "role id must be a valid UUID"))
		return
	}

	if err := h.svc.RemoveRoleFromUser(r.Context(), tenantID, userID, roleID); err != nil {
		common.Error(w, common.AsAppError(err))
		return
	}

	common.NoContent(w)
}

// listEffectivePermissions handles GET /api/v1/users/{id}/permissions
func (h *Handler) listEffectivePermissions(w http.ResponseWriter, r *http.Request) {
	tenantID, userID, ok := h.parseTenantAndID(w, r, "id", "user id must be a valid UUID")
	if !ok {
		return
	}

	perms, err := h.svc.GetEffectivePermissions(r.Context(), tenantID, userID)
	if err != nil {
		common.Error(w, common.AsAppError(err))
		return
	}

	common.OK(w, perms)
}

func (h *Handler) parseTenantAndID(w http.ResponseWriter, r *http.Request, param, errMsg string) (uuid.UUID, uuid.UUID, bool) {
	tenantID := common.GetTenantID(r.Context())
	if tenantID == uuid.Nil {
		common.Error(w, common.ErrUnauthorized)
		return uuid.Nil, uuid.Nil, false
	}

	id, err := uuid.Parse(chi.URLParam(r, param))
	if err != nil {
		common.Error(w, common.New(http.StatusBadRequest, "INVALID_ID", errMsg))
		return uuid.Nil, uuid.Nil, false
	}

	return tenantID, id, true
}

