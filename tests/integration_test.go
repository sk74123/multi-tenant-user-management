package tests

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/yourusername/multi-tenant-user-management/internal/auth"
	"github.com/yourusername/multi-tenant-user-management/internal/common"
	"github.com/yourusername/multi-tenant-user-management/internal/domain"
)

func loginHelper(t *testing.T, env *TestSuiteEnv, email, password, tenantCode string) auth.AuthResponse {
	t.Helper()
	w := env.DoRequest(t, http.MethodPost, "/api/v1/auth/login", "", map[string]string{
		"email":       email,
		"password":    password,
		"tenant_code": tenantCode,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 on login for %s, got %d: %s", email, w.Code, w.Body.String())
	}
	var resp struct {
		Data auth.AuthResponse `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return resp.Data
}

func TestIntegration_AuthFlow(t *testing.T) {
	env := SetupTestEnv(t)

	// 1. Register new user in Acme
	wReg := env.DoRequest(t, http.MethodPost, "/api/v1/auth/register", "", map[string]string{
		"email":       "newmember@acme.com",
		"password":    "StrongPass123!",
		"first_name":  "New",
		"last_name":   "Member",
		"tenant_code": "acme",
	})
	if wReg.Code != http.StatusCreated {
		t.Fatalf("expected 201 on register, got %d: %s", wReg.Code, wReg.Body.String())
	}

	// 2. Login as Acme Admin
	loginData := loginHelper(t, env, "admin@acme.com", "Admin@123", "acme")

	// 3. Access protected route with access token
	wUsers := env.DoRequest(t, http.MethodGet, "/api/v1/users", loginData.AccessToken, nil)
	if wUsers.Code != http.StatusOK {
		t.Fatalf("expected 200 on protected GET /users, got %d", wUsers.Code)
	}

	// 4. Refresh tokens -> rotates refresh token and revokes old one
	wRefresh := env.DoRequest(t, http.MethodPost, "/api/v1/auth/refresh", "", map[string]string{
		"refresh_token": loginData.RefreshToken,
	})
	if wRefresh.Code != http.StatusOK {
		t.Fatalf("expected 200 on refresh, got %d", wRefresh.Code)
	}
	var refreshEnv struct{ Data auth.AuthResponse `json:"data"` }
	_ = json.Unmarshal(wRefresh.Body.Bytes(), &refreshEnv)

	wOld := env.DoRequest(t, http.MethodPost, "/api/v1/auth/refresh", "", map[string]string{
		"refresh_token": loginData.RefreshToken,
	})
	if wOld.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 when reusing rotated refresh token, got %d", wOld.Code)
	}

	// 5. Logout revokes new refresh token
	wLogout := env.DoRequest(t, http.MethodPost, "/api/v1/auth/logout", refreshEnv.Data.AccessToken, map[string]string{
		"refresh_token": refreshEnv.Data.RefreshToken,
	})
	if wLogout.Code != http.StatusOK {
		t.Fatalf("expected 200 on logout, got %d", wLogout.Code)
	}

	wAfterLogout := env.DoRequest(t, http.MethodPost, "/api/v1/auth/refresh", "", map[string]string{
		"refresh_token": refreshEnv.Data.RefreshToken,
	})
	if wAfterLogout.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 on refresh after logout, got %d", wAfterLogout.Code)
	}
}

func TestIntegration_CrossTenantIsolationAndPermissionEnforcement(t *testing.T) {
	env := SetupTestEnv(t)

	acmeAdmin := loginHelper(t, env, "admin@acme.com", "Admin@123", "acme")
	acmeViewer := loginHelper(t, env, "user@acme.com", "User@123", "acme")

	// 1. Cross-tenant isolation: Acme admin accessing Globex user or tenant returns 404
	wCrossUser := env.DoRequest(t, http.MethodGet, "/api/v1/users/"+env.AdminGlobex.ID.String(), acmeAdmin.AccessToken, nil)
	if wCrossUser.Code != http.StatusNotFound {
		t.Fatalf("expected 404 on cross-tenant user access, got %d", wCrossUser.Code)
	}

	wCrossTenant := env.DoRequest(t, http.MethodGet, "/api/v1/tenants/"+env.TenantGlobex.ID.String(), acmeAdmin.AccessToken, nil)
	if wCrossTenant.Code != http.StatusNotFound {
		t.Fatalf("expected 404 on cross-tenant tenant access, got %d", wCrossTenant.Code)
	}

	// 2. Permission enforcement: viewer without user:create gets 403 Forbidden
	wForbidden := env.DoRequest(t, http.MethodPost, "/api/v1/users", acmeViewer.AccessToken, map[string]string{
		"email": "blocked@acme.com", "password": "Password123!", "first_name": "Blocked", "last_name": "User",
	})
	if wForbidden.Code != http.StatusForbidden {
		t.Fatalf("expected 403 when viewer attempts user:create, got %d", wForbidden.Code)
	}

	// 3. Soft-delete & Audit logs
	wCreate := env.DoRequest(t, http.MethodPost, "/api/v1/users", acmeAdmin.AccessToken, map[string]string{
		"email": "temp@acme.com", "password": "Password123!", "first_name": "Temp", "last_name": "User",
	})
	if wCreate.Code != http.StatusCreated {
		t.Fatalf("expected 201 on admin user create, got %d", wCreate.Code)
	}
	var createdUser struct{ Data domain.User `json:"data"` }
	_ = json.Unmarshal(wCreate.Body.Bytes(), &createdUser)

	wDel := env.DoRequest(t, http.MethodDelete, "/api/v1/users/"+createdUser.Data.ID.String(), acmeAdmin.AccessToken, nil)
	if wDel.Code != http.StatusNoContent {
		t.Fatalf("expected 204 on soft-delete, got %d", wDel.Code)
	}

	wGetDeleted := env.DoRequest(t, http.MethodGet, "/api/v1/users/"+createdUser.Data.ID.String(), acmeAdmin.AccessToken, nil)
	if wGetDeleted.Code != http.StatusNotFound || env.UsersMap[createdUser.Data.ID].DeletedAt == nil {
		t.Fatalf("expected 404 on GET for soft-deleted user while retained in storage")
	}

	wAudit := env.DoRequest(t, http.MethodGet, "/api/v1/audit-logs?target_type=user", acmeAdmin.AccessToken, nil)
	if wAudit.Code != http.StatusOK {
		t.Fatalf("expected 200 on GET /audit-logs, got %d", wAudit.Code)
	}
	var auditEnv struct{ Meta common.Meta `json:"meta"` }
	_ = json.Unmarshal(wAudit.Body.Bytes(), &auditEnv)
	if auditEnv.Meta.Total < 3 {
		t.Fatalf("expected at least 3 audit logs, got %d", auditEnv.Meta.Total)
	}
}

func TestIntegration_CRUDFlows(t *testing.T) {
	env := SetupTestEnv(t)
	adminAuth := loginHelper(t, env, "admin@acme.com", "Admin@123", "acme")
	tok := adminAuth.AccessToken

	// 1. Tenant read & update
	wGetTenant := env.DoRequest(t, http.MethodGet, "/api/v1/tenants/"+env.TenantAcme.ID.String(), tok, nil)
	if wGetTenant.Code != http.StatusOK {
		t.Fatalf("expected 200 on GET /tenants/{id}, got %d", wGetTenant.Code)
	}
	wPatchTenant := env.DoRequest(t, http.MethodPatch, "/api/v1/tenants/"+env.TenantAcme.ID.String(), tok, map[string]string{
		"name": "Acme Corporation",
	})
	if wPatchTenant.Code != http.StatusOK {
		t.Fatalf("expected 200 on PATCH /tenants/{id}, got %d", wPatchTenant.Code)
	}

	// 2. Permission create & list
	wCreatePerm := env.DoRequest(t, http.MethodPost, "/api/v1/permissions", tok, map[string]string{
		"resource":    "billing",
		"action":      "export",
		"description": "Export billing invoices",
	})
	if wCreatePerm.Code != http.StatusCreated {
		t.Fatalf("expected 201 on POST /permissions, got %d: %s", wCreatePerm.Code, wCreatePerm.Body.String())
	}
	var permEnv struct{ Data domain.Permission `json:"data"` }
	_ = json.Unmarshal(wCreatePerm.Body.Bytes(), &permEnv)

	wListPerms := env.DoRequest(t, http.MethodGet, "/api/v1/permissions", tok, nil)
	if wListPerms.Code != http.StatusOK {
		t.Fatalf("expected 200 on GET /permissions, got %d", wListPerms.Code)
	}

	// 3. Role create, update, assign permission, assign to user, delete
	wCreateRole := env.DoRequest(t, http.MethodPost, "/api/v1/roles", tok, map[string]string{
		"name":        "finance",
		"description": "Finance team role",
	})
	if wCreateRole.Code != http.StatusCreated {
		t.Fatalf("expected 201 on POST /roles, got %d", wCreateRole.Code)
	}
	var roleEnv struct{ Data domain.Role `json:"data"` }
	_ = json.Unmarshal(wCreateRole.Body.Bytes(), &roleEnv)

	wAssignPerm := env.DoRequest(t, http.MethodPost, "/api/v1/roles/"+roleEnv.Data.ID.String()+"/permissions", tok, map[string]string{
		"permission_id": permEnv.Data.ID.String(),
	})
	if wAssignPerm.Code != http.StatusOK {
		t.Fatalf("expected 200 on assign permission to role, got %d", wAssignPerm.Code)
	}

	wAssignUserRole := env.DoRequest(t, http.MethodPost, "/api/v1/users/"+env.ViewerAcme.ID.String()+"/roles", tok, map[string]string{
		"role_id": roleEnv.Data.ID.String(),
	})
	if wAssignUserRole.Code != http.StatusOK {
		t.Fatalf("expected 200 on assign role to user, got %d", wAssignUserRole.Code)
	}

	wDelRole := env.DoRequest(t, http.MethodDelete, "/api/v1/roles/"+roleEnv.Data.ID.String(), tok, nil)
	if wDelRole.Code != http.StatusNoContent {
		t.Fatalf("expected 204 on DELETE /roles/{id}, got %d", wDelRole.Code)
	}
}

