package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/google/shlex"
	"github.com/itchyny/gojq"
	"github.com/peterh/liner"

	"github.com/telnet2/mysql-vfs/internal/config"
)

const (
	prompt          = "vfs> "
	requestTimeout  = 15 * time.Second
	metadataService = "metadata"
	contentService  = "content"
	actorHeader     = "X-VFS-Actor"
)

type directoryDTO struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	ParentID  *string    `json:"parent_id"`
	Path      string     `json:"path"`
	Version   int64      `json:"version"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at"`
}

type fileDTO struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	DirectoryID  string          `json:"directory_id"`
	Path         string          `json:"path"`
	Version      int64           `json:"version"`
	OriginFileID *string         `json:"origin_file_id"`
	StorageMode  string          `json:"storage_mode"`
	BlobKey      *string         `json:"blob_key"`
	InlineJSON   json.RawMessage `json:"inline_json"`
	Checksum     *string         `json:"checksum"`
	Size         *int64          `json:"size"`
	MimeType     *string         `json:"mime_type"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
	DeletedAt    *time.Time      `json:"deleted_at"`
}

type listDirectoryResponse struct {
	Directories []directoryDTO `json:"directories"`
	Files       []fileDTO      `json:"files"`
}

type uploadResponse struct {
	StorageMode string          `json:"storage_mode"`
	BlobKey     *string         `json:"blob_key"`
	JSONPayload json.RawMessage `json:"json_payload"`
	Checksum    string          `json:"checksum"`
	Size        int64           `json:"size"`
	MimeType    string          `json:"mime_type"`
}

type fileVersionDTO struct {
	ID          string          `json:"id"`
	Index       int             `json:"index"`
	StorageMode string          `json:"storage_mode"`
	BlobKey     *string         `json:"blob_key"`
	JSONPayload json.RawMessage `json:"json_payload"`
	Metadata    map[string]any  `json:"metadata"`
	CreatedBy   string          `json:"created_by"`
	CreatedAt   time.Time       `json:"created_at"`
}

type policyManifestDTO struct {
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	SourcePath  string         `json:"source_path"`
	DirectoryID string         `json:"directory_id"`
	Scope       string         `json:"scope"`
	Inheritance string         `json:"inheritance"`
	AppliesTo   []string       `json:"applies_to,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type policyUserDTO struct {
	ID          string         `json:"id"`
	DisplayName string         `json:"display_name,omitempty"`
	Email       string         `json:"email,omitempty"`
	Groups      []string       `json:"groups,omitempty"`
	Attributes  map[string]any `json:"attributes,omitempty"`
}

type policyGroupDTO struct {
	ID          string         `json:"id"`
	DisplayName string         `json:"display_name,omitempty"`
	Description string         `json:"description,omitempty"`
	Members     []string       `json:"members,omitempty"`
	Attributes  map[string]any `json:"attributes,omitempty"`
}

type principalSetDTO struct {
	Users  []policyUserDTO  `json:"users,omitempty"`
	Groups []policyGroupDTO `json:"groups,omitempty"`
}

type policyResolutionDTO struct {
	DirectoryID string              `json:"directory_id"`
	Manifests   []policyManifestDTO `json:"manifests"`
	Principals  principalSetDTO     `json:"principals"`
}

type apiError struct {
	Status  int
	Code    string
	Message string
}

func (e apiError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("%s (%s)", e.Message, e.Code)
	}
	return e.Message
}

type metadataClient struct {
	baseURL    string
	httpClient *http.Client
	actor      string
}

type contentClient struct {
	baseURL    string
	httpClient *http.Client
}

type cli struct {
	metadata    *metadataClient
	content     *contentClient
	cwdDir      directoryDTO
	dirCache    map[string]directoryDTO
	actor       string
	stdout      io.Writer
	stderr      io.Writer
	line        *liner.State
	historyPath string
}

func main() {
	help := flag.Bool("help", false, "show usage")
	flag.BoolVar(help, "h", false, "show usage")
	flag.Parse()

	if *help {
		fmt.Println("vfscli - interact with the VFS services")
		fmt.Println("Environment variables: VFS_CONFIG_PATH to override config path.")
		fmt.Println("Commands are interactive; launch the CLI without arguments.")
		return
	}

	cfg := config.Load()

	metadataURL, err := serviceBaseURL(cfg, metadataService)
	if err != nil {
		fmt.Fprintf(os.Stderr, "metadata service: %v\n", err)
		os.Exit(1)
	}
	contentURL, err := serviceBaseURL(cfg, contentService)
	if err != nil {
		fmt.Fprintf(os.Stderr, "content service: %v\n", err)
		os.Exit(1)
	}

	actor := resolveActor()

	cli := &cli{
		metadata: &metadataClient{
			baseURL:    metadataURL,
			httpClient: &http.Client{Timeout: requestTimeout},
			actor:      actor,
		},
		content: &contentClient{
			baseURL:    contentURL,
			httpClient: &http.Client{Timeout: requestTimeout},
		},
		dirCache: make(map[string]directoryDTO),
		actor:    actor,
		stdout:   os.Stdout,
		stderr:   os.Stderr,
	}
	cli.line = liner.NewLiner()
	cli.line.SetCtrlCAborts(true)

	if err := cli.initialize(); err != nil {
		fmt.Fprintf(os.Stderr, "init: %v\n", err)
		os.Exit(1)
	}

	defer cli.close()

	if err := cli.run(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func (c *cli) initialize() error {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	dir, err := c.metadata.ResolveDirectory(ctx, "/")
	if err != nil {
		if apiErr, ok := err.(apiError); ok && apiErr.Status == http.StatusNotFound {
			created, cerr := c.metadata.CreateDirectory(ctx, "/", nil)
			if cerr != nil {
				if aerr, ok := cerr.(apiError); ok && aerr.Status == http.StatusConflict {
					dir, err = c.metadata.ResolveDirectory(ctx, "/")
				} else {
					return cerr
				}
			} else {
				dir = created
			}
		}
	}
	if err != nil {
		return err
	}
	c.dirCache[dir.Path] = dir
	c.cwdDir = dir
	return nil
}

func (c *cli) run() error {
	c.setupReadline()

	for {
		line, err := c.line.Prompt(prompt)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, liner.ErrPromptAborted) {
				return nil
			}
			fmt.Fprintf(c.stderr, "error: %v\n", err)
			continue
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		c.line.AppendHistory(line)
		args, err := shlex.Split(line)
		if err != nil {
			fmt.Fprintf(c.stderr, "parse: %v\n", err)
			continue
		}
		if len(args) == 0 {
			continue
		}
		cmd := strings.ToLower(args[0])
		switch cmd {
		case "exit", "quit":
			return nil
		case "help":
			c.printHelp()
		case "pwd":
			fmt.Fprintln(c.stdout, c.cwdDir.Path)
		case "cd":
			c.handleCD(args)
		case "ls":
			c.handleLS(args)
		case "import":
			c.handleImport(args)
		case "cat":
			c.handleCat(args)
		case "jq":
			c.handleJQ(args)
		case "mkdir":
			c.handleMkdir(args)
		case "rmdir":
			c.handleRmdir(args)
		case "rm":
			c.handleRm(args)
		case "xattr":
			c.handleXAttr(args)
		case "mv":
			c.handleMv(args)
		case "cp":
			c.handleCp(args)
		case "tree":
			c.handleTree(args)
		case "stat":
			c.handleStat(args)
		case "history":
			c.handleHistory(args)
		case "principals":
			c.handlePrincipals(args)
		default:
			fmt.Fprintf(c.stderr, "unknown command: %s\n", cmd)
		}
	}
}

func (c *cli) setupReadline() {
	if c.line == nil {
		return
	}
	c.line.SetTabCompletionStyle(liner.TabCircular)
	c.line.SetCompleter(func(line string) []string {
		if strings.TrimSpace(line) == "" {
			return c.commandList()
		}
		tokens := strings.Fields(line)
		trailingSpace := len(line) > 0 && unicode.IsSpace(rune(line[len(line)-1]))
		if len(tokens) == 0 {
			return c.commandList()
		}
		if len(tokens) == 1 {
			if trailingSpace {
				prefix := line
				completions := c.pathCompletionForCommand(strings.ToLower(tokens[0]), 0, "")
				return prependPrefix(prefix, completions)
			}
			return filterByPrefix(c.commandList(), strings.ToLower(tokens[0]))
		}

		args := make([]string, len(tokens)-1)
		copy(args, tokens[1:])
		var current string
		if trailingSpace {
			args = append(args, "")
			current = ""
		} else {
			current = args[len(args)-1]
		}
		argPos := nonFlagArgPosition(args, len(args)-1)
		if argPos < 0 {
			return nil
		}
		prefix := line
		if !trailingSpace {
			offset := len(line) - len(current)
			if offset < 0 {
				offset = 0
			}
			prefix = line[:offset]
		}
		completions := c.pathCompletionForCommand(strings.ToLower(tokens[0]), argPos, current)
		return prependPrefix(prefix, completions)
	})
	historyPath := historyFilePath()
	if historyPath != "" {
		c.historyPath = historyPath
		if file, err := os.Open(historyPath); err == nil {
			c.line.ReadHistory(file)
			file.Close()
		}
	}
}

func (c *cli) teardownReadline() {
	if c.line == nil {
		return
	}
	if c.historyPath != "" {
		if file, err := os.Create(c.historyPath); err == nil {
			c.line.WriteHistory(file)
			file.Close()
		}
	}
}

func (c *cli) close() {
	if c.line != nil {
		c.teardownReadline()
		c.line.Close()
	}
}

func (c *cli) commandList() []string {
	return []string{
		"help", "exit", "quit", "pwd", "cd", "ls", "import", "cat", "jq",
		"mkdir", "rmdir", "rm", "xattr", "mv", "cp", "tree", "stat", "history", "principals",
	}
}
func filterByPrefix(items []string, prefix string) []string {
	res := make([]string, 0, len(items))
	lower := strings.ToLower(prefix)
	for _, item := range items {
		if strings.HasPrefix(strings.ToLower(item), lower) {
			res = append(res, item)
		}
	}
	return res
}

func historyFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	dir := filepath.Join(home, ".vfscli")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return ""
	}
	return filepath.Join(dir, "history")
}

