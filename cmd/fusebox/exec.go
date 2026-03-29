package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/lableaks/fusebox/internal/rpc"
	"github.com/spf13/cobra"
)

var execPort int

var execCmd = &cobra.Command{
	Use:   "exec <action> [--param=value ...]",
	Short: "Execute a local action via the RPC bridge",
	Long: `Sends an exec request through the RPC tunnel to the local daemon, which
validates the action against fusebox.yaml and streams stdout/stderr back.`,
	Args:               cobra.MinimumNArgs(1),
	DisableFlagParsing: true,
	RunE:               runExec,
}

func runExec(cmd *cobra.Command, args []string) error {
	// Manual flag parsing since we mix positional args with --param=value flags.
	// First non-flag arg is the action name. Everything else is --key=value params.
	action, params, port, err := parseExecArgs(args)
	if err != nil {
		return err
	}

	secret, err := os.ReadFile(secretPath)
	if err != nil {
		return fmt.Errorf("reading shared secret: %w", err)
	}

	client, err := rpc.Dial(rpc.ClientConfig{
		Address: fmt.Sprintf("localhost:%d", port),
		Secret:  strings.TrimSpace(string(secret)),
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error: local machine unreachable")
		os.Exit(1)
	}
	defer client.Close()

	handler := &execHandler{}
	err = client.ExecStream(action, params, os.Stderr, handler)
	if err != nil {
		if handler.rpcError {
			// Error already printed by handler
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	os.Exit(handler.exitCode)
	return nil // unreachable, but satisfies compiler
}

func parseExecArgs(args []string) (action string, params map[string]string, port int, err error) {
	params = make(map[string]string)
	port = defaultTunnelPort

	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			return "", nil, 0, fmt.Errorf("usage: fusebox exec <action> [--param=value ...]")
		}

		if strings.HasPrefix(arg, "--port=") {
			_, err := fmt.Sscanf(arg, "--port=%d", &port)
			if err != nil {
				return "", nil, 0, fmt.Errorf("invalid port: %s", arg)
			}
			continue
		}

		if strings.HasPrefix(arg, "--") {
			kv := strings.TrimPrefix(arg, "--")
			parts := strings.SplitN(kv, "=", 2)
			if len(parts) != 2 {
				return "", nil, 0, fmt.Errorf("invalid param flag %q (expected --key=value)", arg)
			}
			params[parts[0]] = parts[1]
			continue
		}

		if action == "" {
			action = arg
			continue
		}

		return "", nil, 0, fmt.Errorf("unexpected argument %q", arg)
	}

	if action == "" {
		return "", nil, 0, fmt.Errorf("action name required")
	}

	return action, params, port, nil
}

// execHandler implements rpc.StreamHandler for CLI output.
type execHandler struct {
	exitCode int
	rpcError bool
}

func (h *execHandler) OnStdout(line string) {
	fmt.Println(line)
}

func (h *execHandler) OnStderr(line string) {
	fmt.Fprintln(os.Stderr, line)
}

func (h *execHandler) OnExit(code int, durationMs int64) {
	h.exitCode = code
	fmt.Fprintf(os.Stderr, "[exit %d, %dms]\n", code, durationMs)
}

func (h *execHandler) OnError(code, message string) {
	h.rpcError = true
	switch code {
	case "AUTH_ERROR":
		fmt.Fprintln(os.Stderr, "Error: authentication failed (invalid secret)")
	case "INVALID_ACTION":
		fmt.Fprintf(os.Stderr, "Error: %s\n", message)
	case "INVALID_PARAMS":
		fmt.Fprintf(os.Stderr, "Error: parameter validation failed\n%s\n", message)
	default:
		fmt.Fprintf(os.Stderr, "Error [%s]: %s\n", code, message)
	}
}
