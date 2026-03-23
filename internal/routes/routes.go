package routes

import (
	"github.com/gofiber/fiber/v3"
	"github.com/orchestra-mcp/gateway/internal/config"
	"github.com/orchestra-mcp/gateway/internal/health"
	"github.com/orchestra-mcp/gateway/internal/mcp"
	"github.com/orchestra-mcp/gateway/internal/middleware"
	"github.com/orchestra-mcp/gateway/internal/tunnel"
	"gorm.io/gorm"
)

// Register wires all route handlers onto the Fiber app.
// It combines routes from the former apps/web (tunnel, health, actions) and
// apps/cloud-mcp (MCP Streamable HTTP transport).
func Register(app *fiber.App, db *gorm.DB, cfg *config.Config) {
	// Health check (unauthenticated, used by deploy scripts and monitoring).
	app.Get("/health", func(c fiber.Ctx) error {
		sqlDB, err := db.DB()
		if err != nil {
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"status": "unhealthy",
				"error":  "database connection unavailable",
			})
		}
		if err := sqlDB.Ping(); err != nil {
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"status": "unhealthy",
				"error":  "database ping failed",
			})
		}
		return c.JSON(fiber.Map{
			"status":   "healthy",
			"service":  "orchestra-gateway",
			"version":  "1.0.0",
			"protocol": "2025-11-25",
		})
	})

	// Global middleware.
	app.Use(middleware.Logger())
	app.Use(middleware.CORS(cfg.AllowedOrigins))

	// ── MCP Streamable HTTP transport (from apps/cloud-mcp) ──────────
	// Auth is handled per-tool inside the handler (JWT, API key, or anonymous).
	mcpHandler := mcp.NewHandler(db, cfg)
	app.Post("/mcp", mcpHandler.HandlePost)
	app.Get("/mcp", mcpHandler.HandleGet) // SSE stream

	// ── Tunnel Hub (shared between all tunnel handlers) ──────────────
	tunnelHub := tunnel.NewTunnelHub()

	// ── WebSocket routes (before auth middleware — auth done via token/params) ─
	api := app.Group("/api")
	tunnelReverseHandler := tunnel.NewTunnelReverseHandler(db, tunnelHub)
	tunnelProxyHandler := tunnel.NewTunnelProxyHandler(db, cfg, tunnelHub)
	api.Get("/tunnels/:id/ws", tunnelProxyHandler.Handle)
	api.Get("/tunnels/reverse", tunnelReverseHandler.Handle)

	// Tunnel claim (no JWT auth — nonce is the secret).
	tunnelHandler := tunnel.NewTunnelHandler(db, tunnelHub)
	api.Post("/tunnels/claim", tunnelHandler.Claim)

	// ── Authenticated routes ─────────────────────────────────────────
	protected := api.Group("", middleware.Auth(db, cfg))

	// Tunnels (CRUD + heartbeat + auto-register).
	tunnels := protected.Group("/tunnels")
	tunnels.Get("/", tunnelHandler.List)
	tunnels.Post("/register", tunnelHandler.Register)
	tunnels.Post("/auto-register", tunnelHandler.AutoRegister)
	tunnels.Post("/heartbeat", tunnelHandler.Heartbeat)
	tunnels.Get("/:id", tunnelHandler.Show)
	tunnels.Put("/:id", tunnelHandler.Update)
	tunnels.Delete("/:id", tunnelHandler.Delete)
	tunnels.Get("/:id/status", tunnelHandler.Status)

	// Smart actions (dispatch actions through tunnel to local CLI).
	smartActionHandler := tunnel.NewSmartActionHandler(db, tunnelHub)
	tunnels.Get("/:id/actions", smartActionHandler.SupportedActions)
	tunnels.Post("/:id/actions", smartActionHandler.Execute)
	tunnels.Post("/:id/action", smartActionHandler.Dispatch)
	tunnels.Get("/:id/actions/history", smartActionHandler.History)
	tunnels.Get("/:id/action-log", smartActionHandler.ActionLog)

	// Action history (across all tunnels).
	protected.Get("/actions/history", smartActionHandler.AllHistory)

	// Health Debug API.
	healthRepo := health.NewHealthRepository(db)
	healthSvc := health.NewHealthService(healthRepo)
	healthHandler := health.NewHealthHandler(healthSvc)
	health.RegisterHealthRoutes(protected, healthHandler)
}
