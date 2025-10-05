package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/telnet2/mysql-vfs/cli/commands"
)

var editCmd = &cobra.Command{
	Use:   "edit <path>",
	Short: "Edit file using $EDITOR or vim",
	Long:  `Edit a file in the VFS using your preferred editor.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("file path required")
		}
		editCommand := &commands.EditCommand{}
		return editCommand.Execute(ctx, args)
	},
}
