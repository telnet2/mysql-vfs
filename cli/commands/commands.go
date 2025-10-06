package commands

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/telnet2/mysql-vfs/cli/client"
	"github.com/telnet2/mysql-vfs/cli/session"
)

// Context holds command execution context
type Context struct {
	Client  *client.Client
	Session *session.Session
	Stdin   io.Reader
	Stdout  io.Writer
	Stderr  io.Writer
}

// expandVFSGlob expands glob patterns in VFS paths
func expandVFSGlob(ctx *Context, pattern string) ([]string, error) {
	// Check if pattern contains glob characters
	if !strings.ContainsAny(pattern, "*?[") {
		// No glob characters, return as-is if it exists
		if !session.IsValidPath(pattern) {
			return nil, fmt.Errorf("invalid path: %s", pattern)
		}
		return []string{pattern}, nil
	}

	// Split pattern into directory and filename parts
	dir := filepath.Dir(pattern)
	base := filepath.Base(pattern)

	// If pattern starts with /, dir will be "/"
	if dir == "." {
		dir = ""
	}

	// Resolve the directory path
	var searchDir string
	if dir == "" || dir == "/" {
		searchDir = "/"
	} else {
		searchDir = ctx.Session.ResolvePath(dir)
	}

	if !session.IsValidPath(searchDir) {
		return nil, fmt.Errorf("invalid directory: %s", searchDir)
	}

	// List directory contents
	resp, err := ctx.Client.ListDirectory(searchDir, 1000, "") // Use larger limit for globbing
	if err != nil {
		return nil, fmt.Errorf("failed to list directory %s: %w", searchDir, err)
	}

	var matches []string
	for _, entry := range resp.Entries {
		// Match the filename against the glob pattern
		matched, err := filepath.Match(base, entry.Name)
		if err != nil {
			continue // Skip invalid patterns
		}
		if matched {
			// Construct full path
			if searchDir == "/" {
				matches = append(matches, "/"+entry.Name)
			} else {
				matches = append(matches, filepath.Join(searchDir, entry.Name))
			}
		}
	}

	if len(matches) == 0 {
		return nil, fmt.Errorf("no files match pattern: %s", pattern)
	}

	return matches, nil
}

// hasGlobChars checks if a string contains glob characters
func hasGlobChars(s string) bool {
	return strings.ContainsAny(s, "*?[")
}

// detectContentType detects content type based on file extension
func (c *LsCommand) detectContentType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".json":
		return "application/json"
	case ".txt", ".md":
		return "text/plain"
	case ".html":
		return "text/html"
	case ".xml":
		return "application/xml"
	case ".csv":
		return "text/csv"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".pdf":
		return "application/pdf"
	case ".zip":
		return "application/zip"
	case ".rego":
		return "application/rego"
	case ".group":
		return "application/group"
	default:
		return "application/octet-stream"
	}
}

// formatSize formats file size in human readable format
func (c *LsCommand) formatSize(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%d", size)
	} else if size < 1024*1024 {
		return fmt.Sprintf("%.1fK", float64(size)/1024)
	} else if size < 1024*1024*1024 {
		return fmt.Sprintf("%.1fM", float64(size)/(1024*1024))
	} else {
		return fmt.Sprintf("%.1fG", float64(size)/(1024*1024*1024))
	}
}

// Command represents a CLI command
type Command interface {
	Execute(ctx *Context, args []string) error
	Help() string
}

// LsCommand lists directory contents
type LsCommand struct{}

func (c *LsCommand) Execute(ctx *Context, args []string) error {
	recursive := false
	long := false
	targetPath := ""
	var err error

	// Parse flags
	for _, arg := range args {
		if arg == "-r" || arg == "--recursive" {
			recursive = true
		} else if arg == "-l" || arg == "--long" {
			long = true
		} else if !strings.HasPrefix(arg, "-") {
			targetPath = arg
		}
	}

	// Resolve path
	if targetPath == "" {
		targetPath = ctx.Session.GetCurrentDirectory()
	} else {
		targetPath = ctx.Session.ResolvePath(targetPath)
	}

	if recursive {
		if hasGlobChars(targetPath) {
			return fmt.Errorf("recursive listing with glob patterns is not supported")
		}
		return c.listRecursive(ctx, targetPath, 0, long)
	}

	// Check if path contains glob characters
	if hasGlobChars(targetPath) {
		return c.listGlob(ctx, targetPath, long)
	}

	// Check if it's a file or directory
	if strings.HasSuffix(targetPath, "/") {
		// Explicitly a directory
		return c.listSingle(ctx, strings.TrimSuffix(targetPath, "/"), long)
	}

	// Try to list as directory first
	_, err = ctx.Client.ListDirectory(targetPath, 1, "")
	if err == nil {
		// It's a directory
		return c.listSingle(ctx, targetPath, long)
	}

	// Try to get as file
	_, _, _, err = ctx.Client.GetFile(targetPath)
	if err != nil {
		return fmt.Errorf("path not found: %s", targetPath)
	}

	// It's a file - show file info
	return c.listFile(ctx, targetPath, long)
}

func (c *LsCommand) listGlob(ctx *Context, pattern string, long bool) error {
	matches, err := expandVFSGlob(ctx, pattern)
	if err != nil {
		return err
	}

	for _, match := range matches {
		if err := c.listFile(ctx, match, long); err != nil {
			fmt.Fprintf(ctx.Stderr, "Warning: failed to list %s: %v\n", match, err)
		}
	}

	return nil
}

func (c *LsCommand) listFile(ctx *Context, path string, long bool) error {
	if long {
		return c.listFileLong(ctx, path)
	}

	// For files, we need to get file info from the parent directory
	dir := filepath.Dir(path)
	base := filepath.Base(path)

	resp, err := ctx.Client.ListDirectory(dir, 100, "")
	if err != nil {
		return err
	}

	for _, entry := range resp.Entries {
		if entry.Name == base {
			if entry.Type == "directory" {
				fmt.Fprintf(ctx.Stdout, "%s/\n", path)
			} else {
				fmt.Fprintf(ctx.Stdout, "%s  (%d bytes)\n", path, entry.SizeBytes)
			}
			return nil
		}
	}

	return fmt.Errorf("file not found: %s", path)
}

