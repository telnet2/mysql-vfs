package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/c-bata/go-prompt"
	"github.com/spf13/cobra"
	"github.com/telnet2/mysql-vfs/cli/client"
	"github.com/telnet2/mysql-vfs/cli/commands"
	"github.com/telnet2/mysql-vfs/cli/pipe"
	"github.com/telnet2/mysql-vfs/cli/session"
)

var (
	vfsServiceURL    string
	onBehalfOf       string
	delegationReason string
	ctx              *commands.Context
)

type interactiveState struct {
	shellMode bool
}

var safeShellCommands = map[string]bool{
	"ls":       true,
	"pwd":      true,
	"cd":       true,
	"echo":     true,
	"cat":      true,
	"grep":     true,
	"find":     true,
	"wc":       true,
	"head":     true,
	"tail":     true,
	"sort":     true,
	"uniq":     true,
	"diff":     true,
	"less":     true,
	"more":     true,
	"clear":    true,
	"env":      true,
	"printenv": true,
	"whoami":   true,
	"uname":    true,
	"date":     true,
	"touch":    true,
}

// rootCmd represents the base command when called without any subcommands
var rootCmd *cobra.Command

func init() {
	rootCmd = &cobra.Command{
		Use:   "vfs-cli",
		Short: "VFS CLI - Virtual File System Command Line Interface",
		Long: `A command-line interface for interacting with the MySQL VFS service.
Supports file operations, directory management, and interactive shell mode.`,
		Run: func(cmd *cobra.Command, args []string) {
			// Start interactive shell mode
			runInteractiveMode()
		},
		// Silence cobra's default usage on error (we'll handle it ourselves)
		SilenceUsage:  true,
		SilenceErrors: true,
	}
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() error {
	initRootCmd()
	return rootCmd.Execute()
}

func initRootCmd() {
	cobra.OnInitialize(initConfig)

	// Global flags
	rootCmd.PersistentFlags().StringVar(&vfsServiceURL, "url", "", "VFS service URL (default: env VFS_SERVICE_URL or http://localhost:18080)")
	rootCmd.PersistentFlags().StringVar(&onBehalfOf, "on-behalf-of", "", "Act on behalf of another user (requires impersonate permission)")
	rootCmd.PersistentFlags().StringVar(&delegationReason, "reason", "", "Reason for delegation (audit trail)")

	// Add all subcommands
	rootCmd.AddCommand(helpCmd)
	rootCmd.AddCommand(lsCmd)
	rootCmd.AddCommand(treeCmd)
	rootCmd.AddCommand(cdCmd)
	rootCmd.AddCommand(pwdCmd)
	rootCmd.AddCommand(mkdirCmd)
	rootCmd.AddCommand(rmdirCmd)
	rootCmd.AddCommand(catCmd)
	rootCmd.AddCommand(editCmd)
	rootCmd.AddCommand(importCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(rmCmd)
	rootCmd.AddCommand(mvCmd)
	rootCmd.AddCommand(cpCmd)
	rootCmd.AddCommand(grepCmd)
	rootCmd.AddCommand(findCmd)
	rootCmd.AddCommand(attrCmd)
	rootCmd.AddCommand(aliasCmd)
	rootCmd.AddCommand(jqCmd)
	rootCmd.AddCommand(loginCmd)
	rootCmd.AddCommand(logoutCmd)
	rootCmd.AddCommand(historyCmd)
	rootCmd.AddCommand(getCwdCmd)
	rootCmd.AddCommand(setCwdCmd)
	rootCmd.AddCommand(createSampleFilesCmd)
	rootCmd.AddCommand(createTriggerCmd)
}

func initConfig() {
	// Get service URL
	if vfsServiceURL == "" {
		vfsServiceURL = getEnv("VFS_SERVICE_URL", "http://localhost:18080")
	}

	// Initialize client and session (only if ctx is nil)
	if ctx == nil {
		vfsClient := client.NewClient(vfsServiceURL)
		sess := session.NewSession()

		// Load auth token (priority: env vars > saved file)
		token := os.Getenv("VFS_AUTH_TOKEN")
		if token == "" {
			token = os.Getenv("SYSTEM_ADMIN_TOKEN")
		}
		if token == "" {
			// Fall back to saved token file
			if savedToken, err := commands.LoadToken(); err == nil && savedToken != "" {
				token = savedToken
			}
		}

		if token != "" {
			vfsClient.SetAuthToken(token)
			sess.SetAuthToken(token)
		}

		// Set delegation headers if provided
		if onBehalfOf != "" {
			vfsClient.SetOnBehalfOf(onBehalfOf, delegationReason)
		}

		// Create command context
		ctx = &commands.Context{
			Client:  vfsClient,
			Session: sess,
			Stdin:   os.Stdin,
			Stdout:  os.Stdout,
			Stderr:  os.Stderr,
		}
	} else {
		// Update client URL if it changed
		if ctx.Client != nil && vfsServiceURL != "" {
			ctx.Client = client.NewClient(vfsServiceURL)
			// Preserve auth token
			if token := ctx.Session.GetAuthToken(); token != "" {
				ctx.Client.SetAuthToken(token)
			}
			// Set delegation headers if provided
			if onBehalfOf != "" {
				ctx.Client.SetOnBehalfOf(onBehalfOf, delegationReason)
			}
		}
	}
}

func runInteractiveMode() {
	fmt.Println("=== VFS CLI ===")
	fmt.Printf("Connecting to: %s\n", vfsServiceURL)

	healthy, err := ctx.Client.HealthCheck()
	if err != nil || !healthy {
		fmt.Printf("Warning: Cannot connect to VFS service: %v\n", err)
		fmt.Println("Commands will fail until service is available.")
	} else {
		fmt.Println("Connected successfully!")
	}

	fmt.Println("Type 'help' for available commands or 'exit' to quit")
	fmt.Println("Type '$' to toggle between VFS mode and shell mode, or '$<command>' or '$ <command>' to run a single shell command")
	fmt.Println("Press TAB to autocomplete commands")
	fmt.Println("Press CTRL+C twice to exit")
	fmt.Println()

	state := &interactiveState{}
	lastInterruptTime := new(int64)

	executor := func(input string) {
		input = strings.TrimSpace(input)
		if input == "" {
			return
		}

		// Handle shell commands - $ toggles mode, $command or "$ command" runs single command
		if strings.HasPrefix(input, "$") {
			if input == "$" {
				// Toggle mode
				state.shellMode = !state.shellMode
				if state.shellMode {
					fmt.Println("Entering shell mode... (type '$' to return to VFS mode)")
				} else {
					fmt.Println("Returning to VFS mode...")
				}
				return
			} else {
				// Run single shell command ($command or "$ command")
				var shellCmd string
				if strings.HasPrefix(input, "$ ") {
					shellCmd = strings.TrimPrefix(input, "$ ")
				} else {
					shellCmd = strings.TrimPrefix(input, "$")
				}
				if err := runSingleShellCommand(shellCmd); err != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				}
				return
			}
		}

		// Handle exit
		if input == "exit" || input == "quit" {
			fmt.Println("Goodbye!")
			os.Exit(0)
		}

		// Add to history for history command reuse
		cmdHistory = append(cmdHistory, input)
		saveHistory(cmdHistory)

		if state.shellMode {
			if err := runShellCommand(input); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			}
			return
		}

		// VFS mode - Check for pipelines
		if strings.Contains(input, "|") {
			if err := executePipeline(input); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			}
			return
		}

		args := strings.Fields(input)
		if len(args) == 0 {
			return
		}

		// Check for aliases and expand if found
		if expanded := expandAlias(args[0]); expanded != args[0] {
			// Replace the alias with its expansion
			expandedArgs := strings.Fields(expanded)
			args = append(expandedArgs, args[1:]...)
		}

		// Get the command name for better error messages
		commandName := args[0]

		rootCmd.SetArgs(args)

		// Reset flags before each execution
		rootCmd.ResetFlags()
		initConfig()

		if err := rootCmd.Execute(); err != nil {
			// Format and print REPL-friendly error
			errorMsg := FormatREPLError(err, commandName)
			fmt.Fprintln(os.Stderr, errorMsg)
		}
	}

	completer := newCompleter(state)

	promptInstance := prompt.New(
		executor,
		completer,
		prompt.OptionTitle("VFS CLI"),
		prompt.OptionHistory(cmdHistory),
		prompt.OptionPrefix(""),
		prompt.OptionLivePrefix(func() (string, bool) {
			return state.prompt(), true
		}),
		prompt.OptionAddKeyBind(prompt.KeyBind{
			Key: prompt.ControlC,
			Fn: func(buf *prompt.Buffer) {
				now := time.Now().Unix()
				if now-*lastInterruptTime < 2 {
					// Second CTRL+C within 2 seconds - exit
					fmt.Println("\nGoodbye!")
					os.Exit(0)
				} else {
					// First CTRL+C - show message
					fmt.Println("\n(Press CTRL+C again to exit)")
					*lastInterruptTime = now
				}
			},
		}),
	)

	promptInstance.Run()
}

