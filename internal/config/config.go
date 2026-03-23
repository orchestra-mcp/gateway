package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config holds the unified gateway configuration merged from apps/web and apps/cloud-mcp.
type Config struct {
	// Database (shared PostgreSQL).
	DSN string

	// Auth — JWT secret shared across services.
	JWTSecret string

	// Server.
	Port int
	Env  string

	// CORS.
	AllowedOrigins []string

	// File uploads.
	UploadDir string

	// Repo workspace base directory.
	RepoBaseDir string

	// Web app base URL for cloud-mcp admin tool forwarding.
	WebAPIBaseURL string

	// Rate limiting.
	PublicRateLimit int // requests per minute per IP
}

// Load reads configuration from environment variables with sensible defaults.
func Load() *Config {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://postgres:postgres@localhost:5432/orchestra_web?sslmode=disable"
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "orchestra-secret-change-in-production"
	}

	port := 8080
	if p := os.Getenv("PORT"); p != "" {
		if n, err := strconv.Atoi(p); err == nil {
			port = n
		}
	}

	env := os.Getenv("APP_ENV")
	if env == "" {
		env = "development"
	}

	repoBaseDir := os.Getenv("REPO_BASE_DIR")
	if repoBaseDir == "" {
		repoBaseDir = "/var/orchestra/repos"
	}

	uploadDir := os.Getenv("UPLOAD_DIR")
	if uploadDir == "" {
		uploadDir = "uploads"
	}
	// Resolve to absolute path so file serving works regardless of working directory.
	if !filepath.IsAbs(uploadDir) {
		if abs, err := filepath.Abs(uploadDir); err == nil {
			uploadDir = abs
		}
	}

	webAPI := os.Getenv("WEB_API_BASE_URL")
	if webAPI == "" {
		webAPI = "https://orchestra-mcp.dev"
	}
	// Strip trailing /api if misconfigured in env — tools append /api/... themselves.
	webAPI = strings.TrimSuffix(strings.TrimRight(webAPI, "/"), "/api")

	rateLimit := 10
	if r := os.Getenv("PUBLIC_RATE_LIMIT"); r != "" {
		if n, err := strconv.Atoi(r); err == nil {
			rateLimit = n
		}
	}

	return &Config{
		DSN:            dsn,
		JWTSecret:      jwtSecret,
		Port:           port,
		Env:            env,
		AllowedOrigins: parseOrigins(os.Getenv("ALLOWED_ORIGINS")),
		UploadDir:      uploadDir,
		RepoBaseDir:    repoBaseDir,
		WebAPIBaseURL:  webAPI,
		PublicRateLimit: rateLimit,
	}
}

// parseOrigins splits a comma-separated list of origins.
// Returns defaults for local development if the input is empty.
func parseOrigins(raw string) []string {
	if raw == "" {
		return []string{
			"https://orchestra-mcp.dev",
			"https://www.orchestra-mcp.dev",
			"https://app.orchestra-mcp.dev",
			"http://localhost:3000",
			"http://localhost:5173",
			"http://localhost:8080",
		}
	}
	parts := strings.Split(raw, ",")
	origins := make([]string, 0, len(parts))
	for _, p := range parts {
		if o := strings.TrimSpace(p); o != "" {
			origins = append(origins, o)
		}
	}
	return origins
}
