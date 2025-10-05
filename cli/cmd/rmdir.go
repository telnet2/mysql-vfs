package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/telnet2/mysql-vfs/cli/commands"
)

var rmdirRecursive bool

var rmdirCmd = &cobra.Command{
	Use:   "rmdir <path>",
	Short: "Remove directory",
	Long:  `Remove a directory from the VFS.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("directory path required")
		}

		rmdirCommand := &commands.RmdirCommand{}
		cmdArgs := []string{}
		if rmdirRecursive {
			cmdArgs = append(cmdArgs, "-r")
		}
		cmdArgs = append(cmdArgs, args[0])

		return rmdirCommand.Execute(ctx, cmdArgs)
	},
}

func init() {
	rmdirCmd.Flags().BoolVarP(&rmdirRecursive, "recursive", "r", false, "Remove directory recursively")
}
