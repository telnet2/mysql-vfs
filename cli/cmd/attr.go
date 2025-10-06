package cmd

import (
	"github.com/spf13/cobra"
	"github.com/telnet2/mysql-vfs/cli/commands"
)

var attrCmd = &cobra.Command{
	Use:   "attr <command> <path> [key or key=value] ...",
	Short: "Get or set file attributes",
	Long:  `Get or set file attributes. Commands: get, set. 'get' shows all file attributes including metadata. 'set' currently supports content-type.`,
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		attrCommand := &commands.AttrCommand{}
		return attrCommand.Execute(ctx, args)
	},
}
