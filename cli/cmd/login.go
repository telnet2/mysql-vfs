package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/telnet2/mysql-vfs/cli/commands"
)

var loginCmd = &cobra.Command{
	Use:   "login <username> <password>",
	Short: "Authenticate with VFS",
	Long:  `Authenticate with the VFS service using username and password.`,
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) < 2 {
			return fmt.Errorf("username and password required")
		}
		loginCommand := &commands.LoginCommand{}
		return loginCommand.Execute(ctx, args)
	},
}
