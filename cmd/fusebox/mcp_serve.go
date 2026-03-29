package main

import (
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/lableaks/fusebox/internal/mcp"
	"github.com/lableaks/fusebox/internal/rpc"
	"github.com/spf13/cobra"
)

var mcpServePort int

var mcpServeCmd = &cobra.Command{
	Use:    "mcp-serve",
	Short:  "Start MCP stdio server (used by Claude Code)",
	Hidden: true,
	RunE:   runMCPServe,
}

func init() {
	mcpServeCmd.Flags().IntVar(&mcpServePort, "port", defaultTunnelPort, "tunnel port to connect to")
}

// rpcDaemonClient adapts the rpc.Client to the mcp.DaemonClient interface.
type rpcDaemonClient struct {
	client *rpc.Client
}

func (r *rpcDaemonClient) RequestActions() ([]mcp.ActionDescriptor, error) {
	resp, err := r.client.RequestActions()
	if err != nil {
		return nil, err
	}

	actions := make([]mcp.ActionDescriptor, len(resp.Actions))
	for i, a := range resp.Actions {
		params := make(map[string]mcp.ParamDescriptor, len(a.Params))
		for name, p := range a.Params {
			params[name] = mcp.ParamDescriptor{
				Type:    p.Type,
				Pattern: p.Pattern,
				Values:  p.Values,
				Min:     p.Min,
				Max:     p.Max,
			}
		}
		actions[i] = mcp.ActionDescriptor{
			Name:        a.Name,
			Description: a.Description,
			Params:      params,
		}
	}
	return actions, nil
}

func (r *rpcDaemonClient) Exec(action string, params map[string]string) (*mcp.ExecResult, error) {
	var output strings.Builder
	var exitCode int
	var durationMs int64

	handler := &execCollector{
		output:     &output,
		exitCode:   &exitCode,
		durationMs: &durationMs,
	}

	if err := r.client.ExecStream(action, params, os.Stderr, handler); err != nil {
		// If it's an RPC error that was already captured via OnError, return the result.
		if handler.gotExit || handler.gotError {
			return &mcp.ExecResult{
				Output:     output.String(),
				ExitCode:   exitCode,
				DurationMs: durationMs,
			}, nil
		}
		return nil, err
	}

	return &mcp.ExecResult{
		Output:     output.String(),
		ExitCode:   exitCode,
		DurationMs: durationMs,
	}, nil
}

func (r *rpcDaemonClient) Close() error {
	return r.client.Close()
}

// execCollector implements rpc.StreamHandler and collects output.
type execCollector struct {
	output     *strings.Builder
	exitCode   *int
	durationMs *int64
	gotExit    bool
	gotError   bool
}

func (e *execCollector) OnStdout(line string) { e.output.WriteString(line + "\n") }
func (e *execCollector) OnStderr(line string) { e.output.WriteString(line + "\n") }
func (e *execCollector) OnExit(code int, durationMs int64) {
	*e.exitCode = code
	*e.durationMs = durationMs
	e.gotExit = true
}
func (e *execCollector) OnError(code, message string) {
	e.output.WriteString(fmt.Sprintf("error [%s]: %s\n", code, message))
	*e.exitCode = 1
	e.gotError = true
}

func runMCPServe(cmd *cobra.Command, args []string) error {
	secret, err := os.ReadFile(secretPath)
	if err != nil {
		return fmt.Errorf("reading shared secret: %w", err)
	}

	conn, err := net.Dial("tcp", fmt.Sprintf("localhost:%d", mcpServePort))
	if err != nil {
		// Start server anyway; tools/list and tools/call will report unreachable.
		daemon := &unreachableDaemon{}
		srv := mcp.NewServer(daemon, os.Stdin, os.Stdout)
		return srv.Serve()
	}

	client := rpc.NewClient(conn, rpc.ClientConfig{
		Secret: strings.TrimSpace(string(secret)),
	})
	defer client.Close()

	daemon := &rpcDaemonClient{client: client}
	srv := mcp.NewServer(daemon, os.Stdin, os.Stdout)
	return srv.Serve()
}

// unreachableDaemon returns connection errors for all operations.
type unreachableDaemon struct{}

func (u *unreachableDaemon) RequestActions() ([]mcp.ActionDescriptor, error) {
	return nil, fmt.Errorf("local machine unreachable: connection refused")
}

func (u *unreachableDaemon) Exec(action string, params map[string]string) (*mcp.ExecResult, error) {
	return nil, fmt.Errorf("local machine unreachable: connection refused")
}

func (u *unreachableDaemon) Close() error { return nil }
