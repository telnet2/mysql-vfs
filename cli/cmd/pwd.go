package cmd

import (
	"github.com/spf13/cobra"
	"github.com/telnet2/mysql-vfs/cli/commands"
)

var pwdCmd = &cobra.Command{
	Use:   "pwd",
	Short: "Print working directory",
	Long:  `Print the current working directory in the VFS.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		pwdCommand := &commands.PwdCommand{}
		return pwdCommand.Execute(ctx, args)
	},
}
