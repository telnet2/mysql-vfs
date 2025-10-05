package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/telnet2/mysql-vfs/cli/commands"
)

var jqCmd = &cobra.Command{
	Use:   "jq <path> <expression>",
	Short: "Query JSON file with jq",
	Long:  `Query a JSON file in the VFS using jq expressions.`,
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) < 2 {
			return fmt.Errorf("file path and jq expression required")
		}
		jqCommand := &commands.JqCommand{}
		return jqCommand.Execute(ctx, args)
	},
}
