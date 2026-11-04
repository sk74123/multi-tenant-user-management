package role

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
	"github.com/yourusername/multi-tenant-user-management/internal/common"
	"github.com/yourusername/multi-tenant-user-management/internal/domain"
	"github.com/yourusername/multi-tenant-user-management/internal/middleware"
	"github.com/yourusername/multi-tenant-user-management/internal/permission"
	"github.com/yourusername/multi-tenant-user-management/internal/user"
)

type memRoleRepo struct {
	roles     map[uuid.UUID]*domain.Role
	rolePerms map[uuid.UUID]map[uuid.UUID]bool
	userRoles map[string]map[uuid.UUID]bool // key: tenantID:userID -> set of roleID
	permsByID map[uuid.UUID]*domain.Permission
}

func newMemRoleRepo(perms map[uuid.UUID]*domain.Permission) *memRoleRepo {
	return &memRoleRepo{
		roles:     make(map[uuid.UUID]*domain.Role),
		rolePerms: make(map[uuid.UUID]map[uuid.UUID]bool),
		userRoles: make(map[string]map[uuid.UUID]bool),
		permsByID: perms,
	}
}

func (m *memRoleRepo) Create(_ context.Context, r *domain.Role) (*domain.Role, error) {
	created := &domain.Role{
		ID:          uuid.New(),
		TenantID:    r.TenantID,
		Name:        r.Name,
		Description: r.Description,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	m.roles[created.ID] = created
	return created, nil
}

func (m *memRoleRepo) GetByID(_ context.Context, tenantID, roleID uuid.UUID) (*domain.Role, error) {
	if r, ok := m.roles[roleID]; ok && r.TenantID == tenantID {
		return r, nil
	}
	return nil, common.ErrNotFound
}

func (m *memRoleRepo) GetByName(_ context.Context, tenantID uuid.UUID, name string) (*domain.Role, error) {
	for _, r := range m.roles {
		if r.TenantID == tenantID && r.Name == name {
			return r, nil
		}
	}
	return nil, common.ErrNotFound
}

func (m *memRoleRepo) Update(_ context.Context, role *domain.Role) (*domain.Role, error) {
	if existing, ok := m.roles[role.ID]; ok && existing.TenantID == role.TenantID {
		existing.Name = role.Name
		existing.Description = role.Description
		existing.UpdatedAt = time.Now().UTC()
		return existing, nil
	}
	return nil, common.ErrNotFound
}

func (m *memRoleRepo) Delete(_ context.Context, tenantID, roleID uuid.UUID) error {
	if existing, ok := m.roles[roleID]; ok && existing.TenantID == tenantID {
		delete(m.roles, roleID)
		return nil
	}
	return common.ErrNotFound
}

func (m *memRoleRepo) List(_ context.Context, tenantID uuid.UUID, _ ListParams) ([]*domain.Role, int64, error) {
	out := make([]*domain.Role, 0)
	for _, r := range m.roles {
		if r.TenantID == tenantID {
			out = append(out, r)
		}
	}
	return out, int64(len(out)), nil
}

func (m *memRoleRepo) AssignPermission(_ context.Context, roleID, permID uuid.UUID) error {
	if m.rolePerms[roleID] == nil {
		m.rolePerms[roleID] = make(map[uuid.UUID]bool)
	}
	m.rolePerms[roleID][permID] = true
	return nil
}

func (m *memRoleRepo) RemovePermission(_ context.Context, roleID, permID uuid.UUID) error {
	if m.rolePerms[roleID] == nil || !m.rolePerms[roleID][permID] {
		return common.ErrNotFound
	}
	delete(m.rolePerms[roleID], permID)
	return nil
}

func (m *memRoleRepo) ListPermissions(_ context.Context, roleID uuid.UUID) ([]*domain.Permission, error) {
	out := make([]*domain.Permission, 0)
	for pid := range m.rolePerms[roleID] {
		if p, ok := m.permsByID[pid]; ok {
			out = append(out, p)
		}
	}
	return out, nil
}

func (m *memRoleRepo) AssignRoleToUser(_ context.Context, tenantID, userID, roleID uuid.UUID) error {
	key := tenantID.String() + ":" + userID.String()
	if m.userRoles[key] == nil {
		m.userRoles[key] = make(map[uuid.UUID]bool)
	}
	m.userRoles[key][roleID] = true
	return nil
}

func (m *memRoleRepo) RemoveRoleFromUser(_ context.Context, tenantID, userID, roleID uuid.UUID) error {
	key := tenantID.String() + ":" + userID.String()
	if m.userRoles[key] == nil || !m.userRoles[key][roleID] {
		return common.ErrNotFound
	}
	delete(m.userRoles[key], roleID)
	return nil
}

func (m *memRoleRepo) ListUserRoles(_ context.Context, tenantID, userID uuid.UUID) ([]*domain.Role, error) {
	key := tenantID.String() + ":" + userID.String()
	out := make([]*domain.Role, 0)
	for rid := range m.userRoles[key] {
		if r, ok := m.roles[rid]; ok && r.TenantID == tenantID {
			out = append(out, r)
		}
	}
	return out, nil
}

func (m *memRoleRepo) ListEffectivePermissions(_ context.Context, tenantID, userID uuid.UUID) ([]*domain.Permission, error) {
	key := tenantID.String() + ":" + userID.String()
	seen := make(map[uuid.UUID]bool)
	out := make([]*domain.Permission, 0)
	for rid := range m.userRoles[key] {
		if r, ok := m.roles[rid]; ok && r.TenantID == tenantID {
			for pid := range m.rolePerms[rid] {
				if !seen[pid] {
					seen[pid] = true
					if p, ok := m.permsByID[pid]; ok {
						out = append(out, p)
					}
				}
			}
		}
	}
	return out, nil
}

type memPermRepo struct {
	perms map[uuid.UUID]*domain.Permission
}

func (m *memPermRepo) Create(_ context.Context, p *domain.Permission) (*domain.Permission, error) {
	return p, nil
}
func (m *memPermRepo) GetByID(_ context.Context, id uuid.UUID) (*domain.Permission, error) {
	if p, ok := m.perms[id]; ok {
		return p, nil
	}
	return nil, common.ErrNotFound
}
func (m *memPermRepo) GetByResourceAction(_ context.Context, _, _ string) (*domain.Permission, error) {
	return nil, common.ErrNotFound
}
func (m *memPermRepo) List(_ context.Context, _ permission.ListParams) ([]*domain.Permission, int64, error) {
	return nil, 0, nil
}

type memUserRepo struct {
	tenantUsers map[string]*domain.User
}

func (m *memUserRepo) GetByEmail(_ context.Context, _ string) (*domain.User, error) {
	return nil, common.ErrNotFound
}
func (m *memUserRepo) Create(_ context.Context, u *domain.User) (*domain.User, error) { return u, nil }
func (m *memUserRepo) GetByIDInTenant(_ context.Context, tenantID, userID uuid.UUID) (*domain.User, error) {
	if u, ok := m.tenantUsers[tenantID.String()+":"+userID.String()]; ok {
		return u, nil
	}
	return nil, common.ErrNotFound
}
func (m *memUserRepo) Update(_ context.Context, u *domain.User) (*domain.User, error) { return u, nil }
func (m *memUserRepo) SoftDelete(_ context.Context, _, _ uuid.UUID) error            { return nil }
func (m *memUserRepo) List(_ context.Context, _ uuid.UUID, _ user.ListParams) ([]*domain.User, int64, error) {
	return nil, 0, nil
}
func (m *memUserRepo) AddToTenant(_ context.Context, _, _ uuid.UUID) error { return nil }
func (m *memUserRepo) IsInTenant(_ context.Context, tenantID, userID uuid.UUID) (bool, error) {
	_, ok := m.tenantUsers[tenantID.String()+":"+userID.String()]
	return ok, nil
}

func TestRBACFlowAndCrossTenantIsolation(t *testing.T) {
	tenantA := uuid.New()
	tenantB := uuid.New()
	userA := uuid.New()

	permRead := &domain.Permission{ID: uuid.New(), Resource: "user", Action: "read"}
	permWrite := &domain.Permission{ID: uuid.New(), Resource: "user", Action: "update"}
	perms := map[uuid.UUID]*domain.Permission{
		permRead.ID:  permRead,
		permWrite.ID: permWrite,
	}

	roleRepo := newMemRoleRepo(perms)
	permRepo := &memPermRepo{perms: perms}
	userRepo := &memUserRepo{
		tenantUsers: map[string]*domain.User{
			tenantA.String() + ":" + userA.String(): {ID: userA, Email: "a@acme.com", IsActive: true},
		},
	}

	svc := NewService(roleRepo, permRepo, userRepo)
	h := NewHandler(svc)
	uh := user.NewHandler(user.NewService(userRepo, 4))

	r := chi.NewRouter()
	r.Use(middleware.TenantFromHeader)
	uh.RegisterRoutes(r)
	h.RegisterRoutes(r)

	// 1. Create two roles in Tenant A
	role1 := createRoleViaHTTP(t, r, tenantA, `{"name":"viewer","description":"Read only"}`)
	role2 := createRoleViaHTTP(t, r, tenantA, `{"name":"editor","description":"Can edit"}`)

	// 2. Accessing Tenant A's role from Tenant B returns 404 Not Found
	reqCross := httptest.NewRequest(http.MethodGet, "/roles/"+role1.ID.String(), nil)
	reqCross.Header.Set("X-Tenant-ID", tenantB.String())
	wCross := httptest.NewRecorder()
	r.ServeHTTP(wCross, reqCross)
	if wCross.Code != http.StatusNotFound {
		t.Fatalf("expected 404 on cross-tenant role access, got %d", wCross.Code)
	}

	// 3. Assign permRead to role1, and both permRead + permWrite to role2
	reqPerm1 := httptest.NewRequest(http.MethodPost, "/roles/"+role1.ID.String()+"/permissions",
		bytes.NewReader([]byte(`{"permission_id":"`+permRead.ID.String()+`"}`)))
	reqPerm1.Header.Set("X-Tenant-ID", tenantA.String())
	wPerm1 := httptest.NewRecorder()
	r.ServeHTTP(wPerm1, reqPerm1)
	if wPerm1.Code != http.StatusOK {
		t.Fatalf("expected 200 on assign perm1, got %d: %s", wPerm1.Code, wPerm1.Body.String())
	}

	reqPerm2 := httptest.NewRequest(http.MethodPost, "/roles/"+role2.ID.String()+"/permissions",
		bytes.NewReader([]byte(`{"permission_ids":["`+permRead.ID.String()+`","`+permWrite.ID.String()+`"]}`)))
	reqPerm2.Header.Set("X-Tenant-ID", tenantA.String())
	wPerm2 := httptest.NewRecorder()
	r.ServeHTTP(wPerm2, reqPerm2)
	if wPerm2.Code != http.StatusOK {
		t.Fatalf("expected 200 on assign perm2, got %d: %s", wPerm2.Code, wPerm2.Body.String())
	}

	// 4. Assign both roles to userA in Tenant A
	reqAssign := httptest.NewRequest(http.MethodPost, "/users/"+userA.String()+"/roles",
		bytes.NewReader([]byte(`{"role_ids":["`+role1.ID.String()+`","`+role2.ID.String()+`"]}`)))
	reqAssign.Header.Set("X-Tenant-ID", tenantA.String())
	wAssign := httptest.NewRecorder()
	r.ServeHTTP(wAssign, reqAssign)
	if wAssign.Code != http.StatusOK {
		t.Fatalf("expected 200 on assign user roles, got %d: %s", wAssign.Code, wAssign.Body.String())
	}

	// 5. Query effective permissions for userA in Tenant A -> union of role1 & role2 (2 unique permissions)
	reqEff := httptest.NewRequest(http.MethodGet, "/users/"+userA.String()+"/permissions", nil)
	reqEff.Header.Set("X-Tenant-ID", tenantA.String())
	wEff := httptest.NewRecorder()
	r.ServeHTTP(wEff, reqEff)
	if wEff.Code != http.StatusOK {
		t.Fatalf("expected 200 on effective permissions, got %d: %s", wEff.Code, wEff.Body.String())
	}

	var effEnv struct {
		Success bool                `json:"success"`
		Data    []domain.Permission `json:"data"`
	}
	if err := json.Unmarshal(wEff.Body.Bytes(), &effEnv); err != nil {
		t.Fatalf("unmarshal effective perms: %v", err)
	}
	if !effEnv.Success || len(effEnv.Data) != 2 {
		t.Fatalf("expected 2 effective permissions, got %+v", effEnv)
	}

	// 6. Query userA's roles from Tenant B -> 404 Not Found (cross-tenant isolation)
	reqCrossUser := httptest.NewRequest(http.MethodGet, "/users/"+userA.String()+"/roles", nil)
	reqCrossUser.Header.Set("X-Tenant-ID", tenantB.String())
	wCrossUser := httptest.NewRecorder()
	r.ServeHTTP(wCrossUser, reqCrossUser)
	if wCrossUser.Code != http.StatusNotFound {
		t.Fatalf("expected 404 on cross-tenant user roles access, got %d", wCrossUser.Code)
	}
}

func createRoleViaHTTP(t *testing.T, r http.Handler, tenantID uuid.UUID, payload string) domain.Role {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/roles", bytes.NewReader([]byte(payload)))
	req.Header.Set("X-Tenant-ID", tenantID.String())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 on create role, got %d: %s", w.Code, w.Body.String())
	}

	var env struct {
		Success bool        `json:"success"`
		Data    domain.Role `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal role: %v", err)
	}
	return env.Data
}