func (s *interactiveState) prompt() string {
	if s.shellMode {
		cwd, _ := os.Getwd()
		return fmt.Sprintf("shell:%s$ ", cwd)
	}
	return fmt.Sprintf("%s> ", ctx.Session.GetCurrentDirectory())
}

func newCompleter(state *interactiveState) prompt.Completer {
	vfsSuggestions := buildVFSSuggestions()
	shellSuggestions := buildShellSuggestions()

	return func(d prompt.Document) []prompt.Suggest {
		word := strings.TrimSpace(d.GetWordBeforeCursor())

		if state.shellMode {
			if word == "" {
				return []prompt.Suggest{}
			}
			return prompt.FilterHasPrefix(shellSuggestions, word, true)
		}

		trimmed := strings.TrimSpace(d.TextBeforeCursor())
		if strings.Contains(trimmed, " ") {
			// We're completing command parameters
			return completeParameters(ctx, trimmed, d)
		}

		if word == "" {
			return []prompt.Suggest{}
		}

		return prompt.FilterHasPrefix(vfsSuggestions, word, true)
	}
}

func buildVFSSuggestions() []prompt.Suggest {
	cmds := rootCmd.Commands()
	sort.Slice(cmds, func(i, j int) bool {
		return cmds[i].Name() < cmds[j].Name()
	})

	seen := make(map[string]struct{})
	suggestions := make([]prompt.Suggest, 0, len(cmds)+4)
	for _, cmd := range cmds {
		name := cmd.Name()
		if _, ok := seen[name]; !ok {
			suggestions = append(suggestions, prompt.Suggest{Text: name, Description: cmd.Short})
			seen[name] = struct{}{}
		}
		for _, alias := range cmd.Aliases {
			if alias == "" {
				continue
			}
			if _, ok := seen[alias]; ok {
				continue
			}
			desc := fmt.Sprintf("alias for %s", name)
			suggestions = append(suggestions, prompt.Suggest{Text: alias, Description: desc})
			seen[alias] = struct{}{}
		}
	}

	static := []prompt.Suggest{
		{Text: "help", Description: "Show help"},
		{Text: "exit", Description: "Exit CLI"},
		{Text: "quit", Description: "Exit CLI"},
		{Text: "$", Description: "Toggle shell mode"},
	}

	return append(suggestions, static...)
}

