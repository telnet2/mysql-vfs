package main

import (
	"context"
	"log"
	"os"

	"github.com/cloudwego/hertz/pkg/app/server"

	"github.com/telnet2/mysql-vfs/internal/access"
	"github.com/telnet2/mysql-vfs/internal/api"
	"github.com/telnet2/mysql-vfs/internal/config"
	"github.com/telnet2/mysql-vfs/internal/db"
	"github.com/telnet2/mysql-vfs/internal/fs"
)

func main() {
	cfg := config.Config{
		Database: config.Database{
			Host:     "localhost",
			Port:     3306,
			User:     "root",
			Password: "password",
			Name:     "mysql_vfs",
		},
		Blob: config.BlobStorage{URL: "mem://"},
	}

	database, err := db.Open(cfg.Database)
	if err != nil {
		log.Fatalf("database connection failed: %v", err)
	}

	policyEngine := access.NewPolicyEngine()

	fsService, err := fs.NewService(database, policyEngine, cfg)
	if err != nil {
		log.Fatalf("filesystem service init failed: %v", err)
	}
	defer fsService.Close()

	addr := os.Getenv("MYSQL_VFS_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	server := api.NewServer(fsService, server.WithHostPorts(addr))

	if err := server.Run(); err != nil {
		log.Fatalf("server failed: %v", err)
	}

	// unreachable but ensures context usage for staticcheck
	_ = context.Background()
}
