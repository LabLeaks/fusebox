package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sort"
	"strings"

	"github.com/lableaks/fusebox/internal/rpc"
	"github.com/spf13/cobra"
)

const (
	defaultTunnelPort = 9600
	secretPath        = "/root/.fusebox/secret"
)

var actionsPort int

var actionsCmd = &cobra.Command{
	Use:   "actions",
	Short: "List available actions from fusebox.yaml",
	Long: `Queries the local daemon through the RPC tunnel and prints a formatted
table of available actions with their descriptions and parameters.`,
	RunE: runActions,
}

func init() {
	actionsCmd.Flags().IntVar(&actionsPort, "port", defaultTunnelPort, "tunnel port to connect to")
}

func runActions(cmd *cobra.Command, args []string) error {
	secret, err := os.ReadFile(secretPath)
	if err != nil {
		return fmt.Errorf("reading shared secret: %w", err)
	}

	conn, err := net.Dial("tcp", fmt.Sprintf("localhost:%d", actionsPort))
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error: local machine unreachable")
		return nil
	}
	defer conn.Close()

	encoder := rpc.NewEncoder(conn)
	decoder := rpc.NewDecoder(conn)

	req := rpc.ActionsRequest{
		Type:   rpc.TypeActions,
		Secret: strings.TrimSpace(string(secret)),
	}
	if err := encoder.Send(req); err != nil {
		return fmt.Errorf("sending actions request: %w", err)
	}

	msgType, raw, err := decoder.ReceiveEnvelope()
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

	if msgType == rpc.TypeError {
		var errResp rpc.ErrorResponse
		if err := json.Unmarshal(raw, &errResp); err != nil {
			return fmt.Errorf("parsing error response: %w", err)
		}
		return fmt.Errorf("daemon error: %s", errResp.Message)
	}

	if msgType != rpc.TypeActionsResponse {
		return fmt.Errorf("unexpected response type: %s", msgType)
	}

	var resp rpc.ActionsResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return fmt.Errorf("parsing actions response: %w", err)
	}

	if len(resp.Actions) == 0 {
		fmt.Println("No actions configured.")
		return nil
	}

	printActions(resp.Actions)
	return nil
}

func printActions(actions []rpc.ActionInfo) {
	// Find longest action name for column alignment
	maxName := 0
	for _, a := range actions {
		if len(a.Name) > maxName {
			maxName = len(a.Name)
		}
	}

	// Sort by name for consistent output
	sort.Slice(actions, func(i, j int) bool {
		return actions[i].Name < actions[j].Name
	})

	for _, a := range actions {
		line := fmt.Sprintf("%-*s  %s", maxName, a.Name, a.Description)
		if len(a.Params) > 0 {
			names := make([]string, 0, len(a.Params))
			for name := range a.Params {
				names = append(names, name)
			}
			sort.Strings(names)
			line += fmt.Sprintf(" (params: %s)", strings.Join(names, ", "))
		}
		fmt.Println(line)
	}
}
