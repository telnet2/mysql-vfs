package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"
)

func main() {
	vfsServiceURL := getEnv("VFS_SERVICE_URL", "http://vfs-service:8080")

	fmt.Println("=== VFS CLI ===")
	fmt.Printf("Connected to: %s\n", vfsServiceURL)
	fmt.Println("Type 'help' for available commands or 'exit' to quit")
	fmt.Println()

	// Placeholder: Will be implemented in Phase 5
	fmt.Println("CLI skeleton running (Phase 5 implementation pending)")
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)
	currentDir := "/"

	for {
		fmt.Printf("%s> ", currentDir)
		input, err := reader.ReadString('\n')
		if err != nil {
			log.Printf("Error reading input: %v", err)
			continue
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		parts := strings.Fields(input)
		command := parts[0]

		switch command {
		case "exit", "quit":
			fmt.Println("Goodbye!")
			return
		case "help":
			printHelp()
		default:
			fmt.Printf("Command '%s' not implemented yet (Phase 5)\n", command)
		}
	}
}

func printHelp() {
	fmt.Println("Available commands (to be implemented in Phase 5):")
	fmt.Println("  ls [-r] [path]         - List directory contents")
	fmt.Println("  cd <path>              - Change directory")
	fmt.Println("  mkdir <path>           - Create directory")
	fmt.Println("  rmdir <path>           - Remove directory")
	fmt.Println("  import <local> <vfs>   - Import file to VFS")
	fmt.Println("  cat <path>             - Display file contents")
	fmt.Println("  jq <path> <expr>       - Query JSON file")
	fmt.Println("  mv <src> <dst>         - Move/rename file")
	fmt.Println("  rm <path>              - Remove file")
	fmt.Println("  help                   - Show this help")
	fmt.Println("  exit                   - Exit CLI")
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
