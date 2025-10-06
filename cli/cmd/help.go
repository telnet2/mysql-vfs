package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

var helpCmd = &cobra.Command{
	Use:   "help [command]",
	Short: "Show help for commands",
	Long:  `Display help information for VFS CLI commands.`,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			showGeneralHelp()
		} else {
			showCommandHelp(args[0])
		}
	},
}

func showGeneralHelp() {
	fmt.Println(Colorize(ColorBold+ColorCyan, "=== VFS CLI Help ==="))
	fmt.Println()
	fmt.Println(Colorize(ColorBold, "Available Commands:"))
	fmt.Println()

	// Group commands by category
	fileCommands := []string{"cat", "edit", "import", "rm", "mv"}
	dirCommands := []string{"ls", "tree", "cd", "pwd", "mkdir", "rmdir"}
	utilCommands := []string{"jq", "version"}
	authCommands := []string{"login", "logout"}
	systemCommands := []string{"history", "get-cwd", "set-cwd", "help", "exit"}

	printCommandCategory("File Operations", fileCommands)
	printCommandCategory("Directory Operations", dirCommands)
	printCommandCategory("Utilities", utilCommands)
	printCommandCategory("Authentication", authCommands)
	printCommandCategory("System", systemCommands)

	fmt.Println()
	fmt.Println(Colorize(ColorGray, "Type 'help <command>' for more information about a specific command"))
	fmt.Println(Colorize(ColorGray, "Type '$' to toggle shell mode, or '$<command>' to run a single shell command"))
}

func printCommandCategory(category string, commands []string) {
	fmt.Println(Colorize(ColorBold+ColorYellow, category+":"))
	sort.Strings(commands)

	for _, cmdName := range commands {
		cmd, _, err := rootCmd.Find([]string{cmdName})
		if err != nil || cmd == nil {
			continue
		}
		fmt.Printf("  %s - %s\n",
			Colorize(ColorCyan, fmt.Sprintf("%-12s", cmdName)),
			Colorize(ColorGray, cmd.Short))
	}
	fmt.Println()
}

func showCommandHelp(commandName string) {
	cmd, _, err := rootCmd.Find([]string{commandName})
	if err != nil || cmd == nil {
		fmt.Fprintln(os.Stderr, ErrorMsg(fmt.Sprintf("Unknown command: %s", commandName)))
		fmt.Fprintln(os.Stderr, Colorize(ColorGray, "Type 'help' to see all available commands"))
		return
	}

	// Show command-specific help based on the command
	switch commandName {
	case "ls":
		showLsHelp()
	case "cd":
		showCdHelp()
	case "pwd":
		showPwdHelp()
	case "cat":
		showCatHelp()
	case "mkdir":
		showMkdirHelp()
	case "rmdir":
		showRmdirHelp()
	case "rm":
		showRmHelp()
	case "mv":
		showMvHelp()
	case "tree":
		showTreeHelp()
	case "edit":
		showEditHelp()
	case "import":
		showImportHelp()
	case "jq":
		showJqHelp()
	case "login":
		showLoginHelp()
	case "logout":
		showLogoutHelp()
	case "history":
		showHistoryHelp()
	case "version":
		showVersionHelp()
	default:
		// Generic help
		fmt.Println(Colorize(ColorBold, "Command: ")+CommandName(cmd.Use))
		fmt.Println()
		if cmd.Long != "" {
			fmt.Println(cmd.Long)
		} else {
			fmt.Println(cmd.Short)
		}
		fmt.Println()
		if cmd.Flags().HasFlags() {
			fmt.Println(Colorize(ColorBold, "Flags:"))
			fmt.Println(cmd.Flags().FlagUsages())
		}
	}
}

func showLsHelp() {
	usage := FormatREPLUsage(
		"ls",
		"["+ArgName("-r")+"] ["+ArgName("path")+"]",
		map[string]string{
			"-r, --recursive": "List directories recursively",
		},
		[]string{
			"ls",
			"ls /data",
			"ls -r /projects",
		},
	)
	fmt.Println(usage)
	fmt.Println(Colorize(ColorGray, "Lists the contents of a directory. Shows directories with '/' suffix."))
}

func showCdHelp() {
	usage := FormatREPLUsage(
		"cd",
		"["+ArgName("path")+"]",
		nil,
		[]string{
			"cd /data",
			"cd validation",
			"cd ..",
			"cd",
		},
	)
	fmt.Println(usage)
	fmt.Println(Colorize(ColorGray, "Changes the current working directory. Without arguments, goes to root (/)."))
}

func showPwdHelp() {
	usage := FormatREPLUsage(
		"pwd",
		"",
		nil,
		[]string{"pwd"},
	)
	fmt.Println(usage)
	fmt.Println(Colorize(ColorGray, "Prints the current working directory path."))
}

