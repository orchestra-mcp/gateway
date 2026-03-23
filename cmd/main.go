package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/orchestra-mcp/gateway/internal/config"
	"github.com/orchestra-mcp/gateway/internal/routes"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func main() {
	dbDSN := flag.String("db-dsn", "", "PostgreSQL DSN (overrides DATABASE_URL env var)")
	flag.Parse()

	cfg := config.Load()
	if *dbDSN != "" {
		cfg.DSN = *dbDSN
	}

	// Connect to database.
	db, err := gorm.Open(postgres.Open(cfg.DSN), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Warn),
	})
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}

	// Configure connection pool.
	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("failed to get underlying sql.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetConnMaxLifetime(5 * time.Minute)

	// NOTE: No AutoMigrate — the gateway assumes the schema is managed
	// externally (via the web app or migration tooling).

	// Build Fiber app.
	app := fiber.New(fiber.Config{
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
		AppName:      "Orchestra Gateway",
	})

	// Register all routes (tunnel, health, MCP, smart actions).
	routes.Register(app, db, cfg)

	// Graceful shutdown.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		addr := fmt.Sprintf(":%d", cfg.Port)
		log.Printf("Orchestra Gateway listening on %s (MCP protocol 2025-11-25)", addr)
		if err := app.Listen(addr, fiber.ListenConfig{
			DisableStartupMessage: true,
		}); err != nil {
			log.Printf("server stopped: %v", err)
		}
	}()

	<-quit
	log.Println("shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := app.ShutdownWithContext(ctx); err != nil {
		log.Fatalf("server forced to shutdown: %v", err)
	}

	log.Println("server exited gracefully")
}
