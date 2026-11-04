package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/yourusername/multi-tenant-user-management/internal/audit"
	"github.com/yourusername/multi-tenant-user-management/internal/common"
	"github.com/yourusername/multi-tenant-user-management/internal/domain"
)

type auditResponseRecorder struct {
	http.ResponseWriter
	statusCode int
	body       bytes.Buffer
}

func newAuditResponseRecorder(w http.ResponseWriter) *auditResponseRecorder {
	return &auditResponseRecorder{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}
}

func (r *auditResponseRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *auditResponseRecorder) Write(b []byte) (int, error) {
	r.body.Write(b)
	return r.ResponseWriter.Write(b)
}

// Audit captures mutations automatically and writes structured audit log entries.
func Audit(auditSvc audit.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if auditSvc == nil || !isAuditedMethod(r.Method) {
				next.ServeHTTP(w, r)
				return
			}

			var reqBodyBytes []byte
			if r.Body != nil {
				reqBodyBytes, _ = io.ReadAll(r.Body)
				r.Body = io.NopCloser(bytes.NewBuffer(reqBodyBytes))
			}

			clientIP := extractClientIP(r)
			meta := &common.AuditMeta{}

			ctx := r.Context()
			ctx = common.SetClientIP(ctx, clientIP)
			ctx = common.WithAuditMeta(ctx, meta)

			rec := newAuditResponseRecorder(w)
			next.ServeHTTP(rec, r.WithContext(ctx))

			// Only log successful mutations (2xx)
			if rec.statusCode < http.StatusOK || rec.statusCode >= http.StatusMultipleChoices || meta.Skip {
				return
			}

			action, targetType := classifyRoute(r)
			if meta.Action != "" {
				action = meta.Action
			}
			if meta.TargetType != "" {
				targetType = meta.TargetType
			}
			if action == "" {
				return
			}

			tenantID := common.GetTenantID(ctx)
			if tenantID == uuid.Nil && meta.TenantID != uuid.Nil {
				tenantID = meta.TenantID
			}

			var actorID *uuid.UUID
			if uid := common.GetUserID(ctx); uid != uuid.Nil {
				actorID = &uid
			} else if meta.ActorID != nil && *meta.ActorID != uuid.Nil {
				actorID = meta.ActorID
			}

			reqMap := parseSanitizedJSON(reqBodyBytes)
			respData := extractResponseData(rec.body.Bytes())

			if tenantID == uuid.Nil {
				tenantID = extractUUIDField(respData, "tenant_id")
			}
			if tenantID == uuid.Nil {
				return
			}

			targetID := resolveTargetID(r, meta, respData)
			changes := buildAuditChanges(r, meta, reqMap, respData)

			entry := &domain.AuditLog{
				TenantID:   tenantID,
				ActorID:    actorID,
				Action:     action,
				TargetType: targetType,
				TargetID:   targetID,
				Changes:    changes,
				IPAddress:  clientIP,
			}

			_, _ = auditSvc.Log(ctx, entry)
		})
	}
}

func isAuditedMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPatch, http.MethodPut, http.MethodDelete:
		return true
	default:
		return false
	}
}

func classifyRoute(r *http.Request) (action string, targetType string) {
	path := strings.TrimSuffix(r.URL.Path, "/")
	path = strings.TrimPrefix(path, "/api/v1")
	if path == "" {
		path = "/"
	}

	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		return "", ""
	}

	switch parts[0] {
	case "auth":
		if len(parts) >= 2 {
			switch parts[1] {
			case "login":
				return "auth.login", "user"
			case "register":
				return "auth.register", "user"
			case "logout":
				return "auth.logout", "user"
			}
		}
	case "users":
		switch {
		case len(parts) == 1 && r.Method == http.MethodPost:
			return "user.create", "user"
		case len(parts) == 2 && r.Method == http.MethodPatch:
			return "user.update", "user"
		case len(parts) == 2 && r.Method == http.MethodDelete:
			return "user.delete", "user"
		case len(parts) == 3 && parts[2] == "roles" && r.Method == http.MethodPost:
			return "user.assign_role", "user"
		case len(parts) == 4 && parts[2] == "roles" && r.Method == http.MethodDelete:
			return "user.remove_role", "user"
		}
	case "roles":
		switch {
		case len(parts) == 1 && r.Method == http.MethodPost:
			return "role.create", "role"
		case len(parts) == 2 && r.Method == http.MethodPatch:
			return "role.update", "role"
		case len(parts) == 2 && r.Method == http.MethodDelete:
			return "role.delete", "role"
		case len(parts) == 3 && parts[2] == "permissions" && r.Method == http.MethodPost:
			return "role.assign_permission", "role"
		case len(parts) == 4 && parts[2] == "permissions" && r.Method == http.MethodDelete:
			return "role.remove_permission", "role"
		}
	case "tenants":
		switch {
		case len(parts) == 1 && r.Method == http.MethodPost:
			return "tenant.create", "tenant"
		case len(parts) == 2 && r.Method == http.MethodPatch:
			return "tenant.update", "tenant"
		}
	case "permissions":
		if len(parts) == 1 && r.Method == http.MethodPost {
			return "permission.create", "permission"
		}
	}

	return "", ""
}

