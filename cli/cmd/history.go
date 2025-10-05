package cmd

import (
	"github.com/spf13/cobra"
	"github.com/telnet2/mysql-vfs/cli/commands"
)

var cmdHistory []string

var historyCmd = &cobra.Command{
	Use:   "history",
	Short: "Show command history",
	Long:  `Display the command history for the current session.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		historyCommand := commands.NewHistoryCommand(cmdHistory)
		return historyCommand.Execute(ctx, args)
	},
}
