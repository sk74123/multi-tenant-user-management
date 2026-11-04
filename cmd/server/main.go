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
	"github.com/go-chi/chi/v5/middleware"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"

	"github.com/yourusername/multi-tenant-user-management/internal/common"
	"github.com/yourusername/multi-tenant-user-management/internal/config"
	"github.com/yourusername/multi-tenant-user-management/internal/database"
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

	// ── Build router ──────────────────────────────────────────────────────────
	r := chi.NewRouter()

	// Global middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	// Health check (unauthenticated)
	r.Get("/health", healthHandler(pool))

	// API v1 (domain routes will be mounted here in later phases)
	r.Route("/api/v1", func(r chi.Router) {
		// Placeholder — routes will be registered in Phases 2–5
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			common.OK(w, map[string]string{"message": "Multi-Tenant User Management System API v1"})
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

	// Graceful shutdown
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

