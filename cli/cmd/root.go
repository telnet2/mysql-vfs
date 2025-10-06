package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"github.com/telnet2/mysql-vfs/cli/client"
	"github.com/telnet2/mysql-vfs/cli/commands"
	"github.com/telnet2/mysql-vfs/cli/pipe"
	"github.com/telnet2/mysql-vfs/cli/session"
)

var (
	vfsServiceURL string
	ctx           *commands.Context
)

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

	// Add all subcommands
	rootCmd.AddCommand(lsCmd)
	rootCmd.AddCommand(treeCmd)
	rootCmd.AddCommand(cdCmd)
	rootCmd.AddCommand(pwdCmd)
	rootCmd.AddCommand(mkdirCmd)
	rootCmd.AddCommand(rmdirCmd)
	rootCmd.AddCommand(catCmd)
	rootCmd.AddCommand(editCmd)
	rootCmd.AddCommand(importCmd)
	rootCmd.AddCommand(rmCmd)
	rootCmd.AddCommand(mvCmd)
	rootCmd.AddCommand(jqCmd)
	rootCmd.AddCommand(loginCmd)
	rootCmd.AddCommand(logoutCmd)
	rootCmd.AddCommand(historyCmd)
	rootCmd.AddCommand(getCwdCmd)
	rootCmd.AddCommand(setCwdCmd)
}

func initConfig() {
	// Get service URL
	if vfsServiceURL == "" {
		vfsServiceURL = getEnv("VFS_SERVICE_URL", "http://localhost:18080")
	}

	// Initialize client and session
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

	// Create command context
	ctx = &commands.Context{
		Client:  vfsClient,
		Session: sess,
		Stdin:   os.Stdin,
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
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
	fmt.Println("Type '$' to toggle between VFS mode and shell mode")
	fmt.Println()

	// REPL loop
	reader := bufio.NewReader(os.Stdin)
	shellMode := false

	for {
		var prompt string
		if shellMode {
			cwd, _ := os.Getwd()
			prompt = fmt.Sprintf("shell:%s$ ", cwd)
		} else {
			prompt = fmt.Sprintf("%s> ", ctx.Session.GetCurrentDirectory())
		}

		fmt.Print(prompt)
		input, err := reader.ReadString('\n')
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
			continue
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		// Handle mode switching - $ at the beginning toggles mode
		if input == "$" {
			shellMode = !shellMode
			if shellMode {
				fmt.Println("Entering shell mode... (type '$' to return to VFS mode)")
			} else {
				fmt.Println("Returning to VFS mode...")
			}
			continue
		}

		// Handle exit
		if input == "exit" || input == "quit" {
			fmt.Println("Goodbye!")
			return
		}

		// Add to history
		cmdHistory = append(cmdHistory, input)

		// Shell mode - run bash commands
		if shellMode {
			if err := runShellCommand(input); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			}
			continue
		}

		// VFS mode - Check if this is a pipeline
		if strings.Contains(input, "|") {
			if err := executePipeline(input); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			}
			continue
		}

		// Parse and execute command using cobra
		args := strings.Fields(input)
		rootCmd.SetArgs(args)

		// Reset flags before each execution
		rootCmd.ResetFlags()
		initConfig()

		if err := rootCmd.Execute(); err != nil {
			// Error already printed by cobra
		}
	}
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

	// Whitelist of safe commands
	safeCommands := map[string]bool{
		"ls":     true,
		"pwd":    true,
		"cd":     true,
		"echo":   true,
		"cat":    true,
		"grep":   true,
		"find":   true,
		"wc":     true,
		"head":   true,
		"tail":   true,
		"sort":   true,
		"uniq":   true,
		"diff":   true,
		"date":   true,
		"whoami": true,
		"env":    true,
	}

	if !safeCommands[cmdName] {
		return fmt.Errorf("command '%s' not allowed in shell mode (safe commands: ls, pwd, cd, echo, cat, grep, find, wc, head, tail, sort, uniq, diff, date, whoami, env)", cmdName)
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
