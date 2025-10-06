package cmd

import (
	"github.com/spf13/cobra"
	"github.com/telnet2/mysql-vfs/cli/commands"
)

var findCmd = &cobra.Command{
	Use:   "find <path> [options]",
	Short: "Find files and directories",
	Long:  `Find files and directories in the VFS. Supports searching by name patterns, types, and sizes.`,
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		findCommand := &commands.FindCommand{}
		return findCommand.Execute(ctx, args)
	},
}
