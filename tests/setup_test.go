package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/yourusername/multi-tenant-user-management/internal/audit"
	"github.com/yourusername/multi-tenant-user-management/internal/auth"
	"github.com/yourusername/multi-tenant-user-management/internal/config"
	"github.com/yourusername/multi-tenant-user-management/internal/domain"
	"github.com/yourusername/multi-tenant-user-management/internal/middleware"
	"github.com/yourusername/multi-tenant-user-management/internal/permission"
	"github.com/yourusername/multi-tenant-user-management/internal/role"
	"github.com/yourusername/multi-tenant-user-management/internal/tenant"
	"github.com/yourusername/multi-tenant-user-management/internal/user"
)

type TestSuiteEnv struct {
	Router       http.Handler
	TenantAcme   *domain.Tenant
	TenantGlobex *domain.Tenant
	AdminAcme    *domain.User
	ViewerAcme   *domain.User
	AdminGlobex  *domain.User
	UsersMap     map[uuid.UUID]*domain.User
}

func SetupTestEnv(t *testing.T) *TestSuiteEnv {
	t.Helper()
	ctx := context.Background()

	tRepo := newInMemTenantRepo()
	uRepo := newInMemUserRepo()
	pRepo := newInMemPermRepo()
	rRepo := newInMemRoleRepo(pRepo)
	aRepo := newInMemAuthRepo()
	audRepo := newInMemAuditRepo()

	permCache := middleware.NewPermissionCache(5 * time.Minute)
	jwtMgr := auth.NewJWTManager(config.JWTConfig{
		AccessSecret:  "integration-test-access-secret-min-32-chars",
		RefreshSecret: "integration-test-refresh-secret-min-32-chars",
		AccessExpiry:  15 * time.Minute,
		RefreshExpiry: 168 * time.Hour,
	})

	tenantSvc := tenant.NewService(tRepo)
	userSvc := user.NewService(uRepo, 4)
	permSvc := permission.NewService(pRepo)
	roleSvc := role.NewService(rRepo, pRepo, uRepo, permCache)
	authSvc := auth.NewService(aRepo, uRepo, tRepo, jwtMgr, 4)
	auditSvc := audit.NewService(audRepo)
	permAuth := middleware.NewPermissionAuthorizer(rRepo, permCache)

	router := chi.NewRouter()
	router.Route("/api/v1", func(r chi.Router) {
		r.Use(middleware.Audit(auditSvc))
		auth.NewHandler(authSvc).RegisterPublicRoutes(r)
		r.Group(func(r chi.Router) {
			r.Use(middleware.Authenticate(jwtMgr))
			auth.NewHandler(authSvc).RegisterProtectedRoutes(r)
			tenant.NewHandler(tenantSvc).WithPermissionCheck(permAuth.Require).RegisterRoutes(r)
			user.NewHandler(userSvc).WithPermissionCheck(permAuth.Require).RegisterRoutes(r)
			role.NewHandler(roleSvc).WithPermissionCheck(permAuth.Require).RegisterRoutes(r)
			permission.NewHandler(permSvc).WithPermissionCheck(permAuth.Require).RegisterRoutes(r)
			audit.NewHandler(auditSvc).WithPermissionCheck(permAuth.Require).RegisterRoutes(r)
		})
	})

	acme, _ := tRepo.Create(ctx, &domain.Tenant{Name: "Acme Corp", Code: "acme", Status: domain.TenantStatusActive})
	globex, _ := tRepo.Create(ctx, &domain.Tenant{Name: "Globex Inc", Code: "globex", Status: domain.TenantStatusActive})

	seedSlugs := [][2]string{
		{"tenant", "read"}, {"tenant", "update"},
		{"user", "create"}, {"user", "read"}, {"user", "update"}, {"user", "delete"}, {"user", "list"}, {"user", "assign_role"},
		{"role", "create"}, {"role", "read"}, {"role", "list"}, {"role", "update"}, {"role", "delete"},
		{"permission", "create"}, {"permission", "list"},
		{"audit", "read"},
	}
	var allPermIDs, viewerPermIDs []uuid.UUID
	for _, ra := range seedSlugs {
		p, _ := pRepo.Create(ctx, &domain.Permission{Resource: ra[0], Action: ra[1]})
		allPermIDs = append(allPermIDs, p.ID)
		if ra[1] == "read" || ra[1] == "list" {
			viewerPermIDs = append(viewerPermIDs, p.ID)
		}
	}

	acmeAdminRole, _ := roleSvc.Create(ctx, acme.ID, role.CreateRequest{Name: "admin"})
	acmeViewerRole, _ := roleSvc.Create(ctx, acme.ID, role.CreateRequest{Name: "viewer"})
	globexAdminRole, _ := roleSvc.Create(ctx, globex.ID, role.CreateRequest{Name: "admin"})

	_, _ = roleSvc.AssignPermissions(ctx, acme.ID, acmeAdminRole.ID, role.AssignPermissionsRequest{PermissionIDs: allPermIDs})
	_, _ = roleSvc.AssignPermissions(ctx, acme.ID, acmeViewerRole.ID, role.AssignPermissionsRequest{PermissionIDs: viewerPermIDs})
	_, _ = roleSvc.AssignPermissions(ctx, globex.ID, globexAdminRole.ID, role.AssignPermissionsRequest{PermissionIDs: allPermIDs})

	adminAcme, _ := userSvc.Create(ctx, acme.ID, user.CreateRequest{Email: "admin@acme.com", Password: "Admin@123", FirstName: "Acme", LastName: "Admin"})
	viewerAcme, _ := userSvc.Create(ctx, acme.ID, user.CreateRequest{Email: "user@acme.com", Password: "User@123", FirstName: "Acme", LastName: "User"})
	adminGlobex, _ := userSvc.Create(ctx, globex.ID, user.CreateRequest{Email: "admin@globex.com", Password: "Admin@123", FirstName: "Globex", LastName: "Admin"})

	_, _ = roleSvc.AssignRolesToUser(ctx, acme.ID, adminAcme.ID, role.AssignUserRolesRequest{RoleID: &acmeAdminRole.ID})
	_, _ = roleSvc.AssignRolesToUser(ctx, acme.ID, viewerAcme.ID, role.AssignUserRolesRequest{RoleID: &acmeViewerRole.ID})
	_, _ = roleSvc.AssignRolesToUser(ctx, globex.ID, adminGlobex.ID, role.AssignUserRolesRequest{RoleID: &globexAdminRole.ID})

	return &TestSuiteEnv{
		Router:       router,
		TenantAcme:   acme,
		TenantGlobex: globex,
		AdminAcme:    adminAcme,
		ViewerAcme:   viewerAcme,
		AdminGlobex:  adminGlobex,
		UsersMap:     uRepo.users,
	}
}

func (e *TestSuiteEnv) DoRequest(t *testing.T, method, path, bearerToken string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var b []byte
	if body != nil {
		b, _ = json.Marshal(body)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+bearerToken)
	}
	w := httptest.NewRecorder()
	e.Router.ServeHTTP(w, req)
	return w
}