func (c *LsCommand) listFileLong(ctx *Context, path string) error {
	// Get file metadata
	content, contentType, version, err := ctx.Client.GetFile(path)
	if err != nil {
		return err
	}

	// Detect content type if not provided
	if contentType == "" {
		contentType = c.detectContentType(filepath.Base(path))
	}

	// Use version from GetFile response
	latestVersion := version
	timestamp := "unknown" // For single file, we don't have timestamp from GetFile

	// Print table header
	fmt.Fprintf(ctx.Stdout, "%-19s %-12s %-8s %-18s %s\n",
		"Modified", "Size", "Version", "Type", "Name")
	fmt.Fprintf(ctx.Stdout, "%-19s %-12s %-8s %-18s %s\n",
		strings.Repeat("-", 19), strings.Repeat("-", 12), strings.Repeat("-", 8), strings.Repeat("-", 18), strings.Repeat("-", 35))

	// Print file info
	name := filepath.Base(path)
	sizeStr := c.formatSize(int64(len(content)))
	versionStr := fmt.Sprintf("%d", latestVersion)

	fmt.Fprintf(ctx.Stdout, "%-19s %-12s %-8s %-18s %s\n",
		timestamp, sizeStr, versionStr, contentType, name)

	return nil
}

func (c *LsCommand) listSingle(ctx *Context, path string, long bool) error {
	resp, err := ctx.Client.ListDirectory(path, 100, "")
	if err != nil {
		return err
	}

	if len(resp.Entries) == 0 {
		fmt.Fprintln(ctx.Stdout, "(empty directory)")
		return nil
	}

	if long {
		return c.listDirectoryLong(ctx, path, resp.Entries)
	}

	for _, entry := range resp.Entries {
		if entry.Type == "directory" {
			fmt.Fprintf(ctx.Stdout, "%s/\n", entry.Name)
		} else {
			fmt.Fprintf(ctx.Stdout, "%s  (%d bytes)\n", entry.Name, entry.SizeBytes)
		}
	}

	return nil
}

func (c *LsCommand) listDirectoryLong(ctx *Context, dirPath string, entries []client.DirectoryEntry) error {
	// Print table header with Linux-like format
	fmt.Fprintf(ctx.Stdout, "%-19s %-12s %-8s %-18s %s\n",
		"Modified", "Size", "Version", "Type", "Name")
	fmt.Fprintf(ctx.Stdout, "%-19s %-12s %-8s %-18s %s\n",
		strings.Repeat("-", 19), strings.Repeat("-", 12), strings.Repeat("-", 8), strings.Repeat("-", 18), strings.Repeat("-", 35))

	// For each entry, get detailed information
	for _, entry := range entries {
		name := entry.Name
		entryType := entry.Type

		fullPath := filepath.Join(dirPath, name)
		if entryType == "directory" {
			// For directories, show basic info
			fmt.Fprintf(ctx.Stdout, "%-19s %-12s %-8s %-18s %s\n",
				entry.ModifiedAt.Format("2006-01-02 15:04:05"), "-", "-", "directory", name+"/")
			continue
		}

		// For files, get detailed metadata
		content, contentType, _, err := ctx.Client.GetFile(fullPath)
		if err != nil {
			// Fallback to basic info if we can't get file details
			sizeStr := c.formatSize(entry.SizeBytes)
			contentType = c.detectContentType(name)
			versionStr := fmt.Sprintf("%d", entry.Version)
			if entry.Version == 0 {
				versionStr = "-"
			}
			fmt.Fprintf(ctx.Stdout, "%-19s %-12s %-8s %-18s %s\n",
				entry.ModifiedAt.Format("2006-01-02 15:04:05"), sizeStr, versionStr, contentType, name)
			continue
		}

		// Detect content type if not provided
		if contentType == "" {
			contentType = c.detectContentType(name)
		}

		// Get version from directory entry
		var latestVersion int64 = entry.Version
		var timestamp string = entry.ModifiedAt.Format("2006-01-02 15:04:05")

		// If version is not available in directory entry, try to get from version history
		if latestVersion == 0 {
			versions, err := ctx.Client.ListVersions(fullPath)
			if err == nil && len(versions) > 0 {
				// Find latest version
				latest := versions[0]
				for _, v := range versions {
					if v.Version > latest.Version {
						latest = v
					}
				}
				latestVersion = latest.Version
				timestamp = latest.CreatedAt.Format("2006-01-02 15:04:05")
			} else {
				latestVersion = 1 // Default to 1 if we can't get version info
			}
		}

		sizeStr := c.formatSize(int64(len(content)))
		versionStr := fmt.Sprintf("%d", latestVersion)

		fmt.Fprintf(ctx.Stdout, "%-19s %-12s %-8s %-18s %s\n",
			timestamp, sizeStr, versionStr, contentType, name)
	}

	return nil
}

