package auth

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/yourusername/multi-tenant-user-management/internal/common"
)

// Handler wires HTTP routes for authentication.
type Handler struct {
	svc Service
}

// NewHandler creates a new auth HTTP handler.
func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterPublicRoutes mounts unauthenticated auth routes (/auth/register, /auth/login, /auth/refresh).
func (h *Handler) RegisterPublicRoutes(r chi.Router) {
	r.Post("/auth/register", h.register)
	r.Post("/auth/login", h.login)
	r.Post("/auth/refresh", h.refresh)
}

// RegisterProtectedRoutes mounts JWT-protected auth routes (/auth/logout).
func (h *Handler) RegisterProtectedRoutes(r chi.Router) {
	r.Post("/auth/logout", h.logout)
}

// register handles POST /api/v1/auth/register
func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if !common.Decode(w, r, &req) {
		return
	}

	resp, err := h.svc.Register(r.Context(), req)
	if err != nil {
		common.Error(w, common.AsAppError(err))
		return
	}

	common.Created(w, resp)
}

// login handles POST /api/v1/auth/login
func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if !common.Decode(w, r, &req) {
		return
	}

	resp, err := h.svc.Login(r.Context(), req)
	if err != nil {
		common.Error(w, common.AsAppError(err))
		return
	}

	common.OK(w, resp)
}

// refresh handles POST /api/v1/auth/refresh
func (h *Handler) refresh(w http.ResponseWriter, r *http.Request) {
	var req RefreshRequest
	if !common.Decode(w, r, &req) {
		return
	}

	resp, err := h.svc.Refresh(r.Context(), req)
	if err != nil {
		common.Error(w, common.AsAppError(err))
		return
	}

	common.OK(w, resp)
}

// logout handles POST /api/v1/auth/logout
func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	var req LogoutRequest
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
			common.Error(w, common.New(http.StatusBadRequest, "INVALID_JSON", "request body is not valid JSON"))
			return
		}
	}

	if err := h.svc.Logout(r.Context(), req); err != nil {
		common.Error(w, common.AsAppError(err))
		return
	}

	common.OK(w, map[string]string{"message": "logged out successfully"})
}
