// Package middleware provides HTTP middleware for the application.
// Phase 2: TenantFromHeader is a temporary middleware — replaced by JWT in Phase 4.
package middleware

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/yourusername/multi-tenant-user-management/internal/common"
)

// TenantFromHeader extracts the tenant UUID from the X-Tenant-ID request header
// and stores it in the request context.
//
// NOTE: This is a Phase 2 testing shim. Phase 4 replaces this with JWT-based
// tenant extraction — the tenant_id will come from the token only.
func TenantFromHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := r.Header.Get("X-Tenant-ID")
		if raw == "" {
			common.Error(w, common.New(http.StatusBadRequest, "MISSING_TENANT", "X-Tenant-ID header is required"))
			return
		}

		tenantID, err := uuid.Parse(raw)
		if err != nil {
			common.Error(w, common.New(http.StatusBadRequest, "INVALID_TENANT_ID", "X-Tenant-ID must be a valid UUID"))
			return
		}

		ctx := common.SetTenantID(r.Context(), tenantID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

