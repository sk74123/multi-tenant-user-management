package middleware

import (
	"net/http"
	"strings"

	"github.com/yourusername/multi-tenant-user-management/internal/auth"
	"github.com/yourusername/multi-tenant-user-management/internal/common"
)

// Authenticate returns an HTTP middleware that extracts a Bearer JWT from the
// Authorization header, validates it, and sets tenant_id, user_id, and email
// into the request context.
//
// Per Phase 4 blueprint: tenant_id comes strictly from the JWT token, never headers.
func Authenticate(jwtManager *auth.JWTManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
			if authHeader == "" {
				common.Error(w, common.ErrUnauthorized)
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				common.Error(w, common.New(http.StatusUnauthorized, "INVALID_TOKEN", "authorization header format must be Bearer {token}"))
				return
			}

			tokenStr := strings.TrimSpace(parts[1])
			if tokenStr == "" {
				common.Error(w, common.ErrUnauthorized)
				return
			}

			claims, err := jwtManager.ValidateAccessToken(tokenStr)
			if err != nil {
				common.Error(w, common.New(http.StatusUnauthorized, "INVALID_TOKEN", "invalid or expired access token"))
				return
			}

			ctx := r.Context()
			ctx = common.SetTenantID(ctx, claims.TenantID)
			ctx = common.SetUserID(ctx, claims.UserID)
			ctx = common.SetUserEmail(ctx, claims.Email)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
