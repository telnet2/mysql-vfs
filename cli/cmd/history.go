package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/telnet2/mysql-vfs/cli/commands"
)

const maxHistorySize = 1000
const historyFile = ".vfs_history"

func loadHistory() []string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return []string{}
	}

	historyPath := filepath.Join(homeDir, historyFile)
	data, err := os.ReadFile(historyPath)
	if err != nil {
		return []string{}
	}

	var history []string
	if err := json.Unmarshal(data, &history); err != nil {
		return []string{}
	}

	return history
}

func saveHistory(history []string) {
	if len(history) == 0 {
		return
	}

	// Limit history size
	if len(history) > maxHistorySize {
		history = history[len(history)-maxHistorySize:]
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return
	}

	historyPath := filepath.Join(homeDir, historyFile)
	data, err := json.Marshal(history)
	if err != nil {
		return
	}

	os.WriteFile(historyPath, data, 0644)
}

var cmdHistory = loadHistory()

// Alias management
const aliasesFile = ".vfs_aliases"

func loadAliases() map[string]string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return make(map[string]string)
	}

	aliasesPath := filepath.Join(homeDir, aliasesFile)
	data, err := os.ReadFile(aliasesPath)
	if err != nil {
		return make(map[string]string)
	}

	var aliases map[string]string
	if err := json.Unmarshal(data, &aliases); err != nil {
		return make(map[string]string)
	}

	return aliases
}

func saveAliases(aliases map[string]string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	aliasesPath := filepath.Join(homeDir, aliasesFile)
	data, err := json.Marshal(aliases)
	if err != nil {
		return err
	}

	return os.WriteFile(aliasesPath, data, 0644)
}

func expandAlias(alias string) string {
	aliases := loadAliases()
	if command, exists := aliases[alias]; exists {
		return command
	}
	return alias
}

var historyCmd = &cobra.Command{
	Use:   "history",
	Short: "Show command history",
	Long:  `Display the command history for the current session.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		historyCommand := commands.NewHistoryCommand(cmdHistory)
		return historyCommand.Execute(ctx, args)
	},
}
