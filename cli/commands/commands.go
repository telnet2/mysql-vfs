package commands

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
	_, _, _, _, err = ctx.Client.GetFile(targetPath)
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
	content, contentType, version, modifiedAt, err := ctx.Client.GetFile(path)
	if err != nil {
		return err
	}

	// Detect content type if not provided
	if contentType == "" {
		contentType = detectContentType(filepath.Base(path))
	}

	// Use version from GetFile response
	latestVersion := version

	// Use modification time from GetFile response
	timestamp := "unknown"
	if !modifiedAt.IsZero() {
		timestamp = modifiedAt.Format("2006-01-02 15:04:05")
	}

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
		content, contentType, _, _, err := ctx.Client.GetFile(fullPath)
		if err != nil {
			// Fallback to basic info if we can't get file details
			sizeStr := c.formatSize(entry.SizeBytes)
			contentType = detectContentType(name)
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
			contentType = detectContentType(name)
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
				content, contentType, _, _, err := ctx.Client.GetFile(fullPath)
				if err != nil {
					contentType = detectContentType(entry.Name)
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
					contentType = detectContentType(entry.Name)
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
		return fmt.Errorf("usage: cat [-v version] [-i] <path> - Display file contents (shows version info with -v or -i)")
	}

	var version int64
	var path string
	hasVersion := false
	showInfo := false

	// Parse args
	for i := 0; i < len(args); i++ {
		if args[i] == "-v" && i+1 < len(args) {
			_, err := fmt.Sscanf(args[i+1], "%d", &version)
			if err != nil {
				return fmt.Errorf("invalid version number")
			}
			hasVersion = true
			i++
		} else if args[i] == "-i" {
			showInfo = true
		} else {
			path = args[i]
		}
	}

	if path == "" {
		return fmt.Errorf("usage: cat [-v version] [-i] <path> - Display file contents (shows version info with -v or -i)")
	}

	path = ctx.Session.ResolvePath(path)

	if !session.IsValidPath(path) {
		return fmt.Errorf("invalid path: %s", path)
	}

	var contentType string
	var err error

	if hasVersion {
		content, contentType, actualVersion, err := ctx.Client.GetFileVersion(path, version)
		if err != nil {
			return err
		}
		// Display version information
		fmt.Fprintf(ctx.Stdout, "version: %d\n", actualVersion)
		fmt.Fprintf(ctx.Stdout, "payload:\n")
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

	reader, contentType, actualVersion, err := ctx.Client.GetFileStream(path)
	if err != nil {
		return err
	}
	defer reader.Close()

	// Display version information if requested
	if showInfo {
		fmt.Fprintf(ctx.Stdout, "version: %d\n", actualVersion)
		fmt.Fprintf(ctx.Stdout, "payload:\n")
	}

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
	return "cat [-v version] [-i] <path> - Display file contents (shows version info with -v or -i)"
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

// CpCommand copies files
type CpCommand struct{}

func (c *CpCommand) Execute(ctx *Context, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: cp <source> <destination>")
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

	// Check if any source is a directory
	for _, sourcePath := range sourcePaths {
		// Try to list as directory - if successful, it's a directory
		if _, err := ctx.Client.ListDirectory(sourcePath, 1, ""); err == nil {
			return fmt.Errorf("cp does not support copying directories: %s", sourcePath)
		}
	}

	// Check if destination is a directory
	isDestDir := false
	if _, err := ctx.Client.ListDirectory(destPattern, 1, ""); err == nil {
		isDestDir = true
	}

	copiedCount := 0
	for _, sourcePath := range sourcePaths {
		var finalDest string

		if isDestDir {
			// Copying into a directory - keep the original filename
			fileName := filepath.Base(sourcePath)
			finalDest = filepath.Join(destPattern, fileName)
		} else if len(sourcePaths) > 1 {
			// Multiple sources but destination is not a directory - not allowed
			return fmt.Errorf("cannot copy multiple files to a single destination file")
		} else {
			// Single file copy
			finalDest = destPattern
		}

		if !session.IsValidPath(finalDest) {
			return fmt.Errorf("invalid destination path: %s", finalDest)
		}

		// Get source file content
		content, contentType, _, _, err := ctx.Client.GetFile(sourcePath)
		if err != nil {
			fmt.Fprintf(ctx.Stderr, "Failed to read source file %s: %v\n", sourcePath, err)
			continue
		}

		// Show progress for large files
		fileSize := len(content)
		if fileSize > 1024*1024 { // Show progress for files > 1MB
			fmt.Fprintf(ctx.Stdout, "Copying %s (%d bytes)...\n", sourcePath, fileSize)
		}

		// Create destination file
		dirPath := filepath.Dir(finalDest)
		fileName := filepath.Base(finalDest)

		_, err = ctx.Client.CreateFile(dirPath, fileName, contentType, string(content))
		if err != nil {
			fmt.Fprintf(ctx.Stderr, "Failed to copy %s to %s: %v\n", sourcePath, finalDest, err)
			continue
		}

		if fileSize > 1024*1024 {
			fmt.Fprintf(ctx.Stdout, "Completed: %s -> %s\n", sourcePath, finalDest)
		} else {
			fmt.Fprintf(ctx.Stdout, "Copied: %s -> %s\n", sourcePath, finalDest)
		}
		copiedCount++
	}

	if copiedCount == 0 {
		return fmt.Errorf("no files were copied")
	}

	fmt.Fprintf(ctx.Stdout, "Copied %d file(s)\n", copiedCount)
	return nil
}

func (c *CpCommand) Help() string {
	return "cp <source> <destination> - Copy file(s) (supports glob patterns)"
}

// GrepCommand searches for patterns in files
type GrepCommand struct{}

func (c *GrepCommand) Execute(ctx *Context, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: grep <pattern> <path>")
	}

	pattern := args[0]
	searchPath := ctx.Session.ResolvePath(args[1])

	if !session.IsValidPath(searchPath) {
		return fmt.Errorf("invalid path: %s", searchPath)
	}

	// Check if search path is a directory or file
	isDir := false
	if _, err := ctx.Client.ListDirectory(searchPath, 1, ""); err == nil {
		isDir = true
	}

	if isDir {
		return c.searchDirectory(ctx, pattern, searchPath)
	} else {
		return c.searchFile(ctx, pattern, searchPath)
	}
}

func (c *GrepCommand) searchDirectory(ctx *Context, pattern, dirPath string) error {
	// Walk through directory recursively
	return c.walkDirectory(ctx, pattern, dirPath, "")
}

func (c *GrepCommand) walkDirectory(ctx *Context, pattern, dirPath, prefix string) error {
	resp, err := ctx.Client.ListDirectory(dirPath, 1000, "")
	if err != nil {
		return err
	}

	for _, entry := range resp.Entries {
		fullPath := filepath.Join(dirPath, entry.Name)

		if entry.Type == "directory" {
			// Recurse into subdirectory
			if err := c.walkDirectory(ctx, pattern, fullPath, prefix+entry.Name+"/"); err != nil {
				return err
			}
		} else {
			// Search in file
			if err := c.searchFile(ctx, pattern, fullPath); err != nil {
				// Continue on error, just log it
				fmt.Fprintf(ctx.Stderr, "Warning: failed to search %s: %v\n", fullPath, err)
			}
		}
	}

	return nil
}

func (c *GrepCommand) searchFile(ctx *Context, pattern, filePath string) error {
	// Get file content
	content, contentType, _, _, err := ctx.Client.GetFile(filePath)
	if err != nil {
		return err
	}

	// Skip binary files (non-text content types)
	if !strings.Contains(contentType, "text/") &&
		!strings.Contains(contentType, "json") &&
		!strings.Contains(contentType, "xml") &&
		contentType != "application/octet-stream" { // Allow octet-stream as it might be text
		return nil
	}

	contentStr := string(content)
	lines := strings.Split(contentStr, "\n")

	found := false
	for i, line := range lines {
		if strings.Contains(line, pattern) {
			if !found {
				fmt.Fprintf(ctx.Stdout, "%s:\n", filePath)
				found = true
			}
			fmt.Fprintf(ctx.Stdout, "%d: %s\n", i+1, line)
		}
	}

	return nil
}

func (c *GrepCommand) Help() string {
	return "grep <pattern> <path> - Search for pattern in file contents"
}

// FindCommand finds files and directories
type FindCommand struct{}

func (c *FindCommand) Execute(ctx *Context, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: find <path> [-name pattern] [-type f|d] [-size +n|-n|n]")
	}

	searchPath := ctx.Session.ResolvePath(args[0])

	if !session.IsValidPath(searchPath) {
		return fmt.Errorf("invalid path: %s", searchPath)
	}

	// Parse options
	var namePattern string
	var fileType string
	var sizeOp string
	var sizeValue int64

	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "-name":
			if i+1 < len(args) {
				namePattern = args[i+1]
				i++
			}
		case "-type":
			if i+1 < len(args) {
				fileType = args[i+1]
				i++
			}
		case "-size":
			if i+1 < len(args) {
				sizeStr := args[i+1]
				if len(sizeStr) > 0 {
					switch sizeStr[0] {
					case '+':
						sizeOp = "+"
						sizeStr = sizeStr[1:]
					case '-':
						sizeOp = "-"
						sizeStr = sizeStr[1:]
					default:
						sizeOp = "="
					}
				}
				// Parse size (assume bytes for now)
				if _, err := fmt.Sscanf(sizeStr, "%d", &sizeValue); err != nil {
					return fmt.Errorf("invalid size format: %s", args[i+1])
				}
				i++
			}
		default:
			return fmt.Errorf("unknown option: %s", args[i])
		}
	}

	// Start search
	return c.findRecursive(ctx, searchPath, namePattern, fileType, sizeOp, sizeValue, "")
}

