package domain

import (
	"context"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/telnet2/mysql-vfs/pkg/models"
	"github.com/telnet2/mysql-vfs/pkg/persistence/db"
)

// WorkflowDefinition represents a parsed workflow with resolved directories
type WorkflowDefinition struct {
	WorkflowPath             string
	WorkflowHome             string
	StateDirectories         map[string]string
	RelativeStateDirectories map[string]string
	InitialState             string
	States                   map[string]StateDefinition
	GatePolicy               string
	GatePolicyRef            string
	WorkflowDirectoryID      string
	WorkflowFileID           string

	stateDirectoryList []stateDirectoryEntry
}

// StateDefinition describes transitions available from a state
type StateDefinition struct {
	Transitions []TransitionDefinition
}

// TransitionDefinition represents a state transition
type TransitionDefinition struct {
	To          string
	Description string
}

// WorkflowLoader loads and caches workflow definitions
type WorkflowLoader struct {
	fileRepo db.FileRepository
	dirRepo  db.DirectoryRepository
	cache    *sync.Map
	cacheTTL time.Duration
}

type workflowCacheEntry struct {
	Definition *WorkflowDefinition
	NotFound   bool
	ExpiresAt  time.Time
}

type stateDirectoryEntry struct {
	state string
	path  string
}

// NewWorkflowLoader creates a workflow loader with caching
func NewWorkflowLoader(fileRepo db.FileRepository, dirRepo db.DirectoryRepository, cacheTTL time.Duration) *WorkflowLoader {
	if cacheTTL <= 0 {
		cacheTTL = 5 * time.Minute
	}

	return &WorkflowLoader{
		fileRepo: fileRepo,
		dirRepo:  dirRepo,
		cache:    &sync.Map{},
		cacheTTL: cacheTTL,
	}
}

// LoadForPath loads the workflow governing the provided file or directory path
func (l *WorkflowLoader) LoadForPath(ctx context.Context, filePath string) (*WorkflowDefinition, error) {
	if l == nil {
		return nil, fmt.Errorf("workflow loader is nil")
	}

	normalizedPath := normalizePath(filePath)
	directoryPath := normalizedPath

	if normalizedPath != "/" {
		exists, err := l.dirRepo.Exists(ctx, normalizedPath)
		if err != nil {
			return nil, err
		}
		if !exists {
			directoryPath = path.Dir(normalizedPath)
			if directoryPath == "." {
				directoryPath = "/"
			}
		}
	}

	visited := make(map[string]struct{})

	for {
		directoryPath = normalizePath(directoryPath)
		if _, seen := visited[directoryPath]; seen {
			break
		}
		visited[directoryPath] = struct{}{}

		definition, err := l.loadWorkflowAtDirectory(ctx, directoryPath)
		if err == nil {
			return definition, nil
		}
		if !errors.Is(err, ErrNotFound) {
			return nil, err
		}

		if directoryPath == "/" {
			break
		}
		nextDir := path.Dir(directoryPath)
		if nextDir == directoryPath || nextDir == "." {
			break
		}
		directoryPath = nextDir
	}

	return nil, ErrNotFound
}

// Invalidate clears a cached workflow definition for the given workflow path
func (l *WorkflowLoader) Invalidate(workflowPath string) {
	if l == nil {
		return
	}
	l.cache.Delete(normalizePath(workflowPath))
}

// InvalidateAll clears the entire workflow cache
func (l *WorkflowLoader) InvalidateAll() {
	if l == nil {
		return
	}
	l.cache = &sync.Map{}
}

func (l *WorkflowLoader) loadWorkflowAtDirectory(ctx context.Context, directoryPath string) (*WorkflowDefinition, error) {
	workflowPath := normalizePath(path.Join(directoryPath, string(SpecialFileTypeWorkflow)))

	if entry, ok := l.getFromCache(workflowPath); ok {
		if entry.NotFound {
			return nil, ErrNotFound
		}
		return entry.Definition, nil
	}

	dir, err := l.dirRepo.FindByPath(ctx, directoryPath)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			l.putNotFoundInCache(workflowPath)
			return nil, ErrNotFound
		}
		return nil, err
	}

	file, err := l.fileRepo.FindByDirectoryAndName(ctx, dir.ID, string(SpecialFileTypeWorkflow))
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			l.putNotFoundInCache(workflowPath)
			return nil, ErrNotFound
		}
		return nil, err
	}

	version, err := l.fileRepo.GetLatestVersion(ctx, file.ID)
	if err != nil {
		return nil, err
	}

	var content []byte
	switch {
	case version.TextContent != nil:
		content = []byte(*version.TextContent)
	case version.JSONContent != nil:
		content = []byte(*version.JSONContent)
	default:
		content = []byte{}
	}

	cfg, err := decodeWorkflowConfig(content)
	if err != nil {
		return nil, err
	}

	definition, err := l.buildWorkflowDefinition(ctx, dir, file, workflowPath, cfg)
	if err != nil {
		return nil, err
	}

	l.putInCache(workflowPath, definition)
	return definition, nil
}

