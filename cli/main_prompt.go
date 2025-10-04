package main

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/c-bata/go-prompt"
	"github.com/telnet2/mysql-vfs/cli/client"
	"github.com/telnet2/mysql-vfs/cli/commands"
	"github.com/telnet2/mysql-vfs/cli/session"
)

// REPLState holds the state of the REPL
type REPLState struct {
	vfsClient   *client.Client
	session     *session.Session
	localMode   bool
	localDir    string
	vfsServiceURL string
}

func main() {
	vfsServiceURL := getEnv("VFS_SERVICE_URL", "http://localhost:8080")

	// Initialize VFS client and session
	vfsClient := client.NewClient(vfsServiceURL)
	sess := session.NewSession()

	// Get local working directory
	localDir, err := os.Getwd()
	if err != nil {
		localDir = os.Getenv("HOME")
	}

	state := &REPLState{
		vfsClient:   vfsClient,
		session:     sess,
		localMode:   false,
		localDir:    localDir,
		vfsServiceURL: vfsServiceURL,
	}

	// Show banner
	fmt.Println("=== VFS CLI (go-prompt) ===")
	fmt.Printf("VFS Service: %s\n", vfsServiceURL)
	fmt.Printf("Local Dir: %s\n", localDir)
	fmt.Println("\nCommands:")
	fmt.Println("  VFS: ls, cd, pwd, mkdir, rmdir, cat, import, rm, mv, jq")
	fmt.Println("  Local: $<cmd> (e.g., $ls, $cat, $pwd)")
	fmt.Println("  Mode: $ (local mode), / (VFS mode)")
	fmt.Println("  Quit: exit, quit, Ctrl+D")
	fmt.Println()

	// Check VFS connectivity
	healthy, err := vfsClient.HealthCheck()
	if err != nil || !healthy {
		fmt.Printf("⚠️  Warning: Cannot connect to VFS service: %v\n", err)
	} else {
		fmt.Println("✓ Connected to VFS service")
	}
	fmt.Println()

	// Start prompt
	p := prompt.New(
		func(in string) { executor(in, state) },
		func(d prompt.Document) []prompt.Suggest { return completer(d, state) },
		prompt.OptionPrefix(getPromptPrefix(state)),
		prompt.OptionLivePrefix(func() (string, bool) {
			return getPromptPrefix(state), true
		}),
		prompt.OptionTitle("vfs-cli"),
		prompt.OptionHistory([]string{}),
		prompt.OptionPrefixTextColor(prompt.Yellow),
		prompt.OptionPreviewSuggestionTextColor(prompt.Blue),
		prompt.OptionSelectedSuggestionBGColor(prompt.LightGray),
		prompt.OptionSuggestionBGColor(prompt.DarkGray),
	)

	p.Run()
}

// getPromptPrefix returns the current prompt prefix
func getPromptPrefix(state *REPLState) string {
	if state.localMode {
		return fmt.Sprintf("local:%s> ", state.localDir)
	}
	return fmt.Sprintf("vfs:%s> ", state.session.GetCurrentDirectory())
}

// executor executes the command
func executor(input string, state *REPLState) {
	input = strings.TrimSpace(input)
	if input == "" {
		return
	}

	// Handle exit
	if input == "exit" || input == "quit" {
		fmt.Println("Goodbye!")
		os.Exit(0)
	}

	// Handle mode switching
	if input == "$" {
		state.localMode = true
		fmt.Println("Switched to local mode (shell commands)")
		return
	}
	if input == "/" {
		state.localMode = false
		fmt.Println("Switched to VFS mode")
		return
	}

	// Handle local commands with $ prefix
	if strings.HasPrefix(input, "$") {
		executeLocalCommand(input[1:], state)
		return
	}

	// Execute in appropriate mode
	if state.localMode {
		executeLocalCommand(input, state)
	} else {
		executeVFSCommand(input, state)
	}
}

// executeLocalCommand executes a local shell command using zsh
func executeLocalCommand(input string, state *REPLState) {
	// Special handling for cd command
	if strings.HasPrefix(input, "cd ") || input == "cd" {
		args := strings.Fields(input)
		var targetDir string

		if len(args) == 1 {
			targetDir = os.Getenv("HOME")
		} else {
			targetDir = args[1]
		}

		// Expand ~ to home directory
		if strings.HasPrefix(targetDir, "~/") {
			targetDir = filepath.Join(os.Getenv("HOME"), targetDir[2:])
		} else if targetDir == "~" {
			targetDir = os.Getenv("HOME")
		}

		// Make absolute if relative
		if !filepath.IsAbs(targetDir) {
			targetDir = filepath.Join(state.localDir, targetDir)
		}

		// Change directory
		if err := os.Chdir(targetDir); err != nil {
			fmt.Fprintf(os.Stderr, "cd: %v\n", err)
			return
		}

		// Update state
		newDir, _ := os.Getwd()
		state.localDir = newDir
		return
	}

	// Execute command with zsh
	cmd := exec.Command("zsh", "-c", input)
	cmd.Dir = state.localDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		// Don't print error if it's just a non-zero exit code
		if _, ok := err.(*exec.ExitError); !ok {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
	}
}

