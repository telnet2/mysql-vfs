package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/telnet2/mysql-vfs/cli/commands"
)

var createSampleFilesCmd = &cobra.Command{
	Use:   "create-sample-files <directory>",
	Short: "Create sample _files configuration files",
	Long:  `Create sample _files configuration files in the specified directory. These files demonstrate schema validation rules for the VFS.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("directory path required")
		}
		createSampleFilesCommand := &commands.CreateSampleFilesCommand{}
		return createSampleFilesCommand.Execute(ctx, args)
	},
}
