package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/telnet2/mysql-vfs/cli/commands"
)

var getCwdCmd = &cobra.Command{
	Use:   "get-cwd",
	Short: "Show local working directory",
	Long:  `Display the current local working directory.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		getCwdCommand := &commands.GetCwdCommand{}
		return getCwdCommand.Execute(ctx, args)
	},
}

var setCwdCmd = &cobra.Command{
	Use:   "set-cwd <path>",
	Short: "Change local working directory",
	Long:  `Change the current local working directory.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("path required")
		}
		setCwdCommand := &commands.SetCwdCommand{}
		return setCwdCommand.Execute(ctx, args)
	},
}
