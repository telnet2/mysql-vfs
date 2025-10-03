package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/telnet2/mysql-vfs/pkg/db"
	"github.com/telnet2/mysql-vfs/pkg/models"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type VFSServer struct {
	db *gorm.DB
}

func main() {
	// Load configuration from environment
	dsn := getEnv("DB_DSN", "root:root@tcp(localhost:3306)/vfs?charset=utf8mb4&parseTime=True&loc=Local")
	port := getEnv("PORT", "8080")
	logLevel := getEnv("LOG_LEVEL", "info")

	// Parse log level
	gormLogLevel := logger.Info
	if logLevel == "debug" {
		gormLogLevel = logger.Info
	} else if logLevel == "warn" {
		gormLogLevel = logger.Warn
	} else if logLevel == "error" {
		gormLogLevel = logger.Error
	}

	// Connect to database
	log.Println("Connecting to database...")
	database, err := db.Connect(db.Config{
		DSN:      dsn,
		LogLevel: gormLogLevel,
	})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Run migrations
	log.Println("Running database migrations...")
	if err := db.AutoMigrate(database); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}
	log.Println("Migrations completed successfully")

	// Create server instance
	vfsServer := &VFSServer{db: database}

	// Initialize Hertz server
	h := server.Default(server.WithHostPorts(":" + port))

	// Register routes
	h.GET("/health", vfsServer.healthHandler)
	h.GET("/ready", vfsServer.readyHandler)

	// API v1 routes (stubs for Phase 1)
	v1 := h.Group("/api/v1")
	{
		// Directory routes
		v1.POST("/directories", vfsServer.createDirectoryStub)
		v1.GET("/directories/*path", vfsServer.listDirectoryStub)
		v1.DELETE("/directories/*path", vfsServer.deleteDirectoryStub)

		// File routes
		v1.POST("/files", vfsServer.createFileStub)
		v1.GET("/files/*path", vfsServer.getFileStub)
		v1.PUT("/files/*path", vfsServer.updateFileStub)
		v1.DELETE("/files/*path", vfsServer.deleteFileStub)
		v1.POST("/files/move", vfsServer.moveFileStub)
	}

	log.Printf("VFS Service starting on port %s", port)
	h.Spin()
}

// healthHandler returns service health status
func (s *VFSServer) healthHandler(ctx context.Context, c *app.RequestContext) {
	checks := make(map[string]string)

	// Check database connectivity
	if err := db.HealthCheck(s.db); err != nil {
		checks["database"] = fmt.Sprintf("unhealthy: %v", err)
		c.JSON(consts.StatusServiceUnavailable, map[string]interface{}{
			"status": "degraded",
			"checks": checks,
		})
		return
	}
	checks["database"] = "ok"

	// Check if tables exist
	if !s.db.Migrator().HasTable(&models.Directory{}) {
		checks["migrations"] = "not applied"
		c.JSON(consts.StatusServiceUnavailable, map[string]interface{}{
			"status": "degraded",
			"checks": checks,
		})
		return
	}
	checks["migrations"] = "ok"

	c.JSON(consts.StatusOK, map[string]interface{}{
		"status": "ok",
		"checks": checks,
	})
}

// readyHandler returns readiness status (for Kubernetes)
func (s *VFSServer) readyHandler(ctx context.Context, c *app.RequestContext) {
	if err := db.HealthCheck(s.db); err != nil {
		c.JSON(consts.StatusServiceUnavailable, map[string]interface{}{
			"ready": false,
			"reason": err.Error(),
		})
		return
	}

	c.JSON(consts.StatusOK, map[string]interface{}{
		"ready": true,
	})
}

// Stub handlers for Phase 1 (will be implemented in Phase 2)
func (s *VFSServer) createDirectoryStub(ctx context.Context, c *app.RequestContext) {
	c.JSON(consts.StatusNotImplemented, map[string]string{
		"message": "createDirectory will be implemented in Phase 2",
	})
}

func (s *VFSServer) listDirectoryStub(ctx context.Context, c *app.RequestContext) {
	c.JSON(consts.StatusNotImplemented, map[string]string{
		"message": "listDirectory will be implemented in Phase 2",
	})
}

func (s *VFSServer) deleteDirectoryStub(ctx context.Context, c *app.RequestContext) {
	c.JSON(consts.StatusNotImplemented, map[string]string{
		"message": "deleteDirectory will be implemented in Phase 2",
	})
}

func (s *VFSServer) createFileStub(ctx context.Context, c *app.RequestContext) {
	c.JSON(consts.StatusNotImplemented, map[string]string{
		"message": "createFile will be implemented in Phase 2",
	})
}

func (s *VFSServer) getFileStub(ctx context.Context, c *app.RequestContext) {
	c.JSON(consts.StatusNotImplemented, map[string]string{
		"message": "getFile will be implemented in Phase 2",
	})
}

func (s *VFSServer) updateFileStub(ctx context.Context, c *app.RequestContext) {
	c.JSON(consts.StatusNotImplemented, map[string]string{
		"message": "updateFile will be implemented in Phase 2",
	})
}

func (s *VFSServer) deleteFileStub(ctx context.Context, c *app.RequestContext) {
	c.JSON(consts.StatusNotImplemented, map[string]string{
		"message": "deleteFile will be implemented in Phase 2",
	})
}

func (s *VFSServer) moveFileStub(ctx context.Context, c *app.RequestContext) {
	c.JSON(consts.StatusNotImplemented, map[string]string{
		"message": "moveFile will be implemented in Phase 2",
	})
}

// getEnv retrieves an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