func prependPrefix(prefix string, completions []string) []string {
	if len(completions) == 0 {
		return completions
	}
	results := make([]string, 0, len(completions))
	for _, item := range completions {
		results = append(results, prefix+item)
	}
	return results
}

func nonFlagArgPosition(args []string, upto int) int {
	count := -1
	if upto >= len(args) {
		upto = len(args) - 1
	}
	for i := 0; i <= upto && i >= 0 && i < len(args); i++ {
		token := args[i]
		if token == "" && i == upto {
			count++
			continue
		}
		if token != "" && strings.HasPrefix(token, "-") && token != "-" {
			continue
		}
		count++
	}
	return count
}

func (c *cli) pathCompletionForCommand(cmd string, argIndex int, current string) []string {
	if strings.HasPrefix(current, "-") && current != "-" {
		return nil
	}
	switch cmd {
	case "cd":
		if argIndex == 0 {
			return c.pathCompletions(current, true)
		}
	case "ls":
		if argIndex == 0 {
			return c.pathCompletions(current, false)
		}
	case "import":
		if argIndex == 1 {
			return c.pathCompletions(current, false)
		}
	case "mkdir", "rmdir":
		if argIndex == 0 {
			return c.pathCompletions(current, true)
		}
	case "rm", "cat", "jq", "xattr", "stat", "history", "tree":
		if argIndex == 0 {
			return c.pathCompletions(current, false)
		}
	case "mv", "cp":
		if argIndex == 0 || argIndex == 1 {
			return c.pathCompletions(current, false)
		}
	case "principals":
		if argIndex == 0 {
			return c.pathCompletions(current, false)
		}
	}
	return nil
}

func (c *cli) pathCompletions(input string, dirsOnly bool) []string {
	basePrefix, partial := splitCompletionInput(input)
	parentPath := c.completionParentPath(input, basePrefix)
	parentDir, err := c.lookupDirectory(parentPath)
	if err != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	listing, err := c.metadata.ListDirectory(ctx, parentDir.ID, false)
	cancel()
	if err != nil {
		return nil
	}
	partialLower := strings.ToLower(partial)
	seen := make(map[string]struct{})
	suggestions := make([]string, 0, len(listing.Directories)+len(listing.Files))
	add := func(name string, isDir bool) {
		if partialLower != "" && !strings.HasPrefix(strings.ToLower(name), partialLower) {
			return
		}
		if dirsOnly && !isDir {
			return
		}
		candidate := basePrefix + name
		if isDir && !strings.HasSuffix(candidate, "/") {
			candidate += "/"
		}
		if _, ok := seen[candidate]; ok {
			return
		}
		seen[candidate] = struct{}{}
		suggestions = append(suggestions, candidate)
	}
	for _, d := range listing.Directories {
		add(path.Base(d.Path), true)
	}
	if !dirsOnly {
		for _, f := range listing.Files {
			add(path.Base(f.Path), false)
		}
	}
	sort.Strings(suggestions)
	return suggestions
}

func splitCompletionInput(input string) (string, string) {
	if input == "" {
		return "", ""
	}
	if strings.HasSuffix(input, "/") {
		return input, ""
	}
	if idx := strings.LastIndex(input, "/"); idx >= 0 {
		return input[:idx+1], input[idx+1:]
	}
	return "", input
}

func (c *cli) completionParentPath(input, basePrefix string) string {
	switch {
	case input == "":
		return c.cwdDir.Path
	case strings.HasSuffix(input, "/"):
		if strings.HasPrefix(input, "/") {
			return normalizePath(input)
		}
		return normalizePath(path.Join(c.cwdDir.Path, input))
	case basePrefix == "":
		return c.cwdDir.Path
	case strings.HasPrefix(basePrefix, "/"):
		return normalizePath(basePrefix)
	default:
		return normalizePath(path.Join(c.cwdDir.Path, basePrefix))
	}
}

func (c *cli) handleCD(args []string) {
	target := "/"
	if len(args) > 1 {
		target = c.resolvePath(args[1])
	}
	dir, err := c.lookupDirectory(target)
	if err != nil {
		fmt.Fprintf(c.stderr, "cd: %v\n", err)
		return
	}
	c.cwdDir = dir
}

func (c *cli) handleLS(args []string) {
	recursive := false
	var targetPath string
	for _, arg := range args[1:] {
		switch arg {
		case "-r", "--recursive":
			recursive = true
		default:
			if targetPath != "" {
				fmt.Fprintln(c.stderr, "ls: too many arguments")
				return
			}
			targetPath = arg
		}
	}
	if targetPath == "" {
		targetPath = c.cwdDir.Path
	} else {
		targetPath = c.resolvePath(targetPath)
	}

	dir, err := c.lookupDirectory(targetPath)
	if err != nil {
		fmt.Fprintf(c.stderr, "ls: %v\n", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	listing, err := c.metadata.ListDirectory(ctx, dir.ID, recursive)
	if err != nil {
		fmt.Fprintf(c.stderr, "ls: %v\n", err)
		return
	}

	dirs := listing.Directories
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].Path < dirs[j].Path })
	files := listing.Files
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })

	for _, d := range dirs {
		if d.Path == dir.Path {
			continue
		}
		fmt.Fprintf(c.stdout, "dir  %s\n", d.Path)
	}
	for _, f := range files {
		extra := make([]string, 0, 2)
		if f.StorageMode != "" {
			extra = append(extra, f.StorageMode)
		}
		if f.Size != nil {
			extra = append(extra, fmt.Sprintf("%dB", *f.Size))
		}
		if len(extra) > 0 {
			fmt.Fprintf(c.stdout, "file %s (%s)\n", f.Path, strings.Join(extra, ", "))
		} else {
			fmt.Fprintf(c.stdout, "file %s\n", f.Path)
		}
	}
}
func (c *cli) handleCat(args []string) {
	if len(args) != 2 {
		fmt.Fprintln(c.stderr, "usage: cat <path>")
		return
	}
	target := c.resolvePath(args[1])
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	file, err := c.metadata.ResolveFile(ctx, target)
	cancel()
	if err != nil {
		fmt.Fprintf(c.stderr, "cat: %v\n", err)
		return
	}

	switch file.StorageMode {
	case "inline_json":
		if len(file.InlineJSON) == 0 {
			fmt.Fprintln(c.stderr, "cat: no inline content")
			return
		}
		var buf bytes.Buffer
		if err := json.Indent(&buf, file.InlineJSON, "", "  "); err != nil {
			buf.Reset()
			buf.Write(file.InlineJSON)
		}
		buf.WriteByte('\n')
		if _, err := c.stdout.Write(buf.Bytes()); err != nil {
			fmt.Fprintf(c.stderr, "cat: %v\n", err)
		}
	case "blob":
		if file.BlobKey == nil {
			fmt.Fprintln(c.stderr, "cat: missing blob key")
			return
		}
		ctxDownload, cancelDownload := context.WithTimeout(context.Background(), requestTimeout)
		data, err := c.content.Download(ctxDownload, *file.BlobKey)
		cancelDownload()
		if err != nil {
			fmt.Fprintf(c.stderr, "cat: %v\n", err)
			return
		}
		if _, err := c.stdout.Write(data); err != nil {
			fmt.Fprintf(c.stderr, "cat: %v\n", err)
		}
	default:
		fmt.Fprintf(c.stderr, "cat: unsupported storage mode %q\n", file.StorageMode)
	}
}

