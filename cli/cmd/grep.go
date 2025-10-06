package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/telnet2/mysql-vfs/cli/commands"
)

var grepCmd = &cobra.Command{
	Use:   "grep <pattern> <path>",
	Short: "Search for pattern in files",
	Long:  `Search for a pattern in file contents. Supports glob patterns for file selection.`,
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) < 2 {
			return fmt.Errorf("pattern and path required")
		}
		grepCommand := &commands.GrepCommand{}
		return grepCommand.Execute(ctx, args)
	},
}
