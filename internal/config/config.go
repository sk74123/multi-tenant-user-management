package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

// Config holds all application configuration loaded from environment variables.
type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	JWT      JWTConfig
	Bcrypt   BcryptConfig
}

type ServerConfig struct {
	Port string
	Env  string
}

type DatabaseConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string
	MaxConns int32
	MinConns int32
}

type JWTConfig struct {
	AccessSecret  string
	RefreshSecret string
	AccessExpiry  time.Duration
	RefreshExpiry time.Duration
}

type BcryptConfig struct {
	Cost int
}

// DSN returns the PostgreSQL connection string.
func (d DatabaseConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		d.Host, d.Port, d.User, d.Password, d.DBName, d.SSLMode,
	)
}

// Load reads .env (if present) then environment variables and returns a Config.
func Load() (*Config, error) {
	// Best-effort: ignore error if .env doesn't exist
	_ = godotenv.Load()

	accessExpiry, err := time.ParseDuration(getEnv("JWT_ACCESS_EXPIRY", "15m"))
	if err != nil {
		return nil, fmt.Errorf("invalid JWT_ACCESS_EXPIRY: %w", err)
	}
	refreshExpiry, err := time.ParseDuration(getEnv("JWT_REFRESH_EXPIRY", "168h"))
	if err != nil {
		return nil, fmt.Errorf("invalid JWT_REFRESH_EXPIRY: %w", err)
	}

	maxConns, _ := strconv.ParseInt(getEnv("DB_MAX_CONNS", "25"), 10, 32)
	minConns, _ := strconv.ParseInt(getEnv("DB_MIN_CONNS", "5"), 10, 32)
	bcryptCost, _ := strconv.Atoi(getEnv("BCRYPT_COST", "12"))

	cfg := &Config{
		Server: ServerConfig{
			Port: getEnv("APP_PORT", "8080"),
			Env:  getEnv("APP_ENV", "development"),
		},
		Database: DatabaseConfig{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnv("DB_PORT", "5432"),
			User:     getEnv("DB_USER", "mtums"),
			Password: getEnv("DB_PASSWORD", "mtums_secret"),
			DBName:   getEnv("DB_NAME", "mtums_db"),
			SSLMode:  getEnv("DB_SSL_MODE", "disable"),
			MaxConns: int32(maxConns),
			MinConns: int32(minConns),
		},
		JWT: JWTConfig{
			AccessSecret:  getEnv("JWT_ACCESS_SECRET", ""),
			RefreshSecret: getEnv("JWT_REFRESH_SECRET", ""),
			AccessExpiry:  accessExpiry,
			RefreshExpiry: refreshExpiry,
		},
		Bcrypt: BcryptConfig{
			Cost: bcryptCost,
		},
	}

	if cfg.JWT.AccessSecret == "" {
		return nil, fmt.Errorf("JWT_ACCESS_SECRET must be set")
	}
	if cfg.JWT.RefreshSecret == "" {
		return nil, fmt.Errorf("JWT_REFRESH_SECRET must be set")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

