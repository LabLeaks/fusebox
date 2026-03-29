package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/lableaks/fusebox/internal/config"
	"github.com/spf13/cobra"
)

var statusJSON bool

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current fusebox session status",
	Long: `Displays the current project session status including container state,
sync state, RPC state, and available actions.`,
	RunE: runStatus,
}

func init() {
	statusCmd.Flags().BoolVar(&statusJSON, "json", false, "output status as JSON")
}

// statusInfo holds structured status for both human and JSON output.
type statusInfo struct {
	Project   string   `json:"project"`
	Server    string   `json:"server"`
	Container string   `json:"container"`
	Sync      string   `json:"sync"`
	RPC       string   `json:"rpc"`
	Actions   []string `json:"actions"`
}

func runStatus(cmd *cobra.Command, args []string) error {
	resolved, err := config.Resolve(config.ResolveOptions{})
	if err != nil {
		return err
	}

	projectName := filepath.Base(resolved.ProjectRoot)
	serverDisplay := resolved.Server.Host
	if serverDisplay == "" {
		serverDisplay = "(not configured)"
	}

	actionNames := make([]string, 0, len(resolved.Project.Actions))
	for name := range resolved.Project.Actions {
		actionNames = append(actionNames, name)
	}
	sort.Strings(actionNames)

	// TODO: read daemon status from Unix socket (~/.fusebox/run/<project>.sock)
	// once P4.2 (local daemon) is implemented. For now, report as unavailable.
	info := statusInfo{
		Project:   projectName,
		Server:    serverDisplay,
		Container: "(unknown - daemon not running)",
		Sync:      "(unknown - daemon not running)",
		RPC:       "disconnected",
		Actions:   actionNames,
	}

	if statusJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(info)
	}

	actionsDisplay := fmt.Sprintf("%d registered", len(actionNames))
	if len(actionNames) > 0 {
		actionsDisplay += fmt.Sprintf(" (%s)", strings.Join(actionNames, ", "))
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Project:    %s\n", info.Project)
	fmt.Fprintf(cmd.OutOrStdout(), "Server:     %s\n", info.Server)
	fmt.Fprintf(cmd.OutOrStdout(), "Container:  %s\n", info.Container)
	fmt.Fprintf(cmd.OutOrStdout(), "Sync:       %s\n", info.Sync)
	fmt.Fprintf(cmd.OutOrStdout(), "RPC:        %s\n", info.RPC)
	fmt.Fprintf(cmd.OutOrStdout(), "Actions:    %s\n", actionsDisplay)

	return nil
}
