package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/telnet2/mysql-vfs/cli/commands"
)

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

		version, _ := cmd.Flags().GetInt64("version")
		showInfo, _ := cmd.Flags().GetBool("info")

		cmdArgs := []string{}
		if version > 0 {
			cmdArgs = append(cmdArgs, "-v", fmt.Sprintf("%d", version))
		}
		if showInfo {
			cmdArgs = append(cmdArgs, "-i")
		}
		cmdArgs = append(cmdArgs, args[0])

		return catCommand.Execute(ctx, cmdArgs)
	},
}

func init() {
	catCmd.Flags().Int64P("version", "v", 0, "Specific version to retrieve")
	catCmd.Flags().BoolP("info", "i", false, "Show version information")
}