func (c *LsCommand) listRecursive(ctx *Context, path string, depth int, long bool) error {
	if depth > 100 {
		return fmt.Errorf("maximum recursion depth (100) exceeded")
	}

	resp, err := ctx.Client.ListDirectory(path, 100, "")
	if err != nil {
		return err
	}

	indent := strings.Repeat("  ", depth)

	if depth == 0 {
		fmt.Fprintf(ctx.Stdout, "%s/\n", path)
		if long {
			// Print header for long recursive listing
			fmt.Fprintf(ctx.Stdout, "%-19s %-12s %-8s %-18s %s\n",
				"Modified", "Size", "Version", "Type", "Name")
			fmt.Fprintf(ctx.Stdout, "%-19s %-12s %-8s %-18s %s\n",
				strings.Repeat("-", 19), strings.Repeat("-", 12), strings.Repeat("-", 8), strings.Repeat("-", 18), strings.Repeat("-", 35))
		}
	}

	for _, entry := range resp.Entries {
		if entry.Type == "directory" {
			if long {
				nameWithIndent := indent + entry.Name + "/"
				fmt.Fprintf(ctx.Stdout, "%-19s %-12s %-8s %-18s %s\n",
					entry.ModifiedAt.Format("2006-01-02 15:04:05"), "-", "-", "directory", nameWithIndent)
			} else {
				nameWithIndent := indent + entry.Name + "/"
				fmt.Fprintf(ctx.Stdout, "%s\n", nameWithIndent)
			}
			subPath := filepath.Join(path, entry.Name)
			if err := c.listRecursive(ctx, subPath, depth+1, long); err != nil {
				return err
			}
		} else {
			if long {
				// For long recursive listing, show detailed info with indentation
				fullPath := filepath.Join(path, entry.Name)
				content, contentType, _, err := ctx.Client.GetFile(fullPath)
				if err != nil {
					contentType = c.detectContentType(entry.Name)
					versionStr := fmt.Sprintf("%d", entry.Version)
					if entry.Version == 0 {
						versionStr = "-"
					}
					nameWithIndent := indent + entry.Name
					fmt.Fprintf(ctx.Stdout, "%-19s %-12s %-8s %-18s %s\n",
						entry.ModifiedAt.Format("2006-01-02 15:04:05"), c.formatSize(entry.SizeBytes), versionStr, contentType, nameWithIndent)
					continue
				}

				// Detect content type if not provided
				if contentType == "" {
					contentType = c.detectContentType(entry.Name)
				}

				// Get version from directory entry
				var latestVersion int64 = entry.Version
				var timestamp string = entry.ModifiedAt.Format("2006-01-02 15:04:05")

				// If version is not available in directory entry, try to get from version history
				if latestVersion == 0 {
					versions, err := ctx.Client.ListVersions(fullPath)
					if err == nil && len(versions) > 0 {
						// Find latest version
						latest := versions[0]
						for _, v := range versions {
							if v.Version > latest.Version {
								latest = v
							}
						}
						latestVersion = latest.Version
						timestamp = latest.CreatedAt.Format("2006-01-02 15:04:05")
					} else {
						latestVersion = 1 // Default to 1 if we can't get version info
					}
				}

				sizeStr := c.formatSize(int64(len(content)))
				versionStr := fmt.Sprintf("%d", latestVersion)
				nameWithIndent := indent + entry.Name

				fmt.Fprintf(ctx.Stdout, "%-19s %-12s %-8s %-18s %s\n",
					timestamp, sizeStr, versionStr, contentType, nameWithIndent)
			} else {
				fmt.Fprintf(ctx.Stdout, "%s%s  (%d bytes)\n", indent, entry.Name, entry.SizeBytes)
			}
		}
	}

	return nil
}

func (c *LsCommand) Help() string {
	return "ls [-r] [path] - List directory contents"
}

// CdCommand changes directory
type CdCommand struct{}

func (c *CdCommand) Execute(ctx *Context, args []string) error {
	if len(args) == 0 {
		ctx.Session.SetCurrentDirectory("/")
		return nil
	}

	targetPath := ctx.Session.ResolvePath(args[0])

	if !session.IsValidPath(targetPath) {
		return fmt.Errorf("invalid path: %s", targetPath)
	}

	// Verify directory exists
	_, err := ctx.Client.ListDirectory(targetPath, 1, "")
	if err != nil {
		return fmt.Errorf("directory not found: %s", targetPath)
	}

	ctx.Session.SetCurrentDirectory(targetPath)
	return nil
}

func (c *CdCommand) Help() string {
	return "cd [path] - Change current directory"
}

// PwdCommand prints working directory
type PwdCommand struct{}

func (c *PwdCommand) Execute(ctx *Context, args []string) error {
	fmt.Fprintln(ctx.Stdout, ctx.Session.GetCurrentDirectory())
	return nil
}

func (c *PwdCommand) Help() string {
	return "pwd - Print working directory"
}

// MkdirCommand creates a directory
type MkdirCommand struct{}

func (c *MkdirCommand) Execute(ctx *Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: mkdir <name>")
	}

	name := args[0]
	parentPath := ctx.Session.GetCurrentDirectory()

	resp, err := ctx.Client.CreateDirectory(parentPath, name)
	if err != nil {
		return err
	}

	fmt.Fprintf(ctx.Stdout, "Created directory: %s\n", resp.Path)
	return nil
}

func (c *MkdirCommand) Help() string {
	return "mkdir <name> - Create a new directory"
}

// RmdirCommand removes a directory
type RmdirCommand struct{}

func (c *RmdirCommand) Execute(ctx *Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: rmdir [-r] <path>")
	}

	recursive := false
	targetPath := ""

	for _, arg := range args {
		if arg == "-r" || arg == "--recursive" {
			recursive = true
		} else {
			targetPath = arg
		}
	}

	if targetPath == "" {
		return fmt.Errorf("usage: rmdir [-r] <path>")
	}

	fullPath := ctx.Session.ResolvePath(targetPath)

	if !session.IsValidPath(fullPath) {
		return fmt.Errorf("invalid path: %s", fullPath)
	}

	if err := ctx.Client.DeleteDirectory(fullPath, recursive); err != nil {
		return err
	}

	fmt.Fprintf(ctx.Stdout, "Deleted directory: %s\n", fullPath)
	return nil
}

func (c *RmdirCommand) Help() string {
	return "rmdir [-r] <path> - Remove directory (use -r for recursive)"
}

// CatCommand displays file contents
type CatCommand struct{}

