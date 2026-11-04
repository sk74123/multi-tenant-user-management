package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"

	"github.com/yourusername/multi-tenant-user-management/internal/audit"
	"github.com/yourusername/multi-tenant-user-management/internal/auth"
	"github.com/yourusername/multi-tenant-user-management/internal/common"
	"github.com/yourusername/multi-tenant-user-management/internal/config"
	"github.com/yourusername/multi-tenant-user-management/internal/database"
	appmiddleware "github.com/yourusername/multi-tenant-user-management/internal/middleware"
	"github.com/yourusername/multi-tenant-user-management/internal/permission"
	"github.com/yourusername/multi-tenant-user-management/internal/role"
	"github.com/yourusername/multi-tenant-user-management/internal/tenant"
	"github.com/yourusername/multi-tenant-user-management/internal/user"
)

func main() {
	// ── Load config ───────────────────────────────────────────────────────────
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	// ── Run database migrations ───────────────────────────────────────────────
	if err := runMigrations(cfg); err != nil {
		log.Fatalf("migrations: %v", err)
	}

	// ── Connect to database ───────────────────────────────────────────────────
	ctx := context.Background()
	pool, err := database.NewPool(ctx, cfg.Database)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()
	log.Println("database connection pool established")

	// ── Wire dependencies ─────────────────────────────────────────────────────
	// In-memory permission cache (TTL 5 minutes, invalidated on RBAC changes)
	permCache := appmiddleware.NewPermissionCache(5 * time.Minute)

	// Tenant
	tenantRepo := tenant.NewRepository(pool)
	tenantSvc := tenant.NewService(tenantRepo)

	// User
	userRepo := user.NewRepository(pool)
	userSvc := user.NewService(userRepo, cfg.Bcrypt.Cost)

	// Permission
	permissionRepo := permission.NewRepository(pool)
	permissionSvc := permission.NewService(permissionRepo)

	// Role (RBAC)
	roleRepo := role.NewRepository(pool)
	roleSvc := role.NewService(roleRepo, permissionRepo, userRepo, permCache)

	// Auth (JWT + Refresh Tokens)
	jwtManager := auth.NewJWTManager(cfg.JWT)
	authRepo := auth.NewRepository(pool)
	authSvc := auth.NewService(authRepo, userRepo, tenantRepo, jwtManager, cfg.Bcrypt.Cost)
	authHandler := auth.NewHandler(authSvc)

	// Audit
	auditRepo := audit.NewRepository(pool)
	auditSvc := audit.NewService(auditRepo)

	// Permission authorizer
	permAuth := appmiddleware.NewPermissionAuthorizer(roleRepo, permCache)

	// Handlers with per-route permission enforcement
	tenantHandler := tenant.NewHandler(tenantSvc).WithPermissionCheck(permAuth.Require)
	userHandler := user.NewHandler(userSvc).WithPermissionCheck(permAuth.Require)
	roleHandler := role.NewHandler(roleSvc).WithPermissionCheck(permAuth.Require)
	permissionHandler := permission.NewHandler(permissionSvc).WithPermissionCheck(permAuth.Require)
	auditHandler := audit.NewHandler(auditSvc).WithPermissionCheck(permAuth.Require)

	// ── Build router ──────────────────────────────────────────────────────────
	r := chi.NewRouter()

	// Global middleware
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.Timeout(60 * time.Second))

	// Health check (unauthenticated)
	r.Get("/health", healthHandler(pool))

	// API v1
	r.Route("/api/v1", func(r chi.Router) {
		// Automatic mutation audit logging middleware
		r.Use(appmiddleware.Audit(auditSvc))

		// Public auth routes (no JWT required)
		authHandler.RegisterPublicRoutes(r)

		// Protected routes (JWT required; tenant_id comes strictly from token)
		r.Group(func(r chi.Router) {
			r.Use(appmiddleware.Authenticate(jwtManager))

			authHandler.RegisterProtectedRoutes(r)
			tenantHandler.RegisterRoutes(r)
			userHandler.RegisterRoutes(r)
			roleHandler.RegisterRoutes(r)
			permissionHandler.RegisterRoutes(r)
			auditHandler.RegisterRoutes(r)
		})
	})

	// ── Start HTTP server ─────────────────────────────────────────────────────
	srv := &http.Server{
		Addr:         ":" + cfg.Server.Port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("server listening on :%s (env=%s)", cfg.Server.Port, cfg.Server.Env)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-quit
	log.Println("shutting down server...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("forced shutdown: %v", err)
	}
	log.Println("server stopped")
}

// healthHandler returns a simple liveness + DB readiness check.
func healthHandler(pool interface{ Ping(context.Context) error }) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		status := "ok"
		dbStatus := "ok"

		if err := pool.Ping(ctx); err != nil {
			dbStatus = "unavailable"
			status = "degraded"
		}

		code := http.StatusOK
		if status == "degraded" {
			code = http.StatusServiceUnavailable
		}

		common.JSON(w, code, map[string]string{
			"status":   status,
			"database": dbStatus,
		})
	}
}

// runMigrations applies all pending up migrations from the ./migrations directory.
func runMigrations(cfg *config.Config) error {
	dsn := fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=%s",
		cfg.Database.User,
		cfg.Database.Password,
		cfg.Database.Host,
		cfg.Database.Port,
		cfg.Database.DBName,
		cfg.Database.SSLMode,
	)

	m, err := migrate.New("file://migrations", dsn)
	if err != nil {
		return fmt.Errorf("create migrator: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("apply migrations: %w", err)
	}

	log.Println("migrations applied successfully")
	return nil
}
