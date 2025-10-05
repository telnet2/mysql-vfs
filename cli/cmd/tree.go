package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/telnet2/mysql-vfs/cli/commands"
)

var treeDepth int

var treeCmd = &cobra.Command{
	Use:   "tree [path]",
	Short: "Display directory tree",
	Long:  `Display a tree view of directory contents.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		treeCommand := &commands.TreeCommand{}

		cmdArgs := []string{}
		if treeDepth != 3 {
			cmdArgs = append(cmdArgs, "-d", fmt.Sprintf("%d", treeDepth))
		}
		if len(args) > 0 {
			cmdArgs = append(cmdArgs, args[0])
		}

		return treeCommand.Execute(ctx, cmdArgs)
	},
}

func init() {
	treeCmd.Flags().IntVarP(&treeDepth, "depth", "d", 3, "Maximum depth to traverse")
}
