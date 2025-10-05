package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/telnet2/mysql-vfs/cli/commands"
)

var rmCmd = &cobra.Command{
	Use:   "rm <path>",
	Short: "Remove file",
	Long:  `Remove a file from the VFS.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("file path required")
		}
		rmCommand := &commands.RmCommand{}
		return rmCommand.Execute(ctx, args)
	},
}