func (l *WorkflowLoader) buildWorkflowDefinition(ctx context.Context, dir *models.Directory, workflowFile *models.File, workflowPath string, cfg *workflowConfig) (*WorkflowDefinition, error) {
	workflowHome := normalizePath(dir.Path)

	definition := &WorkflowDefinition{
		WorkflowPath:             workflowPath,
		WorkflowHome:             workflowHome,
		StateDirectories:         make(map[string]string),
		RelativeStateDirectories: make(map[string]string),
		InitialState:             cfg.InitialState,
		States:                   make(map[string]StateDefinition),
		GatePolicy:               cfg.GatePolicy,
		GatePolicyRef:            cfg.GatePolicyRef,
		WorkflowDirectoryID:      dir.ID,
		WorkflowFileID:           workflowFile.ID,
	}

	for state, stateCfg := range cfg.States {
		transitions := make([]TransitionDefinition, 0, len(stateCfg.Transitions))
		for _, t := range stateCfg.Transitions {
			transitions = append(transitions, TransitionDefinition{
				To:          t.To,
				Description: t.Description,
			})
		}
		definition.States[state] = StateDefinition{Transitions: transitions}
	}

	workflowHomeWithSlash := ensureTrailingSlash(workflowHome)
	for state, relative := range cfg.StateDirectories {
		absolute := normalizePath(path.Join(workflowHome, relative))

		if !strings.HasPrefix(absolute+"/", workflowHomeWithSlash) {
			return nil, newWorkflowValidationError(ErrInvalidStatePath, fmt.Sprintf("state '%s' directory '%s' escapes workflow home", state, absolute), map[string]interface{}{"state": state, "path": absolute})
		}

		exists, err := l.dirRepo.Exists(ctx, absolute)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, newWorkflowValidationError(ErrStateDirectoryNotFound, fmt.Sprintf("state directory '%s' (%s) does not exist", state, absolute), map[string]interface{}{"state": state, "path": absolute})
		}

		definition.StateDirectories[state] = absolute
		definition.RelativeStateDirectories[state] = relative
	}

	if cfg.GatePolicyRef != "" {
		exists, err := l.fileRepo.Exists(ctx, dir.ID, cfg.GatePolicyRef)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, newWorkflowValidationError(ErrGatePolicyNotFound, fmt.Sprintf("gate_policy_ref '%s' not found", cfg.GatePolicyRef), map[string]interface{}{"workflow_path": workflowPath})
		}
	}

	if err := l.ensureNoParentWorkflow(ctx, workflowHome); err != nil {
		return nil, err
	}
	if err := l.ensureNoChildWorkflow(ctx, dir); err != nil {
		return nil, err
	}

	definition.stateDirectoryList = make([]stateDirectoryEntry, 0, len(definition.StateDirectories))
	for state, dirPath := range definition.StateDirectories {
		definition.stateDirectoryList = append(definition.stateDirectoryList, stateDirectoryEntry{state: state, path: dirPath})
	}
	sort.Slice(definition.stateDirectoryList, func(i, j int) bool {
		return len(definition.stateDirectoryList[i].path) > len(definition.stateDirectoryList[j].path)
	})

	return definition, nil
}

func (l *WorkflowLoader) ensureNoParentWorkflow(ctx context.Context, workflowHome string) error {
	current := path.Dir(workflowHome)
	for {
		current = normalizePath(current)
		if current == workflowHome {
			break
		}
		if current == "." {
			current = "/"
		}
		exists, err := l.workflowFileExists(ctx, current)
		if err != nil {
			return err
		}
		if exists {
			workflowPath := normalizePath(path.Join(current, string(SpecialFileTypeWorkflow)))
			return newWorkflowValidationError(ErrNestedWorkflow, fmt.Sprintf("parent workflow '%s' already exists", workflowPath), map[string]interface{}{"workflow_path": workflowPath})
		}
		if current == "/" {
			break
		}
		next := path.Dir(current)
		if next == current || next == "." {
			break
		}
		current = next
	}
	return nil
}

func (l *WorkflowLoader) ensureNoChildWorkflow(ctx context.Context, dir *models.Directory) error {
	queue := []string{dir.ID}
	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]

		cursor := ""
		for {
			dirs, nextCursor, err := l.dirRepo.FindByParentID(ctx, currentID, 100, cursor)
			if err != nil {
				return err
			}
			for _, child := range dirs {
				exists, err := l.fileRepo.Exists(ctx, child.ID, string(SpecialFileTypeWorkflow))
				if err != nil {
					return err
				}
				if exists {
					workflowPath := normalizePath(path.Join(child.Path, string(SpecialFileTypeWorkflow)))
					return newWorkflowValidationError(ErrNestedWorkflow, fmt.Sprintf("nested workflow '%s' detected", workflowPath), map[string]interface{}{"workflow_path": workflowPath})
				}
				queue = append(queue, child.ID)
			}
			if nextCursor == "" {
				break
			}
			cursor = nextCursor
		}
	}
	return nil
}

