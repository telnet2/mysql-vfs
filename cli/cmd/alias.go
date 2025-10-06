package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var aliasCmd = &cobra.Command{
	Use:   "alias [command] [name] [value]",
	Short: "Manage command aliases",
	Long: `Manage command aliases for the VFS CLI.

Commands:
  alias              - List all aliases
  alias set <name> <command> - Set an alias
  alias get <name>   - Get an alias value
  alias unset <name> - Remove an alias`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return executeAliasCommand(args)
	},
}

func executeAliasCommand(args []string) error {
	if len(args) == 0 {
		// List all aliases
		return listAliases()
	}

	command := args[0]
	switch command {
	case "set":
		if len(args) < 3 {
			return fmt.Errorf("usage: alias set <name> <command>")
		}
		return setAlias(args[1], strings.Join(args[2:], " "))
	case "get":
		if len(args) < 2 {
			return fmt.Errorf("usage: alias get <name>")
		}
		return getAlias(args[1])
	case "unset":
		if len(args) < 2 {
			return fmt.Errorf("usage: alias unset <name>")
		}
		return unsetAlias(args[1])
	default:
		return fmt.Errorf("unknown alias command: %s. Use set, get, unset, or no args to list", command)
	}
}

func listAliases() error {
	aliases := loadAliases()
	if len(aliases) == 0 {
		fmt.Println("No aliases defined")
		return nil
	}

	fmt.Println("Aliases:")
	for name, command := range aliases {
		fmt.Printf("  %s='%s'\n", name, command)
	}
	return nil
}

func setAlias(name, command string) error {
	aliases := loadAliases()
	aliases[name] = command
	if err := saveAliases(aliases); err != nil {
		return fmt.Errorf("failed to save alias: %w", err)
	}
	fmt.Printf("Alias set: %s='%s'\n", name, command)
	return nil
}

func getAlias(name string) error {
	aliases := loadAliases()
	if command, exists := aliases[name]; exists {
		fmt.Printf("%s='%s'\n", name, command)
	} else {
		return fmt.Errorf("alias '%s' not found", name)
	}
	return nil
}

func unsetAlias(name string) error {
	aliases := loadAliases()
	if _, exists := aliases[name]; !exists {
		return fmt.Errorf("alias '%s' not found", name)
	}
	delete(aliases, name)
	if err := saveAliases(aliases); err != nil {
		return fmt.Errorf("failed to save aliases: %w", err)
	}
	fmt.Printf("Alias removed: %s\n", name)
	return nil
}
