package template

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/telnet2/mysql-vfs/internal/models"
	"gorm.io/gorm"
)

// Template describes the configuration extracted from template JSON files.
type Template struct {
	Path            string         `json:"path"`
	CreateSchema    map[string]any `json:"createSchema"`
	UpdateSchema    map[string]any `json:"updateSchema"`
	DeleteSchema    map[string]any `json:"deleteSchema"`
	DefaultMetadata map[string]any `json:"defaultMetadata"`
	DefaultPolicy   string         `json:"defaultPolicy"`
	Workflow        map[string]any `json:"workflow"`
	Raw             []byte         `json:"-"`
	LastModified    time.Time      `json:"lastModified"`
}

// Cache is a simple in-memory template cache invalidated when the template node changes.
type Cache struct {
	db    *gorm.DB
	items sync.Map
}

// NewCache creates a cache backed by MySQL nodes.
func NewCache(db *gorm.DB) *Cache {
	return &Cache{db: db}
}

// Load fetches the template node and caches the parsed structure.
func (c *Cache) Load(ctx context.Context, path string) (*Template, error) {
	if v, ok := c.items.Load(path); ok {
		if tpl, ok := v.(*Template); ok {
			return tpl, nil
		}
	}

	var node models.Node
	if err := c.db.WithContext(ctx).Where("path = ?", path).Take(&node).Error; err != nil {
		return nil, err
	}

	tpl := &Template{Path: path, Raw: node.ContentInline, LastModified: node.UpdatedAt}
	if err := json.Unmarshal(node.ContentInline, tpl); err != nil {
		return nil, err
	}

	c.items.Store(path, tpl)

	return tpl, nil
}

// Invalidate removes a template from cache.
func (c *Cache) Invalidate(path string) {
	c.items.Delete(path)
}