func (c *cli) handleImport(args []string) {
	if len(args) != 3 {
		fmt.Fprintln(c.stderr, "usage: import <local_path> <remote_path>")
		return
	}
	localPath := args[1]
	remotePath := c.resolvePath(args[2])
	info, err := os.Stat(localPath)
	if err != nil {
		fmt.Fprintf(c.stderr, "import: %v\n", err)
		return
	}
	if info.IsDir() {
		fmt.Fprintln(c.stderr, "import: local path is a directory")
		return
	}

	data, err := os.ReadFile(localPath)
	if err != nil {
		fmt.Fprintf(c.stderr, "import: read %v\n", err)
		return
	}

	mimeType := mime.TypeByExtension(strings.ToLower(filepath.Ext(localPath)))
	ctxUpload, cancelUpload := context.WithTimeout(context.Background(), requestTimeout)
	uploadResp, err := c.content.Upload(ctxUpload, filepath.Base(remotePath), mimeType, data)
	cancelUpload()
	if err != nil {
		fmt.Fprintf(c.stderr, "import: %v\n", err)
		return
	}

	parentDirPath := parentDirectoryPath(remotePath)
	dir, err := c.ensureDirectory(parentDirPath)
	if err != nil {
		fmt.Fprintf(c.stderr, "import: %v\n", err)
		return
	}
	if isPolicyFilename(remotePath) {
		if err := c.ensurePolicyAdmin(dir.Path); err != nil {
			fmt.Fprintf(c.stderr, "import: %v\n", err)
			return
		}
	}

	createReq := createFileRequest{
		DirectoryID: dir.ID,
		Name:        filepath.Base(remotePath),
		StorageMode: uploadResp.StorageMode,
		Actor:       c.actor,
	}
	if uploadResp.BlobKey != nil {
		createReq.BlobKey = uploadResp.BlobKey
	}
	if len(uploadResp.JSONPayload) > 0 {
		payload := json.RawMessage(uploadResp.JSONPayload)
		createReq.JSONPayload = &payload
	}
	if uploadResp.Checksum != "" {
		checksum := uploadResp.Checksum
		createReq.Checksum = &checksum
	}
	if uploadResp.Size > 0 {
		size := uploadResp.Size
		createReq.Size = &size
	}
	if uploadResp.MimeType != "" {
		mt := uploadResp.MimeType
		createReq.MimeType = &mt
	}

	ctxCreate, cancelCreate := context.WithTimeout(context.Background(), requestTimeout)
	_, err = c.metadata.CreateFile(ctxCreate, createReq)
	cancelCreate()
	if err != nil {
		fmt.Fprintf(c.stderr, "import: %v\n", err)
		return
	}

	fmt.Fprintf(c.stdout, "imported %s (%s)\n", remotePath, uploadResp.StorageMode)
}