// executeVFSCommand executes a VFS command
func executeVFSCommand(input string, state *REPLState) {
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return
	}

	cmdName := parts[0]
	args := parts[1:]

	// Create command context
	ctx := &commands.Context{
		Client:  state.vfsClient,
		Session: state.session,
		Stdin:   os.Stdin,
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
	}

	// Command map
	cmdMap := map[string]commands.Command{
		"ls":     &commands.LsCommand{},
		"cd":     &commands.CdCommand{},
		"pwd":    &commands.PwdCommand{},
		"mkdir":  &commands.MkdirCommand{},
		"rmdir":  &commands.RmdirCommand{},
		"cat":    &commands.CatCommand{},
		"import": &commands.ImportCommand{},
		"rm":     &commands.RmCommand{},
		"mv":     &commands.MvCommand{},
		"jq":     &commands.JqCommand{},
		"help":   commands.NewHelpCommand(nil),
	}

	cmd, exists := cmdMap[cmdName]
	if !exists {
		fmt.Printf("Unknown VFS command: %s (type 'help' for available commands)\n", cmdName)
		fmt.Println("Tip: Use !<cmd> or 'local' mode for shell commands")
		return
	}

	if err := cmd.Execute(ctx, args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	}
}

// completer provides auto-completion suggestions
func completer(d prompt.Document, state *REPLState) []prompt.Suggest {
	text := d.TextBeforeCursor()

	if text == "" {
		if state.localMode {
			return localCommandSuggestions()
		}
		return vfsCommandSuggestions()
	}

	// Mode switch suggestions
	if text == "$" {
		return []prompt.Suggest{
			{Text: "$", Description: "Switch to local shell mode"},
		}
	}
	if text == "/" {
		return []prompt.Suggest{
			{Text: "/", Description: "Switch to VFS mode"},
		}
	}

	// Local command with $ prefix
	if strings.HasPrefix(text, "$") {
		return getLocalPathCompletions(text[1:], state)
	}

	// Path completion based on mode
	if state.localMode {
		return getLocalPathCompletions(text, state)
	}

	return getVFSPathCompletions(text, state)
}

// getLocalPathCompletions returns path completions for local filesystem
func getLocalPathCompletions(text string, state *REPLState) []prompt.Suggest {
	parts := strings.Fields(text)

	// If no parts or only command, show command suggestions
	if len(parts) == 0 {
		return localCommandSuggestions()
	}

	// If only one part, it could be command or path
	if len(parts) == 1 && !strings.Contains(text, " ") {
		cmdSuggestions := prompt.FilterHasPrefix(localCommandSuggestions(), parts[0], true)
		if len(cmdSuggestions) > 0 {
			return cmdSuggestions
		}
	}

	// Get the last part for path completion
	lastPart := parts[len(parts)-1]

	// Determine base directory for completion
	baseDir := state.localDir
	searchPattern := lastPart

	// Handle absolute paths
	if filepath.IsAbs(lastPart) {
		baseDir = filepath.Dir(lastPart)
		searchPattern = filepath.Base(lastPart)
	} else if strings.Contains(lastPart, "/") {
		// Relative path with directory
		baseDir = filepath.Join(state.localDir, filepath.Dir(lastPart))
		searchPattern = filepath.Base(lastPart)
	}

	// Expand ~ to home directory
	if strings.HasPrefix(baseDir, "~") {
		baseDir = filepath.Join(os.Getenv("HOME"), baseDir[1:])
	}

	// Read directory entries
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return []prompt.Suggest{}
	}

	var suggestions []prompt.Suggest
	for _, entry := range entries {
		name := entry.Name()

		// Skip hidden files unless explicitly searching for them
		if strings.HasPrefix(name, ".") && !strings.HasPrefix(searchPattern, ".") {
			continue
		}

		// Filter by search pattern
		if searchPattern != "" && !strings.HasPrefix(name, searchPattern) {
			continue
		}

		// Build full path for display
		var displayPath string
		if filepath.IsAbs(lastPart) {
			displayPath = filepath.Join(baseDir, name)
		} else if strings.Contains(lastPart, "/") {
			displayPath = filepath.Join(filepath.Dir(lastPart), name)
		} else {
			displayPath = name
		}

		// Add / suffix for directories
		description := "file"
		if entry.IsDir() {
			displayPath += "/"
			description = "directory"
		}

		suggestions = append(suggestions, prompt.Suggest{
			Text:        displayPath,
			Description: description,
		})
	}

	return suggestions
}

