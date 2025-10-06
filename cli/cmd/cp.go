package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/telnet2/mysql-vfs/cli/commands"
)

var cpCmd = &cobra.Command{
	Use:   "cp <source> <destination>",
	Short: "Copy file",
	Long:  `Copy a file in the VFS. Supports glob patterns for source files.`,
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) < 2 {
			return fmt.Errorf("source and destination paths required")
		}
		cpCommand := &commands.CpCommand{}
		return cpCommand.Execute(ctx, args)
	},
}
