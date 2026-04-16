// Package main provides the main entry point for the Brokle API server.
//
// This is the HTTP API server that handles:
// - HTTP API endpoints
// - WebSocket real-time connections
// - Multi-database operations (PostgreSQL + ClickHouse)
// - Database migrations (server owns migrations)
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "brokle/docs" // swagger docs
	"brokle/internal/app"
	"brokle/internal/config"
	"brokle/internal/migration"
)

// @title Brokle AI Control Plane API
// @version 1.0.0
// @description The Open-Source AI Control Plane - See Everything. Control Everything. Observability, routing, and governance for AI.
// @termsOfService https://brokle.ai/terms
//
// @contact.name Brokle Platform Team
// @contact.url https://brokle.ai/support
// @contact.email support@brokle.ai
//
// @license.name MIT License
// @license.url https://opensource.org/licenses/MIT
//
// @host localhost:8080
// @schemes http https
//
// @securityDefinitions.apikey ApiKeyAuth
// @in header
// @name Authorization
// @description API key authentication for AI gateway and SDKs. Format: Authorization: Bearer bk_live_... OR X-API-Key: bk_live_... (both supported for flexibility)
//
// @securityDefinitions.apikey CookieAuth
// @in header
// @name Cookie
// @description Cookie-based JWT authentication. Login/Signup set httpOnly cookies (access_token, refresh_token, csrf_token). Browser sends automatically. Testing: Use browser DevTools/Postman/cURL (Swagger UI cannot test cookies).
//
// @securityDefinitions.apikey CSRFToken
// @in header
// @name X-CSRF-Token
// @description CSRF protection for mutations (POST/PUT/PATCH/DELETE). Value must match csrf_token cookie. Required for all non-idempotent operations.
//
// Custom type definitions for Swagger
// @x-extension-openapi {"definitions": {"UUID": {"type": "string", "format": "uuid", "description": "UUIDv7 (RFC 9562)", "example": "018f6b6a-1234-7abc-8def-0123456789ab", "pattern": "^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$"}}}
func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// SERVER OWNS MIGRATIONS - Run before app initialization
	if cfg.Database.AutoMigrate {
		log.Println("Running database migrations...")

		migrationManager, migErr := migration.NewManager(cfg)
		if migErr != nil {
			log.Fatalf("Failed to initialize migration manager: %v", migErr)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		if err := migrationManager.AutoMigrate(ctx); err != nil {
			log.Fatalf("Auto-migration failed: %v", err)
		}

		if err := migrationManager.Shutdown(); err != nil {
			log.Printf("Warning: failed to shutdown migration manager: %v", err)
		}

		log.Println("Migrations completed successfully")
	}

	// Initialize server application (HTTP only, no workers)
	application, err := app.NewServer(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize server: %v", err)
	}

	// Start the HTTP server
	if err := application.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println("Shutting down server...")

	// Graceful shutdown with 30 second timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := application.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	fmt.Println("Server stopped")
}