func buildShellSuggestions() []prompt.Suggest {
	keys := make([]string, 0, len(safeShellCommands))
	for cmd := range safeShellCommands {
		keys = append(keys, cmd)
	}
	sort.Strings(keys)

	suggestions := make([]prompt.Suggest, 0, len(keys)+3)
	for _, key := range keys {
		suggestions = append(suggestions, prompt.Suggest{Text: key, Description: "Shell command"})
	}

	return append(suggestions,
		prompt.Suggest{Text: "exit", Description: "Exit CLI"},
		prompt.Suggest{Text: "quit", Description: "Exit CLI"},
		prompt.Suggest{Text: "$", Description: "Return to VFS mode"},
	)
}

// runShellCommand runs a safe shell command
func runShellCommand(input string) error {
	// Parse command
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return nil
	}

	cmdName := parts[0]
	args := parts[1:]

	if !safeShellCommands[cmdName] {
		return fmt.Errorf("command '%s' is not allowed in shell mode", cmdName)
	}

	// Special handling for cd
	if cmdName == "cd" {
		if len(args) == 0 {
			homeDir, _ := os.UserHomeDir()
			return os.Chdir(homeDir)
		}
		return os.Chdir(args[0])
	}

	// Execute command
	cmd := exec.Command(cmdName, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// runSingleShellCommand runs a single shell command without mode switching
func runSingleShellCommand(input string) error {
	// Parse command
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return nil
	}

	cmdName := parts[0]
	args := parts[1:]

	// Special handling for cd
	if cmdName == "cd" {
		if len(args) == 0 {
			homeDir, _ := os.UserHomeDir()
			return os.Chdir(homeDir)
		}
		return os.Chdir(args[0])
	}

	// Execute command
	cmd := exec.Command(cmdName, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// executePipeline executes a pipeline of commands
func executePipeline(input string) error {
	segments := pipe.ParsePipeline(input)

	if len(segments) == 0 {
		return fmt.Errorf("empty pipeline")
	}

	// Create command map for pipeline execution
	cmdMap := map[string]commands.Command{
		"ls":  &commands.LsCommand{},
		"cat": &commands.CatCommand{},
		"jq":  &commands.JqCommand{},
		"pwd": &commands.PwdCommand{},
	}

	// Single command - no piping needed
	if len(segments) == 1 {
		cmd, exists := cmdMap[segments[0].Command]
		if !exists {
			return fmt.Errorf("unknown command in pipeline: %s", segments[0].Command)
		}
		return cmd.Execute(ctx, segments[0].Args)
	}

	// Create pipes for each connection
	pipes := make([]*io.PipeWriter, len(segments)-1)
	readers := make([]io.Reader, len(segments)-1)

	for i := 0; i < len(segments)-1; i++ {
		r, w := pipe.PipeReader()
		readers[i] = r
		pipes[i] = w.(*io.PipeWriter)
	}

	// Execute all commands concurrently
	errs := make(chan error, len(segments))

	for i, segment := range segments {
		cmd, exists := cmdMap[segment.Command]
		if !exists {
			return fmt.Errorf("unknown command in pipeline: %s", segment.Command)
		}

		pipeCtx := &commands.Context{
			Client:  ctx.Client,
			Session: ctx.Session,
			Stderr:  os.Stderr,
		}

		// Set stdin
		if i == 0 {
			pipeCtx.Stdin = os.Stdin
		} else {
			pipeCtx.Stdin = readers[i-1]
		}

		// Set stdout
		if i == len(segments)-1 {
			pipeCtx.Stdout = os.Stdout
		} else {
			pipeCtx.Stdout = pipes[i]
		}

		// Execute command
		go func(cmd commands.Command, pipeCtx *commands.Context, args []string, idx int) {
			err := cmd.Execute(pipeCtx, args)
			// Close pipe writer if not the last command
			if idx < len(pipes) {
				pipes[idx].Close()
			}
			errs <- err
		}(cmd, pipeCtx, segment.Args, i)
	}

	// Wait for all commands to complete
	var lastErr error
	for range segments {
		if err := <-errs; err != nil {
			lastErr = err
		}
	}

	return lastErr
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// completeLocalPaths completes local filesystem paths
func completeLocalPaths(prefix string) []prompt.Suggest {
	var suggestions []prompt.Suggest

	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return suggestions
	}

	// Determine base directory and remaining prefix
	var baseDir, remaining string
	if strings.Contains(prefix, "/") {
		lastSlash := strings.LastIndex(prefix, "/")
		baseDir = filepath.Join(cwd, prefix[:lastSlash])
		remaining = prefix[lastSlash+1:]
	} else {
		baseDir = cwd
		remaining = prefix
	}

	// Read directory entries
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return suggestions
	}

	// Filter and create suggestions
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, remaining) {
			if entry.IsDir() {
				suggestions = append(suggestions, prompt.Suggest{
					Text:        prefix[:len(prefix)-len(remaining)] + name + "/",
					Description: "directory",
				})
			} else {
				suggestions = append(suggestions, prompt.Suggest{
					Text:        prefix[:len(prefix)-len(remaining)] + name,
					Description: "file",
				})
			}
		}
	}

	return suggestions
}