func (l *WorkflowLoader) workflowFileExists(ctx context.Context, dirPath string) (bool, error) {
	dir, err := l.dirRepo.FindByPath(ctx, dirPath)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return l.fileRepo.Exists(ctx, dir.ID, string(SpecialFileTypeWorkflow))
}

func (l *WorkflowLoader) getFromCache(key string) (*workflowCacheEntry, bool) {
	value, ok := l.cache.Load(key)
	if !ok {
		return nil, false
	}
	entry, ok := value.(*workflowCacheEntry)
	if !ok {
		l.cache.Delete(key)
		return nil, false
	}
	if time.Now().After(entry.ExpiresAt) {
		l.cache.Delete(key)
		return nil, false
	}
	return entry, true
}

func (l *WorkflowLoader) putInCache(key string, def *WorkflowDefinition) {
	entry := &workflowCacheEntry{
		Definition: def,
		ExpiresAt:  time.Now().Add(l.cacheTTL),
	}
	l.cache.Store(key, entry)
}

func (l *WorkflowLoader) putNotFoundInCache(key string) {
	entry := &workflowCacheEntry{
		NotFound:  true,
		ExpiresAt: time.Now().Add(l.cacheTTL),
	}
	l.cache.Store(key, entry)
}

// GetCurrentState determines the workflow state for a file or directory path
func (w *WorkflowDefinition) GetCurrentState(filePath string) (string, error) {
	if w == nil {
		return "", fmt.Errorf("workflow definition is nil")
	}

	normalized := normalizePath(filePath)
	if !isDescendantPath(normalized, w.WorkflowHome) {
		return "", fmt.Errorf("path '%s' is outside workflow scope", filePath)
	}

	candidates := []string{normalized}
	if normalized != "/" {
		dirPath := path.Dir(normalized)
		if dirPath == "." {
			dirPath = "/"
		}
		candidates = append(candidates, dirPath)
	}

	seen := make(map[string]struct{})
	for _, candidate := range candidates {
		candidate = normalizePath(candidate)
		if _, exists := seen[candidate]; exists {
			continue
		}
		seen[candidate] = struct{}{}

		for _, entry := range w.stateDirectoryList {
			if candidate == entry.path || strings.HasPrefix(candidate, entry.path+"/") {
				return entry.state, nil
			}
		}
	}

	return "", fmt.Errorf("path '%s' is not governed by workflow '%s'", filePath, w.WorkflowPath)
}

// IsStateDirectory checks if the provided directory path matches a workflow state directory
func (w *WorkflowDefinition) IsStateDirectory(dirPath string) bool {
	if w == nil {
		return false
	}
	normalized := normalizePath(dirPath)
	for _, entry := range w.stateDirectoryList {
		if entry.path == normalized {
			return true
		}
	}
	return false
}

// GetStateDirectoryPath returns the absolute directory path for a state
func (w *WorkflowDefinition) GetStateDirectoryPath(stateName string) (string, error) {
	if w == nil {
		return "", fmt.Errorf("workflow definition is nil")
	}
	if pathVal, ok := w.StateDirectories[stateName]; ok {
		return pathVal, nil
	}
	return "", newWorkflowValidationError(ErrStateDirectoryNotFound, fmt.Sprintf("state '%s' is not defined", stateName), map[string]interface{}{"state": stateName})
}

// GetStateDirectory is an alias for GetStateDirectoryPath
func (w *WorkflowDefinition) GetStateDirectory(stateName string) (string, error) {
	return w.GetStateDirectoryPath(stateName)
}

// GetWorkflowHome returns the workflow home directory path
func (w *WorkflowDefinition) GetWorkflowHome() string {
	if w == nil {
		return ""
	}
	return w.WorkflowHome
}

func normalizePath(p string) string {
	if p == "" {
		return "/"
	}
	cleaned := path.Clean(p)
	if cleaned == "." {
		cleaned = "/"
	}
	if !strings.HasPrefix(cleaned, "/") {
		cleaned = "/" + cleaned
	}
	return cleaned
}

func ensureTrailingSlash(p string) string {
	if p == "/" {
		return "/"
	}
	if strings.HasSuffix(p, "/") {
		return p
	}
	return p + "/"
}

func isDescendantPath(child, parent string) bool {
	child = normalizePath(child)
	parent = normalizePath(parent)
	if parent == "/" {
		return true
	}
	parentWithSlash := ensureTrailingSlash(parent)
	return child == parent || strings.HasPrefix(child, parentWithSlash)
}