// getVFSPathCompletions returns path completions for VFS
func getVFSPathCompletions(text string, state *REPLState) []prompt.Suggest {
	parts := strings.Fields(text)

	// If no parts or only command, show command suggestions
	if len(parts) == 0 {
		return vfsCommandSuggestions()
	}

	// If only one part, it could be command or path
	if len(parts) == 1 && !strings.Contains(text, " ") {
		cmdSuggestions := prompt.FilterHasPrefix(vfsCommandSuggestions(), parts[0], true)
		if len(cmdSuggestions) > 0 {
			return cmdSuggestions
		}
	}

	// Get the last part for path completion
	lastPart := parts[len(parts)-1]

	// Determine target directory
	var targetDir string
	var searchPattern string

	if strings.HasPrefix(lastPart, "/") {
		// Absolute path
		if strings.HasSuffix(lastPart, "/") {
			targetDir = lastPart
			searchPattern = ""
		} else {
			targetDir = filepath.Dir(lastPart)
			searchPattern = filepath.Base(lastPart)
		}
	} else {
		// Relative path
		currentDir := state.session.GetCurrentDirectory()
		if strings.Contains(lastPart, "/") {
			targetDir = filepath.Join(currentDir, filepath.Dir(lastPart))
			searchPattern = filepath.Base(lastPart)
		} else {
			targetDir = currentDir
			searchPattern = lastPart
		}
	}

	// Fetch directory listing from VFS
	resp, err := state.vfsClient.ListDirectory(targetDir, 100, "")
	if err != nil {
		return []prompt.Suggest{}
	}

	var suggestions []prompt.Suggest
	for _, entry := range resp.Entries {
		// Filter by search pattern
		if searchPattern != "" && !strings.HasPrefix(entry.Name, searchPattern) {
			continue
		}

		// Build display path
		var displayPath string
		if strings.HasPrefix(lastPart, "/") {
			if strings.HasSuffix(lastPart, "/") {
				displayPath = lastPart + entry.Name
			} else {
				displayPath = filepath.Join(filepath.Dir(lastPart), entry.Name)
			}
		} else if strings.Contains(lastPart, "/") {
			displayPath = filepath.Join(filepath.Dir(lastPart), entry.Name)
		} else {
			displayPath = entry.Name
		}

		// Add / suffix for directories
		description := fmt.Sprintf("file (%d bytes)", entry.SizeBytes)
		if entry.Type == "directory" {
			displayPath += "/"
			description = "directory"
		}

		suggestions = append(suggestions, prompt.Suggest{
			Text:        displayPath,
			Description: description,
		})
	}

	return suggestions
}

// vfsCommandSuggestions returns VFS command suggestions
func vfsCommandSuggestions() []prompt.Suggest {
	return []prompt.Suggest{
		{Text: "ls", Description: "List directory contents"},
		{Text: "cd", Description: "Change directory"},
		{Text: "pwd", Description: "Print working directory"},
		{Text: "mkdir", Description: "Create directory"},
		{Text: "rmdir", Description: "Remove directory"},
		{Text: "cat", Description: "Display file contents"},
		{Text: "import", Description: "Import local file to VFS"},
		{Text: "rm", Description: "Remove file"},
		{Text: "mv", Description: "Move/rename file"},
		{Text: "jq", Description: "Query JSON file"},
		{Text: "help", Description: "Show help"},
		{Text: "$", Description: "Switch to local mode"},
		{Text: "/", Description: "Switch to VFS mode"},
		{Text: "exit", Description: "Exit CLI"},
	}
}

// localCommandSuggestions returns local shell command suggestions
func localCommandSuggestions() []prompt.Suggest {
	return []prompt.Suggest{
		{Text: "ls", Description: "List local directory"},
		{Text: "cd", Description: "Change local directory"},
		{Text: "pwd", Description: "Print local working directory"},
		{Text: "cat", Description: "Display local file"},
		{Text: "find", Description: "Find local files"},
		{Text: "grep", Description: "Search in local files"},
		{Text: "tree", Description: "Display directory tree"},
		{Text: "head", Description: "Show first lines of file"},
		{Text: "tail", Description: "Show last lines of file"},
		{Text: "wc", Description: "Count lines, words, bytes"},
		{Text: "file", Description: "Determine file type"},
		{Text: "du", Description: "Disk usage"},
		{Text: "df", Description: "Disk free space"},
		{Text: "/", Description: "Switch to VFS mode"},
		{Text: "exit", Description: "Exit CLI"},
	}
}

// Suppress unused warning
var _ = fs.WalkDir

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
