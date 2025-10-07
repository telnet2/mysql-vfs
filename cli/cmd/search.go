package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/telnet2/mysql-vfs/cli/commands"
)

var searchCmd = &cobra.Command{
	Use:   "search [options]",
	Short: "Search for files by content or metadata",
	Long:  `Search for files in the VFS based on JSON content or metadata fields using JSONPath or JQ expressions.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		searchCommand := &commands.SearchCommand{}

		jsonPath, _ := cmd.Flags().GetString("json-path")
		jqExpr, _ := cmd.Flags().GetString("jq-expression")
		value, _ := cmd.Flags().GetString("value")
		metaKey, _ := cmd.Flags().GetString("meta-key")
		metaValue, _ := cmd.Flags().GetString("meta-value")
		metaJSONPath, _ := cmd.Flags().GetString("meta-json-path")
		metaJQExpr, _ := cmd.Flags().GetString("meta-jq-expression")
		fileType, _ := cmd.Flags().GetString("type")
		limit, _ := cmd.Flags().GetInt("limit")

		cmdArgs := []string{}
		if jsonPath != "" {
			cmdArgs = append(cmdArgs, "--json-path", jsonPath)
		}
		if jqExpr != "" {
			cmdArgs = append(cmdArgs, "--jq-expression", jqExpr)
		}
		if value != "" {
			cmdArgs = append(cmdArgs, "--value", value)
		}
		if metaKey != "" {
			cmdArgs = append(cmdArgs, "--meta-key", metaKey)
		}
		if metaValue != "" {
			cmdArgs = append(cmdArgs, "--meta-value", metaValue)
		}
		if metaJSONPath != "" {
			cmdArgs = append(cmdArgs, "--meta-json-path", metaJSONPath)
		}
		if metaJQExpr != "" {
			cmdArgs = append(cmdArgs, "--meta-jq-expression", metaJQExpr)
		}
		if fileType != "" {
			cmdArgs = append(cmdArgs, "--type", fileType)
		}
		if limit > 0 {
			cmdArgs = append(cmdArgs, "--limit", fmt.Sprintf("%d", limit))
		}

		return searchCommand.Execute(ctx, cmdArgs)
	},
}

func init() {
	searchCmd.Flags().String("json-path", "", "JSON path to search in file content (e.g., '$.name')")
	searchCmd.Flags().String("jq-expression", "", "JQ expression to evaluate on file content (e.g., '.name')")
	searchCmd.Flags().String("value", "", "Value to search for in JSON content or metadata")
	searchCmd.Flags().String("meta-key", "", "Metadata key to search for (simple key-value)")
	searchCmd.Flags().String("meta-value", "", "Metadata value to search for (simple key-value)")
	searchCmd.Flags().String("meta-json-path", "", "JSON path to search in metadata (e.g., '$.permissions.write')")
	searchCmd.Flags().String("meta-jq-expression", "", "JQ expression to evaluate on metadata (e.g., '.permissions.write')")
	searchCmd.Flags().String("type", "", "Type of entries to search (f for files, d for directories)")
	searchCmd.Flags().Int("limit", 100, "Maximum number of results to return")
}