func (c *FindCommand) findRecursive(ctx *Context, path, namePattern, fileType, sizeOp string, sizeValue int64, prefix string) error {
	// Check if current path matches criteria
	if c.matchesCriteria(path, namePattern, fileType, sizeOp, sizeValue, ctx) {
		fmt.Fprintln(ctx.Stdout, path)
	}

	// If it's a directory, recurse
	resp, err := ctx.Client.ListDirectory(path, 1000, "")
	if err != nil {
		// Not a directory, continue
		return nil
	}

	for _, entry := range resp.Entries {
		fullPath := filepath.Join(path, entry.Name)

		// Check if entry matches criteria
		if c.matchesEntry(entry, fullPath, namePattern, fileType, sizeOp, sizeValue) {
			fmt.Fprintln(ctx.Stdout, fullPath)
		}

		// Recurse into directories
		if entry.Type == "directory" {
			if err := c.findRecursive(ctx, fullPath, namePattern, fileType, sizeOp, sizeValue, prefix+entry.Name+"/"); err != nil {
				return err
			}
		}
	}

	return nil
}

func (c *FindCommand) matchesCriteria(path, namePattern, fileType, sizeOp string, sizeValue int64, ctx *Context) bool {
	// For the root path, check if it matches
	baseName := filepath.Base(path)

	// Check name pattern
	if namePattern != "" {
		if !c.matchesPattern(baseName, namePattern) {
			return false
		}
	}

	// Check file type
	if fileType != "" {
		isDir := false
		if _, err := ctx.Client.ListDirectory(path, 1, ""); err == nil {
			isDir = true
		}

		if fileType == "d" && !isDir {
			return false
		}
		if fileType == "f" && isDir {
			return false
		}
	}

	// Size check for files
	if sizeOp != "" && !c.isDirectory(path, ctx) {
		content, _, _, _, err := ctx.Client.GetFile(path)
		if err != nil {
			return false
		}
		fileSize := int64(len(content))

		switch sizeOp {
		case "+":
			if fileSize <= sizeValue {
				return false
			}
		case "-":
			if fileSize >= sizeValue {
				return false
			}
		case "=":
			if fileSize != sizeValue {
				return false
			}
		}
	}

	return true
}