// completeVFSPaths completes VFS paths
func completeVFSPaths(ctx *commands.Context, prefix string) []prompt.Suggest {
	var suggestions []prompt.Suggest

	// Resolve the prefix path
	resolvedPath := ctx.Session.ResolvePath(prefix)
	if !session.IsValidPath(resolvedPath) {
		return suggestions
	}

	// Determine parent directory and basename
	parentDir := filepath.Dir(resolvedPath)
	basename := filepath.Base(resolvedPath)

	// If prefix ends with /, we're completing in that directory
	if strings.HasSuffix(prefix, "/") {
		parentDir = resolvedPath
		basename = ""
	}

	// List directory contents
	resp, err := ctx.Client.ListDirectory(parentDir, 100, "")
	if err != nil {
		return suggestions
	}

	// Filter and create suggestions
	for _, entry := range resp.Entries {
		if strings.HasPrefix(entry.Name, basename) {
			var text string
			var desc string

			if strings.HasSuffix(prefix, "/") {
				text = prefix + entry.Name
			} else if strings.Contains(prefix, "/") {
				lastSlash := strings.LastIndex(prefix, "/")
				text = prefix[:lastSlash+1] + entry.Name
			} else {
				text = entry.Name
			}

			if entry.Type == "directory" {
				text += "/"
				desc = "directory"
			} else {
				desc = fmt.Sprintf("file (%d bytes)", entry.SizeBytes)
			}

			suggestions = append(suggestions, prompt.Suggest{
				Text:        text,
				Description: desc,
			})
		}
	}

	return suggestions
}