func (c *CatCommand) Execute(ctx *Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: cat [-v version] <path>")
	}

	var version int64
	var path string
	hasVersion := false

	// Parse args
	for i := 0; i < len(args); i++ {
		if args[i] == "-v" && i+1 < len(args) {
			_, err := fmt.Sscanf(args[i+1], "%d", &version)
			if err != nil {
				return fmt.Errorf("invalid version number")
			}
			hasVersion = true
			i++
		} else {
			path = args[i]
		}
	}

	if path == "" {
		return fmt.Errorf("usage: cat [-v version] <path>")
	}

	path = ctx.Session.ResolvePath(path)

	if !session.IsValidPath(path) {
		return fmt.Errorf("invalid path: %s", path)
	}

	var content []byte
	var contentType string
	var err error

	if hasVersion {
		content, contentType, err = ctx.Client.GetFileVersion(path, version)
		if err != nil {
			return err
		}
		// Warn if binary
		if !strings.HasPrefix(contentType, "text/") &&
			!strings.HasPrefix(contentType, "application/json") {
			fmt.Fprintf(ctx.Stderr, "Warning: file is binary (%s)\n", contentType)
		}
		_, err = ctx.Stdout.Write(content)
		if err != nil {
			return err
		}
		// Add newline to ensure prompt appears on new line
		_, err = ctx.Stdout.Write([]byte("\n"))
		return err
	}

	reader, contentType, err := ctx.Client.GetFileStream(path)
	if err != nil {
		return err
	}
	defer reader.Close()

	// Warn if binary
	if !strings.HasPrefix(contentType, "text/") &&
		!strings.HasPrefix(contentType, "application/json") {
		fmt.Fprintf(ctx.Stderr, "Warning: file is binary (%s)\n", contentType)
	}

	// Stream to stdout
	_, err = io.Copy(ctx.Stdout, reader)
	if err != nil {
		return err
	}
	// Add newline to ensure prompt appears on new line
	_, err = ctx.Stdout.Write([]byte("\n"))
	return err
}

func (c *CatCommand) Help() string {
	return "cat [-v version] <path> - Display file contents"
}

// ImportCommand imports a local file to VFS
type ImportCommand struct{}

func (c *ImportCommand) Execute(ctx *Context, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: import <local_path> [vfs_path]")
	}

	localPattern := args[0]
	var vfsBasePath string

	if len(args) == 2 {
		vfsBasePath = args[1]
	} else {
		// Single argument: use current directory
		vfsBasePath = ctx.Session.GetCurrentDirectory()
	}

	// Expand local glob pattern
	localMatches, err := filepath.Glob(localPattern)
	if err != nil {
		return fmt.Errorf("invalid glob pattern: %w", err)
	}

	if len(localMatches) == 0 {
		return fmt.Errorf("no files match pattern: %s", localPattern)
	}

	// Filter out directories
	var localFiles []string
	for _, match := range localMatches {
		fileInfo, err := os.Stat(match)
		if err != nil {
			continue // Skip files that can't be accessed
		}
		if !fileInfo.IsDir() {
			localFiles = append(localFiles, match)
		}
	}

	if len(localFiles) == 0 {
		return fmt.Errorf("no files found matching pattern: %s", localPattern)
	}

	// Resolve VFS base path
	vfsBaseResolved := ctx.Session.ResolvePath(vfsBasePath)
	if !session.IsValidPath(vfsBaseResolved) {
		return fmt.Errorf("invalid VFS path: %s", vfsBaseResolved)
	}

	// Check if VFS base path is a directory
	isVfsDir := false
	if _, err := ctx.Client.ListDirectory(vfsBaseResolved, 1, ""); err == nil {
		isVfsDir = true
	}

	importedCount := 0
	for _, localFile := range localFiles {
		// Read local file
		fileInfo, err := os.Stat(localFile)
		if err != nil {
			fmt.Fprintf(ctx.Stderr, "Failed to read local file %s: %v\n", localFile, err)
			continue
		}

		if fileInfo.Size() > client.MaxFileSize {
			fmt.Fprintf(ctx.Stderr, "File %s size (%d bytes) exceeds maximum allowed (%d bytes)\n",
				localFile, fileInfo.Size(), client.MaxFileSize)
			continue
		}

		content, err := os.ReadFile(localFile)
		if err != nil {
			fmt.Fprintf(ctx.Stderr, "Failed to read file %s: %v\n", localFile, err)
			continue
		}

		// Detect content type
		contentType := "application/octet-stream"
		ext := filepath.Ext(localFile)
		switch ext {
		case ".json":
			contentType = "application/json"
		case ".txt":
			contentType = "text/plain"
		case ".xml":
			contentType = "application/xml"
		case ".csv":
			contentType = "text/csv"
		}

		// Determine VFS path
		var vfsFullPath string
		if isVfsDir {
			// Import into directory with original filename
			fileName := filepath.Base(localFile)
			vfsFullPath = filepath.Join(vfsBaseResolved, fileName)
		} else if len(localFiles) == 1 {
			// Single file to specific path
			vfsFullPath = vfsBaseResolved
		} else {
			// Multiple files but destination is not a directory
			fmt.Fprintf(ctx.Stderr, "Cannot import multiple files to a single destination file\n")
			continue
		}

		if !session.IsValidPath(vfsFullPath) {
			fmt.Fprintf(ctx.Stderr, "Invalid VFS path: %s\n", vfsFullPath)
			continue
		}

		dirPath := filepath.Dir(vfsFullPath)
		fileName := filepath.Base(vfsFullPath)

		// Show progress for large files
		if fileInfo.Size() > 10*1024*1024 {
			fmt.Fprintf(ctx.Stdout, "Uploading %s (%d bytes)...\n", fileName, fileInfo.Size())
		}

		resp, err := ctx.Client.CreateFile(dirPath, fileName, contentType, string(content))
		if err != nil {
			fmt.Fprintf(ctx.Stderr, "Failed to import %s: %v\n", localFile, err)
			continue
		}

		fmt.Fprintf(ctx.Stdout, "Imported: %s -> %s (ID: %s, Version: %d)\n",
			localFile, vfsFullPath, resp.ID, resp.Version)
		importedCount++
	}

	if importedCount == 0 {
		return fmt.Errorf("no files were imported")
	}

	fmt.Fprintf(ctx.Stdout, "Imported %d file(s)\n", importedCount)
	return nil
}

func (c *ImportCommand) Help() string {
	return "import <local_path> [vfs_path] - Import local file to VFS"
}

// RmCommand removes a file
type RmCommand struct{}

