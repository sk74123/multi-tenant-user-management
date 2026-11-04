package middleware

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/yourusername/multi-tenant-user-management/internal/common"
	"github.com/yourusername/multi-tenant-user-management/internal/domain"
)

// EffectivePermissionResolver fetches a user's effective permissions within a tenant.
type EffectivePermissionResolver interface {
	ListEffectivePermissions(ctx context.Context, tenantID, userID uuid.UUID) ([]*domain.Permission, error)
}

type cacheEntry struct {
	perms     map[string]bool
	expiresAt time.Time
}

// PermissionCache is a thread-safe in-memory permission cache with TTL and invalidation.
type PermissionCache struct {
	mu      sync.RWMutex
	ttl     time.Duration
	entries map[string]cacheEntry
}

// NewPermissionCache creates a new PermissionCache with the given TTL.
func NewPermissionCache(ttl time.Duration) *PermissionCache {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &PermissionCache{
		ttl:     ttl,
		entries: make(map[string]cacheEntry),
	}
}

func cacheKey(tenantID, userID uuid.UUID) string {
	return tenantID.String() + ":" + userID.String()
}

// Get returns the cached permission set for a user in a tenant if present and not expired.
func (c *PermissionCache) Get(tenantID, userID uuid.UUID) (map[string]bool, bool) {
	if c == nil {
		return nil, false
	}
	key := cacheKey(tenantID, userID)

	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()

	if !ok {
		return nil, false
	}
	if time.Now().UTC().After(entry.expiresAt) {
		c.mu.Lock()
		delete(c.entries, key)
		c.mu.Unlock()
		return nil, false
	}

	return entry.perms, true
}

// Set stores the effective permission set for a user in a tenant with the configured TTL.
func (c *PermissionCache) Set(tenantID, userID uuid.UUID, perms map[string]bool) {
	if c == nil {
		return
	}
	copied := make(map[string]bool, len(perms))
	for k, v := range perms {
		copied[k] = v
	}

	c.mu.Lock()
	c.entries[cacheKey(tenantID, userID)] = cacheEntry{
		perms:     copied,
		expiresAt: time.Now().UTC().Add(c.ttl),
	}
	c.mu.Unlock()
}

// InvalidateUser removes the cached permissions for a specific user in a tenant.
func (c *PermissionCache) InvalidateUser(tenantID, userID uuid.UUID) {
	if c == nil {
		return
	}
	c.mu.Lock()
	delete(c.entries, cacheKey(tenantID, userID))
	c.mu.Unlock()
}

// InvalidateTenant removes all cached permissions for all users within a tenant.
func (c *PermissionCache) InvalidateTenant(tenantID uuid.UUID) {
	if c == nil {
		return
	}
	prefix := tenantID.String() + ":"
	c.mu.Lock()
	for k := range c.entries {
		if strings.HasPrefix(k, prefix) {
			delete(c.entries, k)
		}
	}
	c.mu.Unlock()
}

// InvalidateAll clears the entire permission cache.
func (c *PermissionCache) InvalidateAll() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.entries = make(map[string]cacheEntry)
	c.mu.Unlock()
}

// PermissionAuthorizer checks whether the authenticated user has required permissions.
type PermissionAuthorizer struct {
	resolver EffectivePermissionResolver
	cache    *PermissionCache
}

// NewPermissionAuthorizer creates a PermissionAuthorizer backed by a resolver and cache.
func NewPermissionAuthorizer(resolver EffectivePermissionResolver, cache *PermissionCache) *PermissionAuthorizer {
	return &PermissionAuthorizer{
		resolver: resolver,
		cache:    cache,
	}
}

// Require returns an HTTP middleware that enforces that the authenticated user holds
// at least one of the specified resource:action permissions in the active tenant.
// Returns 401 if unauthenticated, or 403 if the user lacks the required permission.
func (a *PermissionAuthorizer) Require(requiredPerms ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tenantID := common.GetTenantID(r.Context())
			userID := common.GetUserID(r.Context())
			if tenantID == uuid.Nil || userID == uuid.Nil {
				common.Error(w, common.ErrUnauthorized)
				return
			}

			permSet, ok := a.cache.Get(tenantID, userID)
			if !ok {
				perms, err := a.resolver.ListEffectivePermissions(r.Context(), tenantID, userID)
				if err != nil {
					common.Error(w, common.AsAppError(err))
					return
				}
				permSet = make(map[string]bool, len(perms))
				for _, p := range perms {
					permSet[p.Slug()] = true
				}
				a.cache.Set(tenantID, userID, permSet)
			}

			for _, reqPerm := range requiredPerms {
				if permSet[reqPerm] {
					next.ServeHTTP(w, r)
					return
				}
			}

			common.Error(w, common.ErrForbidden)
		})
	}
}
