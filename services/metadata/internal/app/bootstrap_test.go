package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/telnet2/mysql-vfs/internal/db"
	"github.com/telnet2/mysql-vfs/services/metadata/internal/policy"
	"github.com/telnet2/mysql-vfs/services/metadata/internal/service"
)

func TestEnsureAdminPasswordCreatesFile(t *testing.T) {
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { os.Chdir(originalWD) })

	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	password, path, err := ensureAdminPassword()
	if err != nil {
		t.Fatalf("ensureAdminPassword: %v", err)
	}
	if password == "" {
		t.Fatalf("expected non-empty password")
	}
	expected := filepath.Join(tmp, adminPasswordFilename)
	resolvedExpected, err := filepath.EvalSymlinks(expected)
	if err != nil {
		resolvedExpected = expected
	}
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		resolvedPath = path
	}
	if filepath.Clean(resolvedPath) != filepath.Clean(resolvedExpected) {
		t.Fatalf("unexpected password path: got %s want %s", path, expected)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat password: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected 0600 permissions, got %v", info.Mode().Perm())
	}
}

func TestBootstrapAdminIdempotent(t *testing.T) {
	tmp := t.TempDir()
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { os.Chdir(originalWD) })
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	database, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(database); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	dirRepo := policy.NewGormDirectoryRepository(database)
	manifestRepo := policy.NewGormManifestRepository(database)
	loader := policy.NewLoader(dirRepo, manifestRepo)
	registry := policy.NewRegistry(loader)

	deps := Dependencies{DB: database, PolicyRegistry: registry}

	dirSvc := service.NewDirectoryService(database)
	if _, err := dirSvc.Create(context.Background(), service.CreateDirectoryInput{Name: "/", RequestID: "bootstrap"}); err != nil {
		t.Fatalf("create root: %v", err)
	}

	if err := BootstrapAdmin(context.Background(), deps); err != nil {
		t.Fatalf("bootstrap admin first run: %v", err)
	}
	if err := BootstrapAdmin(context.Background(), deps); err != nil {
		t.Fatalf("bootstrap admin second run: %v", err)
	}

	rootSvc := service.NewDirectoryService(database)
	root, err := rootSvc.ResolvePath(context.Background(), "/")
	if err != nil {
		t.Fatalf("resolve root: %v", err)
	}
	fileSvc := service.NewFileService(database, registry, nil, nil)
	group, err := fileSvc.ResolvePath(context.Background(), filepath.Join(root.Path, ".group"))
	if err != nil {
		t.Fatalf("resolve group: %v", err)
	}
	if group.Name != ".group" {
		t.Fatalf("unexpected group name: %s", group.Name)
	}
}