func (c *RmCommand) Execute(ctx *Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: rm <path>")
	}

	pattern := ctx.Session.ResolvePath(args[0])

	// Check if pattern contains glob characters
	if hasGlobChars(pattern) {
		matches, err := expandVFSGlob(ctx, pattern)
		if err != nil {
			return err
		}

		deletedCount := 0
		for _, match := range matches {
			if err := ctx.Client.DeleteFile(match); err != nil {
				fmt.Fprintf(ctx.Stderr, "Failed to delete %s: %v\n", match, err)
			} else {
				fmt.Fprintf(ctx.Stdout, "Deleted file: %s\n", match)
				deletedCount++
			}
		}

		if deletedCount == 0 {
			return fmt.Errorf("no files were deleted")
		}

		fmt.Fprintf(ctx.Stdout, "Deleted %d file(s)\n", deletedCount)
		return nil
	}

	// Single file deletion
	if !session.IsValidPath(pattern) {
		return fmt.Errorf("invalid path: %s", pattern)
	}

	if err := ctx.Client.DeleteFile(pattern); err != nil {
		return err
	}

	fmt.Fprintf(ctx.Stdout, "Deleted file: %s\n", pattern)
	return nil
}

func (c *RmCommand) Help() string {
	return "rm <path> - Remove file"
}

// MvCommand moves or renames a file
type MvCommand struct{}

func (c *MvCommand) Execute(ctx *Context, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: mv <source> <destination>")
	}

	sourcePattern := ctx.Session.ResolvePath(args[0])
	destPattern := ctx.Session.ResolvePath(args[1])

	// Expand source pattern if it contains globs
	var sourcePaths []string
	if hasGlobChars(sourcePattern) {
		matches, err := expandVFSGlob(ctx, sourcePattern)
		if err != nil {
			return err
		}
		sourcePaths = matches
	} else {
		if !session.IsValidPath(sourcePattern) {
			return fmt.Errorf("invalid source path: %s", sourcePattern)
		}
		sourcePaths = []string{sourcePattern}
	}

	// Check if destination is a directory
	isDestDir := false
	if _, err := ctx.Client.ListDirectory(destPattern, 1, ""); err == nil {
		isDestDir = true
	}

	movedCount := 0
	for _, sourcePath := range sourcePaths {
		var finalDest string

		if isDestDir {
			// Moving into a directory - keep the original filename
			fileName := filepath.Base(sourcePath)
			finalDest = filepath.Join(destPattern, fileName)
		} else if len(sourcePaths) > 1 {
			// Multiple sources but destination is not a directory - not allowed
			return fmt.Errorf("cannot move multiple files to a single destination file")
		} else {
			// Single file rename/move
			finalDest = destPattern
		}

		if !session.IsValidPath(finalDest) {
			return fmt.Errorf("invalid destination path: %s", finalDest)
		}

		resp, err := ctx.Client.MoveFile(sourcePath, finalDest)
		if err != nil {
			fmt.Fprintf(ctx.Stderr, "Failed to move %s: %v\n", sourcePath, err)
			continue
		}

		fmt.Fprintf(ctx.Stdout, "Moved: %s -> %s (ID: %s)\n", sourcePath, finalDest, resp.ID)
		movedCount++
	}

	if movedCount == 0 {
		return fmt.Errorf("no files were moved")
	}

	fmt.Fprintf(ctx.Stdout, "Moved %d file(s)\n", movedCount)
	return nil
}

func (c *MvCommand) Help() string {
	return "mv <source> <destination> - Move or rename file"
}

// JqCommand queries JSON files
type JqCommand struct{}

func (c *JqCommand) Execute(ctx *Context, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: jq <path> [expression]")
	}

	path := ctx.Session.ResolvePath(args[0])
	expression := "."
	if len(args) >= 2 {
		expression = args[1]
	}

	if !session.IsValidPath(path) {
		return fmt.Errorf("invalid path: %s", path)
	}

	// Get file content
	content, contentType, _, err := ctx.Client.GetFile(path)
	if err != nil {
		return err
	}

	if !strings.Contains(contentType, "json") {
		return fmt.Errorf("file is not JSON (content-type: %s)", contentType)
	}

	// Execute jq command
	cmd := exec.Command("jq", expression)
	cmd.Stdin = strings.NewReader(string(content))
	cmd.Stdout = ctx.Stdout
	cmd.Stderr = ctx.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("jq command failed: %w", err)
	}

	return nil
}

func (c *JqCommand) Help() string {
	return "jq <path> [expression] - Query JSON file with jq (default expression: .)"
}

// SaveToken saves the auth token to a file (for external auth providers)
func SaveToken(token string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	vfsDir := filepath.Join(homeDir, ".vfs")
	if err := os.MkdirAll(vfsDir, 0700); err != nil {
		return err
	}

	tokenFile := filepath.Join(vfsDir, "token")
	return os.WriteFile(tokenFile, []byte(token), 0600)
}

// LoadToken loads the auth token from a file
func LoadToken() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	tokenFile := filepath.Join(homeDir, ".vfs", "token")
	data, err := os.ReadFile(tokenFile)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(data)), nil
}

// RemoveToken removes the saved auth token
func RemoveToken() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	tokenFile := filepath.Join(homeDir, ".vfs", "token")
	return os.Remove(tokenFile)
}

// HelpCommand shows help
type HelpCommand struct {
	commands map[string]Command
}

func NewHelpCommand(commands map[string]Command) *HelpCommand {
	return &HelpCommand{commands: commands}
}