func (c *FindCommand) matchesEntry(entry client.DirectoryEntry, fullPath, namePattern, fileType, sizeOp string, sizeValue int64) bool {
	// Check name pattern
	if namePattern != "" {
		if !c.matchesPattern(entry.Name, namePattern) {
			return false
		}
	}

	// Check file type
	if fileType != "" {
		var entryType string
		if entry.Type == "directory" {
			entryType = "d"
		} else {
			entryType = "f"
		}

		if fileType != entryType {
			return false
		}
	}

	// Size check
	if sizeOp != "" && entry.Type != "directory" {
		fileSize := entry.SizeBytes

		switch sizeOp {
		case "+":
			if fileSize <= sizeValue {
				return false
			}
		case "-":
			if fileSize >= sizeValue {
				return false
			}
		case "=":
			if fileSize != sizeValue {
				return false
			}
		}
	}

	return true
}

func (c *FindCommand) matchesPattern(name, pattern string) bool {
	// Simple glob matching for now
	if strings.Contains(pattern, "*") {
		// Convert simple glob to regex
		regexPattern := strings.ReplaceAll(pattern, "*", ".*")
		regexPattern = "^" + regexPattern + "$"
		matched, _ := regexp.MatchString(regexPattern, name)
		return matched
	}
	return name == pattern
}

