package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/telnet2/mysql-vfs/cli/commands"
)

var importCmd = &cobra.Command{
	Use:   "import <local_path> <vfs_path>",
	Short: "Import local file to VFS",
	Long:  `Import a file from the local filesystem to the VFS.`,
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) < 2 {
			return fmt.Errorf("local path and VFS path required")
		}
		importCommand := &commands.ImportCommand{}
		return importCommand.Execute(ctx, args)
	},
}
