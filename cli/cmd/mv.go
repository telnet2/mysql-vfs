package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/telnet2/mysql-vfs/cli/commands"
)

var mvCmd = &cobra.Command{
	Use:   "mv <source> <destination>",
	Short: "Move or rename file",
	Long:  `Move or rename a file in the VFS.`,
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) < 2 {
			return fmt.Errorf("source and destination paths required")
		}
		mvCommand := &commands.MvCommand{}
		return mvCommand.Execute(ctx, args)
	},
}
