package tests

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yourusername/multi-tenant-user-management/internal/audit"
	"github.com/yourusername/multi-tenant-user-management/internal/common"
	"github.com/yourusername/multi-tenant-user-management/internal/domain"
	"github.com/yourusername/multi-tenant-user-management/internal/permission"
	"github.com/yourusername/multi-tenant-user-management/internal/role"
	"github.com/yourusername/multi-tenant-user-management/internal/tenant"
	"github.com/yourusername/multi-tenant-user-management/internal/user"
)

type inMemTenantRepo struct {
	byID   map[uuid.UUID]*domain.Tenant
	byCode map[string]*domain.Tenant
}

func newInMemTenantRepo() *inMemTenantRepo {
	return &inMemTenantRepo{byID: make(map[uuid.UUID]*domain.Tenant), byCode: make(map[string]*domain.Tenant)}
}

func (m *inMemTenantRepo) Create(_ context.Context, t *domain.Tenant) (*domain.Tenant, error) {
	created := &domain.Tenant{ID: uuid.New(), Name: t.Name, Code: t.Code, Status: t.Status, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	m.byID[created.ID] = created
	m.byCode[created.Code] = created
	cp := *created
	return &cp, nil
}

func (m *inMemTenantRepo) GetByID(_ context.Context, id uuid.UUID) (*domain.Tenant, error) {
	if t, ok := m.byID[id]; ok {
		cp := *t
		return &cp, nil
	}
	return nil, common.ErrNotFound
}

func (m *inMemTenantRepo) GetByCode(_ context.Context, code string) (*domain.Tenant, error) {
	if t, ok := m.byCode[code]; ok {
		cp := *t
		return &cp, nil
	}
	return nil, common.ErrNotFound
}

func (m *inMemTenantRepo) Update(_ context.Context, t *domain.Tenant) (*domain.Tenant, error) {
	existing, ok := m.byID[t.ID]
	if !ok {
		return nil, common.ErrNotFound
	}
	existing.Name = t.Name
	existing.Status = t.Status
	existing.UpdatedAt = time.Now().UTC()
	cp := *existing
	return &cp, nil
}

func (m *inMemTenantRepo) List(_ context.Context, _ tenant.ListParams) ([]*domain.Tenant, int64, error) {
	out := make([]*domain.Tenant, 0, len(m.byID))
	for _, t := range m.byID {
		cp := *t
		out = append(out, &cp)
	}
	return out, int64(len(out)), nil
}

type inMemUserRepo struct {
	users       map[uuid.UUID]*domain.User
	byEmail     map[string]*domain.User
	memberships map[string]bool
}

func newInMemUserRepo() *inMemUserRepo {
	return &inMemUserRepo{users: make(map[uuid.UUID]*domain.User), byEmail: make(map[string]*domain.User), memberships: make(map[string]bool)}
}

func (m *inMemUserRepo) GetByEmail(_ context.Context, email string) (*domain.User, error) {
	if u, ok := m.byEmail[email]; ok && u.DeletedAt == nil {
		cp := *u
		return &cp, nil
	}
	return nil, common.ErrNotFound
}

func (m *inMemUserRepo) Create(_ context.Context, u *domain.User) (*domain.User, error) {
	created := &domain.User{
		ID: uuid.New(), Email: u.Email, PasswordHash: u.PasswordHash,
		FirstName: u.FirstName, LastName: u.LastName, IsActive: u.IsActive,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	m.users[created.ID] = created
	m.byEmail[created.Email] = created
	cp := *created
	return &cp, nil
}

func (m *inMemUserRepo) GetByIDInTenant(_ context.Context, tenantID, userID uuid.UUID) (*domain.User, error) {
	if !m.memberships[tenantID.String()+":"+userID.String()] {
		return nil, common.ErrNotFound
	}
	u, ok := m.users[userID]
	if !ok || u.DeletedAt != nil {
		return nil, common.ErrNotFound
	}
	cp := *u
	return &cp, nil
}

func (m *inMemUserRepo) Update(_ context.Context, u *domain.User) (*domain.User, error) {
	existing, ok := m.users[u.ID]
	if !ok || existing.DeletedAt != nil {
		return nil, common.ErrNotFound
	}
	existing.FirstName = u.FirstName
	existing.LastName = u.LastName
	existing.IsActive = u.IsActive
	existing.UpdatedAt = time.Now().UTC()
	cp := *existing
	return &cp, nil
}

func (m *inMemUserRepo) SoftDelete(_ context.Context, tenantID, userID uuid.UUID) error {
	if !m.memberships[tenantID.String()+":"+userID.String()] {
		return common.ErrNotFound
	}
	u, ok := m.users[userID]
	if !ok || u.DeletedAt != nil {
		return common.ErrNotFound
	}
	now := time.Now().UTC()
	u.DeletedAt = &now
	return nil
}

func (m *inMemUserRepo) List(_ context.Context, tenantID uuid.UUID, params user.ListParams) ([]*domain.User, int64, error) {
	out := make([]*domain.User, 0)
	for _, u := range m.users {
		if u.DeletedAt != nil || !m.memberships[tenantID.String()+":"+u.ID.String()] {
			continue
		}
		if params.Search != "" && !strings.Contains(strings.ToLower(u.Email), strings.ToLower(params.Search)) {
			continue
		}
		cp := *u
		out = append(out, &cp)
	}
	return out, int64(len(out)), nil
}

func (m *inMemUserRepo) AddToTenant(_ context.Context, tenantID, userID uuid.UUID) error {
	m.memberships[tenantID.String()+":"+userID.String()] = true
	return nil
}

func (m *inMemUserRepo) IsInTenant(_ context.Context, tenantID, userID uuid.UUID) (bool, error) {
	return m.memberships[tenantID.String()+":"+userID.String()], nil
}

type inMemPermRepo struct {
	byID  map[uuid.UUID]*domain.Permission
	byKey map[string]*domain.Permission
}

func newInMemPermRepo() *inMemPermRepo {
	return &inMemPermRepo{byID: make(map[uuid.UUID]*domain.Permission), byKey: make(map[string]*domain.Permission)}
}

func (m *inMemPermRepo) Create(_ context.Context, p *domain.Permission) (*domain.Permission, error) {
	created := &domain.Permission{ID: uuid.New(), Resource: p.Resource, Action: p.Action, Description: p.Description, CreatedAt: time.Now().UTC()}
	m.byID[created.ID] = created
	m.byKey[created.Slug()] = created
	return created, nil
}

func (m *inMemPermRepo) GetByID(_ context.Context, id uuid.UUID) (*domain.Permission, error) {
	if p, ok := m.byID[id]; ok {
		return p, nil
	}
	return nil, common.ErrNotFound
}

func (m *inMemPermRepo) GetByResourceAction(_ context.Context, r, a string) (*domain.Permission, error) {
	if p, ok := m.byKey[r+":"+a]; ok {
		return p, nil
	}
	return nil, common.ErrNotFound
}

func (m *inMemPermRepo) List(_ context.Context, _ permission.ListParams) ([]*domain.Permission, int64, error) {
	out := make([]*domain.Permission, 0, len(m.byID))
	for _, p := range m.byID {
		out = append(out, p)
	}
	return out, int64(len(out)), nil
}

type inMemAuthRepo struct {
	byHash map[string]*domain.RefreshToken
}

func newInMemAuthRepo() *inMemAuthRepo {
	return &inMemAuthRepo{byHash: make(map[string]*domain.RefreshToken)}
}

func (m *inMemAuthRepo) CreateRefreshToken(_ context.Context, rt *domain.RefreshToken) (*domain.RefreshToken, error) {
	cp := *rt
	cp.ID = uuid.New()
	cp.CreatedAt = time.Now().UTC()
	m.byHash[cp.TokenHash] = &cp
	return &cp, nil
}

func (m *inMemAuthRepo) GetRefreshTokenByHash(_ context.Context, h string) (*domain.RefreshToken, error) {
	if rt, ok := m.byHash[h]; ok {
		return rt, nil
	}
	return nil, common.ErrNotFound
}

func (m *inMemAuthRepo) RevokeRefreshToken(_ context.Context, h string) error {
	rt, ok := m.byHash[h]
	if !ok || rt.Revoked {
		return common.ErrNotFound
	}
	rt.Revoked = true
	return nil
}

func (m *inMemAuthRepo) RevokeAllUserRefreshTokens(_ context.Context, tenantID, userID uuid.UUID) error {
	for _, rt := range m.byHash {
		if rt.TenantID == tenantID && rt.UserID == userID {
			rt.Revoked = true
		}
	}
	return nil
}

type inMemAuditRepo struct {
	logs []*domain.AuditLog
}

func newInMemAuditRepo() *inMemAuditRepo { return &inMemAuditRepo{} }

func (m *inMemAuditRepo) Create(_ context.Context, e *domain.AuditLog) (*domain.AuditLog, error) {
	cp := *e
	cp.ID = uuid.New()
	cp.CreatedAt = time.Now().UTC()
	m.logs = append(m.logs, &cp)
	return &cp, nil
}

func (m *inMemAuditRepo) List(_ context.Context, tenantID uuid.UUID, params audit.ListParams) ([]*domain.AuditLog, int64, error) {
	out := make([]*domain.AuditLog, 0)
	for _, l := range m.logs {
		if l.TenantID != tenantID {
			continue
		}
		if params.Action != "" && !strings.Contains(strings.ToLower(l.Action), strings.ToLower(params.Action)) {
			continue
		}
		if params.TargetType != "" && !strings.EqualFold(l.TargetType, params.TargetType) {
			continue
		}
		out = append(out, l)
	}
	return out, int64(len(out)), nil
}

type inMemRoleRepo struct {
	roles     map[uuid.UUID]*domain.Role
	rolePerms map[uuid.UUID]map[uuid.UUID]bool
	userRoles map[string]map[uuid.UUID]bool
	permRepo  *inMemPermRepo
}

func newInMemRoleRepo(p *inMemPermRepo) *inMemRoleRepo {
	return &inMemRoleRepo{
		roles:     make(map[uuid.UUID]*domain.Role),
		rolePerms: make(map[uuid.UUID]map[uuid.UUID]bool),
		userRoles: make(map[string]map[uuid.UUID]bool),
		permRepo:  p,
	}
}

func (m *inMemRoleRepo) Create(_ context.Context, r *domain.Role) (*domain.Role, error) {
	created := &domain.Role{ID: uuid.New(), TenantID: r.TenantID, Name: r.Name, Description: r.Description, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	m.roles[created.ID] = created
	return created, nil
}

func (m *inMemRoleRepo) GetByID(_ context.Context, tenantID, roleID uuid.UUID) (*domain.Role, error) {
	if r, ok := m.roles[roleID]; ok && r.TenantID == tenantID {
		cp := *r
		return &cp, nil
	}
	return nil, common.ErrNotFound
}

func (m *inMemRoleRepo) GetByName(_ context.Context, tenantID uuid.UUID, name string) (*domain.Role, error) {
	for _, r := range m.roles {
		if r.TenantID == tenantID && r.Name == name {
			cp := *r
			return &cp, nil
		}
	}
	return nil, common.ErrNotFound
}

func (m *inMemRoleRepo) Update(_ context.Context, r *domain.Role) (*domain.Role, error) {
	existing, ok := m.roles[r.ID]
	if !ok || existing.TenantID != r.TenantID {
		return nil, common.ErrNotFound
	}
	existing.Name = r.Name
	existing.Description = r.Description
	existing.UpdatedAt = time.Now().UTC()
	cp := *existing
	return &cp, nil
}

func (m *inMemRoleRepo) Delete(_ context.Context, tenantID, roleID uuid.UUID) error {
	if existing, ok := m.roles[roleID]; ok && existing.TenantID == tenantID {
		delete(m.roles, roleID)
		return nil
	}
	return common.ErrNotFound
}

func (m *inMemRoleRepo) List(_ context.Context, tenantID uuid.UUID, _ role.ListParams) ([]*domain.Role, int64, error) {
	out := make([]*domain.Role, 0)
	for _, r := range m.roles {
		if r.TenantID == tenantID {
			out = append(out, r)
		}
	}
	return out, int64(len(out)), nil
}

func (m *inMemRoleRepo) AssignPermission(_ context.Context, roleID, permID uuid.UUID) error {
	if m.rolePerms[roleID] == nil {
		m.rolePerms[roleID] = make(map[uuid.UUID]bool)
	}
	m.rolePerms[roleID][permID] = true
	return nil
}

func (m *inMemRoleRepo) RemovePermission(_ context.Context, roleID, permID uuid.UUID) error {
	if m.rolePerms[roleID] == nil || !m.rolePerms[roleID][permID] {
		return common.ErrNotFound
	}
	delete(m.rolePerms[roleID], permID)
	return nil
}

func (m *inMemRoleRepo) ListPermissions(_ context.Context, roleID uuid.UUID) ([]*domain.Permission, error) {
	out := make([]*domain.Permission, 0)
	for pid := range m.rolePerms[roleID] {
		if p, ok := m.permRepo.byID[pid]; ok {
			out = append(out, p)
		}
	}
	return out, nil
}

func (m *inMemRoleRepo) AssignRoleToUser(_ context.Context, tenantID, userID, roleID uuid.UUID) error {
	k := tenantID.String() + ":" + userID.String()
	if m.userRoles[k] == nil {
		m.userRoles[k] = make(map[uuid.UUID]bool)
	}
	m.userRoles[k][roleID] = true
	return nil
}

func (m *inMemRoleRepo) RemoveRoleFromUser(_ context.Context, tenantID, userID, roleID uuid.UUID) error {
	k := tenantID.String() + ":" + userID.String()
	if m.userRoles[k] == nil || !m.userRoles[k][roleID] {
		return common.ErrNotFound
	}
	delete(m.userRoles[k], roleID)
	return nil
}

func (m *inMemRoleRepo) ListUserRoles(_ context.Context, tenantID, userID uuid.UUID) ([]*domain.Role, error) {
	k := tenantID.String() + ":" + userID.String()
	out := make([]*domain.Role, 0)
	for rid := range m.userRoles[k] {
		if r, ok := m.roles[rid]; ok && r.TenantID == tenantID {
			out = append(out, r)
		}
	}
	return out, nil
}

func (m *inMemRoleRepo) ListEffectivePermissions(_ context.Context, tenantID, userID uuid.UUID) ([]*domain.Permission, error) {
	k := tenantID.String() + ":" + userID.String()
	seen := make(map[uuid.UUID]bool)
	out := make([]*domain.Permission, 0)
	for rid := range m.userRoles[k] {
		if r, ok := m.roles[rid]; ok && r.TenantID == tenantID {
			for pid := range m.rolePerms[rid] {
				if !seen[pid] {
					seen[pid] = true
					if p, ok := m.permRepo.byID[pid]; ok {
						out = append(out, p)
					}
				}
			}
		}
	}
	return out, nil
}


