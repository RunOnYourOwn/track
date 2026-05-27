package cmd

import (
	"github.com/RunOnYourOwn/track/internal/mcp"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(mcpCmd)
}

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start MCP server (stdio transport)",
	Long:  "Start a Model Context Protocol server over stdio (JSON-RPC 2.0). AI agents connect via this transport to interact with Track.",
	// Bypass the PersistentPreRunE that checks db.Open — we open the db ourselves inside mcp.Run
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },
	RunE: func(cmd *cobra.Command, args []string) error {
		return mcp.Run()
	},
}
