package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/telnet2/mysql-vfs/cli/commands"
)

var catVersion int64

var catCmd = &cobra.Command{
	Use:   "cat <path>",
	Short: "Display file contents",
	Long:  `Display the contents of a file in the VFS.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("file path required")
		}
		catCommand := &commands.CatCommand{}

		cmdArgs := []string{}
		if catVersion > 0 {
			cmdArgs = append(cmdArgs, "-v", fmt.Sprintf("%d", catVersion))
		}
		cmdArgs = append(cmdArgs, args[0])

		return catCommand.Execute(ctx, cmdArgs)
	},
}

func init() {
	catCmd.Flags().Int64VarP(&catVersion, "version", "v", 0, "Specific version to retrieve")
}
