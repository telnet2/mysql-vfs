package cmd

import (
	"github.com/spf13/cobra"
	"github.com/telnet2/mysql-vfs/cli/commands"
)

var importCmd = &cobra.Command{
	Use:   "import <local_path> [vfs_path]",
	Short: "Import local file to VFS",
	Long:  `Import a file from the local filesystem to the VFS. If vfs_path is not specified, the file will be uploaded to the current VFS directory with the same filename.`,
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		importCommand := &commands.ImportCommand{}
		return importCommand.Execute(ctx, args)
	},
}