func (c *FindCommand) isDirectory(path string, ctx *Context) bool {
	_, err := ctx.Client.ListDirectory(path, 1, "")
	return err == nil
}

func (c *FindCommand) Help() string {
	return "find <path> [-name pattern] [-type f|d] [-size +n|-n|n] - Find files and directories"
}

// AttrCommand gets or sets file attributes
type AttrCommand struct{}

func (c *AttrCommand) Execute(ctx *Context, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: attr <command> <path> [key or key=value] ...\nCommands: get, set")
	}

	command := args[0]
	path := ctx.Session.ResolvePath(args[1])

	if !session.IsValidPath(path) {
		return fmt.Errorf("invalid path: %s", path)
	}

	switch command {
	case "get":
		return c.getAttributes(ctx, path)
	case "set":
		if len(args) < 3 {
			return fmt.Errorf("usage: attr set <path> key=value")
		}
		return c.setAttributes(ctx, path, args[2:])
	default:
		return fmt.Errorf("unknown command: %s. Use 'get' or 'set'", command)
	}
}

func (c *AttrCommand) getAttributes(ctx *Context, path string) error {
	// Get file metadata
	metadata, err := ctx.Client.GetFileMetadata(path, 0)
	if err != nil {
		return fmt.Errorf("failed to get file metadata: %w", err)
	}

	fmt.Fprintf(ctx.Stdout, "Path: %s\n", path)
	fmt.Fprintf(ctx.Stdout, "ID: %s\n", metadata["id"])
	fmt.Fprintf(ctx.Stdout, "Name: %s\n", metadata["name"])
	fmt.Fprintf(ctx.Stdout, "Content-Type: %s\n", metadata["content_type"])
	fmt.Fprintf(ctx.Stdout, "Size: %v bytes\n", metadata["size_bytes"])
	fmt.Fprintf(ctx.Stdout, "Version: %v\n", metadata["version"])
	fmt.Fprintf(ctx.Stdout, "Storage-Type: %s\n", metadata["storage_type"])
	fmt.Fprintf(ctx.Stdout, "Checksum: %s\n", metadata["checksum"])
	fmt.Fprintf(ctx.Stdout, "Created-At: %s\n", metadata["created_at"])
	fmt.Fprintf(ctx.Stdout, "Updated-At: %s\n", metadata["updated_at"])

	// Display metadata fields
	if meta, ok := metadata["metadata"].(map[string]interface{}); ok {
		fmt.Fprintf(ctx.Stdout, "Owner: %v\n", meta["owner"])
		fmt.Fprintf(ctx.Stdout, "Creator: %v\n", meta["creator"])
		if updatedBy, exists := meta["updated_by"]; exists {
			fmt.Fprintf(ctx.Stdout, "Updated-By: %v\n", updatedBy)
		}
		if system, exists := meta["system"].(bool); exists && system {
			fmt.Fprintf(ctx.Stdout, "System: %v\n", system)
		}
		if delegated, exists := meta["delegated"].(bool); exists && delegated {
			fmt.Fprintf(ctx.Stdout, "Delegated: %v\n", delegated)
			if reason, exists := meta["delegation_reason"]; exists {
				fmt.Fprintf(ctx.Stdout, "Delegation-Reason: %v\n", reason)
			}
		}
	}

	return nil
}

