package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/telnet2/mysql-vfs/cli/commands"
)

var mkdirCmd = &cobra.Command{
	Use:   "mkdir <name>",
	Short: "Create a new directory",
	Long:  `Create a new directory in the current working directory.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("directory name required")
		}
		mkdirCommand := &commands.MkdirCommand{}
		return mkdirCommand.Execute(ctx, args)
	},
}
