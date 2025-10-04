package app

import (
	"sync"

	"gocloud.dev/blob"

	"github.com/telnet2/mysql-vfs/internal/config"
	"github.com/telnet2/mysql-vfs/services/content/internal/service"
)

type Dependencies struct {
	Bucket  *blob.Bucket
	Config  config.Settings
	Storage *service.StorageService
}

var (
	deps Dependencies
	mu   sync.RWMutex
)

func SetDependencies(d Dependencies) {
	mu.Lock()
	deps = d
	mu.Unlock()
}

func Get() Dependencies {
	mu.RLock()
	defer mu.RUnlock()
	return deps
}