func (c *HelpCommand) Execute(ctx *Context, args []string) error {
	fmt.Fprintln(ctx.Stdout, "Available commands:")
	fmt.Fprintln(ctx.Stdout, "")
	fmt.Fprintln(ctx.Stdout, "Files & Directories:")
	fmt.Fprintln(ctx.Stdout, "  ls [-r] [-l] [path]                List directory contents (supports globs, -l for details)")
	fmt.Fprintln(ctx.Stdout, "  cd [path]                          Change directory")
	fmt.Fprintln(ctx.Stdout, "  pwd                                Print working directory")
	fmt.Fprintln(ctx.Stdout, "  mkdir <name>                       Create directory")
	fmt.Fprintln(ctx.Stdout, "  rmdir [-r] <path>                  Remove directory")
	fmt.Fprintln(ctx.Stdout, "  import <local> [vfs]                Import file(s) to VFS (supports globs)")
	fmt.Fprintln(ctx.Stdout, "  cat <path>                         Display file contents")
	fmt.Fprintln(ctx.Stdout, "  version <path>                     Show file version history")
	fmt.Fprintln(ctx.Stdout, "  jq <path> [expression]             Query JSON file (default: ., supports coloring)")
	fmt.Fprintln(ctx.Stdout, "  mv <src> <dst>                     Move/rename file(s) (supports globs)")
	fmt.Fprintln(ctx.Stdout, "  rm <path>                          Remove file(s) (supports globs)")
	fmt.Fprintln(ctx.Stdout, "")
	fmt.Fprintln(ctx.Stdout, "Authentication:")
	fmt.Fprintln(ctx.Stdout, "  login <username> <password>        Authenticate with VFS")
	fmt.Fprintln(ctx.Stdout, "  logout                             Clear authentication")
	fmt.Fprintln(ctx.Stdout, "  whoami                             Show current user")
	fmt.Fprintln(ctx.Stdout, "")
	fmt.Fprintln(ctx.Stdout, "Admin:")
	fmt.Fprintln(ctx.Stdout, "  create-user <user> <email> <pw>    Create new user (admin)")
	fmt.Fprintln(ctx.Stdout, "  list-users                         List all users (admin)")
	fmt.Fprintln(ctx.Stdout, "")
	fmt.Fprintln(ctx.Stdout, "Special Files & Triggers:")
	fmt.Fprintln(ctx.Stdout, "  create-sample-files <dir>          Create sample _files configs")
	fmt.Fprintln(ctx.Stdout, "  create-trigger <dir> <url>         Create webhook trigger for file events")
	fmt.Fprintln(ctx.Stdout, "  create-schema <dir> <file>         Create .jsonschema (admin)")
	fmt.Fprintln(ctx.Stdout, "  create-policy <dir> <file>         Create .rego policy (admin)")
	fmt.Fprintln(ctx.Stdout, "")
	fmt.Fprintln(ctx.Stdout, "Other:")
	fmt.Fprintln(ctx.Stdout, "  help                               Show this help")
	fmt.Fprintln(ctx.Stdout, "  exit                               Exit CLI")
	fmt.Fprintln(ctx.Stdout, "")
	return nil
}

func (c *HelpCommand) Help() string {
	return "help - Show available commands"
}

// TreeCommand displays directory tree
type TreeCommand struct{}

func (c *TreeCommand) Execute(ctx *Context, args []string) error {
	maxDepth := 3
	targetPath := ""

	// Parse flags and args
	for i := 0; i < len(args); i++ {
		if args[i] == "-d" && i+1 < len(args) {
			depth, err := fmt.Sscanf(args[i+1], "%d", &maxDepth)
			if err != nil || depth != 1 {
				return fmt.Errorf("invalid depth value")
			}
			i++
		} else if !strings.HasPrefix(args[i], "-") {
			targetPath = args[i]
		}
	}

	if targetPath == "" {
		targetPath = ctx.Session.GetCurrentDirectory()
	} else {
		targetPath = ctx.Session.ResolvePath(targetPath)
	}

	if !session.IsValidPath(targetPath) {
		return fmt.Errorf("invalid path: %s", targetPath)
	}

	fmt.Fprintf(ctx.Stdout, "%s/\n", targetPath)
	return c.printTree(ctx, targetPath, "", 0, maxDepth)
}

func (c *TreeCommand) printTree(ctx *Context, path, prefix string, depth, maxDepth int) error {
	if depth >= maxDepth {
		return nil
	}

	resp, err := ctx.Client.ListDirectory(path, 100, "")
	if err != nil {
		return err
	}

	for i, entry := range resp.Entries {
		isLast := i == len(resp.Entries)-1
		connector := "├── "
		extension := "│   "
		if isLast {
			connector = "└── "
			extension = "    "
		}

		if entry.Type == "directory" {
			fmt.Fprintf(ctx.Stdout, "%s%s%s/\n", prefix, connector, entry.Name)
			subPath := filepath.Join(path, entry.Name)
			if err := c.printTree(ctx, subPath, prefix+extension, depth+1, maxDepth); err != nil {
				return err
			}
		} else {
			fmt.Fprintf(ctx.Stdout, "%s%s%s\n", prefix, connector, entry.Name)
		}
	}

	return nil
}

func (c *TreeCommand) Help() string {
	return "tree [-d depth] [path] - Display directory tree (default depth: 3)"
}

// EditCommand edits a file using $EDITOR or vim
type EditCommand struct{}

func (c *EditCommand) Execute(ctx *Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: edit <path>")
	}

	path := ctx.Session.ResolvePath(args[0])
	if !session.IsValidPath(path) {
		return fmt.Errorf("invalid path: %s", path)
	}

	// Download file to temp location
	tmpFile, err := os.CreateTemp("", "vfs-edit-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	// Try to get existing content
	fileExists := false
	content, _, _, err := ctx.Client.GetFile(path)
	if err == nil {
		fileExists = true
		if _, err := tmpFile.Write(content); err != nil {
			tmpFile.Close()
			return fmt.Errorf("failed to write temp file: %w", err)
		}
	}
	tmpFile.Close()

	// Get editor
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vim"
	}

	// Open editor
	cmd := exec.Command(editor, tmpPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("editor failed: %w", err)
	}

	// Read edited content
	editedContent, err := os.ReadFile(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to read edited file: %w", err)
	}

	// Upload to VFS
	if fileExists {
		// Update existing file
		_, err = ctx.Client.UpdateFile(path, "text/plain", string(editedContent), 0) // ExpectedVersion 0 means any version
		if err != nil {
			return fmt.Errorf("failed to update file: %w", err)
		}
	} else {
		// Create new file
		dirPath := filepath.Dir(path)
		fileName := filepath.Base(path)
		_, err = ctx.Client.CreateFile(dirPath, fileName, "text/plain", string(editedContent))
		if err != nil {
			return fmt.Errorf("failed to create file: %w", err)
		}
	}

	fmt.Fprintf(ctx.Stdout, "File saved: %s\n", path)
	return nil
}