// completeParameters handles parameter completion for commands
func completeParameters(ctx *commands.Context, input string, d prompt.Document) []prompt.Suggest {
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return []prompt.Suggest{}
	}

	command := parts[0]
	args := parts[1:]

	// Get the word currently being typed
	wordBeforeCursor := d.GetWordBeforeCursor()

	// Determine which argument position we're in
	textBeforeCursor := d.TextBeforeCursor()
	partsBeforeCursor := strings.Fields(textBeforeCursor)

	argIndex := len(partsBeforeCursor) - 1
	if !strings.HasSuffix(textBeforeCursor, " ") && len(partsBeforeCursor) > 0 {
		// We're in the middle of the current argument
		argIndex--
	}

	// Ensure argIndex is not negative
	if argIndex < 0 {
		argIndex = 0
	}

	// Get the current partial argument
	var currentArg string
	if argIndex < len(args) {
		currentArg = args[argIndex]
	} else {
		currentArg = wordBeforeCursor
	}

	// Route to command-specific completion
	switch command {
	case "import":
		if argIndex == 0 {
			return completeLocalPaths(currentArg)
		} else if argIndex == 1 {
			return completeVFSPaths(ctx, currentArg)
		}
	case "cd", "ls", "create-sample-files":
		return completeVFSPaths(ctx, currentArg)
	case "cat", "edit", "rm", "version":
		if argIndex == 0 {
			return completeVFSPaths(ctx, currentArg)
		}
	case "jq":
		if argIndex == 0 {
			return completeVFSPaths(ctx, currentArg)
		}
	case "mv":
		if argIndex == 0 || argIndex == 1 {
			return completeVFSPaths(ctx, currentArg)
		}
	}

	return []prompt.Suggest{}
}
