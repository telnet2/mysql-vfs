package cmd

import (
	"github.com/spf13/cobra"
	"github.com/telnet2/mysql-vfs/cli/commands"
)

var createTriggerCmd = &cobra.Command{
	Use:   "create-trigger <directory> <url>",
	Short: "Create a webhook trigger for file creation events",
	Long: `Create a .events file that triggers an HTTP POST webhook when files are created in a directory.

Example:
  vfs-cli create-trigger /projects https://example.com/webhook

This will create a .events file in /projects that sends HTTP POST requests to the URL
whenever a file is created in that directory.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		createTriggerCommand := &commands.CreateTriggerCommand{}
		return createTriggerCommand.Execute(ctx, args)
	},
}

func init() {
	// This will be added to rootCmd in root.go
}
