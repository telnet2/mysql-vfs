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

// Command represents a CLI command
type Command interface {
	Execute(ctx *Context, args []string) error
	Help() string
}

// LsCommand lists directory contents
type LsCommand struct{}

func (c *LsCommand) Execute(ctx *Context, args []string) error {
	recursive := false
	targetPath := ""

	// Parse flags
	for _, arg := range args {
		if arg == "-r" || arg == "--recursive" {
			recursive = true
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

	if !session.IsValidPath(targetPath) {
		return fmt.Errorf("invalid path: %s", targetPath)
	}

	if recursive {
		return c.listRecursive(ctx, targetPath, 0)
	}

	return c.listSingle(ctx, targetPath)
}

func (c *LsCommand) listSingle(ctx *Context, path string) error {
	resp, err := ctx.Client.ListDirectory(path, 100, "")
	if err != nil {
		return err
	}

	if len(resp.Entries) == 0 {
		fmt.Fprintln(ctx.Stdout, "(empty directory)")
		return nil
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

func (c *LsCommand) listRecursive(ctx *Context, path string, depth int) error {
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
	}

	for _, entry := range resp.Entries {
		if entry.Type == "directory" {
			fmt.Fprintf(ctx.Stdout, "%s%s/\n", indent, entry.Name)
			subPath := filepath.Join(path, entry.Name)
			if err := c.listRecursive(ctx, subPath, depth+1); err != nil {
				return err
			}
		} else {
			fmt.Fprintf(ctx.Stdout, "%s%s  (%d bytes)\n", indent, entry.Name, entry.SizeBytes)
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

	resp, err := ctx.Client.CreateDirectory(parentPath, name, nil)
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
		return fmt.Errorf("usage: cat <path>")
	}

	path := ctx.Session.ResolvePath(args[0])

	if !session.IsValidPath(path) {
		return fmt.Errorf("invalid path: %s", path)
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
	return err
}

func (c *CatCommand) Help() string {
	return "cat <path> - Display file contents"
}

// ImportCommand imports a local file to VFS
type ImportCommand struct{}

func (c *ImportCommand) Execute(ctx *Context, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: import <local_path> <vfs_path>")
	}

	localPath := args[0]
	vfsPath := args[1]

	// Read local file
	fileInfo, err := os.Stat(localPath)
	if err != nil {
		return fmt.Errorf("failed to read local file: %w", err)
	}

	if fileInfo.IsDir() {
		return fmt.Errorf("cannot import directory (only files supported)")
	}

	if fileInfo.Size() > client.MaxFileSize {
		return fmt.Errorf("file size (%d bytes) exceeds maximum allowed (%d bytes)",
			fileInfo.Size(), client.MaxFileSize)
	}

	content, err := os.ReadFile(localPath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// Detect content type
	contentType := "application/octet-stream"
	ext := filepath.Ext(localPath)
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

	// Parse VFS path
	vfsFullPath := ctx.Session.ResolvePath(vfsPath)
	if !session.IsValidPath(vfsFullPath) {
		return fmt.Errorf("invalid VFS path: %s", vfsFullPath)
	}

	dirPath := filepath.Dir(vfsFullPath)
	fileName := filepath.Base(vfsFullPath)

	// Show progress for large files
	if fileInfo.Size() > 10*1024*1024 {
		fmt.Fprintf(ctx.Stdout, "Uploading %s (%d bytes)...\n", fileName, fileInfo.Size())
	}

	resp, err := ctx.Client.CreateFile(dirPath, fileName, contentType, string(content))
	if err != nil {
		return err
	}

	fmt.Fprintf(ctx.Stdout, "Imported: %s (ID: %s, Version: %d)\n",
		vfsFullPath, resp.ID, resp.Version)

	return nil
}

func (c *ImportCommand) Help() string {
	return "import <local_path> <vfs_path> - Import local file to VFS"
}

// RmCommand removes a file
type RmCommand struct{}

func (c *RmCommand) Execute(ctx *Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: rm <path>")
	}

	path := ctx.Session.ResolvePath(args[0])

	if !session.IsValidPath(path) {
		return fmt.Errorf("invalid path: %s", path)
	}

	if err := ctx.Client.DeleteFile(path); err != nil {
		return err
	}

	fmt.Fprintf(ctx.Stdout, "Deleted file: %s\n", path)
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

	sourcePath := ctx.Session.ResolvePath(args[0])
	destPath := ctx.Session.ResolvePath(args[1])

	if !session.IsValidPath(sourcePath) {
		return fmt.Errorf("invalid source path: %s", sourcePath)
	}

	if !session.IsValidPath(destPath) {
		return fmt.Errorf("invalid destination path: %s", destPath)
	}

	resp, err := ctx.Client.MoveFile(sourcePath, destPath)
	if err != nil {
		return err
	}

	fmt.Fprintf(ctx.Stdout, "Moved: %s -> %s (ID: %s)\n", sourcePath, destPath, resp.ID)
	return nil
}

func (c *MvCommand) Help() string {
	return "mv <source> <destination> - Move or rename file"
}

// JqCommand queries JSON files
type JqCommand struct{}

func (c *JqCommand) Execute(ctx *Context, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: jq <path> <expression>")
	}

	path := ctx.Session.ResolvePath(args[0])
	expression := args[1]

	if !session.IsValidPath(path) {
		return fmt.Errorf("invalid path: %s", path)
	}

	// Get file content
	content, contentType, err := ctx.Client.GetFile(path)
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
	return "jq <path> <expression> - Query JSON file with jq"
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
	fmt.Fprintln(ctx.Stdout, "  ls [-r] [path]             List directory contents")
	fmt.Fprintln(ctx.Stdout, "  cd [path]                  Change directory")
	fmt.Fprintln(ctx.Stdout, "  pwd                        Print working directory")
	fmt.Fprintln(ctx.Stdout, "  mkdir <name>               Create directory")
	fmt.Fprintln(ctx.Stdout, "  rmdir [-r] <path>          Remove directory")
	fmt.Fprintln(ctx.Stdout, "  import <local> <vfs>       Import file to VFS")
	fmt.Fprintln(ctx.Stdout, "  cat <path>                 Display file contents")
	fmt.Fprintln(ctx.Stdout, "  jq <path> <expression>     Query JSON file")
	fmt.Fprintln(ctx.Stdout, "  mv <src> <dst>             Move/rename file")
	fmt.Fprintln(ctx.Stdout, "  rm <path>                  Remove file")
	fmt.Fprintln(ctx.Stdout, "  help                       Show this help")
	fmt.Fprintln(ctx.Stdout, "  exit                       Exit CLI")
	fmt.Fprintln(ctx.Stdout, "")
	return nil
}

func (c *HelpCommand) Help() string {
	return "help - Show available commands"
}
