package cmd

import (
	"github.com/spf13/cobra"
	"github.com/telnet2/mysql-vfs/cli/commands"
)

var cdCmd = &cobra.Command{
	Use:   "cd [path]",
	Short: "Change current directory",
	Long:  `Change the current working directory in the VFS.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cdCommand := &commands.CdCommand{}
		return cdCommand.Execute(ctx, args)
	},
}