func showCatHelp() {
	usage := FormatREPLUsage(
		"cat",
		ArgName("file"),
		nil,
		[]string{
			"cat config.json",
			"cat /data/user.json",
		},
	)
	fmt.Println(usage)
	fmt.Println(Colorize(ColorGray, "Displays the contents of a file."))
}

func showMkdirHelp() {
	usage := FormatREPLUsage(
		"mkdir",
		ArgName("name"),
		nil,
		[]string{
			"mkdir projects",
			"mkdir data",
		},
	)
	fmt.Println(usage)
	fmt.Println(Colorize(ColorGray, "Creates a new directory in the current directory."))
}

func showRmdirHelp() {
	usage := FormatREPLUsage(
		"rmdir",
		"["+ArgName("-r")+"] "+ArgName("path"),
		map[string]string{
			"-r, --recursive": "Remove directory and all contents",
		},
		[]string{
			"rmdir temp",
			"rmdir -r old-data",
		},
	)
	fmt.Println(usage)
	fmt.Println(Colorize(ColorGray, "Removes a directory. Use -r to remove non-empty directories."))
}

func showRmHelp() {
	usage := FormatREPLUsage(
		"rm",
		ArgName("file"),
		nil,
		[]string{
			"rm temp.txt",
			"rm /data/old-file.json",
		},
	)
	fmt.Println(usage)
	fmt.Println(Colorize(ColorGray, "Deletes a file."))
}

func showMvHelp() {
	usage := FormatREPLUsage(
		"mv",
		ArgName("source")+" "+ArgName("destination"),
		nil,
		[]string{
			"mv old.txt new.txt",
			"mv /data/file.json /archive/file.json",
		},
	)
	fmt.Println(usage)
	fmt.Println(Colorize(ColorGray, "Moves or renames a file."))
}

func showTreeHelp() {
	usage := FormatREPLUsage(
		"tree",
		"["+ArgName("path")+"]",
		nil,
		[]string{
			"tree",
			"tree /projects",
		},
	)
	fmt.Println(usage)
	fmt.Println(Colorize(ColorGray, "Shows directory structure as a tree."))
}

func showEditHelp() {
	usage := FormatREPLUsage(
		"edit",
		ArgName("file"),
		nil,
		[]string{
			"edit config.json",
			"edit /data/settings.json",
		},
	)
	fmt.Println(usage)
	fmt.Println(Colorize(ColorGray, "Opens a file in your default editor (uses $EDITOR environment variable)."))
}

func showImportHelp() {
	usage := FormatREPLUsage(
		"import",
		ArgName("local-path")+" "+ArgName("vfs-path"),
		nil,
		[]string{
			"import ./local-file.txt /data/file.txt",
			"import /home/user/doc.pdf /documents/doc.pdf",
		},
	)
	fmt.Println(usage)
	fmt.Println(Colorize(ColorGray, "Imports a file from your local filesystem to VFS."))
}

func showJqHelp() {
	usage := FormatREPLUsage(
		"jq",
		ArgName("filter")+" "+ArgName("file"),
		nil,
		[]string{
			"jq '.name' user.json",
			"jq '.items[] | .id' /data/list.json",
		},
	)
	fmt.Println(usage)
	fmt.Println(Colorize(ColorGray, "Queries JSON files using jq syntax."))
}

func showLoginHelp() {
	usage := FormatREPLUsage(
		"login",
		"",
		nil,
		[]string{"login"},
	)
	fmt.Println(usage)
	fmt.Println(Colorize(ColorGray, "Authenticates with the VFS service. Prompts for user ID and password."))
}

func showLogoutHelp() {
	usage := FormatREPLUsage(
		"logout",
		"",
		nil,
		[]string{"logout"},
	)
	fmt.Println(usage)
	fmt.Println(Colorize(ColorGray, "Logs out of the current session."))
}

func showHistoryHelp() {
	usage := FormatREPLUsage(
		"history",
		"",
		nil,
		[]string{"history"},
	)
	fmt.Println(usage)
	fmt.Println(Colorize(ColorGray, "Shows command history for the current session."))
}

func showVersionHelp() {
	usage := FormatREPLUsage(
		"version",
		"["+ArgName("file")+"]",
		nil,
		[]string{
			"version config.json",
			"version /data/document.pdf",
		},
	)
	fmt.Println(usage)
	fmt.Println(Colorize(ColorGray, "Shows version information for a file."))
}

// Remove "vfs-cli" prefix from usage strings for REPL mode
func cleanUsageForREPL(usage string) string {
	usage = strings.ReplaceAll(usage, "vfs-cli ", "")
	return strings.TrimSpace(usage)
}
