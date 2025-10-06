package domain

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"sync"

	"github.com/telnet2/mysql-vfs/pkg/persistence/db"
	"github.com/xeipuuv/gojsonreference"
	"github.com/xeipuuv/gojsonschema"
)

// VFSSchemaLoader implements gojsonschema.JSONLoader for loading schemas from VFS
// Supports schema:// protocol for VFS file references
type VFSSchemaLoader struct {
	ctx         context.Context
	fileRepo    db.FileRepository
	dirRepo     db.DirectoryRepository
	cache       *sync.Map
	reference   string
	document    interface{}
	factory     gojsonschema.JSONLoaderFactory
}

// NewVFSSchemaLoader creates a loader for a specific schema reference
func NewVFSSchemaLoader(
	ctx context.Context,
	fileRepo db.FileRepository,
	dirRepo db.DirectoryRepository,
	cache *sync.Map,
	ref string,
	factory gojsonschema.JSONLoaderFactory,
) *VFSSchemaLoader {
	return &VFSSchemaLoader{
		ctx:       ctx,
		fileRepo:  fileRepo,
		dirRepo:   dirRepo,
		cache:     cache,
		reference: ref,
		factory:   factory,
	}
}

// JsonSource returns the JSON source
func (l *VFSSchemaLoader) JsonSource() interface{} {
	return l.reference
}

// LoadJSON loads the JSON document from VFS
func (l *VFSSchemaLoader) LoadJSON() (interface{}, error) {
	if l.document != nil {
		return l.document, nil
	}

	// Parse the reference to extract the path
	path, err := l.parseSchemaReference(l.reference)
	if err != nil {
		return nil, err
	}

	// Load from VFS
	doc, err := l.loadFromVFS(path)
	if err != nil {
		return nil, err
	}

	l.document = doc
	return doc, nil
}

// JsonReference returns the JSON reference
func (l *VFSSchemaLoader) JsonReference() (gojsonreference.JsonReference, error) {
	return gojsonreference.NewJsonReference(l.reference)
}

// LoaderFactory returns the loader factory
func (l *VFSSchemaLoader) LoaderFactory() gojsonschema.JSONLoaderFactory {
	return l.factory
}

// parseSchemaReference parses schema:// URLs to extract the VFS path
// Supports:
//   - schema:///absolute/path.json → /absolute/path.json
//   - schema://./relative/path.json → relative to base (future)
func (l *VFSSchemaLoader) parseSchemaReference(ref string) (string, error) {
	if !strings.HasPrefix(ref, "schema://") {
		return "", fmt.Errorf("invalid schema reference: must start with 'schema://', got: %s", ref)
	}

	// Parse the URL
	u, err := url.Parse(ref)
	if err != nil {
		return "", fmt.Errorf("invalid schema URL: %w", err)
	}

	// Extract path - schema:///path/to/file.json → /path/to/file.json
	path := u.Path
	if path == "" {
		return "", fmt.Errorf("empty path in schema reference: %s", ref)
	}

	// Ensure absolute path
	if !strings.HasPrefix(path, "/") {
		return "", fmt.Errorf("schema reference must use absolute path: %s", ref)
	}

	return path, nil
}

// loadFromVFS loads a schema from the VFS by path
func (l *VFSSchemaLoader) loadFromVFS(path string) (interface{}, error) {
	// Check cache first
	if cached, ok := l.cache.Load(path); ok {
		return cached, nil
	}

	// Parse the path to get directory and filename
	dirPath := filepath.Dir(path)
	fileName := filepath.Base(path)

	// Find the directory
	dir, err := l.dirRepo.FindByPath(l.ctx, dirPath)
	if err != nil {
		return nil, fmt.Errorf("directory not found: %s (for schema: %s)", dirPath, path)
	}

	// Find the file
	file, err := l.fileRepo.FindByDirectoryAndName(l.ctx, dir.ID, fileName)
	if err != nil {
		return nil, fmt.Errorf("schema file not found: %s", path)
	}

	// Get file content
	var content []byte
	if file.JSONContent != nil {
		content = []byte(*file.JSONContent)
	} else if file.TextContent != nil {
		content = []byte(*file.TextContent)
	} else {
		return nil, fmt.Errorf("schema file has no content: %s", path)
	}

	// Keep as raw JSON - don't unmarshal to preserve number types
	// Validate it's valid JSON
	var temp interface{}
	if err := json.Unmarshal(content, &temp); err != nil {
		return nil, fmt.Errorf("invalid JSON in schema file %s: %w", path, err)
	}

	// Cache the raw bytes as the unmarshaled value
	// We return the temp value which has proper types
	l.cache.Store(path, temp)

	return temp, nil
}

// VFSSchemaLoaderFactory creates loaders for schema:// references
type VFSSchemaLoaderFactory struct {
	ctx      context.Context
	fileRepo db.FileRepository
	dirRepo  db.DirectoryRepository
	cache    *sync.Map
}

// NewVFSSchemaLoaderFactory creates a new loader factory
func NewVFSSchemaLoaderFactory(
	ctx context.Context,
	fileRepo db.FileRepository,
	dirRepo db.DirectoryRepository,
	cache *sync.Map,
) *VFSSchemaLoaderFactory {
	return &VFSSchemaLoaderFactory{
		ctx:      ctx,
		fileRepo: fileRepo,
		dirRepo:  dirRepo,
		cache:    cache,
	}
}

// New creates a new JSON loader for the given source (implements JSONLoaderFactory)
func (f *VFSSchemaLoaderFactory) New(source string) gojsonschema.JSONLoader {
	// Check if this is a schema:// reference
	if strings.HasPrefix(source, "schema://") {
		return NewVFSSchemaLoader(f.ctx, f.fileRepo, f.dirRepo, f.cache, source, f)
	}

	// Fall back to default loaders for http://, file://, etc.
	return gojsonschema.NewReferenceLoader(source)
}

// CreateLoader creates a loader for a specific reference
func (f *VFSSchemaLoaderFactory) CreateLoader(ref string) gojsonschema.JSONLoader {
	return f.New(ref)
}