func (c *cli) handleTree(args []string) {
	targetPath := c.cwdDir.Path
	if len(args) > 2 {
		fmt.Fprintln(c.stderr, "usage: tree [path]")
		return
	}
	if len(args) == 2 {
		targetPath = c.resolvePath(args[1])
	}

	dir, err := c.lookupDirectory(targetPath)
	if err != nil {
		fmt.Fprintf(c.stderr, "tree: %v\n", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	listing, err := c.metadata.ListDirectory(ctx, dir.ID, true)
	cancel()
	if err != nil {
		fmt.Fprintf(c.stderr, "tree: %v\n", err)
		return
	}

	rootNode := buildTree(dir, listing)
	printTree(c.stdout, rootNode)
}

type treeNode struct {
	name     string
	fullPath string
	isDir    bool
	children []*treeNode
}

func buildTree(root directoryDTO, listing listDirectoryResponse) *treeNode {
	nodes := make(map[string]*treeNode)
	getNode := func(fullPath string, isDir bool) *treeNode {
		if n, ok := nodes[fullPath]; ok {
			if isDir {
				n.isDir = true
			}
			return n
		}
		name := path.Base(fullPath)
		if fullPath == "/" {
			name = "/"
		}
		n := &treeNode{name: name, fullPath: fullPath, isDir: isDir}
		nodes[fullPath] = n
		return n
	}

	rootNode := &treeNode{name: root.Path, fullPath: root.Path, isDir: true}
	nodes[root.Path] = rootNode

	for _, d := range listing.Directories {
		node := getNode(d.Path, true)
		parentPath := parentDirectoryPath(d.Path)
		parent := getNode(parentPath, true)
		parent.children = append(parent.children, node)
	}

	for _, f := range listing.Files {
		node := getNode(f.Path, false)
		parentPath := parentDirectoryPath(f.Path)
		parent := getNode(parentPath, true)
		parent.children = append(parent.children, node)
	}

	for _, node := range nodes {
		if len(node.children) == 0 {
			continue
		}
		sort.Slice(node.children, func(i, j int) bool {
			if node.children[i].isDir != node.children[j].isDir {
				return node.children[i].isDir
			}
			return node.children[i].name < node.children[j].name
		})
	}

	return rootNode
}

func printTree(w io.Writer, root *treeNode) {
	fmt.Fprintln(w, ".")
	printTreeChildren(w, root, "")
}

func printTreeChildren(w io.Writer, node *treeNode, prefix string) {
	for i, child := range node.children {
		isLast := i == len(node.children)-1
		connector := "├──"
		childPrefix := prefix + "│   "
		if isLast {
			connector = "└──"
			childPrefix = prefix + "    "
		}
		name := child.name
		if child.fullPath == "/" {
			name = "."
		} else if child.isDir {
			name = child.name + "/"
		}
		fmt.Fprintf(w, "%s%s %s\n", prefix, connector, name)
		printTreeChildren(w, child, childPrefix)
	}
}

func (c *cli) handleStat(args []string) {
	targetPath := c.cwdDir.Path
	if len(args) > 2 {
		fmt.Fprintln(c.stderr, "usage: stat [path]")
		return
	}
	if len(args) == 2 {
		targetPath = c.resolvePath(args[1])
	}

	ctxDir, cancelDir := context.WithTimeout(context.Background(), requestTimeout)
	dir, dirErr := c.metadata.ResolveDirectory(ctxDir, targetPath)
	cancelDir()
	if dirErr == nil {
		attrs := map[string]any{
			"type":       "directory",
			"id":         dir.ID,
			"name":       dir.Name,
			"path":       dir.Path,
			"parent_id":  dir.ParentID,
			"version":    dir.Version,
			"created_at": dir.CreatedAt,
			"updated_at": dir.UpdatedAt,
		}
		if dir.DeletedAt != nil {
			attrs["deleted_at"] = dir.DeletedAt
		}
		if err := writeJSON(c.stdout, attrs); err != nil {
			fmt.Fprintf(c.stderr, "stat: %v\n", err)
		}
		return
	}
	if apiErr, ok := dirErr.(apiError); ok && apiErr.Status != http.StatusNotFound {
		fmt.Fprintf(c.stderr, "stat: %v\n", dirErr)
		return
	}

	file, err := c.resolveFile(targetPath)
	if err != nil {
		fmt.Fprintf(c.stderr, "stat: %v\n", err)
		return
	}
	attrs := map[string]any{
		"type":           "file",
		"id":             file.ID,
		"name":           file.Name,
		"path":           file.Path,
		"directory_id":   file.DirectoryID,
		"origin_file_id": file.OriginFileID,
		"version":        file.Version,
		"storage_mode":   file.StorageMode,
		"checksum":       file.Checksum,
		"size":           file.Size,
		"mime_type":      file.MimeType,
		"created_at":     file.CreatedAt,
		"updated_at":     file.UpdatedAt,
	}
	if file.BlobKey != nil {
		attrs["blob_key"] = *file.BlobKey
	}
	if len(file.InlineJSON) > 0 {
		var decoded any
		if err := json.Unmarshal(file.InlineJSON, &decoded); err == nil {
			attrs["inline_json"] = decoded
		} else {
			attrs["inline_json_raw"] = string(file.InlineJSON)
		}
	}
	if file.DeletedAt != nil {
		attrs["deleted_at"] = file.DeletedAt
	}
	if err := writeJSON(c.stdout, attrs); err != nil {
		fmt.Fprintf(c.stderr, "stat: %v\n", err)
	}
}

func (c *cli) handleHistory(args []string) {
	if len(args) != 2 {
		fmt.Fprintln(c.stderr, "usage: history <file>")
		return
	}
	path := c.resolvePath(args[1])
	file, err := c.resolveFile(path)
	if err != nil {
		fmt.Fprintf(c.stderr, "history: %v\n", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	versions, err := c.metadata.ListFileVersions(ctx, file.ID)
	cancel()
	if err != nil {
		fmt.Fprintf(c.stderr, "history: %v\n", err)
		return
	}
	if len(versions) == 0 {
		fmt.Fprintln(c.stdout, "no history")
		return
	}
	entries := make([]map[string]any, 0, len(versions))
	for _, v := range versions {
		entry := map[string]any{
			"index":        v.Index,
			"storage_mode": v.StorageMode,
			"created_at":   v.CreatedAt,
			"created_by":   v.CreatedBy,
		}
		if v.BlobKey != nil {
			entry["blob_key"] = *v.BlobKey
		}
		if len(v.JSONPayload) > 0 {
			var decoded any
			if err := json.Unmarshal(v.JSONPayload, &decoded); err == nil {
				entry["inline_json"] = decoded
			} else {
				entry["inline_json_raw"] = string(v.JSONPayload)
			}
		}
		if v.Metadata != nil {
			entry["metadata"] = v.Metadata
		}
		entries = append(entries, entry)
	}
	if err := writeJSON(c.stdout, entries); err != nil {
		fmt.Fprintf(c.stderr, "history: %v\n", err)
	}
}

func (c *cli) handlePrincipals(args []string) {
	fs := flag.NewFlagSet("principals", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var typeFilter string
	var directoryID string
	fs.StringVar(&typeFilter, "type", "", "filter by manifest type (user|group)")
	fs.StringVar(&typeFilter, "t", "", "filter by manifest type (user|group)")
	fs.StringVar(&directoryID, "directory", "", "resolve using directory id")
	fs.StringVar(&directoryID, "d", "", "resolve using directory id")
	if err := fs.Parse(args[1:]); err != nil {
		fmt.Fprintln(c.stderr, "usage: principals [--type user|group] [--directory <id>] [path]")
		return
	}

	typeFilter = strings.ToLower(strings.TrimSpace(typeFilter))
	if typeFilter != "" && typeFilter != "user" && typeFilter != "group" {
		fmt.Fprintln(c.stderr, "principals: type must be 'user' or 'group'")
		return
	}

	remaining := fs.Args()
	var targetPath string
	if strings.TrimSpace(directoryID) != "" {
		if len(remaining) > 0 {
			fmt.Fprintln(c.stderr, "usage: principals [--type user|group] [--directory <id>] [path]")
			return
		}
	} else {
		if len(remaining) > 1 {
			fmt.Fprintln(c.stderr, "usage: principals [--type user|group] [--directory <id>] [path]")
			return
		}
		if len(remaining) == 1 {
			targetPath = c.resolvePath(remaining[0])
		} else {
			targetPath = c.cwdDir.Path
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	result, err := c.metadata.ResolvePolicy(ctx, directoryID, targetPath, typeFilter)
	if err != nil {
		if apiErr, ok := err.(apiError); ok && apiErr.Status == http.StatusNotFound {
			fmt.Fprintln(c.stderr, "principals: target not found")
		} else {
			fmt.Fprintf(c.stderr, "principals: %v\n", err)
		}
		return
	}

	output := map[string]any{
		"directory_id": result.DirectoryID,
		"manifests":    result.Manifests,
		"users":        result.Principals.Users,
		"groups":       result.Principals.Groups,
	}
	if err := writeJSON(c.stdout, output); err != nil {
		fmt.Fprintf(c.stderr, "principals: %v\n", err)
	}
}

func (c *cli) handleJQ(args []string) {
	if len(args) < 3 {
		fmt.Fprintln(c.stderr, "usage: jq <path> <expression>")
		return
	}
	target := c.resolvePath(args[1])
	expression := strings.Join(args[2:], " ")

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	file, err := c.metadata.ResolveFile(ctx, target)
	cancel()
	if err != nil {
		fmt.Fprintf(c.stderr, "jq: %v\n", err)
		return
	}
	if file.StorageMode != "inline_json" {
		fmt.Fprintln(c.stderr, "jq: file is not stored as inline JSON")
		return
	}
	if len(file.InlineJSON) == 0 {
		fmt.Fprintln(c.stderr, "jq: empty JSON payload")
		return
	}

	var payload any
	if err := json.Unmarshal(file.InlineJSON, &payload); err != nil {
		fmt.Fprintf(c.stderr, "jq: invalid JSON: %v\n", err)
		return
	}
	queryAST, err := gojq.Parse(expression)
	if err != nil {
		fmt.Fprintf(c.stderr, "jq: parse: %v\n", err)
		return
	}
	compiled, err := gojq.Compile(queryAST)
	if err != nil {
		fmt.Fprintf(c.stderr, "jq: compile: %v\n", err)
		return
	}
	iter := compiled.Run(payload)
	printed := false
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if err, ok := v.(error); ok {
			fmt.Fprintf(c.stderr, "jq: %v\n", err)
			return
		}
		text, err := formatResult(v)
		if err != nil {
			fmt.Fprintf(c.stderr, "jq: %v\n", err)
			return
		}
		fmt.Fprintln(c.stdout, text)
		printed = true
	}
	if !printed {
		fmt.Fprintln(c.stdout, "null")
	}
}

func (c *cli) handleMkdir(args []string) {
	createParents := false
	var targetArg string
	for _, arg := range args[1:] {
		switch arg {
		case "-p", "--parents":
			createParents = true
		default:
			if targetArg != "" {
				fmt.Fprintln(c.stderr, "usage: mkdir [-p] <path>")
				return
			}
			targetArg = arg
		}
	}
	if targetArg == "" {
		fmt.Fprintln(c.stderr, "usage: mkdir [-p] <path>")
		return
	}
	targetPath := c.resolvePath(targetArg)
	if targetPath == "/" {
		fmt.Fprintln(c.stderr, "mkdir: cannot create root directory")
		return
	}
	if createParents {
		dir, err := c.ensureDirectory(targetPath)
		if err != nil {
			fmt.Fprintf(c.stderr, "mkdir: %v\n", err)
			return
		}
		fmt.Fprintf(c.stdout, "directory ready: %s\n", dir.Path)
		return
	}
	parentPath := parentDirectoryPath(targetPath)
	parent, err := c.lookupDirectory(parentPath)
	if err != nil {
		fmt.Fprintf(c.stderr, "mkdir: %v\n", err)
		return
	}
	name := strings.Trim(path.Base(targetPath), "/")
	if name == "" || name == "." || name == ".." {
		fmt.Fprintln(c.stderr, "mkdir: invalid directory name")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	dir, err := c.metadata.CreateDirectory(ctx, name, &parent.ID)
	if err != nil {
		if apiErr, ok := err.(apiError); ok && apiErr.Status == http.StatusConflict {
			fmt.Fprintln(c.stderr, "mkdir: directory already exists")
		} else {
			fmt.Fprintf(c.stderr, "mkdir: %v\n", err)
		}
		return
	}
	c.dirCache[dir.Path] = dir
	fmt.Fprintf(c.stdout, "created %s\n", dir.Path)
}

func (c *cli) handleRmdir(args []string) {
	force := false
	var targetArg string
	for _, arg := range args[1:] {
		switch arg {
		case "-f", "--force":
			force = true
		default:
			if targetArg != "" {
				fmt.Fprintln(c.stderr, "usage: rmdir [-f] <path>")
				return
			}
			targetArg = arg
		}
	}
	if targetArg == "" {
		fmt.Fprintln(c.stderr, "usage: rmdir [-f] <path>")
		return
	}
	targetPath := c.resolvePath(targetArg)
	if targetPath == "/" {
		fmt.Fprintln(c.stderr, "rmdir: cannot remove root directory")
		return
	}
	dir, err := c.lookupDirectory(targetPath)
	if err != nil {
		fmt.Fprintf(c.stderr, "rmdir: %v\n", err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	if err := c.metadata.DeleteDirectory(ctx, dir.ID, force); err != nil {
		if apiErr, ok := err.(apiError); ok {
			switch apiErr.Status {
			case http.StatusNotFound:
				fmt.Fprintln(c.stderr, "rmdir: directory not found")
			case http.StatusConflict:
				fmt.Fprintln(c.stderr, "rmdir: directory not empty (use -f)")
			default:
				fmt.Fprintf(c.stderr, "rmdir: %s\n", apiErr.Error())
			}
		} else {
			fmt.Fprintf(c.stderr, "rmdir: %v\n", err)
		}
		return
	}
	c.invalidateDirCache(dir.Path)
	if strings.HasPrefix(c.cwdDir.Path, dir.Path) {
		root, err := c.lookupDirectory("/")
		if err == nil {
			c.cwdDir = root
		}
	}
	fmt.Fprintf(c.stdout, "removed %s\n", dir.Path)
}

func (c *cli) handleRm(args []string) {
	if len(args) != 2 {
		fmt.Fprintln(c.stderr, "usage: rm <path>")
		return
	}
	targetPath := c.resolvePath(args[1])
	ctxResolve, cancelResolve := context.WithTimeout(context.Background(), requestTimeout)
	file, err := c.metadata.ResolveFile(ctxResolve, targetPath)
	cancelResolve()
	if err != nil {
		if apiErr, ok := err.(apiError); ok && apiErr.Status == http.StatusNotFound {
			fmt.Fprintln(c.stderr, "rm: file not found")
		} else {
			fmt.Fprintf(c.stderr, "rm: %v\n", err)
		}
		return
	}
	if isPolicyFilename(file.Name) {
		if err := c.ensurePolicyAdmin(parentDirectoryPath(file.Path)); err != nil {
			fmt.Fprintf(c.stderr, "rm: %v\n", err)
			return
		}
	}
	ctxDelete, cancelDelete := context.WithTimeout(context.Background(), requestTimeout)
	err = c.metadata.DeleteFile(ctxDelete, file.ID)
	cancelDelete()
	if err != nil {
		fmt.Fprintf(c.stderr, "rm: %v\n", err)
		return
	}
	fmt.Fprintf(c.stdout, "removed %s\n", targetPath)
}

func (c *cli) handleXAttr(args []string) {
	targetPath := c.cwdDir.Path
	if len(args) > 2 {
		fmt.Fprintln(c.stderr, "usage: xattr [path]")
		return
	}
	if len(args) == 2 {
		targetPath = c.resolvePath(args[1])
	}
	ctxDir, cancelDir := context.WithTimeout(context.Background(), requestTimeout)
	dir, dirErr := c.metadata.ResolveDirectory(ctxDir, targetPath)
	cancelDir()
	if dirErr == nil {
		attrs := map[string]any{
			"type":       "directory",
			"id":         dir.ID,
			"name":       dir.Name,
			"parent_id":  dir.ParentID,
			"path":       dir.Path,
			"version":    dir.Version,
			"created_at": dir.CreatedAt,
			"updated_at": dir.UpdatedAt,
		}
		if dir.DeletedAt != nil {
			attrs["deleted_at"] = dir.DeletedAt
		}
		if err := writeJSON(c.stdout, attrs); err != nil {
			fmt.Fprintf(c.stderr, "xattr: %v\n", err)
		}
		return
	}
	if apiErr, ok := dirErr.(apiError); ok && apiErr.Status != http.StatusNotFound {
		fmt.Fprintf(c.stderr, "xattr: %v\n", dirErr)
		return
	}
	ctxFile, cancelFile := context.WithTimeout(context.Background(), requestTimeout)
	file, fileErr := c.metadata.ResolveFile(ctxFile, targetPath)
	cancelFile()
	if fileErr != nil {
		fmt.Fprintf(c.stderr, "xattr: %v\n", fileErr)
		return
	}
	attrs := map[string]any{
		"type":           "file",
		"id":             file.ID,
		"name":           file.Name,
		"directory_id":   file.DirectoryID,
		"path":           file.Path,
		"version":        file.Version,
		"storage_mode":   file.StorageMode,
		"origin_file_id": file.OriginFileID,
		"created_at":     file.CreatedAt,
		"updated_at":     file.UpdatedAt,
	}
	if file.BlobKey != nil {
		attrs["blob_key"] = *file.BlobKey
	}
	if len(file.InlineJSON) > 0 {
		var decoded any
		if err := json.Unmarshal(file.InlineJSON, &decoded); err == nil {
			attrs["inline_json"] = decoded
		} else {
			attrs["inline_json_raw"] = string(file.InlineJSON)
		}
	}
	if file.Checksum != nil {
		attrs["checksum"] = *file.Checksum
	}
	if file.Size != nil {
		attrs["size"] = *file.Size
	}
	if file.MimeType != nil {
		attrs["mime_type"] = *file.MimeType
	}
	if file.DeletedAt != nil {
		attrs["deleted_at"] = file.DeletedAt
	}
	if err := writeJSON(c.stdout, attrs); err != nil {
		fmt.Fprintf(c.stderr, "xattr: %v\n", err)
	}
}

func (c *cli) resolveFile(p string) (fileDTO, error) {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	return c.metadata.ResolveFile(ctx, p)
}

func (c *cli) handleMv(args []string) {
	if len(args) != 3 {
		fmt.Fprintln(c.stderr, "usage: mv <source> <destination>")
		return
	}
	sourcePath := c.resolvePath(args[1])
	destPath := c.resolvePath(args[2])
	if sourcePath == destPath {
		fmt.Fprintln(c.stderr, "mv: source and destination are the same")
		return
	}

	if dir, err := c.lookupDirectory(sourcePath); err == nil {
		if err := c.moveDirectory(dir, destPath); err != nil {
			fmt.Fprintf(c.stderr, "mv: %v\n", err)
		}
		return
	} else {
		if apiErr, ok := err.(apiError); ok && apiErr.Status != http.StatusNotFound {
			fmt.Fprintf(c.stderr, "mv: %v\n", err)
			return
		}
	}

	file, err := c.resolveFile(sourcePath)
	if err != nil {
		fmt.Fprintf(c.stderr, "mv: %v\n", err)
		return
	}
	if err := c.moveFile(file, destPath); err != nil {
		fmt.Fprintf(c.stderr, "mv: %v\n", err)
	}
}

func (c *cli) moveDirectory(dir directoryDTO, dest string) error {
	finalPath, parentID, err := c.prepareMoveDestination(dir.Path, dest, true)
	if err != nil {
		return err
	}
	if strings.HasPrefix(finalPath+"/", dir.Path+"/") {
		return fmt.Errorf("cannot move directory into itself")
	}
	if finalPath == dir.Path {
		return fmt.Errorf("destination is identical to source")
	}

	name := strings.Trim(path.Base(finalPath), "/")
	if name == "" {
		return fmt.Errorf("invalid destination path")
	}

	version := dir.Version
	req := updateDirectoryRequest{
		Name:    &name,
		Version: &version,
	}
	if parentID != nil {
		req.ParentID = parentID
	} else {
		nullParent := ""
		req.ParentID = &nullParent
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	updated, err := c.metadata.UpdateDirectory(ctx, dir.ID, req)
	cancel()
	if err != nil {
		return err
	}

	oldPath := dir.Path
	c.invalidateDirCache(oldPath)
	c.dirCache[updated.Path] = updated
	if strings.HasPrefix(c.cwdDir.Path, oldPath) {
		newPath := strings.TrimPrefix(c.cwdDir.Path, oldPath)
		if !strings.HasPrefix(newPath, "/") {
			newPath = "/" + newPath
		}
		recomputed := normalizePath(updated.Path + newPath)
		if looked, err := c.lookupDirectory(recomputed); err == nil {
			c.cwdDir = looked
		}
	}
	fmt.Fprintf(c.stdout, "moved directory to %s\n", updated.Path)
	return nil
}

func (c *cli) moveFile(file fileDTO, dest string) error {
	finalPath, parentID, err := c.prepareMoveDestination(file.Path, dest, false)
	if err != nil {
		return err
	}
	if finalPath == file.Path {
		return fmt.Errorf("destination is identical to source")
	}
	parent := parentID
	name := path.Base(finalPath)

	version := file.Version
	req := updateFileRequest{
		Actor:   c.actor,
		Version: &version,
	}
	if name != file.Name {
		req.Name = &name
	}
	if parent != nil {
		req.DirectoryID = parent
	} else {
		rootID := ""
		req.DirectoryID = &rootID
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	updated, err := c.metadata.UpdateFile(ctx, file.ID, req)
	cancel()
	if err != nil {
		return err
	}
	fmt.Fprintf(c.stdout, "moved file to %s\n", updated.Path)
	return nil
}

func (c *cli) prepareMoveDestination(sourcePath, dest string, isDir bool) (string, *string, error) {
	destination := normalizePath(dest)
	if destDir, err := c.lookupDirectory(destination); err == nil {
		base := path.Base(sourcePath)
		destination = normalizePath(path.Join(destDir.Path, base))
	} else {
		if apiErr, ok := err.(apiError); ok && apiErr.Status != http.StatusNotFound {
			return "", nil, err
		}
		if strings.HasSuffix(dest, "/") {
			return "", nil, fmt.Errorf("target directory does not exist")
		}
	}

	parentPath := parentDirectoryPath(destination)
	var parentID *string
	if parentPath == "/" {
		parentID = nil
	} else {
		parentDir, err := c.lookupDirectory(parentPath)
		if err != nil {
			return "", nil, fmt.Errorf("destination parent: %w", err)
		}
		parentID = &parentDir.ID
	}
	return destination, parentID, nil
}

func (c *cli) handleCp(args []string) {
	if len(args) != 3 {
		fmt.Fprintln(c.stderr, "usage: cp <source> <destination>")
		return
	}
	sourcePath := c.resolvePath(args[1])
	destPath := c.resolvePath(args[2])
	file, err := c.resolveFile(sourcePath)
	if err != nil {
		fmt.Fprintf(c.stderr, "cp: %v\n", err)
		return
	}

	finalPath, parentID, err := c.prepareMoveDestination(file.Path, destPath, false)
	if err != nil {
		fmt.Fprintf(c.stderr, "cp: %v\n", err)
		return
	}

	if _, err := c.resolveFile(finalPath); err == nil {
		fmt.Fprintln(c.stderr, "cp: destination file exists")
		return
	} else if apiErr, ok := err.(apiError); ok && apiErr.Status != http.StatusNotFound {
		fmt.Fprintf(c.stderr, "cp: %v\n", err)
		return
	}

	var directoryID string
	if parentID == nil {
		root, err := c.lookupDirectory("/")
		if err != nil {
			fmt.Fprintf(c.stderr, "cp: %v\n", err)
			return
		}
		directoryID = root.ID
	} else {
		directoryID = *parentID
	}

	name := path.Base(finalPath)
	createReq := createFileRequest{
		DirectoryID: directoryID,
		Name:        name,
		StorageMode: file.StorageMode,
		Actor:       c.actor,
	}

	ctxVersions, cancelVersions := context.WithTimeout(context.Background(), requestTimeout)
	versions, err := c.metadata.ListFileVersions(ctxVersions, file.ID)
	cancelVersions()
	if err == nil && len(versions) > 0 {
		meta := versions[len(versions)-1].Metadata
		if len(meta) > 0 {
			copyMeta := make(map[string]any, len(meta))
			for k, v := range meta {
				copyMeta[k] = v
			}
			createReq.Metadata = copyMeta
		}
	}

	switch file.StorageMode {
	case "inline_json":
		if len(file.InlineJSON) == 0 {
			fmt.Fprintln(c.stderr, "cp: inline file has no content")
			return
		}
		payload := json.RawMessage(append([]byte(nil), file.InlineJSON...))
		createReq.JSONPayload = &payload
	case "blob":
		if file.BlobKey == nil {
			fmt.Fprintln(c.stderr, "cp: blob key missing")
			return
		}
		ctxDownload, cancelDownload := context.WithTimeout(context.Background(), requestTimeout)
		data, err := c.content.Download(ctxDownload, *file.BlobKey)
		cancelDownload()
		if err != nil {
			fmt.Fprintf(c.stderr, "cp: download: %v\n", err)
			return
		}
		mimeType := ""
		if file.MimeType != nil {
			mimeType = *file.MimeType
		}
		ctxUpload, cancelUpload := context.WithTimeout(context.Background(), requestTimeout)
		uploadResp, err := c.content.Upload(ctxUpload, name, mimeType, data)
		cancelUpload()
		if err != nil {
			fmt.Fprintf(c.stderr, "cp: upload: %v\n", err)
			return
		}
		createReq.StorageMode = uploadResp.StorageMode
		if uploadResp.BlobKey != nil {
			createReq.BlobKey = uploadResp.BlobKey
		}
		if len(uploadResp.JSONPayload) > 0 {
			payload := json.RawMessage(uploadResp.JSONPayload)
			createReq.JSONPayload = &payload
		}
		if uploadResp.Checksum != "" {
			checksum := uploadResp.Checksum
			createReq.Checksum = &checksum
		}
		if uploadResp.Size > 0 {
			size := uploadResp.Size
			createReq.Size = &size
		}
		if uploadResp.MimeType != "" {
			mt := uploadResp.MimeType
			createReq.MimeType = &mt
		}
	default:
		fmt.Fprintf(c.stderr, "cp: unsupported storage mode %s\n", file.StorageMode)
		return
	}

	if file.Checksum != nil && createReq.Checksum == nil {
		checksum := *file.Checksum
		createReq.Checksum = &checksum
	}
	if file.Size != nil && createReq.Size == nil {
		size := *file.Size
		createReq.Size = &size
	}
	if file.MimeType != nil && createReq.MimeType == nil {
		mt := *file.MimeType
		createReq.MimeType = &mt
	}

	ctxCreate, cancelCreate := context.WithTimeout(context.Background(), requestTimeout)
	created, err := c.metadata.CreateFile(ctxCreate, createReq)
	cancelCreate()
	if err != nil {
		fmt.Fprintf(c.stderr, "cp: %v\n", err)
		return
	}
	fmt.Fprintf(c.stdout, "copied to %s\n", created.Path)
}

func (c *cli) invalidateDirCache(prefix string) {
	if prefix == "" {
		prefix = "/"
	}
	for p := range c.dirCache {
		if p == prefix || strings.HasPrefix(p, prefix+"/") {
			delete(c.dirCache, p)
		}
	}
}

func (c *cli) printHelp() {
	fmt.Fprintln(c.stdout, "Commands:")
	fmt.Fprintln(c.stdout, "  help                 Show this help message")
	fmt.Fprintln(c.stdout, "  exit|quit            Exit the CLI")
	fmt.Fprintln(c.stdout, "  pwd                  Print current directory")
	fmt.Fprintln(c.stdout, "  cd [path]            Change directory (default /)")
	fmt.Fprintln(c.stdout, "  ls [-r] [path]       List directory contents")
	fmt.Fprintln(c.stdout, "  mkdir [-p] <path>    Create a directory (use -p for parents)")
	fmt.Fprintln(c.stdout, "  rmdir [-f] <path>    Remove a directory (use -f to force)")
	fmt.Fprintln(c.stdout, "  rm <path>            Delete a file")
	fmt.Fprintln(c.stdout, "  import <src> <dst>   Upload local file to VFS")
	fmt.Fprintln(c.stdout, "  cat <path>           Display file contents")
	fmt.Fprintln(c.stdout, "  jq <path> <expr>     Run jq expression on inline JSON file")
	fmt.Fprintln(c.stdout, "  xattr [path]         Show metadata for a file or directory")
	fmt.Fprintln(c.stdout, "  mv <src> <dst>       Move or rename files/directories")
	fmt.Fprintln(c.stdout, "  cp <src> <dst>       Copy files")
	fmt.Fprintln(c.stdout, "  tree [path]          Display directory tree")
	fmt.Fprintln(c.stdout, "  stat [path]          Show detailed attributes")
	fmt.Fprintln(c.stdout, "  history <path>       List file version history")
	fmt.Fprintln(c.stdout, "  principals [options] [path]  Show resolved users and groups")
}

func (c *cli) resolvePath(input string) string {
	if strings.TrimSpace(input) == "" {
		return c.cwdDir.Path
	}
	if strings.HasPrefix(input, "/") {
		return normalizePath(input)
	}
	return normalizePath(path.Join(c.cwdDir.Path, input))
}

func normalizePath(p string) string {
	cleaned := path.Clean("/" + strings.TrimSpace(p))
	if cleaned == "" {
		return "/"
	}
	return cleaned
}

func parentDirectoryPath(p string) string {
	cleaned := normalizePath(p)
	if cleaned == "/" {
		return "/"
	}
	parent := path.Dir(cleaned)
	if parent == "" {
		return "/"
	}
	return parent
}

func isPolicyFilename(name string) bool {
	switch strings.ToLower(strings.TrimSpace(path.Base(name))) {
	case ".rego", ".jsonschema", ".workflow", ".webhook", ".user", ".group":
		return true
	default:
		return false
	}
}

func normalizeStringList(values []string) []string {
	result := make([]string, 0, len(values))
	for _, v := range values {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func isAdminGroupName(name string) bool {
	return strings.EqualFold(name, "admin") || strings.EqualFold(name, "admins")
}

func groupContains(members []string, actor string) bool {
	for _, member := range members {
		if strings.EqualFold(strings.TrimSpace(member), actor) {
			return true
		}
	}
	return false
}

func actorHasAdminPrivileges(actor string, principals principalSetDTO) bool {
	if strings.EqualFold(actor, "system") || strings.EqualFold(actor, "admin") {
		return true
	}
	adminGroups := make(map[string]policyGroupDTO)
	for _, group := range principals.Groups {
		if isAdminGroupName(group.ID) {
			adminGroups[strings.ToLower(strings.TrimSpace(group.ID))] = group
			if groupContains(group.Members, actor) {
				return true
			}
		}
	}
	for _, user := range principals.Users {
		if !strings.EqualFold(strings.TrimSpace(user.ID), actor) {
			continue
		}
		for _, groupID := range normalizeStringList(user.Groups) {
			if isAdminGroupName(groupID) {
				return true
			}
			if group, ok := adminGroups[strings.ToLower(groupID)]; ok && groupContains(group.Members, actor) {
				return true
			}
		}
	}
	return false
}

func (c *cli) ensurePolicyAdmin(path string) error {
	actor := strings.TrimSpace(c.actor)
	if actor == "" || strings.EqualFold(actor, "system") || strings.EqualFold(actor, "admin") {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	result, err := c.metadata.ResolvePolicy(ctx, "", normalizePath(path), "")
	if err != nil {
		return err
	}
	if actorHasAdminPrivileges(actor, result.Principals) {
		return nil
	}
	return fmt.Errorf("admin privileges required for policy file operations")
}

func (c *cli) lookupDirectory(p string) (directoryDTO, error) {
	if dir, ok := c.dirCache[p]; ok {
		return dir, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	dir, err := c.metadata.ResolveDirectory(ctx, p)
	if err != nil {
		return directoryDTO{}, err
	}
	c.dirCache[p] = dir
	return dir, nil
}

func (c *cli) ensureDirectory(p string) (directoryDTO, error) {
	if dir, ok := c.dirCache[p]; ok {
		return dir, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	dir, err := c.metadata.ResolveDirectory(ctx, p)
	if err == nil {
		c.dirCache[p] = dir
		return dir, nil
	}
	if apiErr, ok := err.(apiError); ok && apiErr.Status == http.StatusNotFound {
		if p == "/" {
			created, cerr := c.metadata.CreateDirectory(ctx, "/", nil)
			if cerr != nil {
				return directoryDTO{}, cerr
			}
			c.dirCache[created.Path] = created
			return created, nil
		}
		parentPath := parentDirectoryPath(p)
		parent, perr := c.ensureDirectory(parentPath)
		if perr != nil {
			return directoryDTO{}, perr
		}
		name := strings.Trim(path.Base(p), "/")
		if name == "" {
			return directoryDTO{}, fmt.Errorf("invalid directory path %s", p)
		}
		created, cerr := c.metadata.CreateDirectory(ctx, name, &parent.ID)
		if cerr != nil {
			if aerr, ok := cerr.(apiError); ok && aerr.Status == http.StatusConflict {
				return c.lookupDirectory(p)
			}
			return directoryDTO{}, cerr
		}
		c.dirCache[created.Path] = created
		return created, nil
	}
	return directoryDTO{}, err
}

type createFileRequest struct {
	DirectoryID string           `json:"directory_id"`
	Name        string           `json:"name"`
	StorageMode string           `json:"storage_mode"`
	BlobKey     *string          `json:"blob_key,omitempty"`
	JSONPayload *json.RawMessage `json:"json_payload,omitempty"`
	Metadata    map[string]any   `json:"metadata,omitempty"`
	Checksum    *string          `json:"checksum,omitempty"`
	Size        *int64           `json:"size,omitempty"`
	MimeType    *string          `json:"mime_type,omitempty"`
	Actor       string           `json:"actor"`
}

type updateDirectoryRequest struct {
	Name     *string `json:"name,omitempty"`
	ParentID *string `json:"parent_id,omitempty"`
	Version  *int64  `json:"version,omitempty"`
}

type updateFileRequest struct {
	DirectoryID *string          `json:"directory_id,omitempty"`
	Name        *string          `json:"name,omitempty"`
	StorageMode *string          `json:"storage_mode,omitempty"`
	BlobKey     *string          `json:"blob_key,omitempty"`
	JSONPayload *json.RawMessage `json:"json_payload,omitempty"`
	Metadata    map[string]any   `json:"metadata,omitempty"`
	Checksum    *string          `json:"checksum,omitempty"`
	Size        *int64           `json:"size,omitempty"`
	MimeType    *string          `json:"mime_type,omitempty"`
	Version     *int64           `json:"version,omitempty"`
	Actor       string           `json:"actor"`
}

func (m *metadataClient) ResolveDirectory(ctx context.Context, path string) (directoryDTO, error) {
	endpoint := fmt.Sprintf("%s/api/v1/directories/resolve?path=%s", m.baseURL, url.QueryEscape(path))
	var out directoryDTO
	if err := m.do(ctx, http.MethodGet, endpoint, nil, &out); err != nil {
		return directoryDTO{}, err
	}
	return out, nil
}

func (m *metadataClient) CreateDirectory(ctx context.Context, name string, parentID *string) (directoryDTO, error) {
	payload := map[string]any{"name": name}
	if parentID != nil {
		payload["parent_id"] = parentID
	}
	endpoint := fmt.Sprintf("%s/api/v1/directories", m.baseURL)
	var out directoryDTO
	if err := m.do(ctx, http.MethodPost, endpoint, payload, &out); err != nil {
		return directoryDTO{}, err
	}
	return out, nil
}

func (m *metadataClient) DeleteDirectory(ctx context.Context, id string, force bool) error {
	endpoint := fmt.Sprintf("%s/api/v1/directories/%s", m.baseURL, url.PathEscape(id))
	if force {
		endpoint += "?force=true"
	}
	return m.do(ctx, http.MethodDelete, endpoint, nil, nil)
}

func (m *metadataClient) UpdateDirectory(ctx context.Context, id string, in updateDirectoryRequest) (directoryDTO, error) {
	endpoint := fmt.Sprintf("%s/api/v1/directories/%s", m.baseURL, url.PathEscape(id))
	var out directoryDTO
	if err := m.do(ctx, http.MethodPatch, endpoint, in, &out); err != nil {
		return directoryDTO{}, err
	}
	return out, nil
}

func (m *metadataClient) ListDirectory(ctx context.Context, parentID string, recursive bool) (listDirectoryResponse, error) {
	params := url.Values{}
	if parentID != "" {
		params.Set("parent_id", parentID)
	}
	if recursive {
		params.Set("recursive", "true")
	}
	endpoint := fmt.Sprintf("%s/api/v1/directories", m.baseURL)
	if enc := params.Encode(); enc != "" {
		endpoint += "?" + enc
	}
	var out listDirectoryResponse
	if err := m.do(ctx, http.MethodGet, endpoint, nil, &out); err != nil {
		return listDirectoryResponse{}, err
	}
	return out, nil
}

func (m *metadataClient) ResolveFile(ctx context.Context, path string) (fileDTO, error) {
	endpoint := fmt.Sprintf("%s/api/v1/files/resolve?path=%s", m.baseURL, url.QueryEscape(path))
	var out fileDTO
	if err := m.do(ctx, http.MethodGet, endpoint, nil, &out); err != nil {
		return fileDTO{}, err
	}
	return out, nil
}

func (m *metadataClient) CreateFile(ctx context.Context, in createFileRequest) (fileDTO, error) {
	endpoint := fmt.Sprintf("%s/api/v1/files", m.baseURL)
	var out fileDTO
	if err := m.do(ctx, http.MethodPost, endpoint, in, &out); err != nil {
		return fileDTO{}, err
	}
	return out, nil
}

func (m *metadataClient) DeleteFile(ctx context.Context, id string) error {
	endpoint := fmt.Sprintf("%s/api/v1/files/%s", m.baseURL, url.PathEscape(id))
	return m.do(ctx, http.MethodDelete, endpoint, nil, nil)
}

func (m *metadataClient) UpdateFile(ctx context.Context, id string, in updateFileRequest) (fileDTO, error) {
	endpoint := fmt.Sprintf("%s/api/v1/files/%s", m.baseURL, url.PathEscape(id))
	var out fileDTO
	if err := m.do(ctx, http.MethodPatch, endpoint, in, &out); err != nil {
		return fileDTO{}, err
	}
	return out, nil
}

func (m *metadataClient) GetFile(ctx context.Context, id string) (fileDTO, error) {
	endpoint := fmt.Sprintf("%s/api/v1/files/%s", m.baseURL, url.PathEscape(id))
	var out fileDTO
	if err := m.do(ctx, http.MethodGet, endpoint, nil, &out); err != nil {
		return fileDTO{}, err
	}
	return out, nil
}

func (m *metadataClient) ListFileVersions(ctx context.Context, id string) ([]fileVersionDTO, error) {
	endpoint := fmt.Sprintf("%s/api/v1/files/%s/versions", m.baseURL, url.PathEscape(id))
	var out []fileVersionDTO
	if err := m.do(ctx, http.MethodGet, endpoint, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (m *metadataClient) ResolvePolicy(ctx context.Context, directoryID, path, typ string) (policyResolutionDTO, error) {
	params := url.Values{}
	if strings.TrimSpace(directoryID) != "" {
		params.Set("directory_id", directoryID)
	}
	if strings.TrimSpace(path) != "" {
		params.Set("path", path)
	}
	if strings.TrimSpace(typ) != "" {
		params.Set("type", typ)
	}
	endpoint := fmt.Sprintf("%s/api/v1/policies/resolve", m.baseURL)
	if enc := params.Encode(); enc != "" {
		endpoint += "?" + enc
	}
	var out policyResolutionDTO
	if err := m.do(ctx, http.MethodGet, endpoint, nil, &out); err != nil {
		return policyResolutionDTO{}, err
	}
	return out, nil
}

func (m *metadataClient) do(ctx context.Context, method, endpoint string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		buf := &bytes.Buffer{}
		if err := json.NewEncoder(buf).Encode(body); err != nil {
			return err
		}
		reader = buf
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if strings.TrimSpace(m.actor) != "" {
		req.Header.Set(actorHeader, m.actor)
	}
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return parseAPIError(resp)
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		io.Copy(io.Discard, resp.Body)
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *contentClient) Upload(ctx context.Context, name, mimeType string, data []byte) (uploadResponse, error) {
	payload := map[string]any{
		"name":      name,
		"mime_type": mimeType,
		"data":      base64.StdEncoding.EncodeToString(data),
	}
	endpoint := fmt.Sprintf("%s/api/v1/content", c.baseURL)
	var out uploadResponse
	if err := c.do(ctx, http.MethodPost, endpoint, payload, &out); err != nil {
		return uploadResponse{}, err
	}
	if out.MimeType == "" {
		out.MimeType = mimeType
	}
	return out, nil
}

func (c *contentClient) Download(ctx context.Context, key string) ([]byte, error) {
	endpoint := fmt.Sprintf("%s/api/v1/content/%s", c.baseURL, url.PathEscape(key))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, parseAPIError(resp)
	}
	return io.ReadAll(resp.Body)
}

func (c *contentClient) do(ctx context.Context, method, endpoint string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		buf := &bytes.Buffer{}
		if err := json.NewEncoder(buf).Encode(body); err != nil {
			return err
		}
		reader = buf
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return parseAPIError(resp)
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		io.Copy(io.Discard, resp.Body)
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func parseAPIError(resp *http.Response) error {
	var payload struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	data, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(data, &payload)
	message := strings.TrimSpace(payload.Error.Message)
	if message == "" && len(data) > 0 {
		message = strings.TrimSpace(string(data))
	}
	if message == "" {
		message = resp.Status
	}
	return apiError{Status: resp.StatusCode, Code: payload.Error.Code, Message: message}
}

func serviceBaseURL(cfg config.Settings, name string) (string, error) {
	svc, ok := cfg.Services[name]
	if !ok {
		return "", fmt.Errorf("service %s not configured", name)
	}
	address := strings.TrimSpace(svc.Address)
	if address == "" {
		return "", fmt.Errorf("service %s address missing", name)
	}
	if strings.HasPrefix(address, "http://") || strings.HasPrefix(address, "https://") {
		return strings.TrimRight(address, "/"), nil
	}
	if strings.HasPrefix(address, ":") {
		return "http://127.0.0.1" + address, nil
	}
	if !strings.Contains(address, "://") {
		return "http://" + address, nil
	}
	parsed, err := url.Parse(address)
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "" {
		parsed.Scheme = "http"
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed.String(), nil
}

func resolveActor() string {
	if u, err := user.Current(); err == nil {
		username := strings.TrimSpace(u.Username)
		if username != "" {
			return username
		}
		if u.Name != "" {
			return strings.TrimSpace(u.Name)
		}
	}
	if login := strings.TrimSpace(os.Getenv("USER")); login != "" {
		return login
	}
	return "cli"
}

func formatResult(v any) (string, error) {
	switch val := v.(type) {
	case nil:
		return "null", nil
	case string:
		return val, nil
	case json.Number:
		return val.String(), nil
	default:
		encoded, err := json.MarshalIndent(val, "", "  ")
		if err != nil {
			encoded, err = json.Marshal(val)
			if err != nil {
				return "", err
			}
		}
		return string(encoded), nil
	}
}

func writeJSON(w io.Writer, v any) error {
	encoded, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		encoded, err = json.Marshal(v)
		if err != nil {
			return err
		}
	}
	encoded = append(encoded, '\n')
	_, err = w.Write(encoded)
	return err
}