func (c *AttrCommand) setAttributes(ctx *Context, path string, attrs []string) error {
	var newContentType string

	// Parse attributes
	for _, attr := range attrs {
		parts := strings.SplitN(attr, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid attribute format: %s (expected key=value)", attr)
		}

		key := strings.ToLower(parts[0])
		value := parts[1]

		switch key {
		case "content-type":
			newContentType = value
		default:
			return fmt.Errorf("unsupported attribute: %s (currently only content-type is supported for setting)", key)
		}
	}

	// Update file metadata with new content-type
	err := ctx.Client.UpdateFileMetadata(path, newContentType)
	if err != nil {
		return fmt.Errorf("failed to update file attributes: %w", err)
	}

	fmt.Fprintf(ctx.Stdout, "Updated attributes for %s\n", path)
	return nil
}

func (c *AttrCommand) Help() string {
	return "attr <command> <path> [key or key=value] ... - Get or set file attributes"
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
	content, contentType, _, _, err := ctx.Client.GetFile(path)
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

// detectContentType detects content type based on file extension
func detectContentType(filename string) string {
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
		return "application/octet-stream" // Default to octet-stream for unknown extensions
	}
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
	fmt.Fprintln(ctx.Stdout, "  grep <pattern> <path>              Search for pattern in file contents")
	fmt.Fprintln(ctx.Stdout, "  find <path> [options]              Find files and directories")
	fmt.Fprintln(ctx.Stdout, "  attr <cmd> <path> [key=value]      Get or set file attributes (content-type)")
	fmt.Fprintln(ctx.Stdout, "  mv <src> <dst>                     Move/rename file(s) (supports globs)")
	fmt.Fprintln(ctx.Stdout, "  cp <src> <dst>                     Copy file(s) (supports globs)")
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
	// Use original filename with extension for better editor support
	originalFileName := filepath.Base(path)
	baseName := strings.TrimSuffix(originalFileName, filepath.Ext(originalFileName))
	extension := filepath.Ext(originalFileName)

	var pattern string
	if extension != "" {
		pattern = fmt.Sprintf("vfs-edit-%s-*.%s", baseName, extension[1:]) // Remove the leading dot
	} else {
		pattern = fmt.Sprintf("vfs-edit-%s-*", baseName)
	}

	tmpFile, err := os.CreateTemp("", pattern)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	// Try to get existing content
	fileExists := false
	content, _, _, _, err := ctx.Client.GetFile(path)
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
	fileName := filepath.Base(path)
	contentType := detectContentType(fileName)

	if fileExists {
		// Update existing file
		_, err = ctx.Client.UpdateFile(path, contentType, string(editedContent), 0) // ExpectedVersion 0 means any version
		if err != nil {
			return fmt.Errorf("failed to update file: %w", err)
		}
	} else {
		// Create new file
		dirPath := filepath.Dir(path)
		_, err = ctx.Client.CreateFile(dirPath, fileName, contentType, string(editedContent))
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
	_, _, _, _, err = ctx.Client.GetFile(eventsPath)
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
