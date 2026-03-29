package main

import "github.com/spf13/cobra"

var rootCmd = &cobra.Command{
	Use:   "fusebox",
	Short: "Local execution bridge for remote AI coding agents",
	Long: `Fusebox bridges your local machine with a remote sandboxed AI coding agent.
The agent runs on a cheap remote Linux server while your local machine handles
native tools at bare-metal speed, connected via a whitelisted RPC bridge.`,
}

func init() {
	rootCmd.AddCommand(upCmd)
	rootCmd.AddCommand(downCmd)
	rootCmd.AddCommand(execCmd)
	rootCmd.AddCommand(actionsCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(authCmd)
	rootCmd.AddCommand(mcpServeCmd)
}