func resolveTargetID(r *http.Request, meta *common.AuditMeta, respData map[string]any) *uuid.UUID {
	if meta != nil && meta.TargetID != nil && *meta.TargetID != uuid.Nil {
		return meta.TargetID
	}

	if rawID := chi.URLParam(r, "id"); rawID != "" {
		if id, err := uuid.Parse(rawID); err == nil && id != uuid.Nil {
			return &id
		}
	}

	path := strings.TrimPrefix(strings.TrimSuffix(r.URL.Path, "/"), "/api/v1/")
	parts := strings.Split(path, "/")
	if len(parts) >= 2 {
		if id, err := uuid.Parse(parts[1]); err == nil && id != uuid.Nil {
			return &id
		}
	}

	if id := extractUUIDField(respData, "id"); id != uuid.Nil {
		return &id
	}
	if userMap, ok := respData["user"].(map[string]any); ok {
		if id := extractUUIDField(userMap, "id"); id != uuid.Nil {
			return &id
		}
	}

	return nil
}

func buildAuditChanges(r *http.Request, meta *common.AuditMeta, reqMap, respData map[string]any) map[string]any {
	changes := make(map[string]any)

	if meta != nil {
		for k, v := range meta.Changes {
			changes[k] = v
		}
		if meta.Before != nil {
			changes["before"] = meta.Before
		}
		if meta.After != nil {
			changes["after"] = meta.After
		}
	}

	if len(reqMap) > 0 {
		changes["input"] = reqMap
	}

	if _, hasAfter := changes["after"]; !hasAfter && len(respData) > 0 && r.Method != http.MethodDelete {
		sanitizedResp := make(map[string]any, len(respData))
		for k, v := range respData {
			if k == "access_token" || k == "refresh_token" {
				continue
			}
			sanitizedResp[k] = v
		}
		if len(sanitizedResp) > 0 {
			changes["after"] = sanitizedResp
		}
	}

	if pid := chi.URLParam(r, "pid"); pid != "" {
		changes["permission_id"] = pid
	}
	if rid := chi.URLParam(r, "rid"); rid != "" {
		changes["role_id"] = rid
	}

	if len(changes) == 0 {
		return nil
	}
	return changes
}

func parseSanitizedJSON(raw []byte) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	delete(m, "password")
	delete(m, "password_hash")
	delete(m, "refresh_token")
	delete(m, "access_token")
	return m
}

func extractResponseData(raw []byte) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var env struct {
		Data any `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil || env.Data == nil {
		return nil
	}
	if m, ok := env.Data.(map[string]any); ok {
		return m
	}
	return map[string]any{"items": env.Data}
}

func extractUUIDField(m map[string]any, key string) uuid.UUID {
	if m == nil {
		return uuid.Nil
	}
	raw, ok := m[key].(string)
	if !ok {
		return uuid.Nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil
	}
	return id
}

func extractClientIP(r *http.Request) string {
	if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
		parts := strings.Split(xff, ",")
		if ip := strings.TrimSpace(parts[0]); ip != "" {
			return ip
		}
	}
	if xrip := strings.TrimSpace(r.Header.Get("X-Real-IP")); xrip != "" {
		return xrip
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}
