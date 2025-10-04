package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/telnet2/mysql-vfs/pkg/config"
	"github.com/telnet2/mysql-vfs/pkg/db"
	"github.com/telnet2/mysql-vfs/pkg/domain"
	"github.com/telnet2/mysql-vfs/pkg/events/handlers"
	"github.com/telnet2/mysql-vfs/pkg/idempotency"
	"github.com/telnet2/mysql-vfs/pkg/middleware"
	"github.com/telnet2/mysql-vfs/pkg/models"
	gormrepo "github.com/telnet2/mysql-vfs/pkg/repository/gorm"
	"github.com/telnet2/mysql-vfs/pkg/services"
	"github.com/telnet2/mysql-vfs/pkg/storage"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type VFSServer struct {
	db                 *gorm.DB
	storage            storage.Storage
	dirService         *services.DirectoryService
	fileService        *services.FileService
	idempotencyService *idempotency.Service
	filesLoader        *domain.FilesLoader
	policyLoader       *domain.PolicyLoader
	quotaLoader        *domain.QuotaLoader
}

func main() {
	// Load configuration from environment
	cfg := config.LoadFromEnv()

	// Parse log level
	gormLogLevel := logger.Info
	if cfg.LogLevel == "debug" {
		gormLogLevel = logger.Info
	} else if cfg.LogLevel == "warn" {
		gormLogLevel = logger.Warn
	} else if cfg.LogLevel == "error" {
		gormLogLevel = logger.Error
	}

	// Connect to database
	log.Println("Connecting to database...")
	database, err := db.Connect(db.Config{
		DSN:      cfg.DatabaseDSN,
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

	// Initialize storage
	log.Println("Initializing storage...")
	ctx := context.Background()
	storageService, err := storage.NewStorageFromEnv(ctx)
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}
	log.Println("Storage initialized successfully")

	// Initialize repositories
	fileRepo := gormrepo.NewGormFileRepository(database)
	dirRepo := gormrepo.NewGormDirectoryRepository(database)

	// Initialize loaders with caching (from config)
	filesLoader := domain.NewFilesLoader(fileRepo, dirRepo, cfg.SchemaCacheTTL)
	policyLoader := domain.NewPolicyLoader(fileRepo, dirRepo, cfg.PolicyCacheTTL)
	quotaLoader := domain.NewQuotaLoader(fileRepo, dirRepo, cfg.QuotaCacheTTL)
	eventsLoader := domain.NewEventsLoader(fileRepo, dirRepo, cfg.SchemaCacheTTL) // Reuse schema TTL for events

	// Initialize event handler registry
	handlerRegistry := handlers.NewRegistry()
	handlerRegistry.Register(handlers.NewWebhookHandler())
	handlerRegistry.Register(handlers.NewLogHandler())
	handlerRegistry.Register(handlers.NewMetricsHandler())

	// Initialize lifecycle event trigger
	eventTrigger := domain.NewLifecycleEventTrigger(
		eventsLoader,
		handlerRegistry,
		domain.EventTriggerConfig{
			MaxConcurrentHandlers: 10,
		},
	)

	// Initialize services
	dirService := services.NewDirectoryServiceWithLifecycle(database, eventTrigger)
	fileService := services.NewFileServiceWithLifecycle(database, storageService, filesLoader, eventTrigger)
	idempotencyService := idempotency.NewServiceWithTTL(database, cfg.IdempotencyTTL)

	// Start idempotency cleanup worker
	go idempotencyService.StartCleanupWorker(ctx, 1*time.Hour)

	// Create server instance
	vfsServer := &VFSServer{
		db:                 database,
		storage:            storageService,
		dirService:         dirService,
		fileService:        fileService,
		idempotencyService: idempotencyService,
		filesLoader:        filesLoader,
		policyLoader:       policyLoader,
		quotaLoader:        quotaLoader,
	}

	// Initialize Hertz server
	h := server.Default(server.WithHostPorts(":" + cfg.ServerPort))

	// Initialize authentication from config
	authExtractor, err := middleware.NewAuthExtractorFromConfig(cfg.Auth, fileRepo, dirRepo)
	if err != nil {
		log.Fatalf("Failed to initialize auth: %v", err)
	}

	authMiddleware := middleware.NewAuthMiddleware(authExtractor, cfg.Auth.AllowAnonymous)

	// Initialize authorization middleware
	authzMiddleware := middleware.NewAuthorizationMiddleware(middleware.AuthorizationConfig{
		PolicyLoader: policyLoader,
		Timeout:      200 * time.Millisecond,
		SkipRoutes:   []string{"/health", "/ready"},
	})

	// Public routes (no auth required)
	h.GET("/health", vfsServer.healthHandler)
	h.GET("/ready", vfsServer.readyHandler)

	// API v1 routes
	v1 := h.Group("/api/v1")
	v1.Use(idempotencyService.Middleware())
	v1.Use(authMiddleware.Handler())   // Authentication (JWT, OAuth, etc.)
	v1.Use(authzMiddleware.Handler())  // Authorization (OPA policies)

	// Directory routes
	v1.POST("/directories", vfsServer.createDirectory)
	v1.GET("/directories/*path", vfsServer.listDirectory)
	v1.DELETE("/directories/*path", vfsServer.deleteDirectory)

	// File routes
	v1.POST("/files", vfsServer.createFile)
	v1.GET("/files/*path", vfsServer.getFile)
	v1.PUT("/files/*path", vfsServer.updateFile)
	v1.DELETE("/files/*path", vfsServer.deleteFile)
	v1.POST("/files/move", vfsServer.moveFile)

	log.Printf("VFS Service starting on port %s", cfg.ServerPort)
	log.Printf("Authentication: %s", cfg.Auth.Provider)
	log.Printf("Authorization: ENABLED (OPA policies via .rego files)")
	log.Printf("Schema validation: ENABLED (.files special files)")
	log.Printf("Lifecycle events: ENABLED (.events special files)")
	log.Printf("Event handlers: webhook, log, metrics")
	log.Printf("Cache TTL - Schema: %v, Policy: %v, Quota: %v", cfg.SchemaCacheTTL, cfg.PolicyCacheTTL, cfg.QuotaCacheTTL)
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

	// Check storage
	exists, err := s.storage.Exists(ctx, ".healthcheck")
	if err != nil {
		checks["storage"] = fmt.Sprintf("unhealthy: %v", err)
	} else {
		checks["storage"] = "ok"
		_ = exists // Ignore result, just checking connectivity
	}

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

// createDirectory creates a new directory
func (s *VFSServer) createDirectory(ctx context.Context, c *app.RequestContext) {
	var req struct {
		ParentPath string `json:"parent_path"`
		Name       string `json:"name"`
	}

	if err := c.BindJSON(&req); err != nil {
		c.JSON(consts.StatusBadRequest, map[string]string{
			"error": "invalid request body",
		})
		return
	}

	// Get request ID and add to context
	requestID := idempotency.GetRequestID(c)
	if requestID != "" {
		ctx = context.WithValue(ctx, "requestID", requestID)
	}

	dir, err := s.dirService.CreateDirectory(ctx, req.ParentPath, req.Name)
	if err != nil {
		c.JSON(consts.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	// Cache response for idempotency
	response := map[string]interface{}{
		"id":         dir.ID,
		"name":       dir.Name,
		"path":       dir.Path,
		"parent_id":  dir.ParentID,
		"created_at": dir.CreatedAt,
	}

	if requestID != "" {
		s.idempotencyService.CacheResponse(requestID, response)
	}

	c.JSON(consts.StatusCreated, response)
}

// listDirectory lists directory contents
func (s *VFSServer) listDirectory(ctx context.Context, c *app.RequestContext) {
	path := string(c.Param("path"))
	if path == "" {
		path = "/"
	} else if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	limit := 100
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil {
			limit = parsed
		}
	}

	cursor := c.Query("cursor")

	directories, files, nextCursor, err := s.dirService.ListDirectory(path, limit, cursor)
	if err != nil {
		c.JSON(consts.StatusNotFound, map[string]string{
			"error": err.Error(),
		})
		return
	}

	// Format response
	entries := []map[string]interface{}{}
	for _, dir := range directories {
		entries = append(entries, map[string]interface{}{
			"name":        dir.Name,
			"type":        "directory",
			"size_bytes":  0,
			"modified_at": dir.UpdatedAt,
		})
	}
	for _, file := range files {
		entries = append(entries, map[string]interface{}{
			"name":        file.Name,
			"type":        "file",
			"size_bytes":  file.SizeBytes,
			"modified_at": file.UpdatedAt,
		})
	}

	c.JSON(consts.StatusOK, map[string]interface{}{
		"entries":     entries,
		"next_cursor": nextCursor,
	})
}

// deleteDirectory deletes a directory
func (s *VFSServer) deleteDirectory(ctx context.Context, c *app.RequestContext) {
	path := string(c.Param("path"))
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	recursive := c.Query("recursive") == "true"

	// Get request ID and add to context
	requestID := idempotency.GetRequestID(c)
	if requestID != "" {
		ctx = context.WithValue(ctx, "requestID", requestID)
	}

	if err := s.dirService.DeleteDirectory(ctx, path, recursive); err != nil {
		c.JSON(consts.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	response := map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("directory %s deleted", path),
	}

	if requestID != "" {
		s.idempotencyService.CacheResponse(requestID, response)
	}

	c.JSON(consts.StatusOK, response)
}

// createFile creates a new file
func (s *VFSServer) createFile(ctx context.Context, c *app.RequestContext) {
	var req struct {
		DirectoryPath string `json:"directory_path"`
		Name          string `json:"name"`
		ContentType   string `json:"content_type"`
		Content       string `json:"content"` // Base64 encoded or plain text
	}

	if err := c.BindJSON(&req); err != nil {
		c.JSON(consts.StatusBadRequest, map[string]string{
			"error": "invalid request body",
		})
		return
	}

	// Get request ID and add to context
	requestID := idempotency.GetRequestID(c)
	if requestID != "" {
		ctx = context.WithValue(ctx, "requestID", requestID)
	}

	size := int64(len(req.Content))
	file, err := s.fileService.CreateFile(ctx, req.DirectoryPath, req.Name, req.ContentType, size, io.NopCloser(strings.NewReader(req.Content)))
	if err != nil {
		c.JSON(consts.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	response := map[string]interface{}{
		"id":           file.ID,
		"name":         file.Name,
		"content_type": file.ContentType,
		"size_bytes":   file.SizeBytes,
		"storage_type": file.StorageType,
		"checksum":     file.ChecksumSHA256,
		"version":      file.Version,
		"created_at":   file.CreatedAt,
	}

	if requestID != "" {
		s.idempotencyService.CacheResponse(requestID, response)
	}

	c.JSON(consts.StatusCreated, response)
}

// getFile retrieves a file
func (s *VFSServer) getFile(ctx context.Context, c *app.RequestContext) {
	path := string(c.Param("path"))
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	file, reader, err := s.fileService.GetFile(ctx, path)
	if err != nil {
		c.JSON(consts.StatusNotFound, map[string]string{
			"error": err.Error(),
		})
		return
	}
	defer reader.Close()

	// Set headers
	c.Response.Header.Set("Content-Type", file.ContentType)
	c.Response.Header.Set("Content-Length", fmt.Sprintf("%d", file.SizeBytes))
	c.Response.Header.Set("X-File-Version", fmt.Sprintf("%d", file.Version))
	c.Response.Header.Set("X-Checksum-SHA256", file.ChecksumSHA256)

	// Stream content
	c.Response.SetStatusCode(consts.StatusOK)
	if _, err := io.Copy(c.Response.BodyWriter(), reader); err != nil {
		log.Printf("Error streaming file: %v", err)
	}
}

// updateFile updates a file
func (s *VFSServer) updateFile(ctx context.Context, c *app.RequestContext) {
	path := string(c.Param("path"))
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	var req struct {
		ContentType     string `json:"content_type"`
		Content         string `json:"content"`
		ExpectedVersion int64  `json:"expected_version"`
	}

	if err := c.BindJSON(&req); err != nil {
		c.JSON(consts.StatusBadRequest, map[string]string{
			"error": "invalid request body",
		})
		return
	}

	// Get request ID and add to context
	requestID := idempotency.GetRequestID(c)
	if requestID != "" {
		ctx = context.WithValue(ctx, "requestID", requestID)
	}

	size := int64(len(req.Content))
	file, err := s.fileService.UpdateFile(ctx, path, req.ContentType, size, io.NopCloser(strings.NewReader(req.Content)), req.ExpectedVersion)
	if err != nil {
		c.JSON(consts.StatusConflict, map[string]string{
			"error": err.Error(),
		})
		return
	}

	response := map[string]interface{}{
		"id":           file.ID,
		"name":         file.Name,
		"content_type": file.ContentType,
		"size_bytes":   file.SizeBytes,
		"version":      file.Version,
		"updated_at":   file.UpdatedAt,
	}

	if requestID != "" {
		s.idempotencyService.CacheResponse(requestID, response)
	}

	c.JSON(consts.StatusOK, response)
}

// deleteFile deletes a file
func (s *VFSServer) deleteFile(ctx context.Context, c *app.RequestContext) {
	path := string(c.Param("path"))
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	// Get request ID and add to context
	requestID := idempotency.GetRequestID(c)
	if requestID != "" {
		ctx = context.WithValue(ctx, "requestID", requestID)
	}

	if err := s.fileService.DeleteFile(ctx, path); err != nil {
		c.JSON(consts.StatusNotFound, map[string]string{
			"error": err.Error(),
		})
		return
	}

	response := map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("file %s deleted", path),
	}

	if requestID != "" {
		s.idempotencyService.CacheResponse(requestID, response)
	}

	c.JSON(consts.StatusOK, response)
}

// moveFile moves or renames a file
func (s *VFSServer) moveFile(ctx context.Context, c *app.RequestContext) {
	var req struct {
		SourcePath      string `json:"source_path"`
		DestinationPath string `json:"destination_path"`
	}

	if err := c.BindJSON(&req); err != nil {
		c.JSON(consts.StatusBadRequest, map[string]string{
			"error": "invalid request body",
		})
		return
	}

	// Get request ID and add to context
	requestID := idempotency.GetRequestID(c)
	if requestID != "" {
		ctx = context.WithValue(ctx, "requestID", requestID)
	}

	file, err := s.fileService.MoveFile(ctx, req.SourcePath, req.DestinationPath)
	if err != nil {
		c.JSON(consts.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	response := map[string]interface{}{
		"id":         file.ID,
		"name":       file.Name,
		"updated_at": file.UpdatedAt,
	}

	if requestID != "" {
		s.idempotencyService.CacheResponse(requestID, response)
	}

	c.JSON(consts.StatusOK, response)
}