func (c *EditCommand) Help() string {
	return "edit <path> - Edit file using $EDITOR or vim"
}

// VersionCommand shows version history of a file
type VersionCommand struct{}

func (c *VersionCommand) Execute(ctx *Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: version <path>")
	}

	path := ctx.Session.ResolvePath(args[0])
	if !session.IsValidPath(path) {
		return fmt.Errorf("invalid path: %s", path)
	}

	versions, err := ctx.Client.ListVersions(path)
	if err != nil {
		return fmt.Errorf("failed to list versions: %w", err)
	}

	if len(versions) == 0 {
		fmt.Fprintln(ctx.Stdout, "No versions found")
		return nil
	}

	fmt.Fprintf(ctx.Stdout, "Version history for %s:\n", path)
	fmt.Fprintln(ctx.Stdout, "Version  Size       Created At          Content Type")
	fmt.Fprintln(ctx.Stdout, "-------- ---------- ------------------- ----------------")

	for _, version := range versions {
		sizeStr := fmt.Sprintf("%d bytes", version.SizeBytes)
		if version.SizeBytes >= 1024*1024 {
			sizeStr = fmt.Sprintf("%.1f MB", float64(version.SizeBytes)/(1024*1024))
		} else if version.SizeBytes >= 1024 {
			sizeStr = fmt.Sprintf("%.1f KB", float64(version.SizeBytes)/1024)
		}

		fmt.Fprintf(ctx.Stdout, "%-8d %-10s %-19s %s\n",
			version.Version,
			sizeStr,
			version.CreatedAt.Format("2006-01-02 15:04:05"),
			version.ContentType)
	}

	return nil
}

func (c *VersionCommand) Help() string {
	return "version <path> - Show version history of a file"
}

// LoginCommand authenticates with VFS
type LoginCommand struct{}

func (c *LoginCommand) Execute(ctx *Context, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: login <username> <password>")
	}

	username := args[0]
	password := args[1]

	token, err := ctx.Client.Login(username, password)
	if err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	ctx.Client.SetAuthToken(token)
	ctx.Session.SetAuthToken(token)

	// Save token
	if err := SaveToken(token); err != nil {
		fmt.Fprintf(ctx.Stderr, "Warning: failed to save token: %v\n", err)
	}

	fmt.Fprintf(ctx.Stdout, "Logged in as %s\n", username)
	return nil
}

func (c *LoginCommand) Help() string {
	return "login <username> <password> - Authenticate with VFS"
}

// LogoutCommand clears authentication
type LogoutCommand struct{}

func (c *LogoutCommand) Execute(ctx *Context, args []string) error {
	ctx.Client.SetAuthToken("")
	ctx.Session.SetAuthToken("")

	if err := RemoveToken(); err != nil {
		// Ignore error if file doesn't exist
	}

	fmt.Fprintf(ctx.Stdout, "Logged out\n")
	return nil
}

func (c *LogoutCommand) Help() string {
	return "logout - Clear authentication"
}

// HistoryCommand shows command history
type HistoryCommand struct {
	history []string
}

func NewHistoryCommand(history []string) *HistoryCommand {
	return &HistoryCommand{history: history}
}

func (c *HistoryCommand) Execute(ctx *Context, args []string) error {
	for i, cmd := range c.history {
		fmt.Fprintf(ctx.Stdout, "%4d  %s\n", i+1, cmd)
	}
	return nil
}

func (c *HistoryCommand) Help() string {
	return "history - Show command history"
}

// GetCwdCommand shows local working directory
type GetCwdCommand struct{}

func (c *GetCwdCommand) Execute(ctx *Context, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	fmt.Fprintln(ctx.Stdout, cwd)
	return nil
}

func (c *GetCwdCommand) Help() string {
	return "get-cwd - Show local working directory"
}

// SetCwdCommand changes local working directory
type SetCwdCommand struct{}

func (c *SetCwdCommand) Execute(ctx *Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: set-cwd <path>")
	}

	if err := os.Chdir(args[0]); err != nil {
		return fmt.Errorf("failed to change directory: %w", err)
	}

	cwd, _ := os.Getwd()
	fmt.Fprintf(ctx.Stdout, "Local directory: %s\n", cwd)
	return nil
}

func (c *SetCwdCommand) Help() string {
	return "set-cwd <path> - Change local working directory"
}

// CreateSampleFilesCommand creates sample _files configuration files
type CreateSampleFilesCommand struct{}

