package cmd

import (
	"github.com/spf13/cobra"
	"github.com/telnet2/mysql-vfs/cli/commands"
)

var lsRecursive bool

var lsCmd = &cobra.Command{
	Use:   "ls [path]",
	Short: "List directory contents",
	Long:  `List the contents of a directory in the VFS.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		lsCommand := &commands.LsCommand{}

		// Build args for the command
		cmdArgs := []string{}
		if lsRecursive {
			cmdArgs = append(cmdArgs, "-r")
		}
		if len(args) > 0 {
			cmdArgs = append(cmdArgs, args[0])
		}

		return lsCommand.Execute(ctx, cmdArgs)
	},
}

func init() {
	lsCmd.Flags().BoolVarP(&lsRecursive, "recursive", "r", false, "List directories recursively")
}
