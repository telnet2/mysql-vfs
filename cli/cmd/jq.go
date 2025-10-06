package cmd

import (
	"github.com/spf13/cobra"
	"github.com/telnet2/mysql-vfs/cli/commands"
)

var jqCmd = &cobra.Command{
	Use:   "jq <path> [expression]",
	Short: "Query JSON file with jq",
	Long:  `Query a JSON file in the VFS using jq expressions.`,
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		jqCommand := &commands.JqCommand{}
		return jqCommand.Execute(ctx, args)
	},
}
