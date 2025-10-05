package cmd

import (
	"github.com/spf13/cobra"
	"github.com/telnet2/mysql-vfs/cli/commands"
)

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Clear authentication",
	Long:  `Clear the current authentication token.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		logoutCommand := &commands.LogoutCommand{}
		return logoutCommand.Execute(ctx, args)
	},
}