func (c *CreateSampleFilesCommand) Execute(ctx *Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: create-sample-files <directory>")
	}

	targetDir := ctx.Session.ResolvePath(args[0])
	if !session.IsValidPath(targetDir) {
		return fmt.Errorf("invalid path: %s", targetDir)
	}

	// Verify directory exists
	_, err := ctx.Client.ListDirectory(targetDir, 1, "")
	if err != nil {
		return fmt.Errorf("directory not found: %s", targetDir)
	}

	// Create sample _files for different scenarios
	samples := []struct {
		name        string
		content     string
		description string
	}{
		{
			name: "_files_basic",
			content: `{
  "rules": [
    {
      "pattern": "*.json",
      "type": "glob",
      "schema": {
        "type": "object",
        "properties": {
          "name": {"type": "string"},
          "age": {"type": "integer", "minimum": 0}
        },
        "required": ["name"]
      }
    }
  ]
}`,
			description: "Basic inline schema validation",
		},
		{
			name: "_files_external_schema",
			content: `{
  "rules": [
    {
      "pattern": "user-*.json",
      "type": "glob",
      "schema": {
        "$ref": "schema:///schemas/user.json"
      }
    }
  ]
}`,
			description: "External schema reference",
		},
		{
			name: "_files_nested_schema",
			content: `{
  "rules": [
    {
      "pattern": "*.json",
      "type": "glob",
      "schema": {
        "$ref": "schema:///schemas/person.json"
      }
    }
  ]
}`,
			description: "Nested schema references",
		},
		{
			name: "_files_multiple_rules",
			content: `{
  "rules": [
    {
      "pattern": "config-*.json",
      "type": "glob",
      "schema": {
        "type": "object",
        "properties": {
          "version": {"type": "string"},
          "settings": {"type": "object"}
        },
        "required": ["version"]
      }
    },
    {
      "pattern": "data-*.json",
      "type": "glob",
      "schema": {
        "type": "object",
        "properties": {
          "id": {"type": "integer"},
          "data": {"type": "string"}
        },
        "required": ["id", "data"]
      }
    }
  ]
}`,
			description: "Multiple validation rules",
		},
	}

	// Create sample schema files first
	schemaDir := filepath.Join(targetDir, "schemas")
	_, err = ctx.Client.CreateDirectory(filepath.Dir(schemaDir), filepath.Base(schemaDir))
	if err != nil {
		// Directory might already exist, continue
		fmt.Fprintf(ctx.Stderr, "Warning: Could not create schemas directory: %v\n", err)
	}

	schemaSamples := []struct {
		name    string
		content string
	}{
		{
			name: "user.json",
			content: `{
  "type": "object",
  "properties": {
    "name": {"type": "string"},
    "email": {"type": "string", "format": "email"}
  },
  "required": ["name", "email"]
}`,
		},
		{
			name: "address.json",
			content: `{
  "type": "object",
  "properties": {
    "street": {"type": "string"},
    "city": {"type": "string"},
    "zip": {"type": "string"}
  },
  "required": ["street", "city"]
}`,
		},
		{
			name: "person.json",
			content: `{
  "type": "object",
  "properties": {
    "name": {"type": "string"},
    "address": {
      "$ref": "schema:///schemas/address.json"
    }
  },
  "required": ["name", "address"]
}`,
		},
	}

	// Create schema files
	for _, schema := range schemaSamples {
		schemaPath := filepath.Join(schemaDir, schema.name)
		dirPath := filepath.Dir(schemaPath)
		fileName := filepath.Base(schemaPath)

		_, err := ctx.Client.CreateFile(dirPath, fileName, "application/json", schema.content)
		if err != nil {
			fmt.Fprintf(ctx.Stderr, "Warning: Could not create schema file %s: %v\n", schemaPath, err)
		} else {
			fmt.Fprintf(ctx.Stdout, "Created schema file: %s\n", schemaPath)
		}
	}

	// Create _files configuration files
	for _, sample := range samples {
		filePath := filepath.Join(targetDir, sample.name)
		dirPath := filepath.Dir(filePath)
		fileName := filepath.Base(filePath)

		_, err := ctx.Client.CreateFile(dirPath, fileName, "application/json", sample.content)
		if err != nil {
			return fmt.Errorf("failed to create %s: %v", filePath, err)
		}

		fmt.Fprintf(ctx.Stdout, "Created sample file: %s (%s)\n", filePath, sample.description)
	}

	fmt.Fprintf(ctx.Stdout, "\nSample files created successfully!\n")
	fmt.Fprintf(ctx.Stdout, "Note: Rename _files to .files to activate validation rules.\n")
	fmt.Fprintf(ctx.Stdout, "Example: mv %s/_files_basic %s/.files\n", targetDir, targetDir)

	return nil
}

func (c *CreateSampleFilesCommand) Help() string {
	return "create-sample-files <directory> - Create sample _files configuration files"
}

// CreateTriggerCommand creates a webhook trigger for file operations
type CreateTriggerCommand struct{}

func (c *CreateTriggerCommand) Execute(ctx *Context, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: create-trigger <directory> <webhook_url> [event_type]")
	}

	targetDir := ctx.Session.ResolvePath(args[0])
	webhookURL := args[1]
	eventType := "file.create.completion.succeeded"

	if len(args) >= 3 {
		eventType = args[2]
	}

	if !session.IsValidPath(targetDir) {
		return fmt.Errorf("invalid path: %s", targetDir)
	}

	// Verify directory exists
	_, err := ctx.Client.ListDirectory(targetDir, 1, "")
	if err != nil {
		return fmt.Errorf("directory not found: %s", targetDir)
	}

	// Create .events configuration
	eventsConfig := fmt.Sprintf(`{
  "handlers": [
    {
      "name": "webhook-trigger",
      "events": ["%s"],
      "type": "webhook",
      "config": {
        "url": "%s",
        "method": "POST",
        "headers": {
          "Content-Type": "application/json"
        }
      }
    }
  ]
}`, eventType, webhookURL)

	eventsPath := filepath.Join(targetDir, ".events")
	dirPath := filepath.Dir(eventsPath)
	fileName := filepath.Base(eventsPath)

	// Check if .events already exists
	_, _, _, err = ctx.Client.GetFile(eventsPath)
	if err == nil {
		return fmt.Errorf(".events file already exists at %s, please edit it manually or delete it first", eventsPath)
	}

	_, err = ctx.Client.CreateFile(dirPath, fileName, "application/json", eventsConfig)
	if err != nil {
		return fmt.Errorf("failed to create .events file: %v", err)
	}

	fmt.Fprintf(ctx.Stdout, "Created webhook trigger at: %s\n", eventsPath)
	fmt.Fprintf(ctx.Stdout, "Event: %s\n", eventType)
	fmt.Fprintf(ctx.Stdout, "URL: %s\n", webhookURL)
	fmt.Fprintf(ctx.Stdout, "\nThe webhook will be called when files are created in %s\n", targetDir)
	fmt.Fprintf(ctx.Stdout, "\nPayload structure:\n")
	fmt.Fprintf(ctx.Stdout, "{\n")
	fmt.Fprintf(ctx.Stdout, "  \"event\": { \"id\": \"...\", \"type\": \"%s\", ... },\n", eventType)
	fmt.Fprintf(ctx.Stdout, "  \"actor\": { \"user_id\": \"...\", \"username\": \"...\" },\n")
	fmt.Fprintf(ctx.Stdout, "  \"file\": { \"id\": \"...\", \"name\": \"...\", \"path\": \"...\", ... }\n")
	fmt.Fprintf(ctx.Stdout, "}\n")

	return nil
}

func (c *CreateTriggerCommand) Help() string {
	return "create-trigger <directory> <webhook_url> [event_type] - Create webhook trigger for file events"
}
