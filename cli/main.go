package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/telnet2/mysql-vfs/cli/client"
	"github.com/telnet2/mysql-vfs/cli/commands"
	"github.com/telnet2/mysql-vfs/cli/pipe"
	"github.com/telnet2/mysql-vfs/cli/session"
)

func main() {
	vfsServiceURL := getEnv("VFS_SERVICE_URL", "http://localhost:8080")

	// Initialize client and session
	vfsClient := client.NewClient(vfsServiceURL)
	sess := session.NewSession()

	// Load auth token (priority: env var > saved file)
	token := os.Getenv("VFS_AUTH_TOKEN")
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

	// Check connectivity
	fmt.Println("=== VFS CLI ===")
	fmt.Printf("Connecting to: %s\n", vfsServiceURL)

	healthy, err := vfsClient.HealthCheck()
	if err != nil || !healthy {
		fmt.Printf("Warning: Cannot connect to VFS service: %v\n", err)
		fmt.Println("Commands will fail until service is available.")
	} else {
		fmt.Println("Connected successfully!")
	}

	fmt.Println("Type 'help' for available commands or 'exit' to quit")
	fmt.Println()

	// Initialize commands
	cmdMap := map[string]commands.Command{
		"ls":            &commands.LsCommand{},
		"cd":            &commands.CdCommand{},
		"pwd":           &commands.PwdCommand{},
		"mkdir":         &commands.MkdirCommand{},
		"rmdir":         &commands.RmdirCommand{},
		"cat":           &commands.CatCommand{},
		"import":        &commands.ImportCommand{},
		"rm":            &commands.RmCommand{},
		"mv":            &commands.MvCommand{},
		"jq":            &commands.JqCommand{},
		"create-schema": &commands.CreateSchemaCommand{},
		"create-policy": &commands.CreatePolicyCommand{},
		"help":          commands.NewHelpCommand(nil),
	}

	// Create command context
	ctx := &commands.Context{
		Client:  vfsClient,
		Session: sess,
		Stdin:   os.Stdin,
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
	}

	// REPL loop
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Printf("%s> ", sess.GetCurrentDirectory())
		input, err := reader.ReadString('\n')
		if err != nil {
			log.Printf("Error reading input: %v", err)
			continue
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		// Handle exit
		if input == "exit" || input == "quit" {
			fmt.Println("Goodbye!")
			return
		}

		// Check if this is a pipeline
		if strings.Contains(input, "|") {
			if err := executePipeline(input, cmdMap, ctx); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			}
			continue
		}

		// Parse single command
		parts := strings.Fields(input)
		if len(parts) == 0 {
			continue
		}

		cmdName := parts[0]
		args := parts[1:]

		// Execute command
		cmd, exists := cmdMap[cmdName]
		if !exists {
			fmt.Printf("Unknown command: %s (type 'help' for available commands)\n", cmdName)
			continue
		}

		if err := cmd.Execute(ctx, args); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
	}
}

// executePipeline executes a pipeline of commands
func executePipeline(input string, cmdMap map[string]commands.Command, baseCtx *commands.Context) error {
	segments := pipe.ParsePipeline(input)

	if len(segments) == 0 {
		return fmt.Errorf("empty pipeline")
	}

	// Single command - no piping needed
	if len(segments) == 1 {
		cmd, exists := cmdMap[segments[0].Command]
		if !exists {
			return fmt.Errorf("unknown command: %s", segments[0].Command)
		}
		return cmd.Execute(baseCtx, segments[0].Args)
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
			return fmt.Errorf("unknown command: %s", segment.Command)
		}

		ctx := &commands.Context{
			Client:  baseCtx.Client,
			Session: baseCtx.Session,
			Stderr:  os.Stderr,
		}

		// Set stdin
		if i == 0 {
			ctx.Stdin = os.Stdin
		} else {
			ctx.Stdin = readers[i-1]
		}

		// Set stdout
		if i == len(segments)-1 {
			ctx.Stdout = os.Stdout
		} else {
			ctx.Stdout = pipes[i]
		}

		// Execute command
		go func(cmd commands.Command, ctx *commands.Context, args []string, idx int) {
			err := cmd.Execute(ctx, args)
			// Close pipe writer if not the last command
			if idx < len(pipes) {
				pipes[idx].Close()
			}
			errs <- err
		}(cmd, ctx, segment.Args, i)
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
