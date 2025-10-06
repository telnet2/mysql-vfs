package cmd

import (
	"github.com/spf13/cobra"
	"github.com/telnet2/mysql-vfs/cli/commands"
)

var versionCmd = &cobra.Command{
	Use:   "version <path>",
	Short: "Show version history of a file",
	Long:  `Display the version history of a file, showing all versions from latest to oldest.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		versionCommand := &commands.VersionCommand{}
		return versionCommand.Execute(ctx, args)
	},
}
